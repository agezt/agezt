// SPDX-License-Identifier: MIT

package agentgw

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
)

// Gateway handles incoming requests from agent subprocess code.
type Gateway struct {
	tokenMgr  *TokenManager
	auditLog  *AuditLogger
	capCheck  *CapabilityChecker
	rateLimit map[string]*RateLimit
	rlMu      sync.RWMutex

	// Kernel integrations (set via Attach)
	bus      *bus.Bus
	mem      *memory.Manager
	roster   *roster.Store
	sockPath string

	// srvMu guards httpSrv: Listen runs on a background goroutine (runtime.Open)
	// while Close may be called from the kernel's shutdown path.
	srvMu   sync.Mutex
	httpSrv *http.Server

	// Config center integration (set via SetConfigCenter)
	configCenter  *configcenter.Center
	configHandler *ConfigHandler
}

// GatewayConfig configures the gateway.
type GatewayConfig struct {
	// SocketPath is the Unix domain socket path to listen on.
	SocketPath string
	// BaseDir is the AGEZT base directory for audit logs.
	BaseDir string
	// TokenSecret is the secret for signing tokens.
	TokenSecret []byte
	// ReadTimeout is the HTTP read timeout.
	ReadTimeout time.Duration
	// WriteTimeout is the HTTP write timeout.
	WriteTimeout time.Duration
}

// maxBodyBytes caps request bodies on the gateway's JSON endpoints so a hostile
// (or buggy) client cannot exhaust memory with an unbounded POST body.
const maxBodyBytes = 1 << 20 // 1 MiB

// DefaultGatewayConfig returns a default gateway configuration. The token secret
// is an ephemeral process-random key (safe-by-default); the daemon overrides it
// with the persisted per-install secret via ResolveTokenSecret.
func DefaultGatewayConfig(baseDir string) GatewayConfig {
	secret, _ := randomSecret()
	return GatewayConfig{
		SocketPath:   "@agezt/agentgw.sock",
		BaseDir:      baseDir,
		TokenSecret:  secret,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// NewGateway creates a new gateway.
func NewGateway(cfg GatewayConfig) *Gateway {
	secret := cfg.TokenSecret
	if len(secret) == 0 {
		// Never sign with an empty/zero key — generate an ephemeral random one.
		secret, _ = randomSecret()
	}
	return &Gateway{
		tokenMgr:  NewTokenManager(secret),
		auditLog:  NewAuditLogger(nil), // real journal wired via SetAuditJournal
		capCheck:  NewCapabilityChecker(),
		rateLimit: make(map[string]*RateLimit),
		sockPath:  cfg.SocketPath,
	}
}

// SetAuditJournal wires the kernel journal so capability access is recorded.
// Called once at startup (runtime.Open) before Listen.
func (g *Gateway) SetAuditJournal(j *journal.Journal) {
	g.auditLog = NewAuditLogger(j)
}

// Attach connects the gateway to the kernel subsystems.
func (g *Gateway) Attach(bus *bus.Bus, mem *memory.Manager, roster *roster.Store) {
	g.bus = bus
	g.mem = mem
	g.roster = roster
}

// SetConfigCenter connects the gateway to the Config Center.
func (g *Gateway) SetConfigCenter(center *configcenter.Center) {
	g.configCenter = center
	g.configHandler = NewConfigHandler(center, g.capCheck)
}

// Listen starts the gateway server.
// Supports both Unix domain sockets and TCP sockets.
// Use tcp://host:port format for TCP, or a Unix socket path otherwise.
func (g *Gateway) Listen(ctx context.Context) error {
	mux := http.NewServeMux()

	// Eventbus endpoints
	mux.HandleFunc("GET /v1/eventbus/subscribe", g.withAuth(g.handleEventbusSubscribe))
	mux.HandleFunc("POST /v1/eventbus/publish", g.withAuth(g.handleEventbusPublish))

	// Memory endpoints
	mux.HandleFunc("POST /v1/memory/write", g.withAuth(g.handleMemoryWrite))
	mux.HandleFunc("DELETE /v1/memory/delete", g.withAuth(g.handleMemoryDelete))
	mux.HandleFunc("GET /v1/memory/search", g.withAuth(g.handleMemorySearch))

	// Log endpoints
	mux.HandleFunc("GET /v1/log/read", g.withAuth(g.handleLogRead))
	mux.HandleFunc("POST /v1/log/write", g.withAuth(g.handleLogWrite))

	// Agent endpoints
	mux.HandleFunc("GET /v1/agent/list", g.withAuth(g.handleAgentList))
	mux.HandleFunc("GET /v1/agent/query", g.withAuth(g.handleAgentQuery))

	// Token endpoint (for creating SUBPROCESS tokens from an authenticated
	// parent token). Behind withAuth: minting requires a valid parent token and
	// the result is capped to the parent's capabilities (see handleTokenCreate).
	mux.HandleFunc("POST /v1/token/create", g.withAuth(g.handleTokenCreate))

	// Config endpoints (require configCenter to be set)
	if g.configHandler != nil {
		mux.HandleFunc("GET /v1/config/", g.withAuth(g.configHandler.handleConfigGet))
		mux.HandleFunc("GET /v1/config", g.withAuth(g.configHandler.handleConfigList))
		mux.HandleFunc("GET /v1/config/search", g.withAuth(g.configHandler.handleConfigSearch))
		mux.HandleFunc("POST /v1/config", g.withAuth(g.configHandler.handleConfigSet))
		mux.HandleFunc("GET /v1/config/audit", g.withAuth(g.configHandler.handleConfigAudit))
	}

	// Health endpoint (no auth)
	mux.HandleFunc("GET /health", g.handleHealth)

	srv := &http.Server{
		Handler:        mux,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	var ln net.Listener
	var err error

	// Support both TCP (tcp://host:port) and Unix socket (unix:/path or /path)
	switch {
	case len(g.sockPath) >= 1 && g.sockPath[0] == '@':
		// Abstract unix socket (the default, e.g. @agezt/agentgw.sock). Go maps
		// the leading @ to the abstract namespace on Linux; without this case it
		// fell through to net.Listen("tcp", ...) and failed everywhere.
		ln, err = net.Listen("unix", g.sockPath)
	case len(g.sockPath) >= 7 && g.sockPath[:6] == "unix://":
		// Unix socket with explicit prefix
		socketPath := g.sockPath[6:]
		ln, err = net.Listen("unix", socketPath)
	case len(g.sockPath) >= 4 && g.sockPath[0] == '/' && g.sockPath[1] != '/':
		// Unix socket path (starts with / but not //)
		ln, err = net.Listen("unix", g.sockPath)
	default:
		// TCP socket
		ln, err = net.Listen("tcp", g.sockPath)
	}

	if err != nil {
		return fmt.Errorf("agentgw: listen: %w", err)
	}

	g.srvMu.Lock()
	g.httpSrv = srv
	g.srvMu.Unlock()

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("agentgw: serve error", "error", err)
		}
	}()

	return nil
}

// Close shuts down the gateway.
func (g *Gateway) Close() error {
	if g.auditLog != nil {
		g.auditLog.Flush()
	}
	g.srvMu.Lock()
	srv := g.httpSrv
	g.srvMu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// withAuth wraps an HTTP handler with authentication and rate limiting.
func (g *Gateway) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"error":"unauthorized","message":"missing token"}`, http.StatusUnauthorized)
			return
		}

		// Validate token
		claims, err := g.tokenMgr.ValidateToken(token)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"unauthorized","message":"%v"}`, err), http.StatusUnauthorized)
			return
		}

		// Check rate limit
		if !g.allowRate(claims.SubprocessID, claims.MaxRate, claims.MaxBurst) {
			http.Error(w, `{"error":"rate_limited","message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}

		// Record the authorized access (audit trail).
		g.auditAccess(r, claims)

		// Store claims in request context
		ctx := context.WithValue(r.Context(), claimsKey{}, claims)
		handler(w, r.WithContext(ctx))
	}
}

// auditAccess records one authorized gateway request to the journal (no-op
// until SetAuditJournal wires a real journal).
func (g *Gateway) auditAccess(r *http.Request, claims *TokenClaims) {
	if g.auditLog == nil {
		return
	}
	g.auditLog.Log(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    claims.ParentTokenID,
		RunID:      claims.RunID,
		Subprocess: claims.SubprocessID,
		Operation:  r.Method,
		Path:       r.URL.Path,
		Success:    true,
		ClientIP:   r.RemoteAddr,
	})
}

// maxRateLimitEntries bounds the per-token rate-limit map so a flood of
// distinct subprocess IDs cannot exhaust memory (CWE-770). When the cap is hit
// we evict idle entries first, then (if all are fresh) drop one to make room.
const maxRateLimitEntries = 4096

// rateLimitIdleEvict is how long a rate-limit bucket may sit unused before it
// becomes eligible for eviction.
const rateLimitIdleEvictMs = 5 * 60_000 // 5 minutes

// allowRate checks and updates rate limit for a token.
func (g *Gateway) allowRate(tid string, maxRate, maxBurst int) bool {
	g.rlMu.Lock()
	rl, ok := g.rateLimit[tid]
	if !ok {
		if len(g.rateLimit) >= maxRateLimitEntries {
			g.evictStaleLocked()
		}
		rl = NewRateLimit(maxRate, maxBurst)
		g.rateLimit[tid] = rl
	}
	g.rlMu.Unlock()
	return rl.Allow()
}

// evictStaleLocked removes idle rate-limit buckets. Caller must hold rlMu.
func (g *Gateway) evictStaleLocked() {
	cutoff := time.Now().UnixMilli() - rateLimitIdleEvictMs
	for k, rl := range g.rateLimit {
		if rl.LastSeen() < cutoff {
			delete(g.rateLimit, k)
		}
	}
	// All buckets still fresh: drop one arbitrary entry to bound memory.
	if len(g.rateLimit) >= maxRateLimitEntries {
		for k := range g.rateLimit {
			delete(g.rateLimit, k)
			break
		}
	}
}

// extractBearerToken extracts the token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return ""
}

// claimsKey is the context key for token claims.
type claimsKey struct{}

// getClaims extracts token claims from the request context.
func getClaims(r *http.Request) *TokenClaims {
	if claims, ok := r.Context().Value(claimsKey{}).(*TokenClaims); ok {
		return claims
	}
	return nil
}

// responseError writes a JSON error response.
func responseError(w http.ResponseWriter, code int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    errCode,
			"message": message,
		},
	})
}

// responseJSON writes a JSON response.
func responseJSON[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// handleHealth handles health check requests.
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	responseJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// handleTokenCreate mints a SUBPROCESS token derived from the caller's
// (authenticated) parent token. It runs behind withAuth, so getClaims is the
// parent. The minted token can never exceed the parent: capabilities are
// intersected with the parent's, expiry is clamped to the parent's, and the
// RunID is inherited (a child cannot mint into a different run). This closes
// the unauthenticated-mint / capability-escalation hole.
func (g *Gateway) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	parent := getClaims(r)
	if parent == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	var req struct {
		SubID    string   `json:"sub_id"`
		Caps     []string `json:"caps"`
		MaxRate  int      `json:"max_rpm"`
		MaxBurst int      `json:"max_burst"`
		ExpiryMs int64    `json:"expiry_ms"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Validate and normalize requested capabilities.
	caps, err := NormalizeCaps(req.Caps)
	if err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_CAPABILITY", err.Error())
		return
	}
	// Reject (don't silently drop) any capability the parent lacks — a child
	// token must be a subset of its parent.
	if missing := CapsSubset(caps, parent.Caps); len(missing) > 0 {
		responseError(w, http.StatusForbidden, "CAP_ESCALATION",
			fmt.Sprintf("requested capabilities exceed parent grant: %v", missing))
		return
	}
	if len(caps) == 0 {
		// Default: inherit the full parent capability set.
		caps = append([]string(nil), parent.Caps...)
	}

	expiry := time.Duration(req.ExpiryMs) * time.Millisecond
	if expiry <= 0 {
		expiry = 10 * time.Minute
	}
	exp := time.Now().Add(expiry)
	if !parent.ExpiresAt.IsZero() && exp.After(parent.ExpiresAt) {
		exp = parent.ExpiresAt // never outlive the parent
	}

	// Clamp rate limits to the parent's (0 == inherit).
	maxRate := req.MaxRate
	if maxRate <= 0 || (parent.MaxRate > 0 && maxRate > parent.MaxRate) {
		maxRate = parent.MaxRate
	}
	maxBurst := req.MaxBurst
	if maxBurst <= 0 || (parent.MaxBurst > 0 && maxBurst > parent.MaxBurst) {
		maxBurst = parent.MaxBurst
	}

	claims := &TokenClaims{
		RunID:         parent.RunID, // inherited — cannot mint into another run
		Caps:          caps,
		MaxRate:       maxRate,
		MaxBurst:      maxBurst,
		ExpiresAt:     exp,
		ParentTokenID: parent.RunID,
		SubprocessID:  req.SubID,
	}

	token, err := g.tokenMgr.CreateToken(claims)
	if err != nil {
		responseError(w, http.StatusInternalServerError, "TOKEN_ERROR", err.Error())
		return
	}

	responseJSON(w, http.StatusCreated, map[string]string{"token": token})
}

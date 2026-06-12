// SPDX-License-Identifier: MIT

package agentgw

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
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
	httpSrv  *http.Server
	sockPath string

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

// DefaultGatewayConfig returns a default gateway configuration.
func DefaultGatewayConfig(baseDir string) GatewayConfig {
	return GatewayConfig{
		SocketPath:   "@agezt/agentgw.sock",
		BaseDir:      baseDir,
		TokenSecret:  []byte("change-me-in-production"),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// NewGateway creates a new gateway.
func NewGateway(cfg GatewayConfig) *Gateway {
	return &Gateway{
		tokenMgr:  NewTokenManager(cfg.TokenSecret),
		auditLog:  NewAuditLogger(nil), // Will be set when attached to kernel
		capCheck:  NewCapabilityChecker(),
		rateLimit: make(map[string]*RateLimit),
		sockPath:  cfg.SocketPath,
	}
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

	// Token endpoint (for creating subprocess tokens)
	mux.HandleFunc("POST /v1/token/create", g.handleTokenCreate)

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

	g.httpSrv = &http.Server{
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

	go func() {
		if err := g.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Printf("agentgw: serve error: %v\n", err)
		}
	}()

	return nil
}

// Close shuts down the gateway.
func (g *Gateway) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if g.httpSrv != nil {
		return g.httpSrv.Shutdown(ctx)
	}
	return nil
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

		// Store claims in request context
		ctx := context.WithValue(r.Context(), claimsKey{}, claims)
		handler(w, r.WithContext(ctx))
	}
}

// allowRate checks and updates rate limit for a token.
func (g *Gateway) allowRate(tid string, maxRate, maxBurst int) bool {
	g.rlMu.Lock()
	rl, ok := g.rateLimit[tid]
	if !ok {
		rl = NewRateLimit(maxRate, maxBurst)
		g.rateLimit[tid] = rl
	}
	g.rlMu.Unlock()
	return rl.Allow()
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
func responseJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// handleHealth handles health check requests.
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	responseJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// handleTokenCreate handles token creation requests.
func (g *Gateway) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RunID    string   `json:"run_id"`
		Caps     []string `json:"caps"`
		MaxRate  int      `json:"max_rpm"`
		MaxBurst int      `json:"max_burst"`
		ExpiryMs int64    `json:"expiry_ms"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Validate and normalize capabilities
	caps, err := NormalizeCaps(req.Caps)
	if err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_CAPABILITY", err.Error())
		return
	}

	expiry := time.Duration(req.ExpiryMs) * time.Millisecond
	if expiry == 0 {
		expiry = 1 * time.Hour
	}

	claims := &TokenClaims{
		RunID:     req.RunID,
		Caps:      caps,
		MaxRate:   req.MaxRate,
		MaxBurst:  req.MaxBurst,
		ExpiresAt: time.Now().Add(expiry),
	}

	token, err := g.tokenMgr.CreateToken(claims)
	if err != nil {
		responseError(w, http.StatusInternalServerError, "TOKEN_ERROR", err.Error())
		return
	}

	responseJSON(w, http.StatusCreated, map[string]string{"token": token})
}

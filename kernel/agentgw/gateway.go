// SPDX-License-Identifier: MIT

// Agent gateway: types + lifecycle.
// Code extracted from gateway.go during the Day-84 god-file split.
// Public API unchanged.
package agentgw


import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"crypto/rand"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"log/slog"
	"net/http"
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
//
// The socket path includes a short random suffix so that concurrent test
// processes (e.g. go test -count=N) don't collide on the abstract Unix
// socket namespace. In production the daemon is a single process, so the
// random suffix is harmless — it just makes the path unique per invocation.
func DefaultGatewayConfig(baseDir string) GatewayConfig {
	secret, _ := randomSecret()
	return GatewayConfig{
		SocketPath:   uniqueSocketPath(),
		BaseDir:      baseDir,
		TokenSecret:  secret,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// uniqueSocketPath returns an abstract Unix socket path with a short random
// suffix, e.g. "@agezt/agentgw-a1b2c3d4.sock". The randomness prevents
// bind failures when multiple test instances run concurrently.
func uniqueSocketPath() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("@agezt/agentgw-%x.sock", b)
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

// listenTarget maps a configured socket path onto the (network, address) pair
// to hand net.ListenConfig.Listen. Three forms select a unix domain socket —
// a leading '@' (abstract namespace, the default), an explicit "unix://"
// prefix, and a plain absolute path — and anything else is treated as TCP.
//
// Split out of Listen so each transport branch is assertable without binding a
// real socket: AC-007 was a dead "unix://" branch (a 6-byte slice compared to
// the 7-byte literal) that silently downgraded filesystem sockets to cleartext
// TCP, and nothing covered the mapping.
func listenTarget(sockPath string) (network, addr string) {
	switch {
	case strings.HasPrefix(sockPath, "@"):
		// Abstract unix socket (the default, e.g. @agezt/agentgw.sock). Go maps
		// the leading @ to the abstract namespace on Linux; without this case it
		// fell through to net.Listen("tcp", ...) and failed everywhere.
		return "unix", sockPath
	case strings.HasPrefix(sockPath, "unix://"):
		return "unix", strings.TrimPrefix(sockPath, "unix://")
	case len(sockPath) >= 4 && sockPath[0] == '/' && sockPath[1] != '/':
		// Plain absolute path.
		return "unix", sockPath
	default:
		return "tcp", sockPath
	}
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

	// Use a ListenConfig with SO_REUSEADDR so that consecutive test runs
	// (-count=N) don't fail with "address already in use" when the
	// previous listener's abstract socket hasn't fully released yet.
	lc := net.ListenConfig{
		Control: setSockOpt,
	}

	network, addr := listenTarget(g.sockPath)
	ln, err := lc.Listen(ctx, network, addr)
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

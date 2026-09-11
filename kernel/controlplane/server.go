// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/chatgptauth"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/update"
)

// Server hosts the control plane for a running Kernel.
type Server struct {
	k       *runtime.Kernel
	baseDir string

	mu       sync.Mutex
	listener net.Listener
	token    string
	done     chan struct{}
	// serveCancel cancels the context handed to acceptLoop/handleConn. It is
	// derived from Start's ctx, so external ctx cancellation still propagates, but
	// it is ALSO cancelled by initiateShutdown — so a direct Stop() releases
	// in-flight streaming handlers (run/pulse) that block on ctx.Done() instead of
	// leaving them to wait out the per-connection deadline (M461).
	serveCancel context.CancelFunc
	stopOnce    sync.Once
	wg          sync.WaitGroup

	// shutdownCh fires (close) when a client sends CmdShutdown. The
	// daemon's main loop selects on this alongside SIGINT/SIGTERM so
	// programmatic shutdown shares the same orderly exit path as the
	// signal-driven one. Closed at most once (guarded by shutdownOnce).
	shutdownCh   chan struct{}
	shutdownOnce sync.Once

	// pulse is the optional resident proactive engine, injected by the
	// daemon via SetPulse. Nil when Pulse is disabled (AGEZT_PULSE=off);
	// the pulse handlers report "disabled" rather than dereferencing it.
	pulse PulseController

	// agentListCache memoises the expensive (11× journal.Range) result of
	// /api/agents. The Roster, Agents, AgentPage and Roster.tsx all poll the
	// endpoint on a 6–8s cadence; without this, every poll re-walks the
	// entire journal. TTL is short (1.5s) so profile edits still surface
	// quickly while collapsing 5+ in-flight polls of a single tab to one
	// underlying walk. Read-heavy (RWMutex); invalidated on agent mutations
	// so writes bypass the cache. See invalidateAgentListCache().
	agentListCacheMu      sync.RWMutex
	agentListCacheKey     uint64 // content hash of the roster at the time of caching
	agentListCacheResult  []any
	agentListCacheTotal   int
	agentListCacheEnabled int
	agentListCacheAt      time.Time

	// standingFire fires a standing order on demand (M765), injected by the
	// daemon via SetStandingFire (it closes over the daemon's fire path + ctx,
	// so this package stays decoupled from the run launcher). Returns false if
	// the id is unknown. Nil until wired; the handler reports that.
	standingFire func(id string) bool

	// observers adds pulse observers (disk-space watches M767, command probes
	// M768) at runtime. The daemon injects an adapter over the live engine via
	// SetPulseObservers (it owns the DiskUsage func and the warden, keeping this
	// package decoupled from kernel/pulse). Nil when pulse is disabled; the
	// handlers report that.
	observers PulseObservers

	// tenants is the optional multi-tenant registry, injected by the daemon
	// via SetTenants. Nil unless multi-tenancy is enabled; the tenant handlers
	// report "disabled" rather than dereferencing it.
	tenants *tenant.Registry

	// configEnvPinned marks config env vars set in the real process environment at
	// startup (before the config-store injection). The Config Center shows these
	// read-only because the real env overrides the store (M693). Set via
	// SetConfigEnvPinned; nil-safe.
	configEnvPinned map[string]bool

	// cancelOnDisconnect, when true, makes a streaming CmdRun cancel its run
	// if the client connection drops before the run finishes (M35). Off by
	// default so a backgrounded `agt run &` (whose client stays alive) is
	// unaffected; only a genuinely-gone client (Ctrl-C / killed) cancels.
	// Set once at startup via SetCancelOnDisconnect.
	cancelOnDisconnect bool

	// diskFree returns (free, total) bytes for the filesystem at a path,
	// injected by the daemon via SetDiskFree (the daemon passes pulse.DiskUsage,
	// so this package never imports kernel/pulse — the same decoupling as
	// SetPulse). Nil when not wired; the disk handler reports it as unavailable.
	diskFree DiskFreeFunc

	// httpBindings lists the daemon's network-exposed HTTP servers (web UI, REST
	// API, OpenAI API) with whether each is loopback-bound, injected by the daemon
	// via SetHTTPBindings. `agt status` surfaces them and the doctor exposure check
	// (M137) warns on any non-loopback bind. Empty when no HTTP server is enabled.
	httpBindings []HTTPBinding

	// channels lists the messaging channels the daemon has configured (Telegram,
	// Slack, Discord), injected via SetChannels. `agt status` surfaces them so an
	// operator can confirm what's listening without scrolling back to the boot
	// banner (M141). Empty when no channel is configured.
	channels []ChannelInfo

	// channelSend delivers an operator-initiated outbound message through a named
	// channel (M142), injected via SetChannelSender. Kept as a primitive func (not a
	// channel.Channel) so this package never imports the channel plugins. Nil when no
	// channel is configured; handleSend reports that as unavailable.
	channelSend ChannelSender

	// credChain is a short human-readable description of the resolved AWS
	// credential chain (which keyless/ambient layers engaged — SSO, assume-role,
	// IRSA/web-identity, IMDS), injected via SetCredChain (M307). `agt status`
	// surfaces it so an operator on EKS can confirm IRSA actually engaged without
	// grepping the boot banner. Empty when AWS credentials aren't in play.
	credChain string

	// boardStore is the daemon's ONE shared kernel/board instance, injected via
	// SetBoard (M937 mailbox). Board WRITES must go through this instance — the
	// `board` tool holds the same one, and a second instance would clobber its
	// last write (each holds the whole message list and saves it whole). Reads
	// fall back to a fresh read-only Open when nil (tests, older daemons).
	boardStore *board.Store

	// boardNotify publishes the board.posted event for a board write (same
	// closure the `board` tool's OnPost uses), so a control-plane or SDK send
	// wakes standing orders exactly like an agent's send. Nil-safe.
	boardNotify func(m board.Message, corr string)

	// updateSvc is the self-update engine (M860), injected via SetUpdateService.
	// Nil when update is disabled; the update handlers report that.
	updateSvc *update.Service

	// oauthPending tracks in-flight channel OAuth flows (Phase 4) by their opaque
	// state token: the kind/label being connected, the client credentials + PKCE
	// verifier, and the terminal status the browser-redirect callback records.
	// Guarded by oauthMu; entries are short-lived (pruned on completion + by age).
	oauthMu      sync.Mutex
	oauthPending map[string]*oauthFlow

	// chatgpt is the lazily-built "Sign in with ChatGPT" token manager; provLogin
	// is the single in-flight provider OAuth login (the 1455 redirect listener).
	chatgptOnce sync.Once
	chatgpt     *chatgptauth.Manager
	provLoginMu sync.Mutex
	provLogin   *providerLogin
	chatgptSync ChatGPTSyncFunc
}

// ChatGPTSyncFunc refreshes the chatgpt catalog entry from the backend after a
// sign-in and reports the resulting model surface: ids most-preferred first plus
// the model to pin when the operator hasn't chosen one. The kernel never imports
// the provider layer, so the daemon supplies this (see Deps.ChatGPTSync).
type ChatGPTSyncFunc func() (models []string, defaultModel string)

// ChannelSender delivers text out a named channel kind to a channel/chat id. The
// daemon wires it to the live channels' Send methods.
type ChannelSender func(ctx context.Context, kind, channelID, text string) error

// HTTPBinding describes one network-exposed HTTP server for the exposure check.
type HTTPBinding struct {
	Name     string // "web ui" | "rest api" | "openai api"
	Addr     string // host:port the operator configured
	Loopback bool   // true when bound to localhost only
}

// SetHTTPBindings records the daemon's enabled HTTP servers so `agt status` and
// `agt doctor` can report whether any is reachable beyond localhost.
func (s *Server) SetHTTPBindings(b []HTTPBinding) { s.httpBindings = b }

// ChannelInfo describes one configured messaging channel for `agt status`.
type ChannelInfo struct {
	Kind      string // "telegram" | "slack" | "discord"
	Inbound   bool   // true when the channel can receive and act on commands
	Addr      string // listen addr for webhook channels (slack/discord); empty otherwise
	Allowlist int    // number of allowlisted chat/channel ids
}

// SetChannels records the daemon's configured messaging channels so `agt status`
// can report what's listening.
func (s *Server) SetChannels(c []ChannelInfo) { s.channels = c }

// SetChannelSender wires operator-initiated outbound (`agt send`) to the live
// channels. Nil leaves `agt send` reporting "no channels configured".
func (s *Server) SetChannelSender(send ChannelSender) { s.channelSend = send }

// SetChatGPTSync wires the post-sign-in catalog refresh + model-surface lookup.
// Nil leaves the sign-in responses without a model list (the console then falls
// back to whatever the catalog already holds).
func (s *Server) SetChatGPTSync(fn ChatGPTSyncFunc) { s.chatgptSync = fn }

// SetCredChain records the resolved AWS credential-chain description so
// `agt status` can report which credential layer engaged (M307).
func (s *Server) SetCredChain(desc string) { s.credChain = desc }

// SetBoard wires the daemon's shared message-board instance and its post
// notifier (M937 mailbox). Board write commands (board_send/board_ack) require
// it — a fresh per-request Open would clobber the `board` tool's writes; notify
// publishes board.posted so SDK sends wake standing orders like agent sends.
func (s *Server) SetBoard(st *board.Store, notify func(m board.Message, corr string)) {
	s.boardStore = st
	s.boardNotify = notify
}

// SetUpdateService wires the self-update engine (M860). Nil when update is
// disabled; update commands report that rather than dereferencing nil.
func (s *Server) SetUpdateService(svc *update.Service) { s.updateSvc = svc }

// DiskFreeFunc returns the free (available) and total bytes for the filesystem
// containing path (M131). The daemon injects a real implementation
// (pulse.DiskUsage) so this package stays free of platform syscalls.
type DiskFreeFunc func(path string) (free, total uint64, err error)

// SetDiskFree injects the disk-usage probe used by the disk-space health check.
func (s *Server) SetDiskFree(fn DiskFreeFunc) { s.diskFree = fn }

// NewServer constructs a Server that will manage runtime files under
// <baseDir>/runtime/ when Start is called.
func NewServer(k *runtime.Kernel, baseDir string) *Server {
	return &Server{
		k:          k,
		baseDir:    baseDir,
		shutdownCh: make(chan struct{}),
	}
}

// Shutdown returns a channel that closes when a client has issued
// CmdShutdown. The daemon's main loop should select on it next to
// the OS-signal channel so `agt shutdown` reaches the same orderly
// exit path as Ctrl+C. The channel never re-opens; the daemon must
// treat a close as terminal.
func (s *Server) Shutdown() <-chan struct{} { return s.shutdownCh }

// signalShutdown closes shutdownCh exactly once. Used by
// handleShutdown after the OK response has been written to the
// client, so the client read completes before the daemon starts
// tearing the process down.
func (s *Server) signalShutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdownCh) })
}

// Start binds to localhost on an ephemeral port, writes the addr+token
// files, and serves connections until ctx is cancelled or Stop is called.
// Returns once the listener is ready; the accept loop runs in a goroutine.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return errors.New("controlplane: already started")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("controlplane: listen: %w", err)
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		ln.Close()
		return fmt.Errorf("controlplane: rand: %w", err)
	}
	s.token = hex.EncodeToString(tokBytes)
	s.listener = ln
	s.done = make(chan struct{})
	// Derive the serving context so both ctx cancellation AND a direct Stop()
	// (which calls serveCancel via initiateShutdown) unblock streaming handlers.
	serveCtx, serveCancel := context.WithCancel(ctx)
	s.serveCancel = serveCancel

	if err := s.writeRuntimeFiles(ln.Addr().String()); err != nil {
		ln.Close()
		s.listener = nil
		return err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(serveCtx)
	}()
	// React to ctx cancellation by initiating shutdown. This goroutine
	// also exits when Stop is called directly (via s.done).
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case <-ctx.Done():
		case <-s.done:
			return
		}
		s.initiateShutdown()
	}()
	return nil
}

// Addr returns the server's bound TCP address (host:port). Empty before Start.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Token returns the server's auth token. Empty before Start.
func (s *Server) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// tokenIsPrimary reports whether presented equals the primary (admin)
// token, using a constant-time comparison (M187). The primary token is
// the daemon's most privileged credential — it authorizes every command
// on every tenant — so a plain `==`/`!=`, which returns as soon as the
// first differing byte is found, leaks the token byte-by-byte to anyone
// who can time the response. This matches the constant-time check the
// tenant registry already uses (tenant.Registry.Authorize). Length
// differences are revealed (the token is fixed-length hex, so length is
// public anyway), but the secret content is compared in constant time.
func (s *Server) tokenIsPrimary(presented string) bool {
	want := s.Token()
	// A blank presented or server token never authorizes (defense in
	// depth, mirroring tenant.Registry.Authorize): subtle.ConstantTimeCompare
	// of two empty strings returns 1, which would let an empty token match
	// an as-yet-unset server token. Emptiness is not token-content, so this
	// short-circuit leaks nothing secret.
	if want == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(want)) == 1
}

// maxRequestBytes bounds a single control-plane request line (M188). The
// request is read before authentication, so any local client reaching
// the loopback port can stream bytes here; 16 MiB is far above any
// legitimate command (even a large inline run prompt) while bounding a
// pre-auth memory-exhaustion DoS.
const maxRequestBytes = 16 << 20

// errRequestTooLarge is returned when a request line exceeds maxRequestBytes.
var errRequestTooLarge = errors.New("controlplane: request exceeds max size")

// readBoundedLine reads one newline-delimited line from r, bounding the
// total to max bytes (M188). It reads in buffer-sized ReadSlice chunks
// (which return bufio.ErrBufferFull for a line longer than the reader's
// buffer), copying each out before the next read so the returned slice is
// stable, and returns errRequestTooLarge once the accumulated line would
// exceed max — instead of allocating without bound. A trailing chunk with
// io.EOF (stream ended mid-line) is returned with that error.
func readBoundedLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > max {
			return nil, errRequestTooLarge
		}
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return buf, err
	}
}

// Stop closes the listener and removes the runtime files. Idempotent;
// safe to call from cleanup hooks even when Start was driven by ctx.
func (s *Server) Stop() error {
	err := s.initiateShutdown()
	s.wg.Wait()
	return err
}

// initiateShutdown closes the listener and signals the ctx-watcher goroutine
// to exit. Idempotent.
func (s *Server) initiateShutdown() error {
	var firstErr error
	s.stopOnce.Do(func() {
		s.mu.Lock()
		ln := s.listener
		s.listener = nil
		done := s.done
		serveCancel := s.serveCancel
		s.mu.Unlock()

		if done != nil {
			close(done)
		}
		// Release in-flight streaming handlers (run/pulse) blocking on ctx.Done(),
		// so a direct Stop() doesn't have to wait out the per-connection deadline.
		if serveCancel != nil {
			serveCancel()
		}
		if ln != nil {
			if err := ln.Close(); err != nil {
				firstErr = err
			}
		}
		_ = os.Remove(filepath.Join(s.baseDir, "runtime", addrFile))
		_ = os.Remove(filepath.Join(s.baseDir, "runtime", tokenFile))
	})
	return firstErr
}

func (s *Server) writeRuntimeFiles(addr string) error {
	dir := filepath.Join(s.baseDir, "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("controlplane: mkdir runtime: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, addrFile), []byte(addr+"\n"), 0o600); err != nil {
		return fmt.Errorf("controlplane: write addr file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte(s.token+"\n"), 0o600); err != nil {
		return fmt.Errorf("controlplane: write token file: %w", err)
	}
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		s.mu.Lock()
		ln := s.listener
		s.mu.Unlock()
		if ln == nil {
			return
		}
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed → exit cleanly.
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

// cancelOnConnClose derives a child context that is cancelled when the client
// closes conn. Dispatch applies it to every StreamLive command (research/
// council/conductor/planner/chat-summarize): a disconnected client means the
// result can no longer be delivered, so continuing to spend model calls is
// pure waste. It generalizes the run disconnect sentinel in handleRun: since
// these handlers read exactly one request and the client then sends nothing, a
// blocking Read unblocks only on disconnect (or when the handler itself
// returns and handleConn closes the conn, at which point the cancel is a
// harmless no-op). The caller must defer the returned cancel, and handlers
// must never layer a second call on the same conn — two goroutines reading one
// conn race (pulse_subscribe runs its own watcher and is therefore dispatched
// without this wrapper). Unlike run, this is always on: these handlers have
// no detach path, so there is nothing to preserve by keeping the work alive.
func cancelOnConnClose(ctx context.Context, conn net.Conn) (context.Context, context.CancelFunc) {
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		_ = conn.SetReadDeadline(time.Time{}) // clear the 10-min handleConn read deadline
		buf := make([]byte, 1)
		_, _ = conn.Read(buf) // blocks until the client disconnects or the conn closes
		cancel()
	}()
	return cctx, cancel
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// Contain a panic ANYWHERE in this connection's handling — including the
	// pre-auth read/parse phase below — to THIS connection: an unrecovered panic
	// in this goroutine crashes the whole process, taking down every in-flight run
	// and channel. A single malformed/edge-case request must not be a daemon-wide
	// DoS — mirror net/http's per-request recover for the control plane's custom
	// TCP protocol. Deferred before parsing; recoverConn reads req.ID at panic time
	// (empty before the request is parsed). Must be deferred directly so its
	// recover() takes effect.
	var req Request
	defer s.recoverConn(conn, &req)

	// Generous read deadline per request — runs can take minutes.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Minute))

	reader := bufio.NewReader(conn)
	// Bounded read (M188): the request line is read BEFORE authentication
	// (the token is inside it), so any local client that can reach the
	// loopback port can stream bytes here. A plain ReadBytes('\n') grows
	// without limit until a newline, so a client that never sends one
	// drives the daemon to OOM — a pre-auth DoS. Cap the request and
	// reject anything larger.
	line, err := readBoundedLine(reader, maxRequestBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			s.writeResp(conn, Response{Type: RespError, Error: "request too large"})
		}
		return
	}
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "bad request: " + err.Error()})
		return
	}
	// Authentication + authorization (M38). The primary (admin) token
	// authorizes everything, on any tenant. Otherwise the request must name a
	// tenant AND present that tenant's own token: the principal is then that
	// tenant, restricted to the TenantAllowed subset of the command registry
	// and pinned to its own tenant. This completes M14 tenant isolation on the
	// control side — a tenant manages its own runs/policy without the primary
	// token, and cannot touch another tenant or daemon-global state.
	primary := s.tokenIsPrimary(req.Token)
	var tenantID string
	if !primary {
		reqTenant, _ := req.Args["tenant"].(string)
		reqTenant = strings.TrimSpace(reqTenant)
		if s.tenants == nil || reqTenant == "" || !s.tenants.Authorize(reqTenant, req.Token) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unauthorized"})
			return
		}
		tenantID = reqTenant
	}

	// Command lookup happens AFTER the auth gate, and the tenant allowlist is
	// applied BEFORE unknown commands are distinguished: a tenant token probing
	// an unregistered command gets the same deny-by-default "forbidden" the
	// legacy tenantTokenAllows gate produced, not an "unknown command" oracle.
	spec, known := commandRegistry[req.Cmd]
	if !primary {
		if !known || !spec.TenantAllowed {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("forbidden: a tenant token cannot run %q (primary token required)", req.Cmd)})
			return
		}
		// Pin the tenant arg to the authorized tenant (defense in depth — it
		// already matched, but no handler should ever see a different value).
		if req.Args == nil {
			req.Args = map[string]any{}
		}
		req.Args["tenant"] = tenantID
	}
	if !known {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown command: " + req.Cmd})
		return
	}

	dc := &DispatchCtx{Ctx: ctx, Conn: conn, Req: req, S: s, K: s.k, Tenant: tenantID, Primary: primary}
	if spec.TenantRouted {
		// Resolve the request's kernel once at the dispatch boundary. For a
		// primary caller without a tenant arg kernelFor("") == s.k, so
		// tenant-routed commands cost nothing extra in the common case.
		k, err := s.kernelFor(tenantOf(req))
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		dc.K = k
	}
	if spec.Streaming == StreamLive {
		// Long-lived streaming work: tie the handler's context to the client
		// connection so a disconnect cancels the (model-heavy) work instead of
		// spending it into a closed connection. Handlers receive the tied
		// context as their ctx parameter and must NOT call cancelOnConnClose
		// themselves — two goroutines reading one conn would race.
		cctx, cancel := cancelOnConnClose(ctx, conn)
		defer cancel()
		dc.Ctx = cctx
	}
	spec.Handler(dc)
}

func (s *Server) writeResp(conn net.Conn, resp Response) {
	_ = writeResp(conn, resp)
}

// recoverConn is deferred per connection: it turns a panic ANYWHERE in the
// connection's handling — the pre-auth parse phase as well as command handling —
// into an error response to the caller instead of an unrecovered goroutine panic
// that would crash the daemon. It takes a *Request (not an id) and reads the id
// at panic time so it can be deferred at the very top of handleConn, before the
// request has been parsed; an early panic simply carries an empty id. Best-effort
// — if the connection is already broken the error write is dropped; the
// load-bearing guarantee is that the process survives so other connections, runs,
// and channels keep working. recoverConn must be deferred DIRECTLY (not wrapped in
// a closure) so its recover() actually stops the panic.
func (s *Server) recoverConn(conn net.Conn, req *Request) {
	if r := recover(); r != nil {
		var id string
		if req != nil {
			id = req.ID
		}
		s.writeResp(conn, Response{ID: id, Type: RespError, Error: "internal error"})
	}
}

// writeResp is the underlying writer that *returns* its error.
// Used by long-lived handlers (pulse) where a broken pipe is the
// client-disconnect signal that should stop the goroutine; the
// method form above keeps the fire-and-forget shape every other
// handler relies on.
func writeResp(conn net.Conn, resp Response) error {
	enc, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	enc = append(enc, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err = conn.Write(enc)
	return err
}

// ----- command handlers -----

func (s *Server) handleVersion(conn net.Conn, req Request) {
	rev, committed, modified := brand.BuildInfo()
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			brand.Binary:       brand.Version,
			"protocol_version": brand.ProtocolVersion,
			// Build provenance (M971) — lets operators confirm which build a
			// daemon is actually running, since the semver only moves per release.
			"revision":       rev,
			"built":          committed,
			"build_modified": modified,
		},
	})
}

func (s *Server) handleHalt(conn net.Conn, req Request) {
	reason, _, err := argString(req.Args, "reason")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.k.HaltWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": true,
		"reason": reason,
	}})
}

// handleCancelRun cancels a single in-flight run by correlation id (M32),
// leaving the kernel un-halted and other runs untouched — the targeted
// alternative to the global halt. Routes to the tenant kernel when a
// tenant is named (empty → primary), mirroring handleRun.
func (s *Server) handleCancelRun(conn net.Conn, req Request) {
	corr, err := requiredArgString(req.Args, "correlation")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	tenantID, _, err := argString(req.Args, "tenant")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cancelled := k.CancelRun(corr)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"cancelled":   cancelled,
	}})
}

func (s *Server) handleResume(conn net.Conn, req Request) {
	reason, _, err := argString(req.Args, "reason")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.k.ResumeWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": false,
		"reason": reason,
	}})
}

func (s *Server) handleWhy(conn net.Conn, req Request) {
	idAny := req.Args["event_id"]
	id, _ := idAny.(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.event_id required"})
		return
	}
	// Tenant-scoped (M53): an empty tenant traces the primary journal; a named
	// tenant traces its own isolated journal, so a tenant walks only its own
	// events — completing tenant isolation on the observability surface (M39
	// did runs list/stats; this does why).
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	events, err := k.Why(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]any, 0, len(events))
	for _, e := range events {
		out = append(out, e)
	}
	// Parent backlink (M42): if this chain belongs to a sub-agent run,
	// surface its lead's correlation so an operator can walk child→parent
	// (only parent→child was visible before). correlation is the chain's
	// shared id; parent_correlation is "" for top-level runs.
	corr := ""
	if len(events) > 0 {
		corr = events[0].CorrelationID
	}
	parent := ""
	if corr != "" {
		parent = k.ParentOf(corr)
	}
	// Causation provenance (SPEC-01 §7.1): the chain of events linked by
	// causation_id from the root cause down to this one, ordered oldest-first.
	// Unlike the correlation grouping above, this crosses correlation
	// boundaries — e.g. a Pulse initiative back to its originating tick, which
	// carries a different correlation and is therefore absent from `events`.
	// Best-effort: a failure here must not sink the whole why response.
	causation := make([]any, 0, 4)
	if chain, cErr := k.Causes(id); cErr == nil && len(chain) > 1 {
		for _, e := range chain {
			causation = append(causation, e)
		}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events":             out,
			"correlation":        corr,
			"parent_correlation": parent,
			"causation_chain":    causation,
		},
	})
}

// handleWhoami reports the authenticated principal (M62). By the time a request
// reaches a handler, handleConn has already verified the token: the primary
// token equals s.Token(); any other token that got here authenticated as the
// tenant named in (and pinned to) req.Args["tenant"]. So identity is a pure
// read of req.Token vs the primary token — no new auth state.
func (s *Server) handleWhoami(conn net.Conn, req Request) {
	if s.tokenIsPrimary(req.Token) {
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"identity": "primary",
				"primary":  true,
				"tenant":   "",
			},
		})
		return
	}
	tenant, _ := req.Args["tenant"].(string)
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"identity": "tenant",
			"primary":  false,
			"tenant":   tenant,
		},
	})
}

func (s *Server) handleVerify(conn net.Conn, req Request) {
	if err := s.k.Verify(); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": true}})
}

func (s *Server) handleApprovals(conn net.Conn, req Request) {
	pending := s.k.Approvals().Pending()
	out := make([]map[string]any, 0, len(pending))
	for _, p := range pending {
		out = append(out, map[string]any{
			"id":                     p.ID,
			"capability":             p.Capability,
			"tool_name":              p.ToolName,
			"input":                  p.Input,
			"reason":                 p.Reason,
			"actor":                  p.Actor,
			"correlation_id":         p.CorrelationID,
			"created_unix":           p.CreatedAt.Unix(),
			"timeout_unix":           p.Timeout.Unix(),
			"effect_class":           p.EffectClass,
			"predicted_effects":      p.PredictedEffects,
			"affected_resources":     p.AffectedResources,
			"rollback_notes":         p.RollbackNotes,
			"confidence":             p.Confidence,
			"canonical_intent":       p.CanonicalIntent,
			"harmful_interpretation": p.HarmfulInterpretation,
			"ambiguity_score":        p.AmbiguityScore,
			"regret_axes":            p.RegretAxes,
			"confirmation_prompt":    p.ConfirmationPrompt,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"pending": out, "count": len(out)},
	})
}

// planSpec is the JSON shape a `agt plan` client submits. It's a thin
// wire shape that the server reifies into scheduler.Plan with the
// kernel's wired LoopRunner + Approvals registry.
type planSpec struct {
	Name        string             `json:"name"`
	MaxParallel int                `json:"max_parallel"`
	Intent      *intentmodel.Frame `json:"intent,omitempty"`
	Nodes       []planNodeSpec     `json:"nodes"`
}

type planNodeSpec struct {
	ID   string   `json:"id"`
	Kind string   `json:"kind"`
	Deps []string `json:"deps,omitempty"`
	// Loop fields.
	Intent string `json:"intent,omitempty"`
	// Gate fields.
	Capability  string `json:"capability,omitempty"`
	Description string `json:"description,omitempty"`
}

func (s *Server) handlePlan(ctx context.Context, conn net.Conn, req Request) {
	rawAny, ok := req.Args["plan_json"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.plan_json required (JSON string)"})
		return
	}
	rawStr, ok := rawAny.(string)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.plan_json must be a JSON string"})
		return
	}
	var spec planSpec
	if err := json.Unmarshal([]byte(rawStr), &spec); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "plan_json parse: " + err.Error()})
		return
	}
	if len(spec.Nodes) == 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "plan has no nodes"})
		return
	}

	runner := s.k.LoopRunner()
	apr := s.k.Approvals()
	nodes := make([]scheduler.Node, 0, len(spec.Nodes))
	for _, ns := range spec.Nodes {
		switch ns.Kind {
		case "loop":
			nodes = append(nodes, &scheduler.LoopNode{
				NodeID: ns.ID, Deps: ns.Deps, Intent: ns.Intent, Runner: runner, IntentFrame: spec.Intent,
			})
		case "gate":
			nodes = append(nodes, &scheduler.GateNode{
				NodeID: ns.ID, Deps: ns.Deps, Approvals: apr,
				Capability: ns.Capability, Description: ns.Description, IntentFrame: spec.Intent,
			})
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("node %q: unknown kind %q (want loop|gate)", ns.ID, ns.Kind)})
			return
		}
	}
	plan := scheduler.Plan{
		Name:        spec.Name,
		MaxParallel: spec.MaxParallel,
		Nodes:       nodes,
	}

	planID := "plan-" + strings.TrimPrefix(req.ID, "q")
	if planID == "plan-" {
		planID = ""
	}

	// Subscribe to per-plan events before launching so the client
	// sees plan.started + every node.* event in order.
	subjectPrefix := "plan."
	sub, err := s.k.Bus().Subscribe(subjectPrefix+">", 1024)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	defer sub.Cancel()

	type planResult struct {
		res *scheduler.PlanResult
		err error
	}
	resultCh := make(chan planResult, 1)
	go func() {
		r, err := s.k.RunPlan(ctx, plan, planID)
		resultCh <- planResult{r, err}
	}()

	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "event subscription closed"})
				return
			}
			s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
		case r := <-resultCh:
			// Drain in-flight events.
			drain := true
			for drain {
				select {
				case ev := <-sub.C:
					if ev == nil {
						drain = false
					} else {
						s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
					}
				default:
					drain = false
				}
			}
			if r.err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: r.err.Error()})
				return
			}
			outputs := map[string]any{}
			for id, nr := range r.res.NodeResults {
				outputs[id] = nr.Output
			}
			s.writeResp(conn, Response{
				ID: req.ID, Type: RespResult,
				Result: map[string]any{
					"plan_id":      r.res.PlanID,
					"node_outputs": outputs,
				},
			})
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) handleDecide(conn net.Conn, req Request) {
	idAny := req.Args["id"]
	id, _ := idAny.(string)
	decAny := req.Args["decision"]
	dec, _ := decAny.(string)
	reasonAny := req.Args["reason"]
	reason, _ := reasonAny.(string)

	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	var decision approval.Decision
	switch dec {
	case "grant":
		decision = approval.DecisionGrant
	case "deny":
		decision = approval.DecisionDeny
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `args.decision must be "grant" or "deny"`})
		return
	}
	if err := s.k.Approvals().Resolve(id, decision, reason, "operator"); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"ok": true, "id": id, "decision": dec},
	})
}

// SetCancelOnDisconnect enables/disables cancelling a streaming run when its
// client connection drops (M35). Called once at startup by the daemon when
// AGEZT_CANCEL_ON_DISCONNECT=on. Off by default.
func (s *Server) SetCancelOnDisconnect(on bool) { s.cancelOnDisconnect = on }

// SetConfigEnvPinned records which config env vars were set in the real process
// environment at startup (before the config-store injection), so the Config
// Center can mark them read-only — the real env wins over an edit (M693). Called
// once at startup; nil-safe (an unset map reads as "nothing pinned").
func (s *Server) SetConfigEnvPinned(pinned map[string]bool) { s.configEnvPinned = pinned }

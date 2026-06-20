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

	"github.com/agezt/agezt/internal/apperrors"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
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

	// standingFire fires a standing order on demand (M765), injected by the
	// daemon via SetStandingFire (it closes over the daemon's fire path + ctx,
	// so this package stays decoupled from the run launcher). Returns false if
	// the id is unknown. Nil until wired; the handler reports that.
	standingFire func(id string) bool

	// diskWatch adds a pulse disk-space observer at runtime (M767), injected by
	// the daemon via SetDiskWatch (the daemon constructs the observer with its
	// DiskUsage func, keeping this package decoupled from kernel/pulse). Returns
	// the observer name + ok. Nil when pulse is disabled; the handler reports that.
	diskWatch func(path string, minPct float64) (string, bool)

	// probeWatch adds a pulse command-probe observer at runtime (M768): the agent
	// runs the command each beat and alerts on red↔green transitions. Injected by
	// the daemon via SetProbeWatch (it builds the warden-gated observer). Nil when
	// pulse is disabled.
	probeWatch func(name string, argv []string) (string, bool)

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
}

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
		return apperrors.Wrap(ctx, "controlplane: listen", err)
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		ln.Close()
		return apperrors.Wrap(ctx, "controlplane: rand", err)
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
		return apperrors.WrapSimple("controlplane: mkdir runtime", err)
	}
	if err := os.WriteFile(filepath.Join(dir, addrFile), []byte(addr+"\n"), 0o600); err != nil {
		return apperrors.WrapSimple("controlplane: write addr file", err)
	}
	if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte(s.token+"\n"), 0o600); err != nil {
		return apperrors.WrapSimple("controlplane: write token file", err)
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
	// tenant, restricted to an allowlist of tenant-routed commands and pinned
	// to its own tenant. This completes M14 tenant isolation on the control
	// side — a tenant manages its own runs/policy without the primary token,
	// and cannot touch another tenant or daemon-global state.
	if !s.tokenIsPrimary(req.Token) {
		reqTenant, _ := req.Args["tenant"].(string)
		reqTenant = strings.TrimSpace(reqTenant)
		if s.tenants == nil || reqTenant == "" || !s.tenants.Authorize(reqTenant, req.Token) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unauthorized"})
			return
		}
		// Authorized as the named tenant. Restrict to tenant-safe commands.
		if !tenantTokenAllows(req.Cmd) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("forbidden: a tenant token cannot run %q (primary token required)", req.Cmd)})
			return
		}
		// Pin the tenant arg to the authorized tenant (defense in depth — it
		// already matched, but no handler should ever see a different value).
		if req.Args == nil {
			req.Args = map[string]any{}
		}
		req.Args["tenant"] = reqTenant
	}

	switch req.Cmd {
	case CmdVersion:
		s.handleVersion(conn, req)
	case CmdRun:
		s.handleRun(ctx, conn, req)
	case CmdHalt:
		s.handleHalt(conn, req)
	case CmdResume:
		s.handleResume(conn, req)
	case CmdWhy:
		s.handleWhy(conn, req)
	case CmdWhoami:
		s.handleWhoami(conn, req)
	case CmdJournalVerify:
		s.handleVerify(conn, req)
	case CmdArtifactGet:
		s.handleArtifactGet(conn, req)
	case CmdArtifactList:
		s.handleArtifactList(conn, req)
	case CmdArtifactDelete:
		s.handleArtifactDelete(conn, req)
	case CmdArtifactCollect:
		s.handleArtifactCollect(conn, req)
	case CmdDataCollections:
		s.handleDataCollections(conn, req)
	case CmdDataRecords:
		s.handleDataRecords(conn, req)
	case CmdDataInsert:
		s.handleDataInsert(conn, req)
	case CmdDataUpdate:
		s.handleDataUpdate(conn, req)
	case CmdDataDelete:
		s.handleDataDelete(conn, req)
	case CmdDataCreateCollection:
		s.handleDataCreateCollection(conn, req)
	case CmdDataDropCollection:
		s.handleDataDropCollection(conn, req)
	case CmdCouncilMembers:
		s.handleCouncilMembers(conn, req)
	case CmdCouncilAsk:
		s.handleCouncilAsk(ctx, conn, req)
	case CmdCouncilSet:
		s.handleCouncilSet(conn, req)
	case CmdApprovals:
		s.handleApprovals(conn, req)
	case CmdApprovalsLog:
		s.handleApprovalsLog(conn, req)
	case CmdApprovalsStats:
		s.handleApprovalsStats(conn, req)
	case CmdDecide:
		s.handleDecide(conn, req)
	case CmdPlan:
		s.handlePlan(ctx, conn, req)
	case CmdPlanHistory:
		s.handlePlanHistory(conn, req)
	case CmdPlanStats:
		s.handlePlanStats(conn, req)
	case CmdCatalogSync:
		s.handleCatalogSync(ctx, conn, req)
	case CmdCatalogList:
		s.handleCatalogList(conn, req)
	case CmdCatalogDiscover:
		s.handleCatalogDiscover(ctx, conn, req)
	case CmdProviderReload:
		s.handleProviderReload(conn, req)
	case CmdProviderConnect:
		s.handleProviderConnect(conn, req)
	case CmdProviderProbe:
		s.handleProviderProbe(conn, req)
	case CmdWhatsAppGatewayStatus:
		s.handleWhatsAppGatewayStatus(conn, req)
	case CmdWhatsAppGatewayQR:
		s.handleWhatsAppGatewayQR(conn, req)
	case CmdProviderLog:
		s.handleProviderLog(conn, req)
	case CmdProviderStats:
		s.handleProviderStats(conn, req)
	case CmdProviderRejections:
		s.handleProviderRejections(conn, req)
	case CmdPulseSubscribe:
		s.handlePulseSubscribe(ctx, conn, req)
	case CmdPlanGenerate:
		s.handlePlanGenerate(ctx, conn, req)
	case CmdPlanRefine:
		s.handlePlanRefine(ctx, conn, req)
	case CmdBudget:
		s.handleBudget(conn, req)
	case CmdBudgetSet:
		s.handleBudgetSet(conn, req)
	case CmdToolList:
		s.handleToolList(conn, req)
	case CmdToolLog:
		s.handleToolLog(conn, req)
	case CmdToolStats:
		s.handleToolStats(conn, req)
	case CmdCacheStats:
		s.handleCacheStats(conn, req)
	case CmdStatus:
		s.handleStatus(conn, req)
	case CmdWardenLog:
		s.handleWardenLog(conn, req)
	case CmdWardenStats:
		s.handleWardenStats(conn, req)
	case CmdPluginList:
		s.handlePluginList(conn, req)
	case CmdShutdown:
		s.handleShutdown(conn, req)
	case CmdJournalTail:
		s.handleJournalTail(conn, req)
	case CmdEdictOverlay:
		s.handleEdictOverlay(conn, req)
	case CmdEdictCompact:
		s.handleEdictCompact(conn, req)
	case CmdEdictLog:
		s.handleEdictLog(conn, req)
	case CmdEdictStats:
		s.handleEdictStats(conn, req)
	case CmdEdictShow:
		s.handleEdictShow(conn, req)
	case CmdEdictTest:
		s.handleEdictTest(conn, req)
	case CmdEdictDenyList:
		s.handleEdictDenyList(conn, req)
	case CmdEdictDenyAdd:
		s.handleEdictDenyAdd(conn, req)
	case CmdEdictDenyRemove:
		s.handleEdictDenyRemove(conn, req)
	case CmdEdictSetLevel:
		s.handleEdictSetLevel(conn, req)
	case CmdEdictSetMode:
		s.handleEdictSetMode(conn, req)
	case CmdStateList:
		s.handleStateList(conn, req)
	case CmdStateGet:
		s.handleStateGet(conn, req)
	case CmdRunsList:
		s.handleRunsList(conn, req)
	case CmdReaperScan:
		s.handleReaperScan(conn, req)
	case CmdRunsStats:
		s.handleRunsStats(conn, req)
	case CmdCancelRun:
		s.handleCancelRun(conn, req)
	case CmdRunPause:
		s.handleRunPause(conn, req)
	case CmdRunResume:
		s.handleRunResume(conn, req)
	case CmdRunStep:
		s.handleRunStep(conn, req)
	case CmdRunSteer:
		s.handleRunSteer(conn, req)
	case CmdRunIntervene:
		s.handleRunIntervene(conn, req)
	case CmdConfig:
		s.handleConfig(conn, req)
	case CmdConfigCenterSet:
		s.handleConfigCenterSet(conn, req)
	case CmdConfigCenterGet:
		s.handleConfigCenterGet(conn, req)
	case CmdConfigCenterList:
		s.handleConfigCenterList(conn, req)
	case CmdConfigCenterDelete:
		s.handleConfigCenterDelete(conn, req)
	case CmdConfigCenterSetRating:
		s.handleConfigCenterSetRating(conn, req)
	case CmdConfigCenterSetAccess:
		s.handleConfigCenterSetAccess(conn, req)
	case CmdConfigCenterAccessLog:
		s.handleConfigCenterAccessLog(conn, req)
	case CmdConfigCenterAudit:
		s.handleConfigCenterAudit(conn, req)
	case CmdConfigCenterHealth:
		s.handleConfigCenterHealth(conn, req)
	case CmdJournalGrep:
		s.handleJournalGrep(conn, req)
	case CmdJournalHead:
		s.handleJournalHead(conn, req)
	case CmdJournalExport:
		s.handleJournalExport(conn, req)
	case CmdRedactTest:
		s.handleRedactTest(conn, req)
	case CmdRateLimitLog:
		s.handleRateLimitLog(conn, req)
	case CmdRateLimitStats:
		s.handleRateLimitStats(conn, req)
	case CmdNetguardLog:
		s.handleNetguardLog(conn, req)
	case CmdWebhookLog:
		s.handleWebhookLog(conn, req)
	case CmdWebhookStats:
		s.handleWebhookStats(conn, req)
	case CmdMemoryAdd:
		s.handleMemoryAdd(conn, req)
	case CmdMemorySupersede:
		s.handleMemorySupersede(conn, req)
	case CmdMemoryList:
		s.handleMemoryList(conn, req)
	case CmdMemoryLog:
		s.handleMemoryLog(conn, req)
	case CmdMemoryGet:
		s.handleMemoryGet(conn, req)
	case CmdMemorySearch:
		s.handleMemorySearch(conn, req)
	case CmdMemoryConsolidate:
		s.handleMemoryConsolidate(conn, req)
	case CmdMemoryForget:
		s.handleMemoryForget(conn, req)
	case CmdMemoryPromote:
		s.handleMemoryPromote(conn, req)
	case CmdMemoryPrune:
		s.handleMemoryPrune(conn, req)
	case CmdMemoryTidy:
		s.handleMemoryTidy(conn, req)
	case CmdMemoryBulkForget:
		s.handleMemoryBulkForget(conn, req)
	case CmdMemoryFindRelated:
		s.handleMemoryFindRelated(conn, req)
	case CmdMemoryAudit:
		s.handleMemoryAudit(conn, req)
	case CmdMemoryClean:
		s.handleMemoryClean(conn, req)
	case CmdScheduleAdd:
		s.handleScheduleAdd(conn, req)
	case CmdScheduleList:
		s.handleScheduleList(conn, req)
	case CmdScheduleSystemTasks:
		s.handleScheduleSystemTasks(conn, req)
	case CmdScheduleRemove:
		s.handleScheduleRemove(conn, req)
	case CmdScheduleRun:
		s.handleScheduleRun(conn, req)
	case CmdScheduleEnable:
		s.handleScheduleEnable(conn, req)
	case CmdScheduleEdit:
		s.handleScheduleEdit(conn, req)
	case CmdScheduleFires:
		s.handleScheduleFires(conn, req)
	case CmdScheduleStats:
		s.handleScheduleStats(conn, req)
	case CmdScheduleTest:
		s.handleScheduleTest(conn, req)
	case CmdTenantCreate:
		s.handleTenantCreate(conn, req)
	case CmdTenantList:
		s.handleTenantList(conn, req)
	case CmdTenantRelease:
		s.handleTenantRelease(conn, req)
	case CmdTenantRemove:
		s.handleTenantRemove(conn, req)
	case CmdTenantToken:
		s.handleTenantToken(conn, req)
	case CmdTenantStats:
		s.handleTenantStats(conn, req)
	case CmdDiskStats:
		s.handleDiskStats(conn, req)
	case CmdStorageStats:
		s.handleStorageStats(conn, req)
	case CmdJournalStats:
		s.handleJournalStats(conn, req)
	case CmdChangelog:
		s.handleChangelog(conn, req)
	case CmdWorldAdd:
		s.handleWorldAdd(conn, req)
	case CmdWorldEdit:
		s.handleWorldEdit(conn, req)
	case CmdWorldRelate:
		s.handleWorldRelate(conn, req)
	case CmdWorldResolve:
		s.handleWorldResolve(conn, req)
	case CmdWorldNeighbors:
		s.handleWorldNeighbors(conn, req)
	case CmdWorldLog:
		s.handleWorldLog(conn, req)
	case CmdWorldList:
		s.handleWorldList(conn, req)
	case CmdWorldGet:
		s.handleWorldGet(conn, req)
	case CmdWorldForget:
		s.handleWorldForget(conn, req)
	case CmdSkillList:
		s.handleSkillList(conn, req)
	case CmdSkillGet:
		s.handleSkillGet(conn, req)
	case CmdSkillHistory:
		s.handleSkillHistory(conn, req)
	case CmdSkillPromote:
		s.handleSkillPromote(conn, req)
	case CmdSkillQuarantine:
		s.handleSkillQuarantine(conn, req)
	case CmdSkillRevert:
		s.handleSkillRevert(conn, req)
	case CmdSkillShare:
		s.handleSkillShare(conn, req)
	case CmdSkillReassign:
		s.handleSkillReassign(conn, req)
	case CmdSkillImport:
		s.handleSkillImport(conn, req)
	case CmdSkillHygiene:
		s.handleSkillHygiene(conn, req)
	case CmdSkillFiles:
		s.handleSkillFiles(conn, req)
	case CmdSkillReadFile:
		s.handleSkillReadFile(conn, req)
	case CmdStandingList:
		s.handleStandingList(conn, req)
	case CmdStandingAdd:
		s.handleStandingAdd(conn, req)
	case CmdStandingEdit:
		s.handleStandingEdit(conn, req)
	case CmdStandingSetEnabled:
		s.handleStandingSetEnabled(conn, req)
	case CmdStandingRemove:
		s.handleStandingRemove(conn, req)
	case CmdStandingFire:
		s.handleStandingFire(conn, req)
	case CmdAgentList:
		s.handleAgentList(conn, req)
	case CmdAgentAdd:
		s.handleAgentAdd(conn, req)
	case CmdAgentEdit:
		s.handleAgentEdit(conn, req)
	case CmdAgentSetEnabled:
		s.handleAgentSetEnabled(conn, req)
	case CmdAgentRemove:
		s.handleAgentRemove(conn, req)
	case CmdAgentTaskUpdate:
		s.handleAgentTaskUpdate(conn, req)
	case CmdAgentImpact:
		s.handleAgentImpact(conn, req)
	case CmdAgentTombstone:
		s.handleAgentTombstone(conn, req)
	case CmdAgentGraveyard:
		s.handleAgentGraveyard(conn, req)
	case CmdAgentPermissions:
		s.handleAgentPermissions(conn, req)
	case CmdAgentCapabilities:
		s.handleAgentCapabilities(conn, req)
	case CmdAgentActivity:
		s.handleAgentActivity(conn, req)
	case CmdAgentRepairStatus:
		s.handleAgentRepairStatus(conn, req)
	case CmdAgentRepair:
		s.handleAgentRepair(conn, req)
	case CmdAgentEscalations:
		s.handleAgentEscalations(conn, req)
	case CmdAgentWake:
		s.handleAgentWake(conn, req)
	case CmdAgentResolve:
		s.handleAgentResolve(conn, req)
	case CmdAgentRetire:
		s.handleAgentRetire(conn, req)
	case CmdAgentRevive:
		s.handleAgentRevive(conn, req)
	case CmdToolforgeList:
		s.handleToolforgeList(conn, req)
	case CmdToolforgeShow:
		s.handleToolforgeShow(conn, req)
	case CmdToolforgeDraft:
		s.handleToolforgeDraft(conn, req)
	case CmdToolforgeEdit:
		s.handleToolforgeEdit(conn, req)
	case CmdToolforgeTest:
		s.handleToolforgeTest(conn, req)
	case CmdToolforgePromote:
		s.handleToolforgePromote(conn, req)
	case CmdToolforgeQuarantine:
		s.handleToolforgeQuarantine(conn, req)
	case CmdToolforgeRemove:
		s.handleToolforgeRemove(conn, req)
	case CmdMCPList:
		s.handleMCPList(conn, req)
	case CmdMCPAdd:
		s.handleMCPAdd(conn, req)
	case CmdMCPAttach:
		s.handleMCPAttach(conn, req)
	case CmdMCPDetach:
		s.handleMCPDetach(conn, req)
	case CmdMCPSetEnabled:
		s.handleMCPSetEnabled(conn, req)
	case CmdMCPRemove:
		s.handleMCPRemove(conn, req)
	case CmdACPAgents:
		s.handleACPAgents(ctx, conn, req)
	case CmdToolboxDetect:
		s.handleToolboxDetect(ctx, conn, req)
	case CmdToolboxOutdated:
		s.handleToolboxOutdated(ctx, conn, req)
	case CmdToolboxInstall:
		s.handleToolboxInstall(ctx, conn, req)
	case CmdMarketList:
		s.handleMarketList(conn, req)
	case CmdMarketShow:
		s.handleMarketShow(conn, req)
	case CmdMarketInstall:
		s.handleMarketInstall(ctx, conn, req)
	case CmdMarketUninstall:
		s.handleMarketUninstall(ctx, conn, req)
	case CmdMarketSources:
		s.handleMarketSources(conn, req)
	case CmdMarketAddSource:
		s.handleMarketAddSource(conn, req)
	case CmdMarketRemoveSource:
		s.handleMarketRemoveSource(conn, req)
	case CmdMarketSync:
		s.handleMarketSync(ctx, conn, req)
	case CmdWorkflowList:
		s.handleWorkflowList(conn, req)
	case CmdWorkflowShow:
		s.handleWorkflowShow(conn, req)
	case CmdWorkflowSave:
		s.handleWorkflowSave(conn, req)
	case CmdWorkflowRemove:
		s.handleWorkflowRemove(conn, req)
	case CmdWorkflowSetEnabled:
		s.handleWorkflowSetEnabled(conn, req)
	case CmdWorkflowRun:
		s.handleWorkflowRun(conn, req)
	case CmdWorkflowDraft:
		s.handleWorkflowDraft(conn, req)
	case CmdWorkflowRefine:
		s.handleWorkflowRefine(conn, req)
	case CmdWorkflowRuns:
		s.handleWorkflowRuns(conn, req)
	case CmdWorkflowTemplates:
		s.handleWorkflowTemplates(conn, req)
	case CmdWorkflowWebhook:
		s.handleWorkflowWebhook(conn, req)
	case CmdWorkflowTestNode:
		s.handleWorkflowTestNode(conn, req)
	case CmdSandboxList:
		s.handleSandboxList(conn, req)
	case CmdSandboxFile:
		s.handleSandboxFile(conn, req)
	case CmdSandboxDelete:
		s.handleSandboxDelete(conn, req)
	case CmdConfigSchema:
		s.handleConfigSchema(conn, req)
	case CmdConfigValues:
		s.handleConfigValues(conn, req)
	case CmdChannelList:
		s.handleChannelList(conn, req)
	case CmdChannelAccountSet:
		s.handleChannelAccountSet(conn, req)
	case CmdChannelAccountRemove:
		s.handleChannelAccountRemove(conn, req)
	case CmdChannelOAuthStart:
		s.handleChannelOAuthStart(conn, req)
	case CmdChannelOAuthCallback:
		s.handleChannelOAuthCallback(conn, req)
	case CmdChannelOAuthStatus:
		s.handleChannelOAuthStatus(conn, req)
	case CmdProviderOAuthStart:
		s.handleProviderOAuthStart(conn, req)
	case CmdProviderOAuthStatus:
		s.handleProviderOAuthStatus(conn, req)
	case CmdProviderOAuthImport:
		s.handleProviderOAuthImport(conn, req)
	case CmdProviderOAuthLogout:
		s.handleProviderOAuthLogout(conn, req)
	case CmdConfigSet:
		s.handleConfigSet(conn, req)
	case CmdConfigSchemaRegister:
		s.handleConfigSchemaRegister(conn, req)
	case CmdConfigSchemaUnregister:
		s.handleConfigSchemaUnregister(conn, req)
	case CmdProviderKeyList:
		s.handleProviderKeyList(conn, req)
	case CmdProviderKeyAdd:
		s.handleProviderKeyAdd(conn, req)
	case CmdProviderKeyActivate:
		s.handleProviderKeyActivate(conn, req)
	case CmdProviderKeyRemove:
		s.handleProviderKeyRemove(conn, req)
	case CmdRoutingGet:
		s.handleRoutingGet(conn, req)
	case CmdRoutingSet:
		s.handleRoutingSet(conn, req)
	case CmdChainsGet:
		s.handleChainsGet(conn, req)
	case CmdChainsSet:
		s.handleChainsSet(conn, req)
	case CmdPersonaGet:
		s.handlePersonaGet(conn, req)
	case CmdPersonaSet:
		s.handlePersonaSet(conn, req)
	case CmdPromptsGet:
		s.handlePromptsGet(conn, req)
	case CmdPromptsSet:
		s.handlePromptsSet(conn, req)
	case CmdChatSummarize:
		s.handleChatSummarize(ctx, conn, req)
	case CmdStandingWhy:
		s.handleStandingWhy(conn, req)
	case CmdReflectRun:
		s.handleReflectRun(conn, req)
	case CmdReflectShow:
		s.handleReflectShow(conn, req)
	case CmdPulseStatus:
		s.handlePulseStatus(conn, req)
	case CmdPulsePause:
		s.handlePulsePause(conn, req)
	case CmdPulseResume:
		s.handlePulseResume(conn, req)
	case CmdPulseBeat:
		s.handlePulseBeat(conn, req)
	case CmdPulseCadence:
		s.handlePulseCadence(conn, req)
	case CmdPulseDial:
		s.handlePulseDial(conn, req)
	case CmdPulseFlush:
		s.handlePulseFlush(conn, req)
	case CmdPulseWatch:
		s.handlePulseWatch(conn, req)
	case CmdPulseProbe:
		s.handlePulseProbe(conn, req)
	case CmdPulseUnwatch:
		s.handlePulseUnwatch(conn, req)
	case CmdPulseQuiet:
		s.handlePulseQuiet(conn, req)
	case CmdInbox:
		s.handleInbox(conn, req)
	case CmdSend:
		s.handleSend(conn, req)
	case CmdBoardRead:
		s.handleBoardRead(conn, req)
	case CmdBoardHelp:
		s.handleBoardHelp(conn, req)
	case CmdBoardSend:
		s.handleBoardSend(conn, req)
	case CmdBoardInbox:
		s.handleBoardInbox(conn, req)
	case CmdBoardAck:
		s.handleBoardAck(conn, req)
	case CmdBoardReplies:
		s.handleBoardReplies(conn, req)
	case CmdBoardGet:
		s.handleBoardGet(conn, req)
	case CmdAutonomyFeed:
		s.handleAutonomyFeed(conn, req)
	case CmdUpdateCheck:
		s.handleUpdateCheck(conn, req)
	case CmdUpdateApply:
		s.handleUpdateApply(conn, req)
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown command: " + req.Cmd})
	}
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
	reason, _ := req.Args["reason"].(string)
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
	corr, _ := req.Args["correlation"].(string)
	if corr == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.correlation required"})
		return
	}
	tenantID, _ := req.Args["tenant"].(string)
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	cancelled := k.CancelRun(corr)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"cancelled":   cancelled,
	}})
}

func (s *Server) handleResume(conn net.Conn, req Request) {
	reason, _ := req.Args["reason"].(string)
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	events, err := k.Why(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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

func (s *Server) handleRun(ctx context.Context, conn net.Conn, req Request) {
	intentAny := req.Args["intent"]
	intent, _ := intentAny.(string)
	intent = strings.TrimSpace(intent)
	if intent == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.intent required"})
		return
	}

	// Optional tenant routing: an empty tenant runs on the primary kernel
	// (unchanged single-tenant path); a named tenant routes to its isolated
	// kernel via the registry.
	tenantID, _ := req.Args["tenant"].(string)
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Per-run model override (M148): `agt run --model <id>` routes THIS run to a
	// different model (a cheaper/bigger one) without restarting the daemon — the
	// same per-request routing the OpenAI-compatible API uses. Empty = the kernel
	// default. effModel is the model this run will actually use; the capability
	// gate below must judge it, not the daemon default.
	modelRaw, _, merr := argString(req.Args, "model")
	if merr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: merr.Error()})
		return
	}
	modelOverride := strings.TrimSpace(modelRaw)

	// Run AS a named agent (M783): `agt run --agent <slug>` resolves a roster
	// profile and applies its soul / model / per-run cost ceiling as this run's
	// DEFAULTS. Explicit per-run overrides still win; the profile fills the
	// gaps. Resolved before the vision gate so the gate judges the model the
	// run will actually use. An unknown or paused agent is a usage error —
	// silently running as the default identity would be worse than failing.
	agentRaw, _, agerr := argString(req.Args, "agent")
	if agerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: agerr.Error()})
		return
	}
	var agentProf *roster.Profile
	if agentRef := strings.TrimSpace(agentRaw); agentRef != "" {
		p, found := k.Roster().Get(agentRef)
		if !found {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + agentRef})
			return
		}
		if p.Retired {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first (agt agent revive " + p.Slug + ")"})
			return
		}
		if !p.Enabled {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused (agt agent resume " + p.Slug + ")"})
			return
		}
		if !p.AllowsDirectCall() {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "called")})
			return
		}
		agentProf = &p
		if modelOverride == "" {
			modelOverride = strings.TrimSpace(p.Model)
		}
		// The agent's memory follows it (M786): recalls — context injection
		// and the memory tool — default to its scope (private notes + shared).
		scope := strings.TrimSpace(p.MemoryScope)
		if scope == "" {
			scope = p.Slug
		}
		ctx = memory.WithScope(ctx, scope)
		// And its working directory (M792): file/shell tools operate inside
		// the profile's workspace subdirectory.
		ctx = agent.WithWorkdir(ctx, p.Workdir)
		// And its identity + daily ceiling for the Governor's ledger (M793).
		ctx = runtime.WithAgentIdent(ctx, p.Slug, p.MaxDailyMc)
		// Its own model fallback chain too (M787): primary (the resolved
		// model — an explicit --model still wins the front slot) followed by
		// the profile's ordered fallbacks; the Governor walks it in order.
		if len(p.Fallbacks) > 0 {
			primary := modelOverride
			if primary == "" {
				primary = k.Model()
			}
			ctx = runtime.WithModelChain(ctx, agentModelChain(primary, p.Fallbacks))
		}
	}
	effModel := k.Model()
	if modelOverride != "" {
		effModel = modelOverride
	}

	// Pre-generate the correlation id here (it was minted just before Subscribe
	// below) so the vision sidecar's journaled event links to this run.
	corr := k.NewCorrelation()

	// Vision capability gate (M91): a run carrying image attachments requires a
	// model confirmed to accept image input. Unlike the M25 tool gate (which
	// allows unknown models because many tolerate tool schemas), an image sent to
	// a non-vision model is a guaranteed hard failure — so this denies unless the
	// active model is confirmed vision-capable (confirmed-or-reject), pre-flight,
	// before any provider call. Enforced here at the submission boundary so the
	// agent loop and message type stay untouched.
	imageRefs, _, ierr := argStringList(req.Args, "images")
	if ierr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: ierr.Error()})
		return
	}
	if len(imageRefs) > 0 {
		var visionOK bool
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(effModel); m != nil {
				visionOK = m.SupportsVision()
			}
		}
		if !visionOK {
			// Vision SIDECAR (M821): rather than rejecting, ask a keyed vision model
			// to describe the image(s) and inject that text into the intent, so a
			// non-vision active model still "reads" the photo. Fall back to the hard
			// rejection only when no vision model is configured.
			caption, derr := k.DescribeImages(ctx, corr, imageRefs, "")
			if derr == nil && strings.TrimSpace(caption) != "" {
				intent += "\n\n[Image description (analyzed by a vision model):\n" + caption + "\n]"
				imageRefs = nil // consumed by the sidecar; don't send raw images downstream
			} else {
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "governor.capability",
					Kind:    event.KindCapabilityRejected,
					Actor:   "controlplane",
					Payload: map[string]any{
						"model":            effModel,
						"capability":       "vision",
						"images_requested": len(imageRefs),
					},
				})
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf(
					"model %q does not support vision (image input); add a vision-capable provider key or attach images only to a vision-capable model (see `agt provider check --caps`)",
					effModel)})
				return
			}
		}
		// Gate passed or sidecar consumed the images (M93/M821): carry any remaining
		// image refs into the run so they reach the initial user message.
		if len(imageRefs) > 0 {
			ctx = runtime.WithImages(ctx, imageRefs)
		}
	}
	// Route this run to the override model when given (M148); the loop reads it
	// via modelFromCtx, the same path the OpenAI API uses.
	if modelOverride != "" {
		ctx = runtime.WithModel(ctx, modelOverride)
	}
	// Per-run system-prompt override (M149): replace the base system prompt for
	// this run only; memory/world/skill injection still layers on top. Stored
	// trimmed so the run, the operator, and the dry-run plan all agree.
	sysRaw, _, serr := argString(req.Args, "system")
	if serr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: serr.Error()})
		return
	}
	systemOverride := strings.TrimSpace(sysRaw)
	if systemOverride == "" && agentProf != nil {
		systemOverride = strings.TrimSpace(agentProf.Soul) // the agent's soul IS its system prompt
	}
	if systemOverride != "" {
		ctx = runtime.WithSystem(ctx, systemOverride)
	}
	// Per-run wall-clock timeout override (M154): bound THIS run without a
	// daemon-wide cap. Parsed as a Go duration; a malformed value is a usage error.
	timeoutRaw, _, terr := argString(req.Args, "timeout")
	if terr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: terr.Error()})
		return
	}
	timeoutRaw = strings.TrimSpace(timeoutRaw)
	if timeoutRaw != "" {
		d, perr := time.ParseDuration(timeoutRaw)
		if perr != nil || d <= 0 {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("invalid timeout %q (want a positive Go duration like 30s, 2m)", timeoutRaw)})
			return
		}
		ctx = runtime.WithRunTimeout(ctx, d)
	}
	// Per-run tool restriction (M158): a "tools" arg (present, possibly empty)
	// scopes THIS run to the named tools only — an empty list = no tools at all
	// (a safe, pure-reasoning run). Absent = unrestricted (all tools). A present
	// but non-array value is a usage error, NOT silently a zero-tool run.
	toolsAllow, toolsSet, toerr := argStringList(req.Args, "tools")
	if toerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: toerr.Error()})
		return
	}
	if toolsSet {
		ctx = runtime.WithTools(ctx, toolsAllow)
	}
	// Per-run cost cap (M166): bound THIS run's cumulative provider spend (in
	// USD-microcents) without a daemon-wide ceiling. A malformed (non-numeric)
	// value is a usage error; a non-positive value is treated as uncapped.
	maxCost, _, mcerr := argInt64(req.Args, "max_cost")
	if mcerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: mcerr.Error()})
		return
	}
	if maxCost <= 0 && agentProf != nil {
		maxCost = agentProf.MaxCostMc // the agent's own per-run spend ceiling
	}
	if maxCost > 0 {
		ctx = runtime.WithMaxCost(ctx, maxCost)
	}

	// Session-scoped auto-approve grant (chat "auto-approve Tool Forge this
	// session"): a comma/space-separated list of edict capabilities to auto-grant
	// when policy would otherwise prompt for HITL approval, for this run and every
	// sub-agent it spawns. Never overrides a hard-deny. Used so standing up an
	// agent army doesn't prompt for each tool-forge approval.
	if caps := parseCapList(req.Args["auto_approve_caps"]); len(caps) > 0 {
		ctx = runtime.WithAutoApproveCapabilities(ctx, caps)
	}

	// Assured run (M651): when assure > 0, run the "do-it-for-sure" loop — run,
	// verify completion, retry with the gap fed back — up to that many attempts,
	// instead of a single pass. A malformed value is a usage error.
	assureN, _, acerr := argInt64(req.Args, "assure")
	if acerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: acerr.Error()})
		return
	}

	// Dry-run (M159): resolve exactly what this run WOULD do — effective model
	// (and its catalog capabilities), the system-prompt source, the effective
	// wall-clock timeout, and the precise tool set the agent loop would see after
	// the per-run filter — then return that plan WITHOUT starting the run or
	// spending a token. Parsed from the SAME locals the real run uses (no
	// re-reading req.Args), so the plan can never drift from what would execute.
	// A mistyped dry_run is rejected here rather than silently executing the run.
	dryRun, _, berr := argBool(req.Args, "dry_run")
	if berr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: berr.Error()})
		return
	}
	if dryRun {
		in := runPlanInput{
			Intent:          intent,
			Tenant:          tenantID,
			Model:           effModel,
			ModelOverridden: modelOverride != "",
			SystemSet:       strings.TrimSpace(k.System()) != "",
			SystemOverride:  systemOverride != "",
			Timeout:         timeoutRaw,
			DaemonTimeout:   k.MaxDuration(),
			AllowSet:        toolsSet,
			Allow:           toolsAllow,
			MaxCostMC:       maxCost,
			ModelPriced:     modelPriced(effModel), // authoritative (catalog → fallback table)
		}
		in.StrictPricing, in.ModelHasPrice = strictPricingPlan(k.Provider(), effModel)
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(effModel); m != nil {
				in.ModelKnown = true
				in.SupportsVision = m.SupportsVision()
				in.SupportsTools = m.ToolCall
				in.ContextLimit = m.Limit.Context
			}
		}
		for name := range k.Tools() {
			in.AllToolNames = append(in.AllToolNames, name)
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: buildRunPlan(in)})
		return
	}

	// corr was pre-generated above (before the vision gate) so we can subscribe to
	// this run's subject *before* starting it. No race; no missed events.
	sub, err := k.Bus().Subscribe(k.SubjectForRun(corr), 1024)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	defer sub.Cancel()

	// Cost-cap inert advisory (M169): a per-run cost cap can only trip if the run
	// accrues PRICED spend. On a model with no known pricing (unknown to the catalog
	// AND absent from the fallback table, or a free/local model) the spend computes
	// as $0, so the cap never binds. Journal an advisory tied to this run's
	// correlation so `agt why <run>` shows the guardrail was inert — the run-time
	// counterpart to the dry-run "will not bind" warning. Best-effort.
	if maxCost > 0 && !modelPriced(effModel) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "governor.budget",
			Kind:          event.KindBudgetCapInert,
			Actor:         "controlplane",
			CorrelationID: corr,
			Payload:       map[string]any{"model": effModel, "cap_microcents": maxCost},
		})
	}

	type runResult struct {
		answer string
		err    error
	}
	resultCh := make(chan runResult, 1)
	go func() {
		if assureN > 0 {
			ans, _, err := k.RunAssured(ctx, corr, intent, int(assureN))
			resultCh <- runResult{ans, err}
			return
		}
		if agentProf != nil && agentProf.RetryPolicy != nil && agentProf.RetryPolicy.MaxAttempts > 1 {
			ans, err := k.RunWithRetry(ctx, corr, intent, *agentProf.RetryPolicy)
			resultCh <- runResult{ans, err}
			return
		}
		ans, err := k.RunWith(ctx, corr, intent)
		resultCh <- runResult{ans, err}
	}()

	// Cancel-on-disconnect (M35): if enabled, watch the client connection and
	// cancel this run the moment the client goes away (Ctrl-C / killed),
	// instead of letting it run on headless. The client sends nothing after
	// its request, so a read unblocks only when the connection closes — at
	// which point we cancel via the same path as `agt runs cancel`. We clear
	// the read deadline first so a long run isn't mistaken for a disconnect
	// when the 10-minute handleConn deadline elapses. When the run finishes
	// normally, handleConn's defer closes the conn, the read returns, and
	// CancelRun is a harmless no-op (the run is already gone).
	if s.cancelOnDisconnect {
		go func() {
			_ = conn.SetReadDeadline(time.Time{})
			buf := make([]byte, 1)
			_, _ = conn.Read(buf) // blocks until the connection closes
			k.CancelRun(corr)
		}()
	}

	// Forward events to the client until the run finishes, then drain
	// the subscription one last time and send the final result.
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				// Subscription closed unexpectedly.
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "event subscription closed"})
				return
			}
			s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
		case r := <-resultCh:
			// Drain any in-flight events for this run.
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
			result := map[string]any{
				"answer":         r.answer,
				"correlation_id": corr,
			}
			if agentProf != nil {
				result["agent"] = agentProf.Slug // who the run executed AS (M789)
			}
			// Enrich with this run's cost/iters/model (M146) so `agt run` can report
			// what the run cost without a second round-trip. Reuses the same journal
			// fold as `agt runs` (so the numbers agree); best-effort — a fold error or
			// an unpriced run (mock) just omits the fields.
			if runs, ferr := s.collectRuns(k); ferr == nil {
				if e := runs[corr]; e != nil {
					result["iters"] = e.Iters
					result["spent_mc"] = e.SpentMicrocents
					if e.Model != "" {
						result["model"] = e.Model
					}
				}
			}
			s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
			return
		case <-ctx.Done():
			return
		}
	}
}

// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/runtime"
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
func (s *Server) SetCancelOnDisconnect(on bool) { s.cancelOnDisconnect = on }

// SetConfigEnvPinned records which config env vars were set in the real process
// environment at startup (before the config-store injection), so the Config
// Center can mark them read-only — the real env wins over an edit (M693). Called
// once at startup; nil-safe (an unset map reads as "nothing pinned").
func (s *Server) SetConfigEnvPinned(pinned map[string]bool) { s.configEnvPinned = pinned }

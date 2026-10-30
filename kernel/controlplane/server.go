// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/app/binding"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/app/system"
	appupdate "github.com/agezt/agezt/kernel/app/update"
	"net"
	"sync"

	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/toolboxapi"
	"github.com/agezt/agezt/kernel/tenant"
)

// Server hosts the control plane for a running Kernel.
type Server struct {
	operationOnce sync.Once
	operations    *app.Dispatcher
	operationErr  error
	// scope is the daemon's SCOPE: every family's ports, filled by the
	// composition root. The server holds no handle — it can reach each
	// family's capabilities and nothing of the kernel behind them.
	scope   binding.Scope
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

	// pulseProvider asks for the optional resident proactive engine. The
	// daemon publishes the engine through it; nil when Pulse is disabled
	// (AGEZT_PULSE=off); the pulse handlers report "disabled" rather than
	// dereferencing it.
	pulseProvider func() PulseController

	// One application-owned list/cache service per primary Server.
	rosterListOnce sync.Once
	rosterList     *approster.ListService

	// standingFireProvider asks for the standing-order runner's callback
	// (M765). The daemon publishes it (it closes over the daemon's fire path
	// + ctx, so this package stays decoupled from the run launcher). Returns
	// false if the id is unknown. Nil until the runner is built; the handler
	// reports that.
	standingFireProvider func() StandingFirer

	// observersProvider asks for the pulse observers (disk-space watches M767,
	// command probes M768) the daemon registers at runtime. The daemon
	// publishes an adapter over the live engine (it owns the DiskUsage func and
	// the warden, keeping this package decoupled from kernel/pulse). Nil when
	// pulse is disabled; the handlers report that.
	observersProvider func() PulseObservers

	// tenants is the optional multi-tenant registry, injected by the daemon
	// through the composition root. Nil unless multi-tenancy is enabled; the tenant handlers
	// report "disabled" rather than dereferencing it.
	tenants *tenant.Registry

	// configEnvPinned marks config env vars set in the real process environment at
	// startup (before the config-store injection). The Config Center shows these
	// read-only because the real env overrides the store (M693). Injected
	// through the composition root; nil-safe.
	configEnvPinned map[string]bool

	// cancelOnDisconnect, when true, makes a streaming CmdRun cancel its run
	// if the client connection drops before the run finishes (M35). Off by
	// default so a backgrounded `agt run &` (whose client stays alive) is
	// unaffected; only a genuinely-gone client (Ctrl-C / killed) cancels.
	cancelOnDisconnect bool

	// diskFree returns (free, total) bytes for the filesystem at a path,
	// injected by the daemon (the daemon passes pulse.DiskUsage, so this
	// package never imports kernel/pulse — the same decoupling as the pulse
	// engine). Nil when not wired; the disk handler reports it as unavailable.
	diskFree DiskFreeFunc

	// httpBindings lists the daemon's network-exposed HTTP servers (web UI, REST
	// API, OpenAI API) with whether each is loopback-bound, injected by the daemon.
	// `agt status` surfaces them and the doctor exposure check
	// (M137) warns on any non-loopback bind. Empty when no HTTP server is enabled.
	httpBindings []HTTPBinding

	// channels lists the messaging channels the daemon has configured (Telegram,
	// Slack, Discord), injected through the composition root. `agt status` surfaces them so an
	// operator can confirm what's listening without scrolling back to the boot
	// banner (M141). Empty when no channel is configured.
	channels []ChannelInfo

	// channelSendProvider asks for the operator-initiated outbound message path
	// through a named channel (M142). Kept as a primitive func (not a
	// channel.Channel) so this package never imports the channel plugins. Nil
	// until the channels are up; handleSend reports that as unavailable.
	channelSendProvider func() ChannelSender

	// credChain is a short human-readable description of the resolved AWS
	// credential chain (which keyless/ambient layers engaged — SSO, assume-role,
	// IRSA/web-identity, IMDS), injected through the composition root (M307). `agt status`
	// surfaces it so an operator on EKS can confirm IRSA actually engaged without
	// grepping the boot banner. Empty when AWS credentials aren't in play.
	credChain string

	// boardStore is the daemon's ONE shared message board, injected through
	// the composition root (M937 mailbox) — the CONTRACT's port (W3.59), not the module's
	// concrete store. Board WRITES must go through this instance — the
	// `board` tool holds the same one, and a second instance would clobber its
	// last write (each holds the whole message list and saves it whole).
	//
	// Nil when the daemon had no board: the board's FILE is the daemon's
	// business, so the control plane never opens one of its own — the reads
	// answer "the board is not available on this daemon" and the writes
	// refuse.
	boardStore boardapi.Store

	// boardNotify publishes the board.posted event for a board write (same
	// closure the `board` tool's OnPost uses), so a control-plane or SDK send
	// wakes standing orders exactly like an agent's send. Nil-safe.
	boardNotify func(m boardapi.Message, corr string)

	// toolboxHost is the host CLI-tool inventory + installer, injected through
	// the composition root — the CONTRACT's port (W5.4b), not the toolbox module.
	//
	// Nil when the daemon has no toolbox: the reads answer "the toolbox is not
	// available on this daemon" and the install refuses — which is NOT the same
	// as a toolbox that found nothing on the machine.
	toolboxHost toolboxapi.Host

	// updateSvc is the self-update engine (M860), injected through the composition root.
	// Nil when update is disabled; the update handlers report that.
	updateSvc appupdate.Backend

	// repairSource is the agent-repair surface the operator's incident
	// resolution drives, injected through the composition root. Nil when unwired; the
	// repair paths report that rather than dereferencing nil.
	repairSource agentapi.RepairSource

	// One stable application-owned channel OAuth state/service per Server.
	channelOAuthOnce    sync.Once
	channelOAuthState   *appchannels.OAuthMemory
	channelOAuthService *appchannels.OAuth

	// providerOAuthState owns the provider login/token-manager lifecycle.
	providerOAuthOnce  sync.Once
	providerOAuthState *providers.OAuth
	chatgptSync        ChatGPTSyncFunc
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
type HTTPBinding = system.HTTPBinding

// ChannelInfo describes one configured messaging channel for `agt status`.
type ChannelInfo = system.ChannelInfo

// DiskFreeFunc returns the free (available) and total bytes for the filesystem
// containing path (M131). The daemon injects a real implementation
// (pulse.DiskUsage) so this package stays free of platform syscalls.
type DiskFreeFunc func(path string) (free, total uint64, err error)

// NewServer constructs a Server that will manage runtime files under
// <baseDir>/runtime/ when Start is called.

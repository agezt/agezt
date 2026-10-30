// SPDX-License-Identifier: MIT

package controlplane

// The adapter's wiring is ONE door.
//
// The composition root builds the scope and hands over what it has; the
// adapter has **no setters** and **no late-binding phase** (the target
// architecture's P8: "wiring is declarative — modules declare
// Requires/Provides; the composition root resolves them").
//
//   - Deps carries everything the daemon has when it builds the adapter. The
//     zero value is fully valid: every field is optional, and nil/false/empty
//     is exactly the state the old setters left behind, so
//     NewServerWithDeps(scope, Deps{}) behaves identically to plain
//     NewServer(scope).
//   - The artifacts that only exist AFTER the adapter is up — the pulse
//     engine, the live channels, the standing-order runner — are PROVIDERS:
//     the root hands over a way to ASK for them, and the answer is nil until
//     the artifact is ready. The adapter asks when it needs one; the root
//     decides when the answer changes. There is no phase to bind, and no
//     window in which the adapter can be handed a half-wired artifact.

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/toolboxapi"
	"github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/tenant"
)

// Deps bundles the dependencies of a Server — everything the daemon can
// supply when it builds the adapter.
//
// The zero value is fully valid: every field is optional, and nil/false/empty
// is exactly the state the corresponding setter would otherwise leave behind
// (board writes unavailable, update disabled, multi-tenancy disabled, ...), so
// NewServerWithDeps(scope, Deps{}) behaves identically to plain NewServer(scope).
type Deps struct {
	// ConfigEnvPinned marks config env vars set in the real environment at
	// startup, shown read-only in the Config Center (M693).
	ConfigEnvPinned map[string]bool
	// Board is the daemon's ONE shared message-board instance (M937) and
	// BoardNotify its board.posted publisher. Leave both nil when the board
	// failed to open.
	//
	// The store is the CONTRACT's port (W3.59): the control plane may not
	// name the board module, and a nil port is a daemon with no board — the
	// reads answer "the board is not available on this daemon" and the writes
	// refuse.
	Board       boardapi.Store
	BoardNotify func(m boardapi.Message, corr string)
	// Toolbox is the host CLI-tool inventory + installer (M956) — the
	// CONTRACT's port (W5.4b). Leave it nil when the daemon has no toolbox:
	// the toolbox commands then answer "the toolbox is not available on this
	// daemon" instead of inspecting a machine they cannot inspect.
	Toolbox toolboxapi.Host
	// UpdateSvc is the self-update engine (M860) — the CONTRACT's port
	// (W5.4c). Nil when update is disabled: the update commands then answer
	// "update is disabled" instead of dereferencing nil.
	UpdateSvc updateapi.Backend
	// RepairSource is the agent-repair surface the operator's incident
	// resolution drives (the overseer's repair source). The daemon injects
	// it: the control plane and the self-repair module both ASK for a
	// repair, and neither imports the tool that provides it — an adapter
	// reaches the module's vocabulary, not a plugin (archcheck's principle
	// P5). Nil when unwired; the repair paths then report that instead of
	// dereferencing nil.
	RepairSource agentapi.RepairSource
	// DiskFree probes free/total bytes for the disk health check (M131).
	DiskFree DiskFreeFunc
	// Tenants is the multi-tenant registry (P6-MULTI); nil = single-tenant.
	// Supplying it here means tenant tokens authorize from the very first
	// accepted connection.
	Tenants *tenant.Registry
	// CancelOnDisconnect makes a streaming run cancel when its client drops (M35).
	CancelOnDisconnect bool
	// HTTPBindings lists the network-exposed HTTP servers for `agt status` and
	// the doctor exposure check (M137).
	HTTPBindings []HTTPBinding
	// CredChain describes the resolved AWS credential chain (M307).
	CredChain string
	// Channels lists the configured messaging channels for `agt status` (M141).
	Channels []ChannelInfo
	// ChatGPTSync refreshes the chatgpt catalog entry after a sign-in and
	// reports the served model surface; nil disables both.
	ChatGPTSync ChatGPTSyncFunc

	// Pulse provides the resident proactive engine. The provider answers nil
	// until the engine is up — the daemon's own readiness, not the adapter's
	// business. Nil when pulse is disabled (AGEZT_PULSE=off); the pulse
	// handlers report "disabled" rather than dereferencing it.
	Pulse func() PulseController
	// Observers provides the pulse engine's runtime observer registry (disk
	// watches M767, command probes M768). The provider answers nil until the
	// engine is up.
	Observers func() PulseObservers
	// ChannelSend provides the live channels' outbound sender (`agt send`,
	// M142). The provider answers nil until the channels are up.
	ChannelSend func() ChannelSender
	// StandingFire provides the resident standing-order runner's callback
	// (M765). The provider answers nil until the runner is built.
	StandingFire func() StandingFirer
}

// NewServerWithDeps constructs a Server with its dependencies applied.
//
// The scope is the COMPOSITION ROOT's: the families' ports, filled. The
// server's own directory is the scope's directory.
//
// The dependencies are applied here and nowhere else: the adapter has no
// setter to call later, and no phase in which the root binds a second half
// of the wiring.
func NewServerWithDeps(scope binding.Scope, d Deps) *Server {
	s := NewServer(scope)
	s.configEnvPinned = d.ConfigEnvPinned
	s.boardStore = d.Board
	s.boardNotify = d.BoardNotify
	s.toolboxHost = d.Toolbox
	s.updateSvc = d.UpdateSvc
	s.repairSource = d.RepairSource
	s.diskFree = d.DiskFree
	s.tenants = d.Tenants
	s.cancelOnDisconnect = d.CancelOnDisconnect
	s.httpBindings = d.HTTPBindings
	s.credChain = d.CredChain
	s.channels = d.Channels
	s.chatgptSync = d.ChatGPTSync
	s.pulseProvider = d.Pulse
	s.observersProvider = d.Observers
	s.channelSendProvider = d.ChannelSend
	s.standingFireProvider = d.StandingFire
	return s
}

// pulseController asks for the resident proactive engine.
//
// The provider answers nil until the engine is up: the daemon decides when
// the engine exists, and the adapter asks when it needs one.
func (s *Server) pulseController() PulseController {
	if s.pulseProvider == nil {
		return nil
	}
	return s.pulseProvider()
}

// pulseObservers asks for the pulse engine's runtime observer registry.
func (s *Server) pulseObservers() PulseObservers {
	if s.observersProvider == nil {
		return nil
	}
	return s.observersProvider()
}

// channelSender asks for the live channels' outbound sender.
func (s *Server) channelSender() ChannelSender {
	if s.channelSendProvider == nil {
		return nil
	}
	return s.channelSendProvider()
}

// standingFirer asks for the resident standing-order runner's callback.
func (s *Server) standingFirer() StandingFirer {
	if s.standingFireProvider == nil {
		return nil
	}
	return s.standingFireProvider()
}

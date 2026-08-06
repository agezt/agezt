// SPDX-License-Identifier: MIT

package controlplane

// Two-phase dependency injection (Phase 2.6 3b-ii), mirroring toolreg's
// Configure/ConfigureLate split:
//
//   - Deps carries everything runDaemon has in hand BEFORE srv.Start — it is
//     applied atomically at construction time via NewServerWithDeps, so the
//     server never accepts a connection without them.
//   - LateDeps carries the closures over artifacts that only exist AFTER the
//     server is up (the live channels, the pulse engine, the standing-order
//     runner); they arrive via Bind at each point of readiness.
//
// Both phases are strictly additive over the individual setters, which remain
// the unit-test surface (30 srv.Set* calls across the controlplane tests) and
// the internal application path.

import (
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/update"
)

// Deps bundles the early injected dependencies of a Server — everything the
// daemon can supply before Start. The zero value is fully valid: every field is
// optional, and nil/false/empty is exactly the state the corresponding setter
// would otherwise leave behind (board writes unavailable, update disabled,
// multi-tenancy disabled, ...), so NewServerWithDeps(k, dir, Deps{}) behaves
// identically to plain NewServer(k, dir).
type Deps struct {
	// ConfigEnvPinned marks config env vars set in the real environment at
	// startup, shown read-only in the Config Center (M693).
	ConfigEnvPinned map[string]bool
	// Board is the daemon's ONE shared message-board instance (M937) and
	// BoardNotify its board.posted publisher. Leave both nil when the board
	// failed to open.
	Board       *board.Store
	BoardNotify func(m board.Message, corr string)
	// DiskFree probes free/total bytes for the disk health check (M131).
	DiskFree DiskFreeFunc
	// UpdateSvc is the self-update engine (M860); nil = update disabled.
	UpdateSvc *update.Service
	// Tenants is the multi-tenant registry (P6-MULTI); nil = single-tenant.
	// Supplying it here (rather than via SetTenants after Start) means tenant
	// tokens authorize from the very first accepted connection.
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
}

// NewServerWithDeps constructs a Server with its early dependencies applied,
// via the same setters the tests use. Call Bind for the late-phase ones.
func NewServerWithDeps(k *runtime.Kernel, baseDir string, d Deps) *Server {
	s := NewServer(k, baseDir)
	s.SetConfigEnvPinned(d.ConfigEnvPinned)
	s.SetBoard(d.Board, d.BoardNotify)
	s.SetDiskFree(d.DiskFree)
	s.SetUpdateService(d.UpdateSvc)
	s.SetTenants(d.Tenants)
	s.SetCancelOnDisconnect(d.CancelOnDisconnect)
	s.SetHTTPBindings(d.HTTPBindings)
	s.SetCredChain(d.CredChain)
	s.SetChannels(d.Channels)
	return s
}

// LateDeps bundles the late-bound dependencies of a Server — closures over
// boot artifacts that only exist after Start (the pulse engine, the live
// channel set, the standing-order runner). Nil fields are skipped.
type LateDeps struct {
	// Pulse is the resident proactive engine; nil = pulse reported disabled.
	Pulse PulseController
	// Observers adds pulse disk/probe watches at runtime (M767/M768).
	Observers PulseObservers
	// ChannelSend delivers operator-initiated outbound (`agt send`, M142).
	ChannelSend ChannelSender
	// StandingFire fires a standing order on demand (M765).
	StandingFire func(id string) bool
}

// Bind applies the non-nil late dependencies.
//
// TIMING: runDaemon calls Bind incrementally — once at each boot point where a
// dependency becomes ready (Pulse+Observers right after the engine starts,
// ChannelSend once the live channels are up, StandingFire once the
// standing-order runner is built) — NOT as one batched call at the end of boot.
// Batching would lengthen the window in which an early client request sees
// "pulse is disabled" / "unavailable" from the engine-start point to the end of
// boot, a real behavior change for requests arriving mid-boot. Because nil
// fields are skipped, each incremental call is an exact no-op for the
// not-yet-ready dependencies, so the per-dependency nil windows are preserved
// byte-for-byte against the old individual-setter wiring.
func (s *Server) Bind(d LateDeps) {
	if d.Pulse != nil {
		s.SetPulse(d.Pulse)
	}
	if d.Observers != nil {
		s.SetPulseObservers(d.Observers)
	}
	if d.ChannelSend != nil {
		s.SetChannelSender(d.ChannelSend)
	}
	if d.StandingFire != nil {
		s.SetStandingFire(d.StandingFire)
	}
}

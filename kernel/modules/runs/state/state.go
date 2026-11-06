// SPDX-License-Identifier: MIT

// Package state is the RUNS MODULE's LIVE-RUN STATE (W3.76ae).
//
// A run in flight has a live state: it is registered in the module's live-run
// registry, it holds a cancel func, a drain ticket and a steering control that
// an operator can pause, step or steer from another goroutine.
//
// That state lived in the daemon — the `k.steers` map (correlation id → the
// live `*steer.Control`) behind the daemon's own `steersMu`, and the three
// verbs that created and released it (`SetupRunState`, `CleanupRunState`,
// `DeregisterRunSteer`). The run engine — which lives INSIDE this module —
// asked the host for all three through its port, and the host held the very
// objects the module already owned: the steering controls are the module's
// type, and the module's steering service asked the host for them back.
//
// This package is that state. The module now owns:
//
//   - the steering registry (the live control of every run in flight);
//   - the per-run registration (the halted check, the duplicate-correlation
//     guard, the tenant and auto-approve decoration, the run's deadline, the
//     registry entry and the drain accounting);
//   - the per-run cleanup (the registry entry, the delegation tallies, the
//     steering slot and the orphaned async spawns).
//
// The daemon keeps the STORE handle (the module's `registry.Registry`) — the
// run-start path registers and unregisters the live runs through it — and it
// hands the module its own configuration (the tenant, the auto-approve
// grant and the run ceiling). Everything else here is the module's.
package state

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/modules/runs/agent"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
	"github.com/agezt/agezt/kernel/tenantctx"
)

// ErrHalted is the sentinel a run gets when the daemon is halted.
//
// This is the ONE definition: the run engine's `ErrHalted` and the daemon's
// `runtime.ErrHalted` are this same value, so `errors.Is` matches every name
// without a translation step.
var ErrHalted = errors.New("runtime: kernel is halted")

// Delegation is the delegation engine's two operations the run's cleanup
// needs: the fan-out and tree tallies of a finished run, and the async
// spawns that were still in flight when it finished.
//
// The engine is the module's own (kernel/modules/runs/delegation); the state
// names it as a PORT so the two packages do not depend on each other's
// internals.
type Delegation interface {
	// ReleaseTallies releases a run's fan-out tally and its tree's total
	// sub-agent tally.
	ReleaseTallies(corr string)
	// CancelSpawns cancels the async delegations of a run that are still
	// pending, and returns their cancel funcs.
	CancelSpawns(corr string) []context.CancelFunc
}

// Dependencies are the doors the live-run state needs.
//
// The registry is the module's own store; the delegation engine is the
// module's own engine; the three configuration values are the HOST's (a run
// is stamped with the tenant it runs under, the operator's auto-approve
// grant and the run ceiling).
type Dependencies struct {
	// Registry is the module's live-run store. Required.
	Registry *registry.Registry

	// Delegation is the module's delegation engine. Optional: a state
	// without it releases no tallies and cancels no spawns.
	Delegation Delegation

	// TenantID is the tenant a run runs under. Empty for the primary.
	TenantID string

	// AutoApproveCapabilities is the operator's daemon-wide grant. Empty
	// means no grant.
	AutoApproveCapabilities map[string]bool

	// MaxDuration is the daemon's run ceiling. Zero means no ceiling.
	MaxDuration time.Duration
}

// State is the runs module's live-run state: the steering registry and the
// per-run registration the run engine asks for.
type State struct {
	mu     sync.Mutex
	steers map[string]*steer.Control
	deps   Dependencies
}

// New returns the live-run state over the given doors.
//
// A state with no registry answers "the run state is not available" instead
// of panicking: a host that never opened itself has no runs.
func New(deps Dependencies) *State {
	return &State{
		steers: make(map[string]*steer.Control),
		deps:   deps,
	}
}

// SetupRunState registers a run that is about to start and decorates its
// context. It returns the decorated run context, the cancel func (the caller
// invokes it on completion), the steering control (the caller wires it into
// the loop's LoopConfig.Steer) and an error.
//
// The method encapsulates:
//   - the halted check (returns ErrHalted if set);
//   - the duplicate-correlation guard (one id per run);
//   - the tenant + auto-approve + timeout context decoration;
//   - the registry's Register(corr, cancel) call;
//   - the drain accounting (registry.Add);
//   - the steering slot registration.
//
// The lock-ordering invariant `registry lock → state lock` is the one the
// daemon documented as `runsMu → steersMu`: the registry's lock is taken
// first, and the steering slot is acquired while it is held.
func (s *State) SetupRunState(corr string, parentCtx context.Context) (context.Context, context.CancelFunc, agent.Steerer, error) {
	if s == nil || s.deps.Registry == nil {
		return nil, nil, nil, errors.New("runtime: the run state is not available")
	}
	reg := s.deps.Registry
	ctx := parentCtx
	reg.Lock()
	if reg.Halted() {
		reg.Unlock()
		return nil, nil, nil, ErrHalted
	}
	// Reject a correlation that is already running: two concurrent runs
	// sharing one id would clobber the run registry — the second's cancel
	// overwrites the first's registry entry, and the first's deferred delete
	// then removes the second's entry, leaving a run uncancellable by
	// Halt/CancelRun. The contract is one id per run; enforce it instead of
	// silently corrupting the registry. (M480)
	if reg.Running(corr) {
		reg.Unlock()
		return nil, nil, nil, fmt.Errorf("runtime: correlation %q is already running", corr)
	}
	// Stamp the tenant identity onto the run context so tenant-aware tools
	// can read it (M219). No-op for the primary kernel (empty TenantID).
	// Done before deriving runCtx so the value propagates through the
	// timeout/cancel context to every tool call.
	ctx = tenantctx.WithTenant(ctx, s.deps.TenantID)
	if len(s.deps.AutoApproveCapabilities) > 0 {
		ctx = runctx.WithAutoApproveCapabilities(ctx, runctx.MergeAutoApproveCapabilities(ctx, s.deps.AutoApproveCapabilities))
	}

	// Per-run wall-clock budget (M31): when configured, the run context
	// carries a deadline so a slow provider / blocking tool can't hang a run
	// forever within a live session. The deadline cancels with
	// DeadlineExceeded (→ task.failed reason=timeout, M30), whereas the
	// cancel stored in the registry (invoked by Halt) cancels with Canceled
	// (→ reason=canceled) — the two stay distinguishable. 0 = no cap. A
	// per-run override (runctx.WithRunTimeout, e.g. `agt run --timeout`)
	// takes precedence over the daemon-wide ceiling; either yields a
	// deadline that cancels with DeadlineExceeded.
	maxDur := s.deps.MaxDuration
	if d := runctx.RunTimeoutFromCtx(ctx); d > 0 {
		maxDur = d
	}
	var runCtx context.Context
	var cancel context.CancelFunc
	if maxDur > 0 {
		runCtx, cancel = context.WithTimeout(ctx, maxDur)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	reg.Register(corr, cancel)
	// Drain accounting (M883): Close waits (bounded) for in-flight runs to
	// settle before tearing down the stores they write to. Add under the
	// same lock as the halted check above, so no run can slip in after Halt
	// flips the flag and Close begins waiting.
	reg.Add(1)
	// Live-steering control surface (M608): registered for the run's whole
	// lifetime so an operator can pause/step/inject from another goroutine.
	// Wired into the agent loop via LoopConfig.Steer below.
	rc := steer.NewControl()
	s.mu.Lock()
	s.steers[corr] = rc
	s.mu.Unlock()
	reg.Unlock()
	return runCtx, cancel, rc, nil
}

// CleanupRunState releases the run's live state and returns the orphan cancel
// funcs (child spawns that were still in flight); the caller invokes them,
// then invokes the run context's cancel.
//
// The delegation engine's own tallies (fan-out, tree total) and its async
// spawn handles live in the module too (W3.76f): the module releases the
// tallies and cancels the pending spawns through its own leaf lock.
//
// The drain accounting is decremented as the LAST step (after every map /
// mutex / spawn-cancel cleanup), so Close's wait sees a counter that
// monotonically tracks the active set.
func (s *State) CleanupRunState(corr string) []context.CancelFunc {
	if s == nil || s.deps.Registry == nil {
		return nil
	}
	reg := s.deps.Registry
	defer reg.Done()
	// Lock ordering: registry lock → state lock (the delegation engine's
	// lock is a leaf — the module never calls back into the daemon while
	// holding it).
	reg.Lock()
	reg.Unregister(corr)
	if s.deps.Delegation != nil {
		// Release this run's fan-out tally (M46) and this tree's total
		// sub-agent tally (M629).
		s.deps.Delegation.ReleaseTallies(corr)
	}
	s.mu.Lock()
	delete(s.steers, corr) // release the steering control (M608)
	s.mu.Unlock()
	var orphans []context.CancelFunc
	if s.deps.Delegation != nil {
		// Cancel any still-pending async delegations of this tree (M881):
		// an un-awaited child must not outlive the run that spawned it. The
		// spawn goroutine observes the cancel, finishes, and journals its
		// terminal events; the handle is dropped here so the id is no longer
		// awaitable.
		orphans = s.deps.Delegation.CancelSpawns(corr)
	}
	reg.Unlock()
	return orphans
}

// DeregisterRunSteer removes the steer handle for corr without taking the
// registry's lock. Used post-run to free the steering slot the instant the
// agent loop returns — BEFORE the deferred CleanupRunState runs (which would
// also delete it, but later in the post-processing pipeline; an operator
// pausing/steering in that window would otherwise get a false success
// against a loop that has already finished, M608). The delete is idempotent.
func (s *State) DeregisterRunSteer(corr string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.steers, corr)
	s.mu.Unlock()
}

// RegisterSteer publishes a child's live-steering control surface in the
// module's steering registry, so an operator can steer an INDIVIDUAL
// sub-agent (M631).
func (s *State) RegisterSteer(corr string, rc *steer.Control) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.steers[corr] = rc
	s.mu.Unlock()
}

// UnregisterSteer removes a child's steering control surface.
func (s *State) UnregisterSteer(corr string) {
	s.DeregisterRunSteer(corr)
}

// Control returns the steering surface for a live run, or nil if there is no
// such active run (finished, never existed, wrong id).
//
// This is the lookup the module's steering service asks for: the live
// controls are the module's own objects, and the module no longer asks the
// host for them.
func (s *State) Control(corr string) *steer.Control {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.steers[corr]
}

// BroadcastShutdown injects the note into every live run's steering control,
// so a run that is being suspended wraps up its current step (best-effort).
func (s *State) BroadcastShutdown(note string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	for _, rc := range s.steers {
		if rc != nil {
			rc.Inject(note, true)
		}
	}
	s.mu.Unlock()
}

// SPDX-License-Identifier: MIT
//
// Runtime runexec lifecycle methods: RunWith + CompleteAgentLifecycle +
// setupRunState + cleanupRunState + deregisterRunSteer + ErrNoVisionModel
// sentinel. Split from runexec_helpers.go during Day 211 god-file
// refactor (#34). Public API unchanged.
package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/tenantctx"
)

// verifyCompletion body moved to runexec.Runner.VerifyCompletion on
// Day 33. The public wrapper (runtime.go:VerifyCompletion) delegates
// to the Runner.

// visionDescribeMaxTokens / assureVerifyMaxTokens / shadowEvalLimit
// moved to runexec/helpers.go on Day 33.

// ErrNoVisionModel is the run-engine sentinel returned by
// DescribeImages when no vision-capable model is available.
// The canonical definition lives in the runtime package
// (referenced by cmd/agezt/main.go:1909); kept here as a
// package-level value (separate identity, same text) so
// callers inside runexec can use it. The *Kernel.DescribeImages
// wrapper translates via errors.Is so external callers see
// the canonical runtime.ErrNoVisionModel.
var ErrNoVisionModel = errors.New("runtime: no vision-capable model available")

// DescribeImages body moved to runexec.Runner.DescribeImages on
// Day 33. Callers in this package reach it through *Kernel.DescribeImages
// (see compose.go / research.go for the production callers).

// RunWith executes one tool-loop using the supplied correlation ID.
// If the kernel is halted before this Run starts, returns ErrHalted. If
// Halt is called during the Run, ctx is cancelled and RunWith returns
// context.Canceled.
//
// Day 34: the body moved to runexec.Runner.RunWith. The lock-
// protected bits (setupRunState / cleanupRunState /
// deregisterRunSteer) live as private methods on *Kernel and are
// reached through SetupRunState / CleanupRunState /
// DeregisterRunSteer KernelAPI wrappers; *Kernel.RunWith just
// delegates so callers compile unchanged.
func (k *Kernel) RunWith(ctx context.Context, corr, intent string) (string, error) {
	if corr == "" {
		return "", errors.New("runtime: correlation id required")
	}
	return k.runexec.RunWith(ctx, corr, intent)
}

// completeAgentLifecycle / shouldRetireAgentAfterComplete /
// resetCompletedCycleTasks moved to the runexec sub-package on
// Day 33 (Runner.CompleteAgentLifecycle + helpers.go). The
// *Kernel.CompleteAgentLifecycle public wrapper above delegates
// to the Runner. RunWith's call site uses that wrapper.

// CompleteAgentLifecycle advances the durable lifecycle for a successful
// non-loop job that ran under an agent profile, such as a scheduled workflow or
// direct tool target. RunWith calls the Runner's CompleteAgentLifecycle
// itself; external runners should call this only after the job has
// genuinely succeeded. Day 33: the body lives in
// runexec.Runner.CompleteAgentLifecycle; the wrapper preserves the
// legacy public surface.
func (k *Kernel) CompleteAgentLifecycle(ctx context.Context, corr string) {
	k.runexec.CompleteAgentLifecycle(ctx, corr)
}

// setupRunState is the private body behind the public
// SetupRunState wrapper (Day 34). Returns the decorated runCtx,
// the cancel func, the steer interface, and an error. Lock
// order runsMu → steersMu; live-steering slot acquisition
// sits AFTER runsMu is released (see runtime.go:~640-650 for
// the history of this invariant).
func (k *Kernel) setupRunState(corr string, parentCtx context.Context) (context.Context, context.CancelFunc, agent.Steerer, error) {
	ctx := parentCtx
	k.runsMu.Lock()
	if k.halted {
		k.runsMu.Unlock()
		return nil, nil, nil, ErrHalted
	}
	// Reject a correlation that is already running: two concurrent
	// RunWith calls sharing one id would clobber the run registry —
	// the second's cancel overwrites the first's k.runs[corr], and
	// the first's deferred delete then removes the second's entry,
	// leaving a run uncancellable by Halt/CancelRun. The contract is
	// one id per run; enforce it instead of silently corrupting the
	// registry. (M480)
	if _, running := k.runs[corr]; running {
		k.runsMu.Unlock()
		return nil, nil, nil, fmt.Errorf("runtime: correlation %q is already running", corr)
	}
	// Per-run wall-clock budget (M31): when configured, the run
	// context carries a deadline so a slow provider / blocking tool
	// can't hang a run forever within a live session. The deadline
	// cancels with DeadlineExceeded (→ task.failed reason=timeout,
	// M30), whereas the cancel stored in k.runs (invoked by Halt)
	// cancels with Canceled (→ reason=canceled) — the two stay
	// distinguishable. 0 = no cap. A per-run override (WithRunTimeout,
	// e.g. `agt run --timeout`) takes precedence over the daemon-wide
	// MaxDuration; either yields a deadline that cancels with
	// DeadlineExceeded.
	// Stamp the tenant identity onto the run context so tenant-aware
	// tools can read it (M219). No-op for the primary kernel (empty
	// TenantID). Done before deriving runCtx so the value propagates
	// through the timeout/cancel context to every tool call.
	ctx = tenantctx.WithTenant(ctx, k.cfg.TenantID)
	if len(k.cfg.AutoApproveCapabilities) > 0 {
		ctx = WithAutoApproveCapabilities(ctx, mergeAutoApproveCapabilities(ctx, k.cfg.AutoApproveCapabilities))
	}

	maxDur := k.cfg.MaxDuration
	if d := runTimeoutFromCtx(ctx); d > 0 {
		maxDur = d
	}
	var runCtx context.Context
	var cancel context.CancelFunc
	if maxDur > 0 {
		runCtx, cancel = context.WithTimeout(ctx, maxDur)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	k.runs[corr] = cancel
	// Drain accounting (M883): Close waits (bounded) for in-flight
	// runs to settle before tearing down the stores they write to.
	// Add under the same lock as the halted check above, so no run
	// can slip in after Halt flips the flag and Close begins waiting.
	k.runWG.Add(1)
	// Live-steering control surface (M608): registered for the run's
	// whole lifetime so an operator can pause/step/inject from
	// another goroutine. Wired into the agent loop via
	// LoopConfig.Steer below. k.steers is guarded by steersMu
	// everywhere else (controlFor, the deregistration paths,
	// resume's shutdown broadcast) — taking only runsMu here raced
	// with them (caught by -race under concurrent Runs). Lock order
	// runsMu → steersMu is respected.
	rc := newRunControl()
	k.steersMu.Lock()
	k.steers[corr] = rc
	k.steersMu.Unlock()
	k.runsMu.Unlock()
	return runCtx, cancel, rc, nil
}

// cleanupRunState is the private body behind the public
// CleanupRunState wrapper. Encapsulates the 5-mutex lock-ordering
// invariant: runsMu → fanoutMu → treeMu → steersMu → spawnsMu.
// Returns the orphan cancel funcs (child spawns that were still in
// flight); caller invokes them, then invokes the runCtx's cancel.
// Day 34: also calls runWG.Done() to balance the Add(1) in
// setupRunState — without it Close() would deadlock on its
// bounded Wait (M883 drain accounting).
func (k *Kernel) cleanupRunState(corr string) []context.CancelFunc {
	// Drain accounting (M883): Close waits (bounded) for in-flight
	// runs to settle before tearing down the stores they write to.
	// Add() lives in setupRunState so it pairs with the halted +
	// duplicate-corr guard; Done() lives here so the waitgroup
	// decrement is the LAST step a run takes (after every map /
	// mutex / spawn-cancel cleanup), and Close's WaitGroup.Wait
	// sees a counter that monotonically tracks the active set.
	defer k.runWG.Done()
	// Lock ordering: runsMu → fanoutMu → treeMu → steersMu → spawnsMu
	// Lock ordering: runsMu → fanoutMu → treeMu → steersMu → spawnsMu
	k.runsMu.Lock()
	delete(k.runs, corr)
	k.fanoutMu.Lock()
	delete(k.fanout, corr) // release this run's fan-out tally (M46)
	k.treeMu.Lock()
	delete(k.tree, corr) // release this tree's total sub-agent tally (M629)
	k.steersMu.Lock()
	delete(k.steers, corr) // release the steering control (M608)
	k.spawnsMu.Lock()
	// Cancel any still-pending async delegations of this tree (M881):
	// an un-awaited child must not outlive the run that spawned it.
	// The spawn goroutine observes the cancel, finishes, and journals
	// its terminal events; the handle is dropped here so the id is no
	// longer awaitable.
	var orphans []context.CancelFunc
	for id, h := range k.spawns {
		if h.rootCorr == corr || h.parentCorr == corr {
			orphans = append(orphans, h.cancel)
			delete(k.spawns, id)
		}
	}
	k.spawnsMu.Unlock()
	k.steersMu.Unlock()
	k.treeMu.Unlock()
	k.fanoutMu.Unlock()
	k.runsMu.Unlock()
	return orphans
}

// deregisterRunSteer removes the steer handle for corr without
// acquiring runsMu. Used post-run to free the steering slot the
// instant the agent loop returns — BEFORE the deferred
// cleanupRunState runs (which would also delete it, but later in
// the post-processing pipeline; an operator pausing/steering in
// that window would otherwise get a false success against a loop
// that has already finished, M608). delete is idempotent.
func (k *Kernel) deregisterRunSteer(corr string) {
	k.steersMu.Lock()
	delete(k.steers, corr)
	k.steersMu.Unlock()
}

// publishHeuristicBypass body moved to runexec.Runner.PublishHeuristicBypass
// on Day 33. RunWith calls it through k.runexec.PublishHeuristicBypass.

// deterministicHeuristicBypass moved to runexec/helpers.go on Day 34
// so the Runner can use it through the package boundary.

// maybeDistill / maybeForge / maybeShadowEval moved to the runexec
// sub-package on Day 32 — Runner.MaybeDistill / MaybeForge /
// MaybeShadowEval hold the bodies. The *Kernel wrappers in
// runtime.go delegate to the Runner.

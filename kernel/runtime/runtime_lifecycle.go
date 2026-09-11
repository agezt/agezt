// SPDX-License-Identifier: MIT

// *Kernel methods: halt/resume/cancel/drain lifecycle, runs map, publish helpers, run-state setup/cleanup, Maybe* helpers, VerifyCompletion, SubjectForRun, NewCorrelation.
// Code extracted from runtime.go during the Day-41 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/runtime/runexec"
)


func (k *Kernel) Halted() bool { return k.halted }

// SetHalted is the write side. Used by lifecycle.Manager under runsMu.
func (k *Kernel) SetHalted(b bool) { k.halted = b }

// Runs returns the live correlation → cancel map. The Manager reads
// and mutates this under runsMu; the kernel's own RunWith also writes
// it, under the same mutex.
func (k *Kernel) Runs() map[string]context.CancelFunc { return k.runs }

// RunsMu exposes the runs mutex so the Manager can honour the lock
// order documented on the Kernel struct (configMu < runsMu < fanoutMu <
// treeMu < steersMu < spawnsMu < mcpMu).
func (k *Kernel) RunsMu() *sync.Mutex { return &k.runsMu }

// RunWG exposes the in-flight WaitGroup so the Manager can wait
// for cancelled runs to unwind (M883).
func (k *Kernel) RunWG() *sync.WaitGroup { return &k.runWG }

// Suspending returns the atomic.Bool that latches true during a
// graceful shutdown (M1002). The Manager reads it but never writes
// it — only kernel/resume.go's Suspend does that.
func (k *Kernel) Suspending() *atomic.Bool { return &k.suspending }

// ConfigMu exposes the config-mutex so the Accessor sub-package can
// honour the same lock order. Day 14.
func (k *Kernel) ConfigMu() *sync.Mutex { return &k.configMu }

// PublishBusEvent is the Accessor sub-package's chokepoint for
// Standing / Roster CRUD side-effects. Returns whatever the bus
// returns; the Accessor ignores the id. Day 15.
func (k *Kernel) PublishBusEvent(spec event.Spec) (*event.Event, error) {
	return k.bus.Publish(spec)
}

// ArtifactStore returns the content-addressed artifact store
// (SPEC-04 §3.6), where the loop offloads oversized tool
// outputs. Exposed for the runexec sub-package's KernelAPI
// (Day 22). The same pointer is reachable via the existing
// Artifacts() method.
func (k *Kernel) ArtifactStore() *artifact.Store { return k.artifacts }

// Runexec sub-package delegations (Day 22 split).
//
// The implementations live in kernel/runtime/runexec. These
// methods preserve the legacy *Kernel.Run / *Kernel.RunAssured
// / *Kernel.RunWith public surface so callers in controlplane,
// cmd/agezt, and the runtime tests keep compiling unchanged.
// Day 23 will move the 260-line RunWith body into the Runner.
//
// Note (Day 22): the legacy Run / RunAssured / RunWith /
// CompleteAgentLifecycle methods still live in runexec.go
// (their post-Day-11 location). They are NOT yet routed through
// the runexec sub-package's Runner; that move is Day 23. The
// public wrappers below are *preparation* for that move: once
// the body migrates into the Runner, these wrappers become
// the public surface and the runexec.go methods become internal.

// DescribeImages runs the vision SIDECAR (M821). The body lives
// in the runexec sub-package's Runner (Day 33); this wrapper
// preserves the legacy *Kernel.DescribeImages public surface.
// The Runner returns its own runexec.ErrNoVisionModel sentinel
// (separate identity, same text) — the wrapper translates to
// the canonical runtime.ErrNoVisionModel so external callers
// can errors.Is the canonical value.
func (k *Kernel) DescribeImages(ctx context.Context, corr string, images []string, hint string) (string, error) {
	desc, err := k.runexec.DescribeImages(ctx, corr, images, hint)
	if errors.Is(err, runexec.ErrNoVisionModel) {
		return desc, ErrNoVisionModel
	}
	return desc, err
}

// MaybeDistill folds the run's journal and, if the run made
// enough tool calls, runs one best-effort distillation. The body
// lives in the runexec sub-package's Runner (Day 32); this wrapper
// preserves the legacy *Kernel.MaybeDistill public surface.
func (k *Kernel) MaybeDistill(ctx context.Context, corr, intent, answer string) {
	k.runexec.MaybeDistill(ctx, corr, intent, answer)
}

// MaybeForge proposes a DRAFT skill. The body lives in the
// runexec sub-package's Runner (Day 32); this wrapper preserves
// the legacy *Kernel.MaybeForge public surface.
func (k *Kernel) MaybeForge(ctx context.Context, corr, intent, answer string) {
	k.runexec.MaybeForge(ctx, corr, intent, answer)
}

// MaybeShadowEval judges shadow skills. The body lives in the
// runexec sub-package's Runner (Day 32); this wrapper preserves
// the legacy *Kernel.MaybeShadowEval public surface.
func (k *Kernel) MaybeShadowEval(ctx context.Context, corr, intent, answer string) {
	k.runexec.MaybeShadowEval(ctx, corr, intent, answer)
}

// VerifyCompletion asks the model whether the given ANSWER
// fully accomplishes TASK. Returns an assure.Verdict with a
// complete/gap judgement. The body lives in the runexec
// sub-package's Runner (Day 33); this wrapper preserves the
// legacy *Kernel.VerifyCompletion public surface.
func (k *Kernel) VerifyCompletion(ctx context.Context, corr, task, answer string) (assure.Verdict, error) {
	return k.runexec.VerifyCompletion(ctx, corr, task, answer)
}

// SetupRunState is the public surface of the per-run registration
// logic. The Runner calls this in place of the inline runsMu +
// steersMu dance that RunWith used to do (Day 34). The method
// encapsulates:
//   - the halted check (returns ErrHalted if set)
//   - the duplicate-corr guard (returns "correlation already running")
//   - the tenant + auto-approve + timeout ctx decoration
//   - the k.runs[corr] = cancel registration
//   - the runWG.Add(1) drain accounting
//   - the steersMu-locked k.steers[corr] = newRunControl() registration
// The lock-ordering invariant `runsMu → steersMu` is documented
// on the Kernel struct and re-checked whenever this method
// changes; the live-steering slot acquisition sits AFTER
// runsMu is released (see runtime.go:~640-650 for the history).
//
// Returns the decorated run context, the cancel func (caller
// invokes on completion), the steer interface (caller wires
// into LoopConfig.Steer), and an error.
func (k *Kernel) SetupRunState(corr string, parentCtx context.Context) (context.Context, context.CancelFunc, agent.Steerer, error) {
	return k.setupRunState(corr, parentCtx)
}

// CleanupRunState is the public surface of the deferred 5-mutex
// cleanup. The Runner calls this in place of the inline deferred
// block that RunWith used to do. The method encapsulates the
// lock-ordering invariant `runsMu → fanoutMu → treeMu → steersMu
// → spawnsMu` and the runWG.Done() call. Returns the orphan
// cancel funcs (child spawns that were still in flight when the
// run finished); the caller invokes them, then invokes the
// runCtx's cancel func.
func (k *Kernel) CleanupRunState(corr string) []context.CancelFunc {
	return k.cleanupRunState(corr)
}

// DeregisterRunSteer removes the steer handle for corr without
// acquiring runsMu. Used post-run to free the steering slot the
// instant the agent loop returns — BEFORE the deferred
// CleanupRunState runs (which would also delete it, but later in
// the post-processing pipeline; an operator pausing/steering in
// that window would otherwise get a false success against a loop
// that has already finished, M608).
func (k *Kernel) DeregisterRunSteer(corr string) {
	k.deregisterRunSteer(corr)
}

// PublishIntentInterpreted publishes the intent.interpreted
// journal event. Wraps the private publishIntentInterpreted in
// intent.go.
func (k *Kernel) PublishIntentInterpreted(corr, actor string, frame intentmodel.Frame) {
	k.publishIntentInterpreted(corr, actor, frame)
}

// PublishContextFailureAnalysis publishes the context-failure
// journal event. Wraps the private publishContextFailureAnalysis
// in context_selection.go.
func (k *Kernel) PublishContextFailureAnalysis(corr, actor string, runErr error) {
	k.publishContextFailureAnalysis(corr, actor, runErr)
}

// PublishHeuristicBypass journals the deterministic-lookup hit
// (what-time-is-it / today's-date) so the run's timeline still
// shows the task, the bypass, and the canned answer. The body
// lives in the runexec sub-package's Runner (Day 33); this wrapper
// preserves the *Kernel-side access for non-Runner callers and
// keeps the KernelAPI contract coherent with the other publish*
// entries.
func (k *Kernel) PublishHeuristicBypass(ctx context.Context, corr, actor, intent, answer string) error {
	return k.runexec.PublishHeuristicBypass(ctx, corr, actor, intent, answer)
}

// PublishBus is the single chokepoint the lifecycle sub-package uses
// to emit kernel.halt / kernel.resume events. Returns whatever the
// bus returns; callers ignore the id.
func (k *Kernel) PublishBus(spec event.Spec) (*event.Event, error) {
	return k.bus.Publish(spec)
}

// Lifecycle sub-package delegations (Day 12 split).
//
// The implementations live in kernel/runtime/lifecycle. These
// methods preserve the legacy *Kernel.Halt / *Kernel.Resume public
// surface so callers in controlplane, cmd/agezt, cmd/agt, and the
// runtime tests keep compiling unchanged. The behaviour is identical
// to the pre-split bodies — they just live in a different file.

// IsHalted reports whether Run will refuse to start.
func (k *Kernel) IsHalted() bool { return k.lifecycle.IsHalted() }

// Halt cancels every in-flight run and prevents new ones.
func (k *Kernel) Halt() { k.lifecycle.Halt() }

// HaltWith is Halt plus a free-text reason recorded on kernel.halt.
func (k *Kernel) HaltWith(reason string) { k.lifecycle.HaltWith(reason) }

// DrainAndHalt cancels all in-flight runs and waits for them to
// unwind, bounded by timeout.
func (k *Kernel) DrainAndHalt(timeout time.Duration) (bool, int) {
	return k.lifecycle.DrainAndHalt(timeout)
}

// CancelRun cancels a single in-flight run by correlation id.
func (k *Kernel) CancelRun(corr string) bool { return k.lifecycle.CancelRun(corr) }

// Resume clears the halt flag, allowing new runs.
func (k *Kernel) Resume() { k.lifecycle.Resume() }

// ResumeWith is Resume plus a free-text reason recorded on
// kernel.resume.
func (k *Kernel) ResumeWith(reason string) { k.lifecycle.ResumeWith(reason) }

// ActiveRuns returns the number of in-flight Run / RunPlan
// invocations.
func (k *Kernel) ActiveRuns() int { return k.lifecycle.ActiveRuns() }

// ActiveRunIDs returns the correlation ids of the runs in flight
// right now, sorted for determinism.
func (k *Kernel) ActiveRunIDs() []string { return k.lifecycle.ActiveRunIDs() }

// NewCorrelation mints a fresh correlation ID suitable for RunWith.
func (k *Kernel) NewCorrelation() string { return k.lifecycle.NewCorrelation() }

// SubjectForRun returns the bus subject pattern that matches every
// event emitted by the agent.Run identified by corr.
func (k *Kernel) SubjectForRun(corr string) string {
	return k.lifecycle.SubjectForRun(corr)
}

// Accessors sub-package delegations (Day 13 split).
//
// These methods preserve the legacy *Kernel.Journal / *Kernel.Bus
// / etc. public surface. They read the underlying fields directly
// (rather than going through *accessors.Accessor) to avoid the
// recursion that would result from the Accessor dispatching back
// into *Kernel via the implicit KernelAPI interface. The Accessor
// itself remains in the kernel/runtime/accessors sub-package and
// is unit-tested independently; it lands its public body once the
// kernel→sub-package call graph is restructured to break the cycle
// (planned for the follow-up commits — the rest of the read
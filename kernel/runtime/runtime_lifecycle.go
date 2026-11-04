// SPDX-License-Identifier: MIT

package runtime

// Provenance: *Kernel methods: publish helpers, run-state setup/cleanup,
//             Maybe* helpers, VerifyCompletion, and the runs module's
//             lifecycle FACE. Code extracted from runtime.go during the Day-41
//             god-file split. Public API unchanged.

import (
	"context"

	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/modules/runs/agent"
	"github.com/agezt/agezt/kernel/modules/runs/api"
)

// The live-run registry is the RUNS MODULE's store (W3.76d:
// kernel/modules/runs/registry): the running correlations, their cancels, the
// halt latch, the drain accounting and the suspend latch. The daemon used
// to expose them as six accessors (Halted, SetHalted, Runs, RunsMu, RunWG,
// Suspending) for the lifecycle manager's benefit; the manager now holds the
// registry itself, so those six doors are gone and the daemon speaks to the
// module through k.registry.

// Runexec sub-package delegations (Day 22 split).
//
// The implementations live in kernel/runtime/runner. These
// methods preserve the legacy *Kernel.Run / *Kernel.RunAssured
// / *Kernel.RunWith public surface so callers in controlplane,
// cmd/agezt, and the runtime tests keep compiling unchanged.
// Day 23 will move the 260-line RunWith body into the Runner.
//
// Note (Day 22): the legacy Run / RunAssured / RunWith /
// CompleteAgentLifecycle methods still live in runner.go
// (their post-Day-11 location). They are NOT yet routed through
// the runner package's Runner; that move is Day 23. The
// public wrappers below are *preparation* for that move: once
// the body migrates into the Runner, these wrappers become
// the public surface and the runner.go methods become internal.

// The RUN ENGINE's own operations live in the runner package's Runner, and
// the daemon spells NONE of them (W3.76ab).
//
// The daemon used to hold six wrappers over the run engine's own body —
// DescribeImages, MaybeDistill, MaybeForge, MaybeShadowEval,
// VerifyCompletion and PublishHeuristicBypass — and the run engine asked the
// HOST for them through its port, and the host asked the run engine back.
// A port entry that is a round trip is a promise the host never had to keep,
// and a wrapper nobody calls outside the daemon is a door to nowhere.
//
// The run engine now calls its own methods, and the daemon calls the module
// where the module's body is (k.runner) — the host holds the run engine, so
// the host names it, but the host does not ANSWER for it a second time.
//
// What stayed in the daemon is the HOST's own: the run's setup and cleanup
// (SetupRunState, CleanupRunState), the publish helpers, and the doors that
// the callers above use to start a run (Run, RunWith, RunWithRetry,
// RunAssured) and to close its lifecycle (CompleteAgentLifecycle).

// SetupRunState is the public surface of the per-run registration
// logic. The Runner calls this in place of the inline runsMu +
// steersMu dance that RunWith used to do (Day 34). The method
// encapsulates:
//   - the halted check (returns ErrHalted if set)
//   - the duplicate-corr guard (returns "correlation already running")
//   - the tenant + auto-approve + timeout ctx decoration
//   - the registry's Register(corr, cancel) call
//   - the runWG.Add(1) drain accounting
//   - the steersMu-locked k.steers[corr] = newRunControl() registration
//
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

// PublishBus is the single chokepoint the lifecycle sub-package uses
// to emit kernel.halt / kernel.resume events. Returns whatever the
// bus returns; callers ignore the id.
func (k *Kernel) PublishBus(spec event.Spec) (*event.Event, error) {
	return k.bus.Publish(spec)
}

// The RUN LIFECYCLE is the runs module's (W3.76aa): the halt latch, the
// drain, the cancel and the read-side queries over the module's live-run
// registry live in kernel/modules/runs/lifecycle, and the module's manager
// answers them under the module's own names.
//
// The daemon used to hold ELEVEN verbs over the module's own operations —
// IsHalted, Halt, HaltWith, DrainAndHalt, CancelRun, Resume, ResumeWith,
// ActiveRuns, ActiveRunIDs, NewCorrelation and SubjectForRun — a second
// spelling of one family. Those are gone: the daemon hands over the module's
// FACE (LifecycleService) and every caller above asks the module.
//
// The daemon keeps the registry itself (k.registry): the run-start path
// registers and unregisters the live runs, and that bookkeeping is the
// host's own.

// LifecycleService returns the RUNS MODULE's run-lifecycle surface — the
// runs module's face.
//
// The daemon holds no verb per lifecycle operation any more (W3.76aa): the
// module keeps the halt latch, the drain, the cancel and the queries over
// its own registry, and the daemon hands over the module's FACE. The verbs
// are the MODULE's names, so one operation has one name.
//
// The face is never absent: the daemon builds the manager while opening
// itself (it hands the module its own doors), and the module's manager
// answers safely even when it was never built.
func (k *Kernel) LifecycleService() api.LifecycleService { return k.lifecycle }

// Accessors sub-package delegations (Day 13 split).
//
// These methods preserve the legacy *Kernel.Journal / *Kernel.Bus
// / etc. public surface. They read the underlying fields directly
// (rather than going through *accessors.Accessor) to avoid the
// recursion that would result from the Accessor dispatching back
// into *Kernel via the implicit KernelAPI interface.
//
// The Accessor itself used to remain in the kernel/runtime/accessors
// sub-package, unit-tested independently — and wired into the kernel
// without ever being called. W3.40 deleted it: the bodies below are the
// only ones, and the sub-package's copy was the reason the kernel's
// module reach was counted twice in the archcheck ratchet.

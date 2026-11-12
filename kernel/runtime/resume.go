// SPDX-License-Identifier: MIT

package runtime

// Provenance: Durable run resume (M1002). The durable ticket and the rules
//             that judge it moved to kernel/modules/runs/resume in W3.76a, and
//             the ticket's LIFECYCLE moved with them in W3.76ah: the daemon
//             keeps the doors it needs for its own boot and shutdown, and the
//             module keeps the operations.

import (
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/resume"
)

// Durable run resume (M1002). See package kernel/modules/runs/resume for the
// on-disk ticket format, the ticket rows, the rules that judge them and the
// ticket's lifecycle — the claim, the per-iteration snapshot and the
// finalization.
//
// The daemon keeps TWO duties over the tickets, and only two:
//
//   - the BOOT-TIME RESUMER: it enumerates the suspended tickets and
//     re-dispatches them. That is the host's boot protocol, so the daemon
//     asks the module for the store and drives the resumption itself.
//   - the SHUTDOWN PROTOCOL: when the daemon goes down, the live tickets are
//     flipped to "suspended" so the resumer picks them up on the next boot
//     instead of the work being cancelled and lost.
//
// Everything else — who owns a ticket, when it is written, when it is
// refreshed and when it is deleted — is the module's, and the run engine asks
// the module for it (W3.76ah). The daemon used to answer for those operations
// through three verbs (ClaimResumeTicket, FinalizeResumeTicket,
// ResumeCheckpointFn) and the run engine — which lives INSIDE that module —
// asked the host for them: a round trip inside one module, three times.

// ResumeStore returns the durable ticket store (nil when resume is disabled) so
// the daemon's boot-time resumer can enumerate suspended runs. (Named to avoid
// clashing with Resume(), which clears the halt flag.)
//
// The store is the MODULE's object (kernel/modules/runs/resume): the daemon
// asks the module for it rather than holding a second handle of its own.
func (k *Kernel) ResumeStore() *resume.Store {
	if k.tickets == nil {
		return nil
	}
	return k.tickets.Store()
}

// ResumeFinalize clears a resumed run's ticket after it terminates, keeping it
// only if the run was interrupted by a subsequent shutdown. The daemon's resumer
// owns the ticket lifecycle for runs it re-dispatches (it marks them owned so the
// inner RunWith/wrapper neither recreates the ticket — which would reset the
// crash-loop attempt counter — nor deletes it), so it calls this when the run
// returns. Same keep/delete rule as the module's own finalization.
func (k *Kernel) ResumeFinalize(corr string, runErr error) {
	if k.tickets == nil {
		return
	}
	k.tickets.Finalize(corr, runErr)
}

// Suspend flips every live run's ticket to "suspended" — the durable signal that
// these runs were interrupted by a restart, not completed — and notifies the
// running agents. It latches the registry's suspend latch so the module's
// finalization keeps (rather than deletes) any ticket whose run is then
// cancelled. Idempotent: a second call is a no-op. Returns the number of
// tickets marked.
//
// Correctness does NOT depend on the agent obeying the notice — a SIGKILL gives
// no grace at all. The continuous per-iteration checkpoint is what actually
// preserves the work; Suspend classifies the shutdown and lets a cooperating loop
// wrap up its current step. Call it BEFORE any cancel on the shutdown path, or a
// cancelled run's ticket is misclassified as operator-cancelled and deleted.
func (k *Kernel) Suspend(reason string) int {
	if k.tickets == nil {
		return 0
	}
	if !k.registry.Suspending().CompareAndSwap(false, true) {
		return 0
	}
	// Notify each running agent at its next boundary (best-effort). The live
	// runs' steering controls are the runs module's own (W3.76ae), so the
	// broadcast asks the module's live-run state.
	k.runState.BroadcastShutdown("The daemon is shutting down and will resume this run on restart. Wrap up the current step if you can; do not start new long-running work.")

	// The SUSPEND TRANSITION is the ticket's own (W3.76ah): the module's
	// lifecycle marks the tickets and publishes the anomaly if it cannot.
	n, _ := k.tickets.SuspendAll()
	payload := map[string]any{"suspended_runs": n}
	if reason != "" {
		payload["reason"] = reason
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "run.suspending",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: payload,
	})
	return n
}

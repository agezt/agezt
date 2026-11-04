// SPDX-License-Identifier: MIT

package api

import "time"

// LifecycleService is the runs module's RUN-LIFECYCLE surface: the halt
// latch, the drain, the cancel, and the read-side queries over the module's
// live-run registry.
//
// The verbs are the MODULE's own names (W3.76aa). The daemon used to answer
// them under names of its own — IsHalted, Halt, HaltWith, DrainAndHalt,
// CancelRun, Resume, ResumeWith, ActiveRuns and ActiveRunIDs — which was a
// second spelling of one family: the module's manager already answered in
// these names, and every caller above had to learn the daemon's.
//
// The port answers with what happened, never with a "the daemon is not
// there" error: a daemon that holds no manager holds no runs, and the caller
// learns that from the answer.
//
// The port does NOT carry the run's identity: minting a correlation id and
// naming a run's bus subject are the HOST's vocabulary (the adapters
// subscribe to them), and the host keeps those two doors.
type LifecycleService interface {
	// IsHalted reports whether new runs are refused.
	IsHalted() bool
	// Halt cancels every in-flight run and prevents new ones.
	Halt()
	// HaltWith is Halt plus a free-text reason recorded on kernel.halt.
	HaltWith(reason string)
	// DrainAndHalt cancels the runs in flight and waits for them to unwind,
	// bounded by timeout. It reports whether the wait timed out and how many
	// runs were still active.
	DrainAndHalt(timeout time.Duration) (timedOut bool, activeRuns int)
	// CancelRun cancels one in-flight run by correlation id.
	CancelRun(corr string) bool
	// Resume clears the halt latch, allowing new runs again.
	Resume()
	// ResumeWith is Resume plus a free-text reason recorded on
	// kernel.resume.
	ResumeWith(reason string)
	// ActiveRuns counts the runs currently in flight.
	ActiveRuns() int
	// ActiveRunIDs lists the correlation ids of the runs in flight, sorted.
	ActiveRunIDs() []string
}

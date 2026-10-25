// SPDX-License-Identifier: MIT

package cadenceapi

import "context"

// Engine is the port over the live cadence resident — the thing that actually
// fires the schedules.
//
// The engine is the DAEMON's (W3.49): the composition root builds it over the
// library it opened and over the kernel's own capabilities (its bus), and
// hands it over as this port. The kernel holds the port and asks the two
// questions it needs answered — "is anything running?" and "wait until it is
// not" — and never names the implementation.
//
// The port is deliberately small: the resident's own firing logic belongs to
// the module, and the host only reads its state.
type Engine interface {
	// Start begins firing the due schedules on the resident's own ticker.
	Start(ctx context.Context)

	// Wait blocks until the in-flight runs are done.
	Wait()

	// WaitIdle blocks until no run is in flight, or the context ends.
	WaitIdle(ctx context.Context) error

	// RunningCount answers how many runs are in flight right now.
	RunningCount() int
}

// SPDX-License-Identifier: MIT

package lifecycle

import (
	"github.com/agezt/agezt/kernel/event"
)

// Host is the minimum surface the Manager needs from the daemon that owns
// it: the two side effects the Manager cannot perform itself.
//
// Keep this interface tight: every new method is a new coupling between the
// runs module and its host, and the whole point of the split is to *narrow*
// that coupling. The registry — the runs, their cancels, the halt latch and
// the drain accounting — is NOT here: the Manager holds it directly,
// because the target architecture gives that data to the runs module.
type Host interface {
	// Suspend latches the daemon's suspend state so an in-flight run that is
	// cancelled during a restart keeps its resume ticket instead of being
	// treated as a clean cancellation (M1002). It returns the number of runs
	// that were suspended.
	Suspend(reason string) int

	// PublishBus publishes an event to the daemon's bus.
	PublishBus(spec event.Spec) (*event.Event, error)
}

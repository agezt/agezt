// SPDX-License-Identifier: MIT

package lifecycle

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/agezt/agezt/kernel/event"
)

// KernelAPI is the minimum surface the Manager needs from its host
// kernel. Every method must be safe to call from any goroutine; the
// Manager respects the runsMu → ... lock order documented on the
// Kernel struct (it takes runsMu itself in Halt/Resume/Cancel and
// expects the host kernel to honour the same order elsewhere).
//
// Keep this interface tight: every new method is a new coupling
// between sub-package and the kernel umbrella, and the whole point
// of the split is to *narrow* that coupling. If a Manager helper
// needs a new piece of state, prefer adding a method to this
// interface over passing the *Kernel itself — that would silently
// re-create the circular import the interface was introduced to
// avoid.
type KernelAPI interface {
	// State accessors (Manager takes runsMu around every call).
	Halted() bool
	SetHalted(bool)

	// Live run registry (Manager reads + mutates under runsMu).
	Runs() map[string]context.CancelFunc
	RunsMu() *sync.Mutex

	// Wait group for the in-flight set (Close drains this; M883).
	RunWG() *sync.WaitGroup

	// Suspending flag (M1002): set during a graceful shutdown so
	// cancelled runs keep their resume ticket instead of being
	// treated as a clean cancellation.
	Suspending() *atomic.Bool

	// Side effects.
	Suspend(reason string) int
	PublishBus(spec event.Spec) (*event.Event, error)
}

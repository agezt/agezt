// SPDX-License-Identifier: MIT

package standing

import (
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The standing application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the standing orders
// need and how each of them answers. The host hands over the handle and
// nothing else — it does not decide what the standing orders are read from,
// and it cannot change the standing wiring without editing the application.
//
// The same shape already exists in app/council and app/roster: the
// application takes the kernel and builds its own ports.
//
// The rows the standing library holds are CONTRACT rows (standingapi.Order is
// the contract's row), so the port hands them over without a conversion.

// Ports are the standing application's capabilities over the standing orders.
type Ports struct {
	// Reader reads the standing library.
	//
	// It is the CONTRACT's store port (W3.50): the daemon opens the file and
	// hands the port over. It is NIL when the daemon holds no standing
	// library — the application answers "no standing orders" rather than
	// calling into nothing. The service already knows this: it refuses a
	// missing reader instead of panicking.
	Reader Reader

	// Writer writes the standing orders: the kernel's four standing verbs.
	//
	// It is a narrow view of the handle — the service can reach these four
	// verbs and nothing else — which is the point of the port.
	Writer Writer

	// Orders lists the standing orders.
	//
	// It is nil-safe: a daemon with no standing library sees no orders
	// rather than a call into nothing. The adapter used to spell that
	// nil-check itself, once per surface; the application spells it once.
	Orders func() []standingapi.Order

	// Remove deletes a standing order.
	Remove func(id string) (bool, error)

	// SetEnabled pauses or resumes a standing order.
	SetEnabled func(id string, enabled bool) error
}

// PortsFromKernel fills the standing application's ports from the kernel
// handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	store := k.Standing()
	return Ports{
		Reader: store,
		Writer: k,
		Orders: func() []standingapi.Order {
			if store == nil {
				return nil
			}
			return store.List()
		},
		Remove: k.RemoveStanding,
		SetEnabled: func(id string, enabled bool) error {
			_, err := k.SetStandingEnabled(id, enabled)
			return err
		},
	}
}

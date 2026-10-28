// SPDX-License-Identifier: MIT

package schedule

import (
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The schedule application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the schedules need
// and how each of them answers. The host hands over the handle and nothing
// else — it does not decide what the schedules are read from, and it cannot
// change the schedule wiring without editing the application.
//
// The same shape already exists in app/council, app/roster and app/standing:
// the application takes the kernel and builds its own ports.
//
// The rows the schedule library holds are CONTRACT rows (cadenceapi.Entry is
// the contract's row), so the port hands them over without a conversion.

// Ports are the schedule application's capabilities over the schedules.
type Ports struct {
	// Store is the schedule library.
	//
	// It is the CONTRACT's host store port (W3.49): the daemon opens the
	// file and hands the port over. It is NIL when the daemon holds no
	// schedule library — the application answers "no schedules" rather than
	// calling into nothing. The service already knows this: it refuses a
	// missing store instead of panicking.
	Store cadenceapi.HostStore

	// Entries lists the schedules.
	//
	// It is nil-safe: a daemon with no schedule library sees no schedules
	// rather than a call into nothing. The adapter used to spell that
	// nil-check itself, once per surface; the application spells it once.
	Entries func() []cadenceapi.Entry

	// Remove deletes a schedule.
	//
	// A daemon with no schedule library has nothing to remove: it says so
	// by removing nothing.
	Remove func(id string) (bool, error)

	// SetEnabled pauses or resumes a schedule.
	//
	// A daemon with no schedule library has nothing to pause: it says so by
	// pausing nothing.
	SetEnabled func(id string, enabled bool) (bool, error)
}

// PortsFromKernel fills the schedule application's ports from the kernel
// handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	store := k.Schedules()
	return Ports{
		Store: store,
		Entries: func() []cadenceapi.Entry {
			if store == nil {
				return nil
			}
			return store.List()
		},
		Remove: func(id string) (bool, error) {
			if store == nil {
				return false, nil
			}
			return store.Remove(id)
		},
		SetEnabled: func(id string, enabled bool) (bool, error) {
			if store == nil {
				return false, nil
			}
			return store.SetEnabled(id, enabled)
		},
	}
}

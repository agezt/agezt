// SPDX-License-Identifier: MIT
package configcenter

import (
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/runtime"
)

// The config center application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the config center
// family needs and how each of them answers. The host hands over the handle
// and nothing else — it does not decide what the config center is read from,
// and it cannot change the config center's wiring without editing the
// application.
//
// The same shape already exists in app/council, app/roster, app/tools and
// app/workboard: the application takes the kernel and builds its own ports.
//
// The rows are the config center's OWN rows (core.ConfigEntry is the config
// center module's row, which the application layer may name), so the port
// hands them over without a conversion.

// Ports are the config center application's capabilities over the config
// center's entries.
type Ports struct {
	// Reader reads the center's entries and its logs.
	//
	// It is NIL when the daemon holds no config center — the service already
	// knows this: it refuses a missing reader instead of panicking.
	Reader Reader

	// Writer writes the center's entries.
	//
	// It is NIL when the daemon holds no config center, for the same reason.
	Writer Writer

	// Entries lists the center's entries and says whether the center exists
	// at all.
	//
	// It is nil-safe: a daemon with no config center sees no entries and no
	// verdict rather than a call into nothing. The adapter used to spell that
	// nil-check itself, once per surface; the application spells it once.
	Entries func() ([]*core.ConfigEntry, bool)

	// Delete removes an entry by key.
	//
	// It is nil-safe in the same way: a daemon with no config center has
	// nothing to delete, and the delete succeeds on nothing.
	Delete func(key string) error

	// Set writes an entry back.
	//
	// It is nil-safe in the same way.
	Set func(entry *core.ConfigEntry) error
}

// PortsFromKernel fills the config center application's ports from the kernel
// handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	return PortsFromCenter(k.ConfigCenter())
}

// PortsFromCenter fills the config center application's ports from a center.
//
// The kernel always opens a config center, so the nil center is not reachable
// through a handle — but the DISCIPLINE has to be there and it has to be
// provable, and this is where it is: a daemon with no config center holds no
// entries, writes nothing and deletes nothing, and it never calls into
// nothing.
func PortsFromCenter(center *core.Center) Ports {
	if center == nil {
		return Ports{
			Entries: func() ([]*core.ConfigEntry, bool) { return nil, false },
			Delete:  func(string) error { return nil },
			Set:     func(*core.ConfigEntry) error { return nil },
		}
	}
	return Ports{
		Reader: center,
		Writer: center,
		Entries: func() ([]*core.ConfigEntry, bool) {
			return center.ListEntries(), true
		},
		Delete: center.Delete,
		Set:    center.Set,
	}
}

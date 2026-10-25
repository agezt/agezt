// SPDX-License-Identifier: MIT

package cadenceapi

import (
	"time"
)

// HostStore is the port through which the HOST drives the schedule library.
//
// The plugin's port (Store) is narrower — the agent-facing schedule tool adds
// schedules, retargets them and lists them. The host's port is wider: the
// control plane retires a schedule when an agent goes away, the guardians
// re-arm one, the app layer edits its cadence, and the status view counts
// what is armed.
//
// The width rule of the whole migration: a port is as wide as the WEAKEST
// holder of it may hold. The tool keeps the narrow one (Store), the host
// drives this one, and the module's store answers both — so the two shapes
// cannot drift apart: if the tool's port grows a verb, the host's port grows
// it too, and the module's implementation is the one that has to answer.
//
// The library's FILE is the daemon's (W3.49): the composition root opens it
// and hands it over. The kernel holds the port and drives the lifecycle
// through it.
type HostStore interface {
	Store

	// AddWindow records a schedule that fires every interval but only inside
	// a daily time window.
	AddWindow(intent string, interval time.Duration, startMin, endMin, days int, tz, model, source string, now time.Time) (Entry, error)

	// Get returns one entry by id; the bool is false if absent.
	Get(id string) (Entry, bool)

	// Count returns how many entries are stored.
	Count() int

	// SetEnabled arms or disarms an entry.
	SetEnabled(id string, enabled bool) (bool, error)

	// SetIntent rewrites what an entry is for.
	SetIntent(id, intent string) (bool, error)

	// SetModel rebinds an entry to another model.
	SetModel(id, model string) (bool, error)

	// SetIntentTarget points an entry back at a plain intent (it stops
	// targeting a workflow, a tool or a system task).
	SetIntentTarget(id string) (bool, error)

	// Reschedule rewrites an entry's cadence in place.
	Reschedule(id, mode string, interval time.Duration, atMinutes, endMinutes, days int, tz string, onceAt, now time.Time) (bool, error)

	// RunNow fires an entry at once, outside its cadence.
	RunNow(id string) (bool, error)
}

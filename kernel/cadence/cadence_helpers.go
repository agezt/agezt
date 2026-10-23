// SPDX-License-Identifier: MIT

package cadence

// Provenance: Cadence Entry: applyZone + nextWindowSlot + dayAllowed + nextDaily
//             (timing helpers). Extracted from cadence_entry.go during the Day-211
//             god-file split. Public API unchanged.
//
// The helpers live in the contract layer now (kernel/contract/cadenceapi),
// because the schedule ROW does: the entry's forecast is a rule about the row,
// and the store computes the same instants when it writes an entry. The module
// keeps thin wrappers so its own callers are unchanged.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/cadenceapi"
)

// applyZone returns now converted into the IANA zone tz, or now unchanged when
// tz is empty (use the caller's zone). It errors on an unloadable zone name.
func applyZone(now time.Time, tz string) (time.Time, error) {
	return cadenceapi.ApplyZone(now, tz)
}

// nextWindowSlot returns the next firing instant strictly after now for a
// windowed-interval schedule: slots are start, start+interval, … up to and
// including end, on permitted weekdays. After the window closes for a day it
// jumps to the next permitted day's start. Walks by calendar date (DST-correct).
func nextWindowSlot(now time.Time, start, end int, intervalSec int64, days int) time.Time {
	return cadenceapi.NextWindowSlot(now, start, end, intervalSec, days)
}

// nextDaily returns the next local-time occurrence of atMinutes-past-midnight,
// strictly after now, that falls on a weekday permitted by days. It walks
// forward by calendar date (not by adding 24h) so it stays correct across DST
// transitions.
func nextDaily(now time.Time, atMinutes, days int) time.Time {
	return cadenceapi.NextDaily(now, atMinutes, days)
}

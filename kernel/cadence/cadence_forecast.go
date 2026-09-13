// SPDX-License-Identifier: MIT

// Cadence Entry: Forecast + advance (dry-run scheduling math).
// Extracted from cadence_entry.go during the Day-211 god-file split.
// Public API unchanged.
package cadence


import (
	"time"
)
// Forecast returns the next n fire times (Unix seconds) strictly after `from`,
// simulating the cadence forward — the dry-run behind `agt schedule test` (M120).
// The first entry matches the engine's current NextRunUnix when that is still in
// the future, so the forecast lines up with what the daemon will actually do; the
// rest are simulated by repeatedly advancing. A `once` schedule yields its single
// future fire (or none). Pure: no engine state, deterministic given `from`.
func (e Entry) Forecast(from time.Time, n int) []int64 {
	if n <= 0 {
		return nil
	}
	if e.Mode == ModeOnce {
		if e.NextRunUnix > from.Unix() {
			return []int64{e.NextRunUnix}
		}
		return nil
	}
	out := make([]int64, 0, n)
	var first int64
	if e.NextRunUnix > from.Unix() {
		first = e.NextRunUnix
	} else {
		first = e.advance(from)
	}
	out = append(out, first)
	cur := time.Unix(first, 0).In(from.Location())
	for len(out) < n {
		t := e.advance(cur)
		if t <= cur.Unix() {
			break // no forward progress (defensive; shouldn't happen for valid entries)
		}
		out = append(out, t)
		cur = time.Unix(t, 0).In(from.Location())
	}
	return out
}

// advance computes the next-run time after firing at now. Wall-clock cadences
// (daily/window) are evaluated in the entry's zone so "09:00" means 09:00 there;
// an empty TZ leaves now in whatever zone the caller passed (the daemon local).
func (e Entry) advance(now time.Time) int64 {
	n, _ := applyZone(now, e.TZ) // e.TZ already validated at write time
	switch e.Mode {
	case ModeDaily:
		return nextDaily(n, e.AtMinutes, e.Days).Unix()
	case ModeWindow:
		return nextWindowSlot(n, e.AtMinutes, e.EndMinutes, e.safeIntervalSec(), e.Days).Unix()
	}
	return now.Add(e.safeInterval()).Unix()
}

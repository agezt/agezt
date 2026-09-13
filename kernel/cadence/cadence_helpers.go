// SPDX-License-Identifier: MIT

// Cadence Entry: applyZone + nextWindowSlot + dayAllowed + nextDaily (timing helpers).
// Extracted from cadence_entry.go during the Day-211 god-file split.
// Public API unchanged.
package cadence


import (
	"strings"
	"time"
)
// applyZone returns now converted into the IANA zone tz, or now unchanged when
// tz is empty (use the caller's zone). It errors on an unloadable zone name.
func applyZone(now time.Time, tz string) (time.Time, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return now, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return now, err
	}
	return now.In(loc), nil
}

// nextWindowSlot returns the next firing instant strictly after now for a
// windowed-interval schedule: slots are start, start+interval, … up to and
// including end, on permitted weekdays. After the window closes for a day it
// jumps to the next permitted day's start. Walks by calendar date (DST-correct).
func nextWindowSlot(now time.Time, start, end int, intervalSec int64, days int) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	iv := time.Duration(intervalSec) * time.Second
	for i := 0; i < 8; i++ {
		day := time.Date(y, m, d+i, 0, 0, 0, 0, loc)
		if !dayAllowed(day.Weekday(), days) {
			continue
		}
		startT := time.Date(y, m, d+i, start/60, start%60, 0, 0, loc)
		endT := time.Date(y, m, d+i, end/60, end%60, 0, 0, loc)
		if now.Before(startT) {
			return startT
		}
		if !now.Before(endT) {
			continue // today's window has closed
		}
		// now is inside [startT, endT): next aligned slot strictly after now.
		k := now.Sub(startT)/iv + 1
		slot := startT.Add(k * iv)
		if !slot.After(endT) {
			return slot
		}
		// no slot left today before end → fall through to the next permitted day
	}
	return now.Add(iv) // unreachable for a valid window
}

// dayAllowed reports whether wd is permitted by the day-mask. A zero mask (or
// AllDays) permits every day.
func dayAllowed(wd time.Weekday, days int) bool {
	if days == 0 || days == AllDays {
		return true
	}
	return days&(1<<uint(wd)) != 0
}

// nextDaily returns the next local-time occurrence of atMinutes-past-midnight,
// strictly after now, that falls on a weekday permitted by days. It walks
// forward by calendar date (not by adding 24h) so it stays correct across DST
// transitions.
func nextDaily(now time.Time, atMinutes, days int) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	nowMin := now.Hour()*60 + now.Minute()
	for i := 0; i < 8; i++ {
		cand := time.Date(y, m, d+i, atMinutes/60, atMinutes%60, 0, 0, loc)
		if !cand.After(now) {
			continue
		}
		// DST fall-back guard (M197): on a fall-back day the wall-clock atMinutes
		// occurs twice (e.g. 01:30 happens at both the DST and standard offset). The
		// second occurrence is After(now) yet shares the just-fired now's wall clock,
		// so without this guard the daily schedule fires AGAIN ~1h later. For today
		// (i==0) require the slot to be strictly later in the day than now; the fold
		// re-entry (same minutes-since-midnight) is rejected and we move to the next
		// permitted day. In normal time this rejects nothing real — a same/earlier
		// today slot already fails cand.After(now).
		if i == 0 && atMinutes <= nowMin {
			continue
		}
		if dayAllowed(cand.Weekday(), days) {
			return cand
		}
	}
	return time.Date(y, m, d+1, atMinutes/60, atMinutes%60, 0, 0, loc) // unreachable for any non-empty mask
}

// SPDX-License-Identifier: MIT

package cadence

// M197: a daily schedule whose time falls in a fall-back DST repeated hour must
// fire ONCE that day, not twice. America/New_York falls back 2026-11-01 at
// 02:00 EDT → 01:00 EST, so 01:30 occurs at both 05:30 UTC (EDT) and 06:30 UTC
// (EST). A daily-at-01:30 that fired at the first occurrence must not re-fire ~1h
// later at the second.

import (
	"testing"
	"time"
)

func TestNextDaily_NoFallBackDoubleFire(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	// First 01:30 on the fall-back day: 05:30 UTC = 01:30 EDT.
	firstFire := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC).In(ny)
	if h, m, _ := firstFire.Clock(); h != 1 || m != 30 {
		t.Fatalf("setup: firstFire wall clock = %02d:%02d, want 01:30", h, m)
	}

	next := nextDaily(firstFire, 90, AllDays) // 90 = 01:30

	// A daily schedule must be ~24h apart, never ~1h (the fold re-entry).
	gap := next.Sub(firstFire)
	if gap < 23*time.Hour {
		t.Errorf("daily re-fired only %s after the previous fire (DST fall-back double-fire); want ~24h", gap)
	}
	// And it should land on the NEXT calendar day, still at 01:30 wall clock.
	if y, mo, d := next.Date(); !(y == 2026 && mo == time.November && d == 2) {
		t.Errorf("next fire date = %04d-%02d-%02d, want 2026-11-02", y, mo, d)
	}
	if h, m, _ := next.Clock(); h != 1 || m != 30 {
		t.Errorf("next fire wall clock = %02d:%02d, want 01:30", h, m)
	}
}

// Normal (non-DST) daily: firing exactly at the slot advances ~24h, not 0.
func TestNextDaily_NormalDayAdvances24h(t *testing.T) {
	utc := time.UTC
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, utc) // 09:00, at the slot
	next := nextDaily(now, 9*60, AllDays)
	if gap := next.Sub(now); gap < 23*time.Hour || gap > 25*time.Hour {
		t.Errorf("normal daily gap = %s, want ~24h", gap)
	}
}

// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"testing"
	"time"
)

func TestMatchesCron(t *testing.T) {
	// Mon 2026-06-08 08:00 (June 8 2026 is a Monday).
	mon0800 := time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		spec string
		t    time.Time
		want bool
	}{
		{"0 8 * * *", mon0800, true},                                      // daily 08:00
		{"0 8 * * *", time.Date(2026, 6, 8, 8, 1, 0, 0, time.UTC), false}, // 08:01
		{"0 8 * * *", time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC), false}, // 09:00
		{"*/15 * * * *", mon0800, true},                                   // minute 0 ∈ */15
		{"*/15 * * * *", time.Date(2026, 6, 8, 8, 15, 0, 0, time.UTC), true},
		{"*/15 * * * *", time.Date(2026, 6, 8, 8, 7, 0, 0, time.UTC), false},
		{"0 8 * * 1-5", mon0800, true},                                   // weekday
		{"0 8 * * 6,0", mon0800, false},                                  // weekend only
		{"0 8 * * 0", time.Date(2026, 6, 7, 8, 0, 0, 0, time.UTC), true}, // Sunday=0
		{"0 8 * * 7", time.Date(2026, 6, 7, 8, 0, 0, 0, time.UTC), true}, // Sunday=7
		{"0 8 8 6 *", mon0800, true},                                     // June 8
		{"0 8 9 6 *", mon0800, false},                                    // June 9
		{"bad", mon0800, false},                                          // malformed
		{"0 8 * *", mon0800, false},                                      // 4 fields
		{"99 8 * * *", mon0800, false},                                   // out of range
	}
	for _, c := range cases {
		if got := matchesCron(c.spec, c.t); got != c.want {
			t.Errorf("matchesCron(%q, %s) = %v, want %v", c.spec, c.t.Format("Mon 15:04"), got, c.want)
		}
	}
}

// TestTickCron_FiresOncePerMinute: a matching cron order fires once at a matching
// minute, not again that same minute; a non-matching minute does not fire.
func TestTickCron_FiresOncePerMinute(t *testing.T) {
	s, _ := Open(t.TempDir())
	o, _ := s.Add(Order{
		Name:     "morning brief",
		Triggers: []Trigger{{Type: TriggerCron, Schedule: "0 8 * * *"}},
		Plan:     "brief me",
	})
	lastFired := map[string]int64{}
	at := time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)

	fired := tickCron(context.Background(), s, at, lastFired, func(context.Context, Order, string) {})
	if len(fired) != 1 || fired[0] != o.ID {
		t.Fatalf("first tick at 08:00 should fire the order, got %v", fired)
	}
	// Same minute again → no re-fire.
	if again := tickCron(context.Background(), s, at.Add(20*time.Second), lastFired, func(context.Context, Order, string) {}); len(again) != 0 {
		t.Errorf("second tick in the same minute should not re-fire, got %v", again)
	}
	// A non-matching minute → nothing.
	if none := tickCron(context.Background(), s, at.Add(time.Hour), lastFired, func(context.Context, Order, string) {}); len(none) != 0 {
		t.Errorf("09:00 should not fire a 08:00 order, got %v", none)
	}
}

// TestTickCron_SkipsDisabled: a paused cron order never fires.
func TestTickCron_SkipsDisabled(t *testing.T) {
	s, _ := Open(t.TempDir())
	o, _ := s.Add(Order{Name: "x", Triggers: []Trigger{{Type: TriggerCron, Schedule: "0 8 * * *"}}})
	_, _ = s.SetEnabled(o.ID, false)
	at := time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)
	if fired := tickCron(context.Background(), s, at, map[string]int64{}, func(context.Context, Order, string) {}); len(fired) != 0 {
		t.Errorf("a disabled cron order must not fire, got %v", fired)
	}
}

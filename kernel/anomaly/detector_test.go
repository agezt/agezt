// SPDX-License-Identifier: MIT

package anomaly

import (
	"testing"
	"time"
)

func TestDetector_DisabledNeverTrips(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	for _, d := range []*Detector{NewDetector(0, time.Second), NewDetector(5, 0), NewDetector(-1, -1)} {
		if d.Enabled() {
			t.Fatalf("detector %+v should be disabled", d)
		}
		for i := 0; i < 100; i++ {
			if trip, _ := d.Observe(base.Add(time.Duration(i) * time.Millisecond)); trip {
				t.Fatal("disabled detector tripped")
			}
		}
	}
}

// TestDetector_TripsOnceCeilingExceeded: with Max=5, the 6th event inside the
// window trips (count 6 > 5); the 5th (count 5) does not.
func TestDetector_TripsWhenCeilingExceeded(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	d := NewDetector(5, 10*time.Second)
	for i := 1; i <= 5; i++ {
		trip, count := d.Observe(base.Add(time.Duration(i) * time.Millisecond))
		if trip {
			t.Fatalf("tripped early at event %d (count=%d, ceiling 5)", i, count)
		}
	}
	trip, count := d.Observe(base.Add(6 * time.Millisecond))
	if !trip {
		t.Fatalf("6th event should trip (count=%d > 5)", count)
	}
	if count != 6 {
		t.Errorf("count=%d want 6", count)
	}
}

// TestDetector_WindowSlidePrunesOldEvents: events spread wider than the window
// never accumulate past the ceiling, so a slow steady stream never trips even
// over many events — only a BURST within the window does.
func TestDetector_WindowSlidePrunesOldEvents(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	d := NewDetector(3, 1*time.Second)
	// One event every 500ms for 20 events: at most 3 fall inside any 1s window
	// (t, t-500ms, and the boundary), so it must never trip.
	for i := 0; i < 20; i++ {
		trip, count := d.Observe(base.Add(time.Duration(i) * 500 * time.Millisecond))
		if trip {
			t.Fatalf("steady 2/s stream tripped at event %d (count=%d) — window pruning broken", i, count)
		}
	}
	// Now a burst: 4 events within the same 1s window must trip.
	burst := base.Add(60 * time.Second)
	var tripped bool
	for i := 0; i < 4; i++ {
		tripped, _ = d.Observe(burst.Add(time.Duration(i) * 100 * time.Millisecond))
	}
	if !tripped {
		t.Fatal("a 4-in-1s burst (ceiling 3) should trip after pruning the old stream")
	}
}

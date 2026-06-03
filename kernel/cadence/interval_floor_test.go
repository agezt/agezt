// SPDX-License-Identifier: MIT

package cadence

// M196: a sub-minimum IntervalSec (0 or negative) must never make the next run
// land on `now`/the past and busy-loop the ticker into firing a run every tick.
// Add rejects such values, but a hand-edited or corrupt schedules.json could
// carry one — so advance floors and OpenStore repairs.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAdvance_FloorsSubMinimumInterval(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	for _, sec := range []int64{0, -100, -1} {
		e := Entry{Mode: ModeInterval, IntervalSec: sec}
		next := e.advance(now)
		if next <= now.Unix() {
			t.Errorf("interval_sec=%d: advance=%d <= now=%d (would busy-loop)", sec, next, now.Unix())
		}
		if next < now.Add(MinInterval).Unix() {
			t.Errorf("interval_sec=%d: advance=%d before now+MinInterval", sec, next)
		}
	}
	// Window mode also consumes IntervalSec.
	e := Entry{Mode: ModeWindow, IntervalSec: 0, AtMinutes: 0, EndMinutes: 1440}
	if next := e.advance(now); next <= now.Unix() {
		t.Errorf("window interval_sec=0: advance=%d <= now (would busy-loop)", next)
	}
}

func TestOpenStore_RepairsSubMinimumInterval(t *testing.T) {
	dir := t.TempDir()
	bad := `[{"id":"x","intent":"hammer","mode":"","interval_sec":0,"source":"operator","enabled":true,"created_unix":1,"next_run_unix":1}]`
	if err := os.WriteFile(filepath.Join(dir, "schedules.json"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("entries=%d want 1", len(list))
	}
	if list[0].Interval() < MinInterval {
		t.Errorf("interval not repaired on load: %s", list[0].Interval())
	}
}

// The end-to-end property: a loaded zero-interval entry fires once and then is
// NOT due again on the same instant — i.e. no busy-loop.
func TestDue_SubMinimumIntervalDoesNotBusyLoop(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	bad := fmt.Sprintf(
		`[{"id":"x","intent":"hammer","mode":"","interval_sec":0,"source":"operator","enabled":true,"created_unix":1,"next_run_unix":%d}]`,
		now.Add(-time.Second).Unix())
	if err := os.WriteFile(filepath.Join(dir, "schedules.json"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if due := s.Due(now); len(due) != 1 {
		t.Fatalf("first Due len=%d want 1 (entry is due)", len(due))
	}
	if due := s.Due(now); len(due) != 0 {
		t.Errorf("second Due len=%d want 0 — busy-loop: re-fires every tick", len(due))
	}
}

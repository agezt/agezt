// SPDX-License-Identifier: MIT

package cadence

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recorder counts and records the intents a RunFunc was asked to run.
type recorder struct {
	mu      sync.Mutex
	intents []string
	block   chan struct{} // if non-nil, Run blocks on it (to simulate a slow run)
}

func (r *recorder) run(_ context.Context, intent, _ string) error {
	if r.block != nil {
		<-r.block
	}
	r.mu.Lock()
	r.intents = append(r.intents, intent)
	r.mu.Unlock()
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.intents)
}

// waitCount waits until the recorder has seen n intents (goroutine deliveries).
func waitCount(t *testing.T, r *recorder, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.count() >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("expected %d runs, got %d", n, r.count())
}

func TestFireDue_FiresWhenIntervalElapses(t *testing.T) {
	rec := &recorder{}
	e := New([]Job{{Interval: time.Hour, Intent: "daily brief"}}, rec.run, 0, nil)

	base := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	e.fireDue(context.Background(), base)                     // arms: next = base+1h
	e.fireDue(context.Background(), base.Add(30*time.Minute)) // not due
	if rec.count() != 0 {
		t.Fatalf("should not fire before interval, got %d", rec.count())
	}
	e.fireDue(context.Background(), base.Add(time.Hour+time.Second)) // due
	waitCount(t, rec, 1)
	if rec.intents[0] != "daily brief" {
		t.Errorf("intent = %q", rec.intents[0])
	}
}

func TestFireDue_RepeatsEachInterval(t *testing.T) {
	rec := &recorder{}
	e := New([]Job{{Interval: time.Hour, Intent: "x"}}, rec.run, 0, nil)
	base := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	e.fireDue(context.Background(), base) // arm
	e.fireDue(context.Background(), base.Add(1*time.Hour+time.Second))
	waitCount(t, rec, 1)
	e.fireDue(context.Background(), base.Add(2*time.Hour+time.Second))
	waitCount(t, rec, 2)
	e.fireDue(context.Background(), base.Add(3*time.Hour+time.Second))
	waitCount(t, rec, 3)
}

func TestFireDue_SkipsOverlappingRun(t *testing.T) {
	rec := &recorder{block: make(chan struct{})}
	e := New([]Job{{Interval: time.Hour, Intent: "slow"}}, rec.run, 0, nil)
	base := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	e.fireDue(context.Background(), base) // arm

	// First fire starts a run that blocks (still running).
	e.fireDue(context.Background(), base.Add(time.Hour+time.Second))
	// Give the goroutine a moment to mark itself running.
	waitRunning(t, e, 0)
	// Second fire while the first is still running → must be skipped.
	e.fireDue(context.Background(), base.Add(2*time.Hour+time.Second))

	close(rec.block) // let the (single) run finish
	waitCount(t, rec, 1)
	time.Sleep(20 * time.Millisecond)
	if c := rec.count(); c != 1 {
		t.Errorf("overlapping fire should be skipped: got %d runs", c)
	}
}

func waitRunning(t *testing.T, e *Engine, idx int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if e.jobs[idx].running.Load() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not enter running state")
}

func TestStart_FiresLiveOnShortInterval(t *testing.T) {
	rec := &recorder{}
	// 1s interval, fine resolution → fires within a couple seconds.
	e := New([]Job{{Interval: time.Second, Intent: "tick"}}, rec.run, 200*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	waitCount(t, rec, 1)
}

func TestStart_NoJobsIsNoop(t *testing.T) {
	var ran atomic.Bool
	e := New(nil, func(context.Context, string, string) error { ran.Store(true); return nil }, 0, nil)
	e.Start(context.Background())
	time.Sleep(20 * time.Millisecond)
	if ran.Load() {
		t.Error("no jobs should mean nothing fires")
	}
}

func TestParseJobs(t *testing.T) {
	jobs, err := ParseJobs("1h=summarise new commits; 24h=daily security audit, with commas")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	if jobs[0].Interval != time.Hour || jobs[0].Intent != "summarise new commits" {
		t.Errorf("job0 = %+v", jobs[0])
	}
	// Commas inside the intent survive (semicolon is the separator).
	if jobs[1].Intent != "daily security audit, with commas" {
		t.Errorf("job1 intent = %q", jobs[1].Intent)
	}

	for _, bad := range []string{"noequals", "notaduration=do x", "500ms=too fast", "1h=  "} {
		if _, err := ParseJobs(bad); err == nil {
			t.Errorf("ParseJobs(%q) should error", bad)
		}
	}
	if j, err := ParseJobs("  "); err != nil || j != nil {
		t.Errorf("empty spec = %v, %v", j, err)
	}
}

func TestNew_ClampsTinyInterval(t *testing.T) {
	e := New([]Job{{Interval: time.Millisecond, Intent: "x"}}, func(context.Context, string, string) error { return nil }, 0, nil)
	if e.jobs[0].Interval < MinInterval {
		t.Errorf("interval not clamped: %s", e.jobs[0].Interval)
	}
}

func TestDescribe(t *testing.T) {
	out := Describe([]Job{{Interval: time.Hour, Intent: "brief me"}})
	if out == "" || !strings.Contains(out, "every 1h") || !strings.Contains(out, "brief me") {
		t.Errorf("describe = %q", out)
	}
}

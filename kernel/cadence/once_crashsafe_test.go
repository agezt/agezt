// SPDX-License-Identifier: MIT

package cadence

import (
	"context"
	"testing"
	"time"
)

// waitStoreCount polls until the store holds exactly n entries (or fails).
func waitStoreCount(t *testing.T, s *Store, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.Count() == n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("expected store count %d, got %d", n, s.Count())
}

// TestStore_CompleteFiring_RecurringIsNoOp proves CompleteFiring only removes
// one-shots: a recurring entry (already advanced by Due) is left untouched, and an
// unknown id is a harmless no-op.
func TestStore_CompleteFiring_RecurringIsNoOp(t *testing.T) {
	s := mustStore(t)
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	e, _ := s.Add("hourly", time.Hour, "", SourceOperator, now)

	ok, err := s.CompleteFiring(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("CompleteFiring on a recurring entry must be a no-op")
	}
	if s.Count() != 1 {
		t.Errorf("recurring entry must remain, count = %d", s.Count())
	}
	if ok, _ := s.CompleteFiring("does-not-exist"); ok {
		t.Error("CompleteFiring on an unknown id must return false")
	}
}

// TestEngine_Once_RemovedAfterRun proves the happy path: a one-shot fires, its run
// completes, and it is then removed — and never fires again.
func TestEngine_Once_RemovedAfterRun(t *testing.T) {
	s := mustStore(t)
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	at := now.Add(30 * time.Minute)
	s.AddOnce("summarise the deploy", at, "", SourceOperator, now)

	rec := &recorder{}
	eng := NewEngine(s, rec.run, 0, nil)

	eng.fireDue(context.Background(), at.Add(time.Second))
	waitCount(t, rec, 1)    // the run happened
	waitStoreCount(t, s, 0) // and the one-shot was removed only after it completed

	// A later tick finds nothing to fire.
	eng.fireDue(context.Background(), at.Add(time.Hour))
	time.Sleep(20 * time.Millisecond)
	if rec.count() != 1 {
		t.Errorf("one-shot must fire exactly once, got %d", rec.count())
	}
}

// TestEngine_Once_SurvivesCrashWindow is the core M199 guarantee: while a one-shot's
// run is in flight, the entry remains in the store — so a crash in that window
// re-fires it on restart instead of silently dropping it — and a concurrent tick
// does not double-fire it.
func TestEngine_Once_SurvivesCrashWindow(t *testing.T) {
	s := mustStore(t)
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	at := now.Add(30 * time.Minute)
	s.AddOnce("summarise the deploy", at, "", SourceOperator, now)

	rec := &recorder{block: make(chan struct{})}
	eng := NewEngine(s, rec.run, 0, nil)

	eng.fireDue(context.Background(), at.Add(time.Second)) // launches; run blocks
	waitRunningCount(t, eng, 1)

	// Mid-run "crash window": the one-shot is still persisted, so a crash here would
	// re-fire it on restart rather than dropping it.
	if s.Count() != 1 {
		t.Fatalf("one-shot must remain in store while its run is in flight, count=%d", s.Count())
	}
	// A second tick while the run is still going must NOT start another run.
	eng.fireDue(context.Background(), at.Add(2*time.Second))
	if rec.count() != 0 {
		t.Fatalf("run should still be blocked, got %d completed", rec.count())
	}

	close(rec.block) // the run completes
	waitCount(t, rec, 1)
	waitStoreCount(t, s, 0) // now, and only now, the one-shot is removed
	if rec.count() != 1 {
		t.Errorf("one-shot fired %d times, want exactly 1", rec.count())
	}
}

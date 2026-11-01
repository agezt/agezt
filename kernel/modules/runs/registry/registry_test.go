// SPDX-License-Identifier: MIT

package registry

import (
	"context"
	"testing"
	"time"
)

// The registry is the runs module's live-run store (W3.76d). These tests
// pin the rules the daemon and the lifecycle manager depend on.

func TestRegistry_RegisterAndUnregister(t *testing.T) {
	r := New()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Lock()
	r.Register("corr-1", cancel)
	r.Unlock()

	if !r.Running("corr-1") {
		t.Fatal("a registered correlation must be running")
	}
	if r.Running("corr-2") {
		t.Fatal("an unknown correlation must not be running")
	}
	// The registry stores the cancel it was given: a run that cannot be
	// cancelled is a run the halt sweep cannot reach.
	if got := r.Cancels(); len(got) != 1 || got[0] == nil {
		t.Fatalf("Cancels() = %+v, want the registered cancel", got)
	}

	r.Lock()
	r.Unregister("corr-1")
	r.Unlock()

	if r.Running("corr-1") {
		t.Fatal("an unregistered correlation must not be running")
	}
	if got := r.Count(); got != 0 {
		t.Fatalf("Count() = %d, want 0", got)
	}
}

// TestRegistry_UnregisterDoesNotCancel pins the ownership rule: the registry
// forgets a correlation, it does NOT cancel it. The caller owns when a run
// is cancelled (a finished run is unregistered, not aborted).
func TestRegistry_UnregisterDoesNotCancel(t *testing.T) {
	r := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Lock()
	r.Register("corr-1", cancel)
	r.Unregister("corr-1")
	r.Unlock()

	if ctx.Err() != nil {
		t.Fatal("Unregister must not cancel the run")
	}
}

// TestRegistry_CancelRemovesAndHandsBack pins the cancel path: the cancel
// function comes back to the caller, which invokes it.
func TestRegistry_CancelRemovesAndHandsBack(t *testing.T) {
	r := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Lock()
	r.Register("corr-1", cancel)
	r.Unlock()

	r.Lock()
	got, ok := r.Cancel("corr-1")
	r.Unlock()

	if !ok {
		t.Fatal("Cancel must report a live correlation")
	}
	if got == nil {
		t.Fatal("Cancel must hand back the cancel function")
	}
	got()
	if ctx.Err() == nil {
		t.Fatal("the handed-back cancel did not cancel the run")
	}
	if r.Running("corr-1") {
		t.Fatal("a cancelled correlation must be forgotten")
	}

	r.Lock()
	_, ok = r.Cancel("corr-1")
	r.Unlock()
	if ok {
		t.Fatal("cancelling an unknown correlation must report false")
	}
}

// TestRegistry_HaltAllTakesEveryRunAndLatches pins the halt sweep: every live
// run's cancel comes back, the registry is emptied, and the halt flag stays
// latched so no new run starts.
func TestRegistry_HaltAllTakesEveryRunAndLatches(t *testing.T) {
	r := New()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Lock()
	r.Register("corr-1", cancel)
	r.Unlock()

	r.Lock()
	cancels := r.HaltAll()
	r.Unlock()

	if len(cancels) != 1 {
		t.Fatalf("HaltAll returned %d cancels, want 1", len(cancels))
	}
	if !r.Halted() {
		t.Fatal("HaltAll must latch the halt flag")
	}
	if got := r.Count(); got != 0 {
		t.Fatalf("Count() after the sweep = %d, want 0", got)
	}
}

// TestRegistry_IDsAndCancelsCoverTheLiveSet pins the read side the lifecycle
// manager uses: the ids and the cancels describe the same live set.
func TestRegistry_IDsAndCancelsCoverTheLiveSet(t *testing.T) {
	r := New()
	for _, id := range []string{"c", "a", "b"} {
		_, cancel := context.WithCancel(context.Background())
		r.Lock()
		r.Register(id, cancel)
		r.Unlock()
	}

	ids := r.IDs()
	if len(ids) != 3 {
		t.Fatalf("IDs() returned %d ids, want 3", len(ids))
	}
	if got := len(r.Cancels()); got != 3 {
		t.Fatalf("Cancels() returned %d cancels, want 3", got)
	}
	if got := r.Count(); got != 3 {
		t.Fatalf("Count() = %d, want 3", got)
	}
}

// TestRegistry_DrainWaitsForTheCountedRuns pins the drain accounting: Wait
// returns only after every counted run released itself.
func TestRegistry_DrainWaitsForTheCountedRuns(t *testing.T) {
	r := New()
	r.Add(1)

	settled := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		r.Done()
		close(settled)
	}()

	waited := make(chan struct{})
	go func() {
		r.Wait()
		close(waited)
	}()

	// The wait must NOT return while the run is still counted.
	select {
	case <-waited:
		t.Fatal("Wait returned while the run was still counted")
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after the run released itself")
	}
	<-settled
}

// TestRegistry_SuspendingIsTheRestartLatch pins the latch's semantics: it is
// set once and never cleared, because the process is on its way out.
func TestRegistry_SuspendingIsTheRestartLatch(t *testing.T) {
	r := New()
	if r.Suspending().Load() {
		t.Fatal("a fresh registry is not suspending")
	}
	if !r.Suspending().CompareAndSwap(false, true) {
		t.Fatal("the first CompareAndSwap must win")
	}
	if r.Suspending().CompareAndSwap(false, true) {
		t.Fatal("the latch must not be settable twice")
	}
	if !r.Suspending().Load() {
		t.Fatal("the latch must stay set")
	}
}

// TestRegistry_SetHaltedLatchesAndClears pins the halt latch's setter: the
// flag is latched by SetHalted(true) and cleared by SetHalted(false). The
// daemon's halt path depends on the set and its resume path depends on the
// clear — a setter that only ever clears leaves the daemon refusing new
// runs after a resume, or accepting them during a halt.
func TestRegistry_SetHaltedLatchesAndClears(t *testing.T) {
	r := New()
	if r.Halted() {
		t.Fatal("a fresh registry is not halted")
	}

	r.Lock()
	r.SetHalted(true)
	r.Unlock()
	if !r.Halted() {
		t.Fatal("SetHalted(true) must latch the halt flag")
	}

	r.Lock()
	r.SetHalted(false)
	r.Unlock()
	if r.Halted() {
		t.Fatal("SetHalted(false) must clear the halt flag")
	}
}

// SPDX-License-Identifier: MIT

package standing_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/standing"
)

func newBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return b
}

// fireRec records orders fired by the runner.
type fireRec struct {
	mu    sync.Mutex
	fired []string
}

func (r *fireRec) fn(_ context.Context, o standing.Order, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fired = append(r.fired, o.ID)
}
func (r *fireRec) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.fired)
}

func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// TestRunner_FiresOnMatchingEvent: an enabled event-triggered order fires when a
// matching event is published; a non-matching event does not.
func TestRunner_FiresOnMatchingEvent(t *testing.T) {
	b := newBus(t)
	s, _ := standing.Open(t.TempDir())
	o, _ := s.Add(standing.Order{
		Name:     "ci watch",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "github.>"}},
	})
	rec := &fireRec{}
	if !standing.StartRunner(context.Background(), b, s, standing.RunnerConfig{}, rec.fn) {
		t.Fatal("StartRunner returned false")
	}

	// A matching event fires the order.
	_, _ = b.Publish(event.Spec{Subject: "github.push", Kind: event.KindTaskReceived, Actor: "x"})
	if !waitFor(t, func() bool { return rec.count() == 1 }) {
		t.Fatalf("order did not fire on matching event (fired=%d)", rec.count())
	}
	_ = o

	// A non-matching event does not.
	_, _ = b.Publish(event.Spec{Subject: "gitlab.push", Kind: event.KindTaskReceived, Actor: "x"})
	time.Sleep(50 * time.Millisecond)
	if rec.count() != 1 {
		t.Errorf("non-matching event should not fire (fired=%d)", rec.count())
	}
}

// TestRunner_SkipsDisabledAndLifecycle: a paused order never fires, and a
// standing.* lifecycle event never triggers anything (no self-trigger loop).
func TestRunner_SkipsDisabledAndLifecycle(t *testing.T) {
	b := newBus(t)
	s, _ := standing.Open(t.TempDir())
	o, _ := s.Add(standing.Order{
		Name:     "paused watch",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "github.>"}},
	})
	_, _ = s.SetEnabled(o.ID, false)

	rec := &fireRec{}
	standing.StartRunner(context.Background(), b, s, standing.RunnerConfig{}, rec.fn)

	_, _ = b.Publish(event.Spec{Subject: "github.push", Kind: event.KindTaskReceived, Actor: "x"})
	_, _ = b.Publish(event.Spec{Subject: "standing.anything", Kind: event.KindStandingCreated, Actor: "x"})
	time.Sleep(50 * time.Millisecond)
	if rec.count() != 0 {
		t.Errorf("disabled order / lifecycle event must not fire (fired=%d)", rec.count())
	}
}

// TestRunner_Cooldown: a burst of matching events fires the order at most once
// within the cooldown window.
func TestRunner_Cooldown(t *testing.T) {
	b := newBus(t)
	s, _ := standing.Open(t.TempDir())
	_, _ = s.Add(standing.Order{
		Name:     "busy watch",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "github.>"}},
	})
	rec := &fireRec{}
	standing.StartRunner(context.Background(), b, s, standing.RunnerConfig{Cooldown: time.Hour}, rec.fn)

	for i := 0; i < 5; i++ {
		_, _ = b.Publish(event.Spec{Subject: "github.push", Kind: event.KindTaskReceived, Actor: "x"})
	}
	if !waitFor(t, func() bool { return rec.count() >= 1 }) {
		t.Fatal("expected at least one fire")
	}
	time.Sleep(50 * time.Millisecond)
	if rec.count() != 1 {
		t.Errorf("cooldown should cap the burst to one fire, got %d", rec.count())
	}
}

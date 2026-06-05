// SPDX-License-Identifier: MIT

package standing_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
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

// TestScopedIntent: an order with scope entities prefixes the intent with a scope
// note; without scope the intent is unchanged.
func TestScopedIntent(t *testing.T) {
	scoped := standing.Order{Name: "watch", ScopeEntities: []string{"project:portfolio", "repo:agezt"}}
	got := standing.ScopedIntent(scoped, "diagnose CI")
	if !strings.Contains(got, "project:portfolio, repo:agezt") || !strings.HasSuffix(got, "diagnose CI") {
		t.Errorf("scoped intent should name the entities and keep the plan, got %q", got)
	}
	if standing.ScopedIntent(standing.Order{Name: "x"}, "do it") != "do it" {
		t.Error("no scope entities should leave the intent unchanged")
	}
}

// TestBriefText: a briefing is produced only when the order names a channel AND
// the run produced a non-empty answer; the text is prefixed with the order name.
func TestBriefText(t *testing.T) {
	withChan := standing.Order{Name: "morning brief", BriefingChan: "webhook"}
	if text, ok := standing.BriefText(withChan, "all green"); !ok || text != "[standing: morning brief]\nall green" {
		t.Errorf("BriefText = %q, %v; want the prefixed text + true", text, ok)
	}
	// No channel → no briefing.
	if _, ok := standing.BriefText(standing.Order{Name: "x"}, "answer"); ok {
		t.Error("no channel should yield no briefing")
	}
	// Empty answer → no briefing (nothing to report).
	if _, ok := standing.BriefText(withChan, "   "); ok {
		t.Error("empty answer should yield no briefing")
	}
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

// TestRunner_CooldownUsesInjectedClock: the cooldown keys off the runner's local
// clock (not the event timestamp), so advancing the clock past the window lets the
// order fire again, and not advancing holds it — M412 (BUG 4 fix).
func TestRunner_CooldownUsesInjectedClock(t *testing.T) {
	b := newBus(t)
	s, _ := standing.Open(t.TempDir())
	_, _ = s.Add(standing.Order{
		Name:     "clocked",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "github.>"}},
	})
	var clockMS atomic.Int64
	clockMS.Store(1_000_000)
	rec := &fireRec{}
	standing.StartRunner(context.Background(), b, s, standing.RunnerConfig{
		Cooldown: time.Minute,
		Now:      func() time.Time { return time.UnixMilli(clockMS.Load()) },
	}, rec.fn)

	_, _ = b.Publish(event.Spec{Subject: "github.push", Kind: event.KindTaskReceived, Actor: "x"})
	if !waitFor(t, func() bool { return rec.count() == 1 }) {
		t.Fatalf("first event should fire, got %d", rec.count())
	}
	// Clock unchanged → within cooldown → no second fire.
	_, _ = b.Publish(event.Spec{Subject: "github.push", Kind: event.KindTaskReceived, Actor: "x"})
	time.Sleep(40 * time.Millisecond)
	if rec.count() != 1 {
		t.Fatalf("within cooldown (clock unchanged) should not re-fire, got %d", rec.count())
	}
	// Advance the local clock past the cooldown → next event fires again.
	clockMS.Add(2 * 60 * 1000)
	_, _ = b.Publish(event.Spec{Subject: "github.push", Kind: event.KindTaskReceived, Actor: "x"})
	if !waitFor(t, func() bool { return rec.count() == 2 }) {
		t.Errorf("after advancing the clock past cooldown the order should fire again, got %d", rec.count())
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

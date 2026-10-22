// SPDX-License-Identifier: MIT

package workflow

// The trigger's source must reach the host: a cron-triggered run is
// journalled as source:"cron" and an event-triggered one as source:"event",
// never as "manual" (which is what a run without a trigger is). The runner
// is the only place that knows which trigger fired, so it is the only place
// that can say so.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// TestTriggerRunner_CronFireCarriesItsSource: an interval trigger fires with
// source "cron" — the host turns that into the run's provenance.
func TestTriggerRunner_CronFireCarriesItsSource(t *testing.T) {
	b := openBus(t)
	store, _ := OpenStore(t.TempDir())
	if _, _, err := store.Save(Workflow{
		Name:  "every-min",
		Nodes: []Node{{ID: "start", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","interval_sec":60}`)}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rec := &fireRecorder{}
	var mu sync.Mutex
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.Local)
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := StartTriggers(ctx, b, store, RunnerConfig{Tick: 20 * time.Millisecond, Now: clock}, rec.fn); err != nil {
		t.Fatalf("StartTriggers: %v", err)
	}
	// The interval is anchored at arm time: the first fire is one full
	// interval later.
	time.Sleep(80 * time.Millisecond)
	if rec.count() != 0 {
		t.Fatalf("premature fire: %d", rec.count())
	}
	mu.Lock()
	now = now.Add(61 * time.Second)
	mu.Unlock()
	rec.waitFor(t, 1)

	rec.mu.Lock()
	got := rec.fires[0].trigger
	rec.mu.Unlock()
	if got.Source != "cron" {
		t.Fatalf("cron fire source = %q, want cron", got.Source)
	}
	if got.Reason == "" {
		t.Fatal("cron fire carries no reason label")
	}
}

// TestTriggerRunner_EventFireCarriesItsSourceAndSubject: an event trigger
// fires with source "event" and the matched subject, so the run's
// provenance names both the kind and the fuel.
func TestTriggerRunner_EventFireCarriesItsSourceAndSubject(t *testing.T) {
	b := openBus(t)
	store, _ := OpenStore(t.TempDir())
	if _, _, err := store.Save(eventTriggeredFlow("on-fail", "task.failed")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec := &fireRecorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := StartTriggers(ctx, b, store, RunnerConfig{EventCooldown: time.Millisecond, Tick: time.Hour}, rec.fn); err != nil {
		t.Fatalf("StartTriggers: %v", err)
	}
	if _, err := b.Publish(event.Spec{Subject: "task.failed", Kind: "task.failed", Actor: "test"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	rec.waitFor(t, 1)

	rec.mu.Lock()
	got := rec.fires[0].trigger
	rec.mu.Unlock()
	if got.Source != "event" {
		t.Fatalf("event fire source = %q, want event", got.Source)
	}
	if got.Subject != "task.failed" {
		t.Fatalf("event fire subject = %q, want task.failed", got.Subject)
	}
}

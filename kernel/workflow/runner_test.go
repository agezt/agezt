// SPDX-License-Identifier: MIT

package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func TestSubjectMatch(t *testing.T) {
	cases := []struct {
		pattern, subject string
		want             bool
	}{
		{"task.failed", "task.failed", true},
		{"task.failed", "task.completed", false},
		{"task.*", "task.failed", true},
		{"task.*", "task.failed.extra", false},
		{"board.dm.*", "board.dm.researcher", true},
		{"board.>", "board.dm.researcher", true},
		{"board.>", "board", false},
		{"*.failed", "task.failed", true},
		{"task.failed", "task", false},
	}
	for _, tc := range cases {
		if got := SubjectMatch(tc.pattern, tc.subject); got != tc.want {
			t.Errorf("SubjectMatch(%q, %q) = %v, want %v", tc.pattern, tc.subject, got, tc.want)
		}
	}
}

// fireRecorder collects trigger firings.
type fireRecorder struct {
	mu    sync.Mutex
	fires []struct {
		name    string
		payload any
		reason  string
	}
}

func (r *fireRecorder) fn(_ context.Context, w Workflow, payload any, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fires = append(r.fires, struct {
		name    string
		payload any
		reason  string
	}{w.Name, payload, reason})
}

func (r *fireRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.fires)
}

func (r *fireRecorder) waitFor(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r.count() >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waited for %d fire(s), have %d", n, r.count())
}

func openBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(b.Close)
	return b
}

func eventTriggeredFlow(name, subject string) Workflow {
	return Workflow{
		Name: name,
		Nodes: []Node{
			{ID: "start", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"event","subject":"` + subject + `"}`)},
			{ID: "shape", Type: NodeTransform, Config: json.RawMessage(`{"template":"got {{trigger.payload.subject}}"}`)},
		},
		Edges: []Edge{{From: "start", To: "shape"}},
	}
}

// TestEventTrigger_FiresWithPayloadAndCooldown: a matching journal event
// fires the workflow with subject+data in the payload; a second event inside
// the cooldown is suppressed; non-matching subjects and disabled workflows
// never fire; workflow.* events are never trigger fuel.
func TestEventTrigger_FiresWithPayloadAndCooldown(t *testing.T) {
	b := openBus(t)
	store, _ := OpenStore(t.TempDir())
	if _, _, err := store.Save(eventTriggeredFlow("on-fail", "task.failed")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec := &fireRecorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := StartTriggers(ctx, b, store, RunnerConfig{EventCooldown: 200 * time.Millisecond, Tick: time.Hour}, rec.fn); err != nil {
		t.Fatalf("StartTriggers: %v", err)
	}

	pub := func(subject string) {
		if _, err := b.Publish(event.Spec{Subject: subject, Kind: "task.failed", Actor: "test",
			Payload: map[string]any{"reason": "boom"}}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	pub("task.failed")
	rec.waitFor(t, 1)
	rec.mu.Lock()
	first := rec.fires[0]
	rec.mu.Unlock()
	if first.name != "on-fail" || !strings.HasPrefix(first.reason, "event ") {
		t.Fatalf("fire = %+v", first)
	}
	p, _ := first.payload.(map[string]any)
	if p["subject"] != "task.failed" || p["kind"] != "event" {
		t.Fatalf("payload = %v", p)
	}
	if data, _ := p["data"].(map[string]any); data["reason"] != "boom" {
		t.Fatalf("event body missing: %v", p)
	}

	// Cooldown suppresses an immediate repeat...
	pub("task.failed")
	time.Sleep(100 * time.Millisecond)
	if rec.count() != 1 {
		t.Fatalf("cooldown leaked: %d fires", rec.count())
	}
	// ...and a later one fires again.
	time.Sleep(150 * time.Millisecond)
	pub("task.failed")
	rec.waitFor(t, 2)

	// Non-matching + workflow.* + disabled → no fire.
	pub("task.completed")
	if _, err := b.Publish(event.Spec{Subject: "workflow.on-fail", Kind: "workflow.completed", Actor: "test"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := store.SetEnabled("on-fail", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	pub("task.failed")
	time.Sleep(150 * time.Millisecond)
	if rec.count() != 2 {
		t.Fatalf("unexpected extra fires: %d", rec.count())
	}
}

// TestCronTrigger_IntervalAndDaily: an interval trigger fires once per
// elapsed interval (anchored at arm time); a daily trigger fires once per
// local day after HH:MM. Driven by an injected clock + fast tick.
func TestCronTrigger_IntervalAndDaily(t *testing.T) {
	b := openBus(t)
	store, _ := OpenStore(t.TempDir())
	if _, _, err := store.Save(Workflow{
		Name:  "every-min",
		Nodes: []Node{{ID: "start", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","interval_sec":60}`)}},
	}); err != nil {
		t.Fatalf("Save interval: %v", err)
	}
	if _, _, err := store.Save(Workflow{
		Name:  "daily-nine",
		Nodes: []Node{{ID: "start", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","daily_at":"09:00"}`)}},
	}); err != nil {
		t.Fatalf("Save daily: %v", err)
	}

	var mu sync.Mutex
	now := time.Date(2026, 6, 10, 8, 59, 0, 0, time.Local)
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	advance := func(d time.Duration) {
		mu.Lock()
		now = now.Add(d)
		mu.Unlock()
	}

	rec := &fireRecorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := StartTriggers(ctx, b, store, RunnerConfig{Tick: 20 * time.Millisecond, Now: clock}, rec.fn); err != nil {
		t.Fatalf("StartTriggers: %v", err)
	}

	// Before the interval elapses and before 09:00 — nothing fires.
	time.Sleep(80 * time.Millisecond)
	if rec.count() != 0 {
		t.Fatalf("premature fire: %d", rec.count())
	}

	// +61s: interval due. Clock still 08:59+61s < 09:01 — daily not yet.
	advance(61 * time.Second)
	rec.waitFor(t, 1)

	// +2min → 09:02: daily fires once; interval fires again (another 60s elapsed).
	advance(2 * time.Minute)
	rec.waitFor(t, 3)
	time.Sleep(80 * time.Millisecond)
	if rec.count() != 3 {
		t.Fatalf("daily fired more than once per day: %d", rec.count())
	}

	names := map[string]int{}
	rec.mu.Lock()
	for _, f := range rec.fires {
		names[f.name]++
	}
	rec.mu.Unlock()
	if names["every-min"] != 2 || names["daily-nine"] != 1 {
		t.Fatalf("fires by name = %v", names)
	}
}

// SPDX-License-Identifier: MIT

package anomaly

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func newBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return bus.New(j)
}

func publishToolCall(t *testing.T, b *bus.Bus) {
	t.Helper()
	if _, err := b.Publish(event.Spec{
		Subject: "agent.run-x.tool",
		Kind:    event.KindToolInvoked,
		Actor:   "agent",
		Payload: map[string]any{"name": "shell"},
	}); err != nil {
		t.Fatalf("publish tool.invoked: %v", err)
	}
}

// TestMonitor_TripsAndHaltsOnToolCallSpike: a burst of tool.invoked events past
// the ceiling must fire onTrip (the daemon wires this to halt) AND journal a
// system.anomaly event — the SPEC-06 §5 auto-halt end to end on a real bus.
func TestMonitor_TripsAndHaltsOnToolCallSpike(t *testing.T) {
	b := newBus(t)
	// Watch for the breaker's own event.
	anomSub, err := b.Subscribe("system.anomaly", 8)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer anomSub.Cancel()

	tripped := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !Start(ctx, b, Config{MaxToolCalls: 5, Window: 10 * time.Second}, func(reason string) {
		tripped <- reason
	}) {
		t.Fatal("Start returned false for an enabled config")
	}

	// Six tool calls > ceiling of five → trip.
	for i := 0; i < 6; i++ {
		publishToolCall(t, b)
	}

	select {
	case reason := <-tripped:
		if reason == "" {
			t.Error("onTrip fired with an empty reason")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onTrip never fired on a 6-call spike (ceiling 5)")
	}

	// The breaker must have journaled a system.anomaly event.
	select {
	case ev := <-anomSub.C:
		if ev.Kind != event.KindAnomalyDetected {
			t.Errorf("anomaly event kind=%s want %s", ev.Kind, event.KindAnomalyDetected)
		}
	case <-time.After(time.Second):
		t.Fatal("no system.anomaly event was published")
	}
}

// TestMonitor_DisabledConfigDoesNotStart: a non-positive ceiling means no
// watcher and no trip even under a heavy spike (operator opt-out).
func TestMonitor_DisabledConfigDoesNotStart(t *testing.T) {
	b := newBus(t)
	tripped := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if Start(ctx, b, Config{MaxToolCalls: 0, Window: 10 * time.Second}, func(string) { tripped <- "x" }) {
		t.Fatal("Start should return false when disabled (MaxToolCalls<=0)")
	}
	for i := 0; i < 50; i++ {
		publishToolCall(t, b)
	}
	select {
	case <-tripped:
		t.Fatal("disabled monitor tripped")
	case <-time.After(300 * time.Millisecond):
		// expected: nothing
	}
}

// TestMonitor_BelowCeilingDoesNotTrip: a number of tool calls at or under the
// ceiling must not trip — no false circuit-break on a normal heavy run.
func TestMonitor_BelowCeilingDoesNotTrip(t *testing.T) {
	b := newBus(t)
	tripped := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Start(ctx, b, Config{MaxToolCalls: 10, Window: 10 * time.Second}, func(string) { tripped <- "x" })
	for i := 0; i < 10; i++ { // exactly the ceiling, not above
		publishToolCall(t, b)
	}
	select {
	case <-tripped:
		t.Fatal("tripped at exactly the ceiling (10) — must require > ceiling")
	case <-time.After(300 * time.Millisecond):
		// expected: nothing
	}
}

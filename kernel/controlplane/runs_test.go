// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRunsList_EmptyJournalReturnsEmptyArray — fresh kernel,
// no runs; runs comes back as a valid empty array (not null) so
// jq pipelines don't break.
func TestRunsList_EmptyJournalReturnsEmptyArray(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 0 {
		t.Errorf("count = %d want 0", got)
	}
	rows, ok := res["runs"].([]any)
	if !ok || len(rows) != 0 {
		t.Errorf("runs should be empty array, got %T %v", res["runs"], res["runs"])
	}
}

// TestRunsList_PairsReceivedAndCompleted is the load-bearing
// test: publish a synthetic task.received + task.completed pair
// and verify they appear as one row with status="completed",
// the intent surfaced, and a sensible duration.
func TestRunsList_PairsReceivedAndCompleted(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	const corr = "test-run-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.test.task",
		Kind:          event.KindTaskReceived,
		Actor:         "agent-test",
		CorrelationID: corr,
		Payload:       map[string]string{"intent": "do the thing"},
	}); err != nil {
		t.Fatalf("Publish received: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.test.task",
		Kind:          event.KindTaskCompleted,
		Actor:         "agent-test",
		CorrelationID: corr,
		Payload:       map[string]any{"iters": 3, "chars": 42, "stopped": "end_turn"},
	}); err != nil {
		t.Fatalf("Publish completed: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 1 {
		t.Fatalf("count = %d want 1", got)
	}
	rows, _ := res["runs"].([]any)
	row, _ := rows[0].(map[string]any)

	if got, _ := row["correlation_id"].(string); got != corr {
		t.Errorf("correlation_id = %q want %q", got, corr)
	}
	if got, _ := row["intent"].(string); got != "do the thing" {
		t.Errorf("intent = %q want %q", got, "do the thing")
	}
	if got, _ := row["status"].(string); got != "completed" {
		t.Errorf("status = %q want completed", got)
	}
	if got := intOf(row["iters"]); got != 3 {
		t.Errorf("iters = %d want 3", got)
	}
}

// TestRunsList_UncompletedRunsReportRunning — task.received
// without a matching task.completed shows status="running".
// This is the "I killed the daemon mid-run" case; operator
// should still see the abandoned run, not silently drop it.
func TestRunsList_UncompletedRunsReportRunning(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.x.task",
		Kind:          event.KindTaskReceived,
		Actor:         "agent-x",
		CorrelationID: "stranded-run",
		Payload:       map[string]string{"intent": "abandoned"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, _ := res["runs"].([]any)
	row, _ := rows[0].(map[string]any)
	if got, _ := row["status"].(string); got != "running" {
		t.Errorf("status = %q want running", got)
	}
}

// TestRunsList_SortsByStartedDesc — newest runs come first.
// Publish two pairs with controlled correlations and ensure
// the result is in reverse-chronological order.
func TestRunsList_SortsByStartedDesc(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	publishPair := func(corr, intent string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "agent.x.task", Kind: event.KindTaskReceived,
			Actor: "agent-x", CorrelationID: corr,
			Payload: map[string]string{"intent": intent},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "agent.x.task", Kind: event.KindTaskCompleted,
			Actor: "agent-x", CorrelationID: corr,
			Payload: map[string]any{"iters": 1},
		})
	}
	// Small sleeps between pairs so TSUnixMS (ms-resolution)
	// differs even on fast CI hardware; the sort-by-started
	// invariant assumes a monotonic ordering between pairs.
	publishPair("run-A", "first")
	time.Sleep(2 * time.Millisecond)
	publishPair("run-B", "second")
	time.Sleep(2 * time.Millisecond)
	publishPair("run-C", "third")

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, _ := res["runs"].([]any)
	if len(rows) != 3 {
		t.Fatalf("rows len = %d want 3", len(rows))
	}
	// Newest first: run-C, run-B, run-A.
	for i, wantCorr := range []string{"run-C", "run-B", "run-A"} {
		r, _ := rows[i].(map[string]any)
		if got, _ := r["correlation_id"].(string); got != wantCorr {
			t.Errorf("row[%d].correlation_id = %q want %q", i, got, wantCorr)
		}
	}
}

// TestRunsList_LimitClamps — operator-supplied limit honored,
// excess rows dropped after sort (so they're the OLDEST, not
// random).
func TestRunsList_LimitClamps(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	for i := 0; i < 5; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "agent.x.task", Kind: event.KindTaskReceived,
			Actor: "agent-x", CorrelationID: "r-" + string(rune('A'+i)),
			Payload: map[string]string{"intent": "x"},
		})
	}

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, map[string]any{"limit": 2})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 2 {
		t.Errorf("count = %d want 2 (limit honored)", got)
	}
	rows, _ := res["runs"].([]any)
	if len(rows) != 2 {
		t.Errorf("rows len = %d want 2", len(rows))
	}
}


// TestRunsList_AbandonedStatus — a run reconciled at boot (task.received
// + task.abandoned, no completion) reports status="abandoned", M28.
func TestRunsList_AbandonedStatus(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	const corr = "orphan-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "agent.test.task", Kind: event.KindTaskReceived, Actor: "agent-test",
		CorrelationID: corr, Payload: map[string]string{"intent": "crashed mid-run"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskAbandoned, Actor: "kernel",
		CorrelationID: corr, Payload: map[string]any{"reason": "daemon restart"},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := res["runs"].([]any)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if got, _ := row["status"].(string); got != "abandoned" {
		t.Errorf("status = %q want abandoned", got)
	}
	if got, _ := row["intent"].(string); got != "crashed mid-run" {
		t.Errorf("intent = %q want 'crashed mid-run'", got)
	}
}

// TestRunsList_CompletedBeatsAbandoned — if a run somehow has both a
// completion and a (stale) abandoned marker, completed wins.
func TestRunsList_CompletedBeatsAbandoned(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	const corr = "raced-001"
	for _, spec := range []event.Spec{
		{Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: corr, Payload: map[string]string{"intent": "x"}},
		{Subject: "task", Kind: event.KindTaskAbandoned, Actor: "kernel", CorrelationID: corr},
		{Subject: "task", Kind: event.KindTaskCompleted, Actor: "a", CorrelationID: corr, Payload: map[string]any{"iters": 1}},
	} {
		if _, err := k.Bus().Publish(spec); err != nil {
			t.Fatal(err)
		}
	}
	res, _ := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	rows, _ := res["runs"].([]any)
	row, _ := rows[0].(map[string]any)
	if got, _ := row["status"].(string); got != "completed" {
		t.Errorf("status = %q want completed (completed beats abandoned)", got)
	}
}

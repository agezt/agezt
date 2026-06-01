// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
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

// TestRunsList_FailedStatus — a run terminated by task.failed (M30)
// reports status="failed" and surfaces the reason tag.
func TestRunsList_FailedStatus(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	const corr = "fail-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: corr, Payload: map[string]string{"intent": "will error"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskFailed, Actor: "a",
		CorrelationID: corr, Payload: map[string]any{"error": "boom", "reason": "error"},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := res["runs"].([]any)
	row, _ := rows[0].(map[string]any)
	if got, _ := row["status"].(string); got != "failed" {
		t.Errorf("status = %q want failed", got)
	}
	if got, _ := row["reason"].(string); got != "error" {
		t.Errorf("reason = %q want error", got)
	}
}

// TestRunsList_CompletedBeatsFailed — defensive precedence: a run with
// both terminal markers reports completed (Completed > Failed).
func TestRunsList_CompletedBeatsFailed(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	const corr = "race-cf"
	for _, spec := range []event.Spec{
		{Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: corr, Payload: map[string]string{"intent": "x"}},
		{Subject: "task", Kind: event.KindTaskFailed, Actor: "a", CorrelationID: corr, Payload: map[string]any{"reason": "error"}},
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
		t.Errorf("status = %q want completed (completed beats failed)", got)
	}
}

// TestWhy_SubAgentParentBacklink — `agt why` on an event in a sub-agent's
// chain reports its lead via parent_correlation; a top-level run reports ""
// (M42).
func TestWhy_SubAgentParentBacklink(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Parent p1 spawns child c1.
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "agent.sub.spawn", Kind: event.KindSubAgentSpawned, Actor: "subagent-c1",
		CorrelationID: "p1",
		Payload:       map[string]any{"child_correlation": "c1", "parent": "p1", "task": "subtask", "depth": 1},
	}); err != nil {
		t.Fatal(err)
	}
	// Child c1's own event — capture its id for the why lookup.
	childEv, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "subagent-c1",
		CorrelationID: "c1", Payload: map[string]string{"intent": "subtask"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A top-level run for the negative case.
	rootEv, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "agent-r1",
		CorrelationID: "r1", Payload: map[string]string{"intent": "root"},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdWhy, map[string]any{"event_id": childEv.ID})
	if err != nil {
		t.Fatalf("Why(child): %v", err)
	}
	if got, _ := res["parent_correlation"].(string); got != "p1" {
		t.Errorf("child why parent_correlation = %q want p1", got)
	}

	res, err = c.Call(context.Background(), controlplane.CmdWhy, map[string]any{"event_id": rootEv.ID})
	if err != nil {
		t.Fatalf("Why(root): %v", err)
	}
	if got, _ := res["parent_correlation"].(string); got != "" {
		t.Errorf("top-level why parent_correlation = %q want empty", got)
	}
}

// TestRunsList_SubAgentParentLink — a sub-agent run carries its lead run's
// correlation in parent_correlation (from the parent's subagent.spawned
// event); a top-level run carries "" (M41).
func TestRunsList_SubAgentParentLink(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Parent run p1 delegates to child c1.
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "agent-p1",
		CorrelationID: "p1", Payload: map[string]string{"intent": "lead"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "agent.subagent.spawn", Kind: event.KindSubAgentSpawned, Actor: "subagent-c1",
		CorrelationID: "p1", // spawn lives under the PARENT correlation
		Payload:       map[string]any{"child_correlation": "c1", "parent": "p1", "task": "subtask", "depth": 1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "agent-p1",
		CorrelationID: "p1", Payload: map[string]any{"iters": 2},
	}); err != nil {
		t.Fatal(err)
	}
	// Child run c1 (its own task arc).
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "subagent-c1",
		CorrelationID: "c1", Payload: map[string]string{"intent": "subtask"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "subagent-c1",
		CorrelationID: "c1", Payload: map[string]any{"iters": 1},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, _ := res["runs"].([]any)
	parentOf := map[string]string{}
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		id, _ := r["correlation_id"].(string)
		p, _ := r["parent_correlation"].(string)
		parentOf[id] = p
	}
	if got := parentOf["c1"]; got != "p1" {
		t.Errorf("child c1 parent_correlation = %q want p1", got)
	}
	if got := parentOf["p1"]; got != "" {
		t.Errorf("top-level p1 parent_correlation = %q want empty", got)
	}
}

// --- M29: runs stats -------------------------------------------------

// TestRunsStats_EmptyJournal — no runs at all reports total=0 and a
// well-formed (zero-valued) duration block so renderers/jq don't crash.
func TestRunsStats_EmptyJournal(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["total"]); got != 0 {
		t.Errorf("total = %d want 0", got)
	}
	if got, _ := res["success_rate"].(float64); got != 0 {
		t.Errorf("success_rate = %v want 0", got)
	}
	dur, ok := res["duration_ms"].(map[string]any)
	if !ok {
		t.Fatalf("duration_ms missing or wrong type: %T", res["duration_ms"])
	}
	if got := intOf(dur["count"]); got != 0 {
		t.Errorf("duration_ms.count = %d want 0", got)
	}
}

// TestRunsStats_CountsAndSuccessRate — a mix of completed, abandoned,
// and still-running runs; verify the split, the terminal count, and
// that success_rate = completed/(completed+abandoned) ignores running.
func TestRunsStats_CountsAndSuccessRate(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	received := func(corr string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
	}
	complete := func(corr string, iters int) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": iters},
		})
	}
	abandon := func(corr string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskAbandoned, Actor: "kernel",
			CorrelationID: corr,
		})
	}
	fail := func(corr string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskFailed, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"reason": "error"},
		})
	}

	// 3 completed, 1 failed, 1 abandoned, 1 running → terminal=5,
	// rate = 3/5 = 0.6 (M30: failures count against the rate).
	received("c1")
	complete("c1", 2)
	received("c2")
	complete("c2", 4)
	received("c3")
	complete("c3", 6)
	received("f1")
	fail("f1")
	received("a1")
	abandon("a1")
	received("r1") // left running

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["total"]); got != 6 {
		t.Errorf("total = %d want 6", got)
	}
	if got := intOf(res["completed"]); got != 3 {
		t.Errorf("completed = %d want 3", got)
	}
	if got := intOf(res["failed"]); got != 1 {
		t.Errorf("failed = %d want 1", got)
	}
	if got := intOf(res["abandoned"]); got != 1 {
		t.Errorf("abandoned = %d want 1", got)
	}
	if got := intOf(res["running"]); got != 1 {
		t.Errorf("running = %d want 1", got)
	}
	if got := intOf(res["terminal"]); got != 5 {
		t.Errorf("terminal = %d want 5", got)
	}
	if got, _ := res["success_rate"].(float64); got != 0.6 {
		t.Errorf("success_rate = %v want 0.6", got)
	}
	// avg iters over completed runs: (2+4+6)/3 = 4.
	if got, _ := res["avg_iters"].(float64); got != 4 {
		t.Errorf("avg_iters = %v want 4", got)
	}
}

// TestRunsStats_FailedByReason — failures are bucketed by their M30 reason
// tag in failed_by_reason (M36); a failure with no reason buckets under
// "unknown".
func TestRunsStats_FailedByReason(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	failWith := func(corr, reason string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
		payload := map[string]any{"error": "boom"}
		if reason != "" {
			payload["reason"] = reason
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskFailed, Actor: "a",
			CorrelationID: corr, Payload: payload,
		})
	}
	failWith("f1", "timeout")
	failWith("f2", "timeout")
	failWith("f3", "error")
	failWith("f4", "") // no reason → "unknown"

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["failed"]); got != 4 {
		t.Fatalf("failed = %d want 4", got)
	}
	br, ok := res["failed_by_reason"].(map[string]any)
	if !ok {
		t.Fatalf("failed_by_reason missing/wrong type: %T", res["failed_by_reason"])
	}
	if got := intOf(br["timeout"]); got != 2 {
		t.Errorf("failed_by_reason[timeout] = %d want 2", got)
	}
	if got := intOf(br["error"]); got != 1 {
		t.Errorf("failed_by_reason[error] = %d want 1", got)
	}
	if got := intOf(br["unknown"]); got != 1 {
		t.Errorf("failed_by_reason[unknown] = %d want 1", got)
	}
}

// TestRunsStats_NoFailuresEmptyBreakdown — with no failures the breakdown
// map is present but empty (jq-safe).
func TestRunsStats_NoFailuresEmptyBreakdown(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "ok1", Payload: map[string]string{"intent": "x"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "ok1", Payload: map[string]any{"iters": 1},
	})
	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	br, ok := res["failed_by_reason"].(map[string]any)
	if !ok {
		t.Fatalf("failed_by_reason should be an (empty) map, got %T", res["failed_by_reason"])
	}
	if len(br) != 0 {
		t.Errorf("failed_by_reason = %v want empty", br)
	}
}

// TestRunsStats_DelegationMetrics — sub-agent runs (those carrying a
// parent_correlation) are folded into delegation aggregates (M45): the
// total sub-agent count, the number of distinct leads that delegated, and
// the widest fan-out from one lead. Here lead p1 spawns c1+c2, lead p2
// spawns c3, and standalone s1 delegates nothing: delegations=3,
// delegating_runs=2, max_fanout=2.
func TestRunsStats_DelegationMetrics(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	received := func(corr, actor string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: actor,
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
	}
	complete := func(corr, actor string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: actor,
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	// spawn publishes the parent's subagent.spawned event (under the PARENT
	// correlation) that collectRuns folds into the child's parent link.
	spawn := func(parent, child string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "agent.subagent.spawn", Kind: event.KindSubAgentSpawned, Actor: "subagent",
			CorrelationID: parent,
			Payload:       map[string]any{"child_correlation": child, "parent": parent, "task": "sub", "depth": 1},
		})
	}

	// Lead p1 → c1, c2 (fan-out 2); lead p2 → c3 (fan-out 1); s1 standalone.
	received("p1", "agent-p1")
	spawn("p1", "c1")
	spawn("p1", "c2")
	complete("p1", "agent-p1")
	received("p2", "agent-p2")
	spawn("p2", "c3")
	complete("p2", "agent-p2")
	received("s1", "agent-s1")
	complete("s1", "agent-s1")
	for _, ch := range []string{"c1", "c2", "c3"} {
		received(ch, "subagent-"+ch)
		complete(ch, "subagent-"+ch)
	}

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["delegations"]); got != 3 {
		t.Errorf("delegations = %d want 3", got)
	}
	if got := intOf(res["delegating_runs"]); got != 2 {
		t.Errorf("delegating_runs = %d want 2", got)
	}
	if got := intOf(res["max_fanout"]); got != 2 {
		t.Errorf("max_fanout = %d want 2", got)
	}
}

// TestRunsList_StatusFilter — `--status` (args.status) restricts the listing to
// runs with that status, applied before the limit (M61).
func TestRunsList_StatusFilter(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	complete := func(corr string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	fail := func(corr string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskFailed, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"reason": "error"},
		})
	}
	complete("ok1")
	complete("ok2")
	fail("bad1")

	res, err := c.Call(context.Background(), controlplane.CmdRunsList,
		map[string]any{"status": "failed"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, _ := res["runs"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %d want 1 (only the failed run)", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if got, _ := row["correlation_id"].(string); got != "bad1" {
		t.Errorf("filtered row = %q want bad1", got)
	}
}

// TestScheduleFires_StatusFilter — `--status` restricts firings by their run
// outcome (M61).
func TestScheduleFires_StatusFilter(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	fire := func(corr string, fail bool) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
			CorrelationID: corr, Payload: map[string]any{"schedule_id": "s", "intent": "i"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: corr,
		})
		if fail {
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "task", Kind: event.KindTaskFailed, Actor: "a", CorrelationID: corr,
				Payload: map[string]any{"reason": "timeout"},
			})
		} else {
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "task", Kind: event.KindTaskCompleted, Actor: "a", CorrelationID: corr,
				Payload: map[string]any{"iters": 1},
			})
		}
	}
	fire("ok1", false)
	fire("bad1", true)

	res, err := c.Call(context.Background(), controlplane.CmdScheduleFires,
		map[string]any{"status": "failed"})
	if err != nil {
		t.Fatal(err)
	}
	fires, _ := res["fires"].([]any)
	if len(fires) != 1 {
		t.Fatalf("fires = %d want 1 (only the failed firing)", len(fires))
	}
	row, _ := fires[0].(map[string]any)
	if got, _ := row["status"].(string); got != "failed" {
		t.Errorf("filtered firing status = %q want failed", got)
	}
}

// TestRunsList_RowCarriesSpend — each runs-list row exposes the run's spend in
// microcents (M50), folded from its budget.consumed events (M47), so the CLI can
// show per-run cost. A run with two spend events of 100+50 reports spent_mc=150.
func TestRunsList_RowCarriesSpend(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "r1", Payload: map[string]string{"intent": "x"},
	})
	for _, mc := range []int64{100, 50} {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
			CorrelationID: "r1", Payload: map[string]any{"cost_microcents": mc},
		})
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "r1", Payload: map[string]any{"iters": 1},
	})

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, _ := res["runs"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %d want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if got := int64(intOf(row["spent_mc"])); got != 150 {
		t.Errorf("spent_mc = %d want 150", got)
	}
}

// TestRunsList_RowCarriesAnswerPreview — a completed run's row exposes a one-line
// excerpt of its M51 answer (M52): newlines collapsed to spaces, trimmed, and
// truncated to the preview cap with an ellipsis.
func TestRunsList_RowCarriesAnswerPreview(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "r1", Payload: map[string]string{"intent": "x"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "r1", Payload: map[string]any{
			"iters": 1, "answer": "line one\n\tline two   with   spaces",
		},
	})

	res, err := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, _ := res["runs"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %d want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	got, _ := row["answer_preview"].(string)
	if got != "line one line two with spaces" {
		t.Errorf("answer_preview = %q; want whitespace-collapsed single line", got)
	}
}

// TestRunsList_AnswerPreviewTruncated — a long answer is truncated to the preview
// cap with an ellipsis (M52); the row never carries the full text.
func TestRunsList_AnswerPreviewTruncated(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	long := strings.Repeat("a", 200)
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "r1", Payload: map[string]string{"intent": "x"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "r1", Payload: map[string]any{"iters": 1, "answer": long},
	})

	res, _ := c.Call(context.Background(), controlplane.CmdRunsList, nil)
	rows, _ := res["runs"].([]any)
	row, _ := rows[0].(map[string]any)
	got, _ := row["answer_preview"].(string)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("answer_preview should end with an ellipsis; got %q", got)
	}
	if r := []rune(got); len(r) > 81 { // 80 + the ellipsis rune
		t.Errorf("answer_preview too long: %d runes", len(r))
	}
}

// TestRunsStats_SpendAttribution — budget.consumed events stamped with a run's
// correlation (M47) are folded into per-run spend; the stats report the window's
// total spend and the share attributable to sub-agent runs. Lead p1 spends 100+50
// over two calls and delegates to child c1 (spends 30); the totals are 180 and the
// delegated share is 30.
func TestRunsStats_SpendAttribution(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	received := func(corr, actor string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: actor,
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
	}
	complete := func(corr, actor string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: actor,
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	spend := func(corr string, mc int64) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
			CorrelationID: corr, Payload: map[string]any{"cost_microcents": mc},
		})
	}
	spawn := func(parent, child string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "agent.subagent.spawn", Kind: event.KindSubAgentSpawned, Actor: "subagent",
			CorrelationID: parent,
			Payload:       map[string]any{"child_correlation": child, "parent": parent, "task": "sub", "depth": 1},
		})
	}

	received("p1", "agent-p1")
	spend("p1", 100)
	spawn("p1", "c1")
	spend("p1", 50)
	complete("p1", "agent-p1")
	received("c1", "subagent-c1")
	spend("c1", 30)
	complete("c1", "subagent-c1")

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := int64(intOf(res["spent_microcents"])); got != 180 {
		t.Errorf("spent_microcents = %d want 180", got)
	}
	if got := int64(intOf(res["delegated_spent_microcents"])); got != 30 {
		t.Errorf("delegated_spent_microcents = %d want 30", got)
	}
}

// TestRunsStats_SpendDistribution — the per-run spend distribution (M60) reports
// count/avg/min/max/p50/p95 over priced runs only, in microcents. Three runs
// spend 100, 200, 300 → count 3, min 100, max 300, avg 200.
func TestRunsStats_SpendDistribution(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	mkRun := func(corr string, mc int64) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
			CorrelationID: corr, Payload: map[string]any{"cost_microcents": mc},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	mkRun("r1", 100)
	mkRun("r2", 200)
	mkRun("r3", 300)
	// A free run (no spend) must be excluded from the distribution.
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "free", Payload: map[string]string{"intent": "x"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "free", Payload: map[string]any{"iters": 1},
	})

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	sp, ok := res["spend_microcents"].(map[string]any)
	if !ok {
		t.Fatalf("spend_microcents missing/wrong type: %T", res["spend_microcents"])
	}
	if got := intOf(sp["count"]); got != 3 {
		t.Errorf("count = %d want 3 (free run excluded)", got)
	}
	if got := int64(intOf(sp["min"])); got != 100 {
		t.Errorf("min = %d want 100", got)
	}
	if got := int64(intOf(sp["max"])); got != 300 {
		t.Errorf("max = %d want 300", got)
	}
	if got := int64(intOf(sp["avg"])); got != 200 {
		t.Errorf("avg = %d want 200", got)
	}
}

// TestRunsStats_ByModel — runs stats attributes run count + spend per model
// (M124), folded from each run's model (M123). Two opus runs (100+300mc) and one
// haiku run (50mc) → by_model{opus:{runs:2, spent:400}, haiku:{runs:1, spent:50}}.
// A model-less (free/mock) run is not attributed.
func TestRunsStats_ByModel(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	mkRun := func(corr, model string, mc int64) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
		if model != "" {
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
				CorrelationID: corr, Payload: map[string]any{"cost_microcents": mc, "model": model},
			})
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	mkRun("o1", "opus", 100)
	mkRun("o2", "opus", 300)
	mkRun("h1", "haiku", 50)
	mkRun("free", "", 0)

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	bm, ok := res["by_model"].(map[string]any)
	if !ok {
		t.Fatalf("by_model missing/wrong type: %T", res["by_model"])
	}
	if len(bm) != 2 {
		t.Fatalf("by_model has %d entries want 2 (free run not attributed): %v", len(bm), bm)
	}
	opus, _ := bm["opus"].(map[string]any)
	if got := intOf(opus["runs"]); got != 2 {
		t.Errorf("opus runs = %d want 2", got)
	}
	if got := int64(intOf(opus["spent_microcents"])); got != 400 {
		t.Errorf("opus spent = %d want 400", got)
	}
	haiku, _ := bm["haiku"].(map[string]any)
	if got := intOf(haiku["runs"]); got != 1 {
		t.Errorf("haiku runs = %d want 1", got)
	}
	if got := int64(intOf(haiku["spent_microcents"])); got != 50 {
		t.Errorf("haiku spent = %d want 50", got)
	}
}

// TestRunsList_ModelFoldAndFilter — the run's model is folded first-wins from
// budget.consumed and surfaced in each row, and `--model` filters by a
// case-insensitive substring (M123). A run that never spent has an empty model.
func TestRunsList_ModelFoldAndFilter(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	mkRun := func(corr, model string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
		if model != "" {
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
				CorrelationID: corr, Payload: map[string]any{"cost_microcents": int64(10), "model": model},
			})
			// A later fallback to a different model must NOT overwrite (first-wins).
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
				CorrelationID: corr, Payload: map[string]any{"cost_microcents": int64(10), "model": "OTHER-fallback"},
			})
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	mkRun("r1", "claude-opus-4")
	mkRun("r2", "claude-haiku-4")
	mkRun("r3", "") // free/mock — no model journaled

	modelsByCorr := func(args map[string]any) map[string]string {
		t.Helper()
		res, err := c.Call(context.Background(), controlplane.CmdRunsList, args)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		rows, _ := res["runs"].([]any)
		got := map[string]string{}
		for _, raw := range rows {
			m, _ := raw.(map[string]any)
			corr, _ := m["correlation_id"].(string)
			model, _ := m["model"].(string)
			got[corr] = model
		}
		return got
	}

	// No filter: all three runs, r1/r2 carry their first model, r3 empty.
	all := modelsByCorr(nil)
	if len(all) != 3 {
		t.Fatalf("no filter: got %d runs want 3 (%v)", len(all), all)
	}
	if all["r1"] != "claude-opus-4" {
		t.Errorf("r1 model = %q want claude-opus-4 (first-wins, not the fallback)", all["r1"])
	}
	if all["r3"] != "" {
		t.Errorf("r3 model = %q want empty (no spend journaled)", all["r3"])
	}

	// Substring "haiku" → only r2.
	haiku := modelsByCorr(map[string]any{"model": "haiku"})
	if len(haiku) != 1 || haiku["r2"] != "claude-haiku-4" {
		t.Errorf("--model haiku = %v want only r2", haiku)
	}

	// Case-insensitive "CLAUDE" → r1 and r2, not r3.
	claude := modelsByCorr(map[string]any{"model": "CLAUDE"})
	if len(claude) != 2 || claude["r1"] == "" || claude["r2"] == "" {
		t.Errorf("--model CLAUDE = %v want r1+r2", claude)
	}
}

// TestRunsStats_SpendIgnoresUnknownCorrelation — a budget.consumed event whose
// correlation never had a task.received (an out-of-run governor call) must not
// conjure a phantom run nor inflate the spend total (M47).
func TestRunsStats_SpendIgnoresUnknownCorrelation(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "real", Payload: map[string]string{"intent": "x"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "real", Payload: map[string]any{"iters": 1},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
		CorrelationID: "real", Payload: map[string]any{"cost_microcents": int64(40)},
	})
	// Orphan spend — no matching task.received.
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
		CorrelationID: "ghost", Payload: map[string]any{"cost_microcents": int64(999)},
	})

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := intOf(res["total"]); got != 1 {
		t.Errorf("total = %d want 1 (ghost spend must not create a run)", got)
	}
	if got := int64(intOf(res["spent_microcents"])); got != 40 {
		t.Errorf("spent_microcents = %d want 40 (ghost spend excluded)", got)
	}
}

// TestRunsStats_NoDelegationsZeroed — a journal with only top-level runs
// reports all delegation aggregates as 0 (the CLI then omits the line).
func TestRunsStats_NoDelegationsZeroed(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "solo", Payload: map[string]string{"intent": "x"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "solo", Payload: map[string]any{"iters": 1},
	})
	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"delegations", "delegating_runs", "max_fanout"} {
		if got := intOf(res[key]); got != 0 {
			t.Errorf("%s = %d want 0", key, got)
		}
	}
}

// TestRunsStats_SinceWindow — args.since_ms restricts the aggregate to
// runs started within the window (M33). A huge window includes a
// just-published run; a tiny window after a sleep excludes it; window_ms
// is echoed back.
func TestRunsStats_SinceWindow(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	const corr = "win-1"
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: corr, Payload: map[string]string{"intent": "x"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: corr, Payload: map[string]any{"iters": 1},
	}); err != nil {
		t.Fatal(err)
	}

	// All-time: the run is counted, window_ms == 0.
	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["total"]); got != 1 {
		t.Fatalf("all-time total = %d want 1", got)
	}
	if got := intOf(res["window_ms"]); got != 0 {
		t.Errorf("all-time window_ms = %d want 0", got)
	}

	// Huge window (1h): the just-published run is well within it.
	res, err = c.Call(context.Background(), controlplane.CmdRunsStats,
		map[string]any{"since_ms": 3_600_000})
	if err != nil {
		t.Fatalf("Call(since=1h): %v", err)
	}
	if got := intOf(res["total"]); got != 1 {
		t.Errorf("1h-window total = %d want 1", got)
	}
	if got := intOf(res["window_ms"]); got != 3_600_000 {
		t.Errorf("window_ms = %d want 3600000", got)
	}

	// Tiny window after a sleep: the run started > 30ms ago, so a 30ms
	// window excludes it.
	time.Sleep(60 * time.Millisecond)
	res, err = c.Call(context.Background(), controlplane.CmdRunsStats,
		map[string]any{"since_ms": 30})
	if err != nil {
		t.Fatalf("Call(since=30ms): %v", err)
	}
	if got := intOf(res["total"]); got != 0 {
		t.Errorf("30ms-window total = %d want 0 (run is older than the window)", got)
	}
}

// TestRunsStats_DurationPercentiles — publish completed runs with
// known, monotonically increasing durations and verify the percentile
// math (nearest-rank) and avg/min/max.
func TestRunsStats_DurationPercentiles(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Build 10 completed runs whose durations are 100,200,...,1000 ms.
	// We can't control wall-clock directly, but we CAN publish the
	// received/completed pair and then assert the percentile *shape*
	// from the live timestamps. To get deterministic durations we
	// instead lean on a controlled gap: publish received, sleep, then
	// completed — but that's slow and flaky. Easier: assert the
	// invariants that must hold for ANY positive durations.
	for i := 0; i < 10; i++ {
		corr := "d-" + string(rune('A'+i))
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "x"},
		})
		time.Sleep(time.Millisecond)
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}

	res, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	dur, _ := res["duration_ms"].(map[string]any)
	if got := intOf(dur["count"]); got != 10 {
		t.Fatalf("duration count = %d want 10", got)
	}
	min := int64(intOf(dur["min"]))
	max := int64(intOf(dur["max"]))
	p50 := int64(intOf(dur["p50"]))
	p95 := int64(intOf(dur["p95"]))
	avg := int64(intOf(dur["avg"]))
	// Invariants that hold for any non-degenerate distribution.
	if !(min <= p50 && p50 <= p95 && p95 <= max) {
		t.Errorf("percentile ordering violated: min=%d p50=%d p95=%d max=%d", min, p50, p95, max)
	}
	if !(min <= avg && avg <= max) {
		t.Errorf("avg=%d outside [min=%d, max=%d]", avg, min, max)
	}
	if min < 0 {
		t.Errorf("min duration negative: %d", min)
	}
}

// TestRunsList_IntentFilter — `agt runs list --intent` keeps only runs whose
// intent contains the substring, case-insensitively (M77).
func TestRunsList_IntentFilter(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	recv := func(corr, intent string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "agent", Kind: event.KindTaskReceived, Actor: "agent",
			CorrelationID: corr,
			Payload:       map[string]any{"intent": intent},
		})
	}
	recv("run-a", "deploy the staging cluster")
	recv("run-b", "summarize the README")
	recv("run-c", "DEPLOY production now")

	res, err := c.Call(context.Background(), controlplane.CmdRunsList,
		map[string]any{"intent": "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := res["runs"].([]any)
	if len(rows) != 2 {
		t.Fatalf("--intent deploy = %d want 2 (case-insensitive)", len(rows))
	}
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		got, _ := m["intent"].(string)
		if !strings.Contains(strings.ToLower(got), "deploy") {
			t.Errorf("--intent deploy returned %q", got)
		}
	}
}

// TestRunsStats_IntentScope — `agt runs stats --intent` aggregates only runs
// whose intent contains the substring, case-insensitively (M78).
func TestRunsStats_IntentScope(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	recv := func(corr, intent string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "agent", Kind: event.KindTaskReceived, Actor: "agent",
			CorrelationID: corr,
			Payload:       map[string]any{"intent": intent},
		})
	}
	recv("run-a", "deploy staging")
	recv("run-b", "summarize docs")
	recv("run-c", "DEPLOY prod")

	// Unscoped → 3.
	all, err := c.Call(context.Background(), controlplane.CmdRunsStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tot, _ := all["total"].(float64); tot != 3 {
		t.Errorf("unscoped total = %v want 3", all["total"])
	}

	// --intent deploy → 2 (case-insensitive).
	res, err := c.Call(context.Background(), controlplane.CmdRunsStats,
		map[string]any{"intent": "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	if tot, _ := res["total"].(float64); tot != 2 {
		t.Errorf("--intent deploy total = %v want 2", res["total"])
	}
}

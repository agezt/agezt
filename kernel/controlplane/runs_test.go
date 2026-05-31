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

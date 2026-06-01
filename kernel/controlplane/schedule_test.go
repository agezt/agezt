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

// TestScheduleFires_JoinsRunOutcome — `agt schedule fires` lists only scheduled
// firings (schedule.fired events), each joined with its run's outcome from the
// shared collectRuns fold: status, spend (M47), and answer preview (M52). A
// manual (non-scheduled) run must NOT appear (M54).
func TestScheduleFires_JoinsRunOutcome(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// A schedule fired under correlation f1, then its run completed with spend.
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
		CorrelationID: "f1", Payload: map[string]any{"intent": "summarize the day", "model": "m1"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "f1", Payload: map[string]string{"intent": "summarize the day"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
		CorrelationID: "f1", Payload: map[string]any{"cost_microcents": int64(2_100_000)},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "f1", Payload: map[string]any{"iters": 1, "answer": "all done"},
	})
	// A manual (non-scheduled) run — must not show up under fires.
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "manual", Payload: map[string]string{"intent": "manual run"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
		CorrelationID: "manual", Payload: map[string]any{"iters": 1},
	})

	res, err := c.Call(context.Background(), controlplane.CmdScheduleFires, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	fires, _ := res["fires"].([]any)
	if len(fires) != 1 {
		t.Fatalf("fires = %d want 1 (only the scheduled firing)", len(fires))
	}
	row, _ := fires[0].(map[string]any)
	if got, _ := row["correlation_id"].(string); got != "f1" {
		t.Errorf("correlation_id = %q want f1", got)
	}
	if got, _ := row["status"].(string); got != "completed" {
		t.Errorf("status = %q want completed", got)
	}
	if got, _ := row["intent"].(string); got != "summarize the day" {
		t.Errorf("intent = %q want summarize the day", got)
	}
	if got := int64(intOf(row["spent_mc"])); got != 2_100_000 {
		t.Errorf("spent_mc = %d want 2100000", got)
	}
	if got, _ := row["answer_preview"].(string); got != "all done" {
		t.Errorf("answer_preview = %q want \"all done\"", got)
	}
}

// TestScheduleFires_EmptyWhenNoFirings — a journal with runs but no
// schedule.fired events returns an empty (non-nil) fires array (M54).
func TestScheduleFires_EmptyWhenNoFirings(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "r1", Payload: map[string]string{"intent": "x"},
	})
	res, err := c.Call(context.Background(), controlplane.CmdScheduleFires, nil)
	if err != nil {
		t.Fatal(err)
	}
	fires, ok := res["fires"].([]any)
	if !ok {
		t.Fatalf("fires should be an array, got %T", res["fires"])
	}
	if len(fires) != 0 {
		t.Errorf("fires = %d want 0", len(fires))
	}
}

func TestScheduleAddListRemove(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "summarise new commits", "interval_sec": 3600, "model": "sonnet",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	id, _ := res["id"].(string)
	if id == "" {
		t.Fatal("add must return an id")
	}
	if sec, _ := res["interval_sec"].(float64); sec != 3600 {
		t.Errorf("interval_sec = %v", res["interval_sec"])
	}

	// List
	res, err = c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	list, _ := res["schedules"].([]any)
	if len(list) != 1 {
		t.Fatalf("list count = %d, want 1", len(list))
	}
	m, _ := list[0].(map[string]any)
	if m["intent"] != "summarise new commits" || m["source"] != "operator" || m["enabled"] != true {
		t.Errorf("listed entry = %v", m)
	}

	// Run now → marks it due
	res, err = c.Call(ctx, controlplane.CmdScheduleRun, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if triggered, _ := res["triggered"].(bool); !triggered {
		t.Error("run should report triggered=true")
	}

	// Remove
	res, err = c.Call(ctx, controlplane.CmdScheduleRemove, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	if removed, _ := res["removed"].(bool); !removed {
		t.Error("rm should report removed=true")
	}

	// List is now empty
	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	if list, _ := res["schedules"].([]any); len(list) != 0 {
		t.Errorf("after rm, list count = %d, want 0", len(list))
	}
}

func TestScheduleAddDailyAndPauseResume(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Daily wall-clock schedule via at_minutes (09:30 = 570).
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "morning brief", "at_minutes": 570,
	})
	if err != nil {
		t.Fatalf("add daily: %v", err)
	}
	if res["mode"] != "daily" {
		t.Errorf("mode = %v, want daily", res["mode"])
	}
	id, _ := res["id"].(string)

	// Pause → enabled=false in the listing.
	if _, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": false}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	list, _ := res["schedules"].([]any)
	m, _ := list[0].(map[string]any)
	if m["enabled"] != false {
		t.Errorf("paused entry should be disabled: %v", m["enabled"])
	}
	if m["cadence"] != "daily at 09:30" {
		t.Errorf("cadence = %v", m["cadence"])
	}

	// Resume.
	if _, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": true}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	list, _ = res["schedules"].([]any)
	m, _ = list[0].(map[string]any)
	if m["enabled"] != true {
		t.Errorf("resumed entry should be enabled")
	}
}

func TestScheduleAddDailyWithDays(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Weekdays-only daily at 09:00. maskWeekdays = Mon..Fri = bits 1..5 = 62.
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "standup nudge", "at_minutes": 540, "days": 62,
	})
	if err != nil {
		t.Fatalf("add daily+days: %v", err)
	}
	if res["mode"] != "daily" {
		t.Errorf("mode = %v, want daily", res["mode"])
	}
	if d, _ := res["days"].(float64); int(d) != 62 {
		t.Errorf("days = %v, want 62", res["days"])
	}

	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	list, _ := res["schedules"].([]any)
	m, _ := list[0].(map[string]any)
	if m["cadence"] != "Mon-Fri at 09:00" {
		t.Errorf("cadence = %v, want Mon-Fri at 09:00", m["cadence"])
	}
}

func TestScheduleAddOnce(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	at := time.Now().Add(time.Hour).Unix()
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "summarise the deploy", "once_at_unix": at,
	})
	if err != nil {
		t.Fatalf("add once: %v", err)
	}
	if res["mode"] != "once" {
		t.Errorf("mode = %v, want once", res["mode"])
	}
	if next, _ := res["next_run_unix"].(float64); int64(next) != at {
		t.Errorf("next_run_unix = %v, want %d", res["next_run_unix"], at)
	}

	// A one-shot in the past is rejected by the store.
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "too late", "once_at_unix": time.Now().Add(-time.Hour).Unix(),
	}); err == nil {
		t.Error("a past one-shot should error")
	}
}

func TestScheduleEdit(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Start with an interval schedule.
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "old", "interval_sec": 3600,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	id, _ := res["id"].(string)

	// Edit intent + reschedule to daily weekdays at 09:30 in one call.
	res, err = c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id": id, "intent": "new", "at_minutes": 570, "days": 62,
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if res["updated"] != true {
		t.Fatalf("updated = %v", res["updated"])
	}
	if res["mode"] != "daily" || res["cadence"] != "Mon-Fri at 09:30" {
		t.Errorf("edit result = %v", res)
	}

	// Verify via list.
	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	list, _ := res["schedules"].([]any)
	m, _ := list[0].(map[string]any)
	if m["intent"] != "new" || m["cadence"] != "Mon-Fri at 09:30" || m["id"] != id {
		t.Errorf("listed after edit = %v", m)
	}

	// Editing a missing id reports updated=false (not an error).
	res, err = c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{"id": "nope", "intent": "x"})
	if err != nil {
		t.Fatalf("edit missing: %v", err)
	}
	if res["updated"] != false {
		t.Errorf("missing edit updated = %v, want false", res["updated"])
	}
}

func TestScheduleAddWindow(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Windowed interval: every 15m (900s) between 09:00–17:00 on weekdays (62).
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "poll the queue", "interval_sec": 900,
		"window_start": 540, "window_end": 1020, "days": 62,
	})
	if err != nil {
		t.Fatalf("add window: %v", err)
	}
	if res["mode"] != "window" {
		t.Errorf("mode = %v, want window", res["mode"])
	}

	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	list, _ := res["schedules"].([]any)
	m, _ := list[0].(map[string]any)
	if m["cadence"] != "every 15m0s 09:00-17:00 Mon-Fri" {
		t.Errorf("cadence = %v", m["cadence"])
	}
	if end, _ := m["end_minutes"].(float64); int(end) != 1020 {
		t.Errorf("end_minutes = %v, want 1020", m["end_minutes"])
	}

	// A window with end <= start is rejected by the store.
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "bad", "interval_sec": 900, "window_start": 1020, "window_end": 540,
	}); err == nil {
		t.Error("inverted window should error")
	}
}

func TestScheduleAddDailyWithTimezone(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "tokyo brief", "at_minutes": 540, "tz": "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("add daily+tz: %v", err)
	}
	if res["mode"] != "daily" {
		t.Errorf("mode = %v", res["mode"])
	}

	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	list, _ := res["schedules"].([]any)
	m, _ := list[0].(map[string]any)
	if m["tz"] != "Asia/Tokyo" || m["cadence"] != "daily at 09:00 Asia/Tokyo" {
		t.Errorf("listed entry = %v", m)
	}

	// An unknown zone is rejected by the store.
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "bad", "at_minutes": 540, "tz": "Mars/Phobos",
	}); err == nil {
		t.Error("unknown timezone should error")
	}
}

func TestScheduleAddValidates(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{"interval_sec": 60}); err == nil {
		t.Error("add without intent must error")
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{"intent": "x", "interval_sec": 0}); err == nil {
		t.Error("add with interval_sec < 1 must error")
	}
}

func TestScheduleRemoveMissing(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdScheduleRemove, map[string]any{"id": "nope"})
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	if removed, _ := res["removed"].(bool); removed {
		t.Error("removing a missing id should report removed=false")
	}
}

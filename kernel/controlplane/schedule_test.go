// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
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
		CorrelationID: "f1", Payload: map[string]any{
			"schedule_id": "sched-A",
			"intent":      "Nightly label",
			"model":       "m1",
			"target":      cadence.TargetWorkflow,
			"workflow":    "nightly-sync",
			"agent":       "ops",
			"autonomy_runbook": map[string]any{
				"trigger_contract":  "operator_schedule_channel",
				"route_contract":    "self_owned",
				"recovery_contract": "retry",
				"sleep_contract":    "cycle",
				"retry_attempts":    3,
			},
		},
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
	if got, _ := row["schedule_id"].(string); got != "sched-A" {
		t.Errorf("schedule_id = %q want sched-A (M55)", got)
	}
	if got, _ := row["status"].(string); got != "completed" {
		t.Errorf("status = %q want completed", got)
	}
	if got, _ := row["intent"].(string); got != "Nightly label" {
		t.Errorf("intent = %q want Nightly label", got)
	}
	if got, _ := row["target"].(string); got != cadence.TargetWorkflow {
		t.Errorf("target = %q want workflow", got)
	}
	if got, _ := row["workflow"].(string); got != "nightly-sync" {
		t.Errorf("workflow = %q want nightly-sync", got)
	}
	if got, _ := row["agent"].(string); got != "ops" {
		t.Errorf("agent = %q want ops", got)
	}
	runbook, _ := row["autonomy_runbook"].(map[string]any)
	if runbook["trigger_contract"] != "operator_schedule_channel" || runbook["recovery_contract"] != "retry" || intOf(runbook["retry_attempts"]) != 3 {
		t.Errorf("autonomy_runbook = %+v", runbook)
	}
	if got, _ := row["action"].(string); got != "run workflow nightly-sync" {
		t.Errorf("action = %q want run workflow nightly-sync", got)
	}
	if got := int64(intOf(row["spent_mc"])); got != 2_100_000 {
		t.Errorf("spent_mc = %d want 2100000", got)
	}
	if got, _ := row["answer_preview"].(string); got != "all done" {
		t.Errorf("answer_preview = %q want \"all done\"", got)
	}
}

func TestScheduleFires_SystemTaskExecutionMetadata(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.fired",
		Kind:          event.KindScheduleFired,
		Actor:         "schedule",
		CorrelationID: "sys-1",
		Payload: map[string]any{
			"schedule_id": "sched-system",
			"intent":      "system task catalog_sync",
			"target":      cadence.TargetSystemTask,
			"system_task": cadence.SystemTaskCatalogSync,
		},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskReceived,
		Actor:         "schedule",
		CorrelationID: "sys-1",
		Payload:       map[string]any{"schedule_id": "sched-system", "target": cadence.TargetSystemTask},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskCompleted,
		Actor:         "schedule",
		CorrelationID: "sys-1",
		Payload:       map[string]any{"schedule_id": "sched-system", "target": cadence.TargetSystemTask, "answer": "system task catalog_sync completed"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdScheduleFires, nil)
	if err != nil {
		t.Fatalf("ScheduleFires: %v", err)
	}
	fires, _ := res["fires"].([]any)
	if len(fires) != 1 {
		t.Fatalf("fires = %d want 1", len(fires))
	}
	row, _ := fires[0].(map[string]any)
	if row["target"] != cadence.TargetSystemTask || row["system_task"] != cadence.SystemTaskCatalogSync {
		t.Fatalf("system task firing row wrong: %+v", row)
	}
	if row["executor"] != "daemon" || row["category"] != "catalog" || row["effect_class"] != "config_update" || row["uses_llm"] != false {
		t.Fatalf("system task execution metadata wrong: %+v", row)
	}
	if row["status"] != "completed" || row["answer_preview"] != "system task catalog_sync completed" {
		t.Fatalf("system task outcome wrong: %+v", row)
	}
}

// TestScheduleList_ShowsLastFiringOutcome — a schedule's row carries its most
// recent firing's outcome (M56), folded from schedule.fired + the run outcome.
func TestScheduleList_ShowsLastFiringOutcome(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Create a real schedule so it appears in the list, capture its id.
	add, err := c.Call(ctx, controlplane.CmdScheduleAdd,
		map[string]any{"intent": "daily brief", "interval_sec": 3600})
	if err != nil {
		t.Fatalf("ScheduleAdd: %v", err)
	}
	schedID, _ := add["id"].(string)
	if schedID == "" {
		t.Fatal("ScheduleAdd returned no id")
	}

	// Two firings of it; the later one (f2) failed and must win as "last".
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
		CorrelationID: "f1", Payload: map[string]any{"schedule_id": schedID, "intent": "daily brief"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: "f1",
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskCompleted, Actor: "a", CorrelationID: "f1",
		Payload: map[string]any{"iters": 1},
	})
	time.Sleep(2 * time.Millisecond) // ensure f2 has a later TSUnixMS
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
		CorrelationID: "f2", Payload: map[string]any{"schedule_id": schedID, "intent": "daily brief"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: "f2",
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskFailed, Actor: "a", CorrelationID: "f2",
		Payload: map[string]any{"reason": "timeout"},
	})

	res, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList: %v", err)
	}
	list, _ := res["schedules"].([]any)
	var row map[string]any
	for _, item := range list {
		m, _ := item.(map[string]any)
		if id, _ := m["id"].(string); id == schedID {
			row = m
		}
	}
	if row == nil {
		t.Fatalf("schedule %s not in list", schedID)
	}
	if got, _ := row["last_status"].(string); got != "failed" {
		t.Errorf("last_status = %q want failed (the later firing)", got)
	}
	if got, _ := row["last_reason"].(string); got != "timeout" {
		t.Errorf("last_reason = %q want timeout", got)
	}
}

func TestScheduleAdd_WorkflowTarget(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	graph := map[string]any{
		"name": "scheduled-flow",
		"nodes": []any{
			map[string]any{"id": "start", "type": "trigger"},
			map[string]any{"id": "shape", "type": "transform", "config": map[string]any{"template": "ok"}},
		},
		"edges": []any{map[string]any{"from": "start", "to": "shape"}},
	}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": graph}); err != nil {
		t.Fatalf("WorkflowSave: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("AgentAdd: %v", err)
	}
	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "workflow",
		"workflow":     "scheduled-flow",
		"agent":        "ops",
		"model":        "gpt-5",
		"payload":      map[string]any{"city": "izmir"},
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd workflow target: %v", err)
	}
	if got, _ := add["target"].(string); got != "workflow" {
		t.Fatalf("target = %q want workflow", got)
	}
	if got, _ := add["workflow"].(string); got != "scheduled-flow" {
		t.Fatalf("workflow = %q want scheduled-flow", got)
	}
	if got, _ := add["agent"].(string); got != "ops" {
		t.Fatalf("workflow schedule agent = %q want ops", got)
	}
	if got, _ := add["model"].(string); got != "gpt-5" {
		t.Fatalf("workflow schedule model = %q want gpt-5", got)
	}
	if add["executor"] != "workflow" || add["uses_llm"] != true || add["execution_contract"] != "cron runs workflow scheduled-flow as ops" {
		t.Fatalf("workflow schedule execution metadata wrong: %v", add)
	}
	if add["execution_authority"] != "agent ops" || add["identity_owner"] != "agent ops" ||
		add["payload_contract"] != "cron passes JSON workflow payload" ||
		add["llm_boundary"] != "workflow may use LLM nodes under workflow policy and invoking agent authority" {
		t.Fatalf("workflow schedule execution boundary wrong: %v", add)
	}

	res, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList: %v", err)
	}
	list, _ := res["schedules"].([]any)
	if len(list) != 1 {
		t.Fatalf("schedules = %d want 1", len(list))
	}
	row, _ := list[0].(map[string]any)
	if got, _ := row["target"].(string); got != "workflow" {
		t.Errorf("listed target = %q want workflow", got)
	}
	if got, _ := row["workflow"].(string); got != "scheduled-flow" {
		t.Errorf("listed workflow = %q want scheduled-flow", got)
	}
	if got, _ := row["agent"].(string); got != "ops" {
		t.Errorf("listed agent = %q want ops", got)
	}
	if got, _ := row["model"].(string); got != "gpt-5" {
		t.Errorf("listed model = %q want gpt-5", got)
	}
	if row["executor"] != "workflow" || row["uses_llm"] != true || row["execution_contract"] != "cron runs workflow scheduled-flow as ops" {
		t.Fatalf("listed workflow schedule execution metadata wrong: %v", row)
	}
	if row["execution_authority"] != "agent ops" || row["identity_owner"] != "agent ops" ||
		row["payload_contract"] != "cron passes JSON workflow payload" ||
		row["llm_boundary"] != "workflow may use LLM nodes under workflow policy and invoking agent authority" {
		t.Fatalf("listed workflow schedule execution boundary wrong: %v", row)
	}
}

func TestScheduleEdit_WorkflowTargetBackToAgentIntent(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	graph := map[string]any{
		"name": "scheduled-flow",
		"nodes": []any{
			map[string]any{"id": "start", "type": "trigger"},
			map[string]any{"id": "shape", "type": "transform", "config": map[string]any{"template": "ok"}},
		},
		"edges": []any{map[string]any{"from": "start", "to": "shape"}},
	}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": graph}); err != nil {
		t.Fatalf("WorkflowSave: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("AgentAdd: %v", err)
	}
	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "workflow",
		"workflow":     "scheduled-flow",
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd workflow target: %v", err)
	}
	id, _ := add["id"].(string)
	edit, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":     id,
		"target": "",
		"intent": "check the queue",
		"agent":  "ops",
	})
	if err != nil {
		t.Fatalf("ScheduleEdit workflow→agent: %v", err)
	}
	if got, _ := edit["target"].(string); got != "" {
		t.Fatalf("target = %q want intent target", got)
	}
	if got, _ := edit["workflow"].(string); got != "" {
		t.Fatalf("workflow should be cleared, got %q", got)
	}
	if got, _ := edit["agent"].(string); got != "ops" {
		t.Fatalf("agent = %q want ops", got)
	}
}

func TestScheduleAdd_SystemTaskTarget(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "catalog_sync",
		"model":        "gpt-5",
		"interval_sec": float64(86400),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd system task target: %v", err)
	}
	if got, _ := add["target"].(string); got != "system_task" {
		t.Fatalf("target = %q want system_task", got)
	}
	if got, _ := add["system_task"].(string); got != "catalog_sync" {
		t.Fatalf("system_task = %q want catalog_sync", got)
	}
	if got, _ := add["agent"].(string); got != "" {
		t.Fatalf("system task schedule should not carry agent, got %q", got)
	}
	if got, _ := add["model"].(string); got != "" {
		t.Fatalf("system task schedule should not carry model, got %q", got)
	}
	if add["executor"] != "daemon" || add["uses_llm"] != false || add["execution_contract"] != "cron runs daemon system task catalog_sync" {
		t.Fatalf("system task schedule execution metadata wrong: %v", add)
	}
	if add["execution_authority"] != "daemon" || add["identity_owner"] != "none; daemon task owns no agent soul" ||
		add["payload_contract"] != "payload not accepted" || add["llm_boundary"] != "no LLM" {
		t.Fatalf("system task schedule execution boundary wrong: %v", add)
	}

	res, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList: %v", err)
	}
	list, _ := res["schedules"].([]any)
	if len(list) != 1 {
		t.Fatalf("schedules = %d want 1", len(list))
	}
	row, _ := list[0].(map[string]any)
	if got, _ := row["target"].(string); got != "system_task" {
		t.Errorf("listed target = %q want system_task", got)
	}
	if got, _ := row["system_task"].(string); got != "catalog_sync" {
		t.Errorf("listed system_task = %q want catalog_sync", got)
	}
	if row["executor"] != "daemon" || row["uses_llm"] != false || row["execution_contract"] != "cron runs daemon system task catalog_sync" {
		t.Fatalf("listed system task schedule execution metadata wrong: %v", row)
	}
	if row["execution_authority"] != "daemon" || row["identity_owner"] != "none; daemon task owns no agent soul" ||
		row["payload_contract"] != "payload not accepted" || row["llm_boundary"] != "no LLM" {
		t.Fatalf("listed system task schedule execution boundary wrong: %v", row)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "memory_clean",
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd memory_clean system task target: %v", err)
	}
	if got, _ := add["system_task"].(string); got != "memory_clean" {
		t.Fatalf("system_task = %q want memory_clean", got)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "artifact_collect",
		"interval_sec": float64(5400),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd artifact_collect system task target: %v", err)
	}
	if got, _ := add["system_task"].(string); got != "artifact_collect" {
		t.Fatalf("system_task = %q want artifact_collect", got)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "memory_tidy",
		"interval_sec": float64(7200),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd memory_tidy system task target: %v", err)
	}
	if got, _ := add["system_task"].(string); got != "memory_tidy" {
		t.Fatalf("system_task = %q want memory_tidy", got)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "log_clean",
		"interval_sec": float64(86400),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd log_clean system task target: %v", err)
	}
	if got, _ := add["system_task"].(string); got != "log_clean" {
		t.Fatalf("system_task = %q want log_clean", got)
	}
}

func TestScheduleAddAndEditContinuousCycle(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "cycle repo-watch",
		"agent":        "ops",
		"cooldown_sec": float64(90),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd continuous: %v", err)
	}
	if got, _ := add["mode"].(string); got != "continuous" {
		t.Fatalf("mode = %q want continuous", got)
	}
	if got := intOf(add["interval_sec"]); got != 90 {
		t.Fatalf("interval_sec = %d want cooldown 90", got)
	}
	if got, _ := add["agent"].(string); got != "ops" {
		t.Fatalf("agent = %q want ops", got)
	}
	if next, created := intOf(add["next_run_unix"]), intOf(add["created_unix"]); next != created {
		t.Fatalf("continuous should be due immediately, next=%d created=%d", next, created)
	}

	id, _ := add["id"].(string)
	edit, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":           id,
		"cooldown_sec": float64(120),
	})
	if err != nil {
		t.Fatalf("ScheduleEdit continuous: %v", err)
	}
	if got, _ := edit["mode"].(string); got != "continuous" {
		t.Fatalf("edited mode = %q want continuous", got)
	}
	if got := intOf(edit["interval_sec"]); got != 120 {
		t.Fatalf("edited interval_sec = %d want cooldown 120", got)
	}
}

func TestScheduleSystemTaskRejectsPayload(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "catalog_sync",
		"payload":      map[string]any{"instructions": "also run this"},
		"interval_sec": float64(86400),
	}); err == nil || !strings.Contains(err.Error(), "do not accept args.payload") {
		t.Fatalf("ScheduleAdd system task payload err = %v, want payload rejection", err)
	}
	list, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList: %v", err)
	}
	if got := len(list["schedules"].([]any)); got != 0 {
		t.Fatalf("system task payload rejection left %d schedule(s)", got)
	}

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "check queue",
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd intent target: %v", err)
	}
	id, _ := add["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":          id,
		"target":      "system_task",
		"system_task": "catalog_sync",
		"intent":      "changed label",
		"payload":     map[string]any{"instructions": "also run this"},
	}); err == nil || !strings.Contains(err.Error(), "do not accept args.payload") {
		t.Fatalf("ScheduleEdit system task payload err = %v, want payload rejection", err)
	}
	list, err = c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList after edit rejection: %v", err)
	}
	rows, _ := list["schedules"].([]any)
	if len(rows) != 1 {
		t.Fatalf("schedules after edit rejection = %d want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if row["target"] == "system_task" || row["system_task"] == "catalog_sync" {
		t.Fatalf("payload-rejected edit changed schedule target: %+v", row)
	}
	if row["intent"] != "check queue" {
		t.Fatalf("payload-rejected edit changed schedule intent: %+v", row)
	}
}

func TestScheduleEdit_InvalidBindingsAreAtomic(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "check queue",
		"model":        "base-model",
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd: %v", err)
	}
	id, _ := add["id"].(string)

	if _, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":     id,
		"intent": "changed by bad agent edit",
		"model":  "changed-model",
		"agent":  "missing-agent",
	}); err == nil || !strings.Contains(err.Error(), "unknown agent: missing-agent") {
		t.Fatalf("ScheduleEdit missing agent err = %v", err)
	}
	row := scheduleRowByID(t, c, ctx, id)
	if row["intent"] != "check queue" || row["model"] != "base-model" || row["agent"] != "" {
		t.Fatalf("missing-agent edit mutated schedule: %+v", row)
	}

	if _, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":     id,
		"intent": "changed by bad tool edit",
		"target": "tool",
		"tool":   "missing-tool",
	}); err == nil || !strings.Contains(err.Error(), "unknown tool: missing-tool") {
		t.Fatalf("ScheduleEdit missing tool err = %v", err)
	}
	row = scheduleRowByID(t, c, ctx, id)
	if row["intent"] != "check queue" || row["target"] == "tool" || row["tool"] == "missing-tool" {
		t.Fatalf("missing-tool edit mutated schedule: %+v", row)
	}
}

func TestScheduleEdit_InvalidCadenceIsAtomic(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "check queue",
		"model":        "base-model",
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd: %v", err)
	}
	id, _ := add["id"].(string)

	if _, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":           id,
		"intent":       "changed by bad cooldown edit",
		"model":        "changed-model",
		"cooldown_sec": float64(0),
	}); err == nil || !strings.Contains(err.Error(), "cooldown") {
		t.Fatalf("ScheduleEdit bad cooldown err = %v, want cooldown rejection", err)
	}
	row := scheduleRowByID(t, c, ctx, id)
	if row["intent"] != "check queue" || row["model"] != "base-model" || row["mode"] == "continuous" || intOf(row["interval_sec"]) != 3600 {
		t.Fatalf("bad cooldown edit mutated schedule: %+v", row)
	}

	if _, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":           id,
		"intent":       "changed by bad interval edit",
		"model":        "changed-again",
		"interval_sec": float64(0),
	}); err == nil || !strings.Contains(err.Error(), "interval") {
		t.Fatalf("ScheduleEdit bad interval err = %v, want interval rejection", err)
	}
	row = scheduleRowByID(t, c, ctx, id)
	if row["intent"] != "check queue" || row["model"] != "base-model" || intOf(row["interval_sec"]) != 3600 {
		t.Fatalf("bad interval edit mutated schedule: %+v", row)
	}
}

func TestScheduleSystemTasksMetadata(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	res, err := c.Call(ctx, controlplane.CmdScheduleSystemTasks, nil)
	if err != nil {
		t.Fatalf("ScheduleSystemTasks: %v", err)
	}
	raw, _ := res["system_tasks"].([]any)
	var got []string
	for _, v := range raw {
		got = append(got, v.(string))
	}
	if want := cadence.SystemTasks(); !reflect.DeepEqual(got, want) {
		t.Fatalf("system_tasks = %v want %v", got, want)
	}
	if intOf(res["count"]) != len(cadence.SystemTasks()) {
		t.Fatalf("count = %v want %d", res["count"], len(cadence.SystemTasks()))
	}
	infoRaw, _ := res["system_task_info"].([]any)
	if len(infoRaw) != len(cadence.SystemTasks()) {
		t.Fatalf("system_task_info len = %d want %d", len(infoRaw), len(cadence.SystemTasks()))
	}
	first, _ := infoRaw[0].(map[string]any)
	if first["name"] != cadence.SystemTaskCatalogSync || first["label"] == "" || first["description"] == "" {
		t.Fatalf("first system_task_info missing metadata: %+v", first)
	}
	if first["category"] != "catalog" || first["executor"] != "daemon" || first["effect_class"] != "config_update" || first["uses_llm"] != false {
		t.Fatalf("first system_task_info execution metadata wrong: %+v", first)
	}
	if effect, _ := first["effect"].(string); !strings.Contains(effect, "models.dev/api.json") || !strings.Contains(effect, "without waking an LLM agent") {
		t.Fatalf("first system_task_info effect should explain practical daemon work: %+v", first)
	}
	if got := intOf(first["recommended_interval_sec"]); got < 12*3600 {
		t.Fatalf("first system_task_info recommended_interval_sec = %d, want a quiet default", got)
	}
}

func TestScheduleListFrequencyWarnings(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "catalog_sync",
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd chatty system task: %v", err)
	}
	row := scheduleRowByID(t, c, ctx, add["id"].(string))
	if got, _ := row["frequency_warning"].(string); !strings.Contains(got, "recommended cadence") {
		t.Fatalf("system task frequency_warning = %q, want recommended cadence warning", got)
	}
	if row["executor"] != "daemon" || row["uses_llm"] != false || row["execution_contract"] != "cron runs daemon system task catalog_sync" {
		t.Fatalf("system task execution metadata wrong: %v", row)
	}
	if row["execution_authority"] != "daemon" || row["identity_owner"] != "none; daemon task owns no agent soul" ||
		row["payload_contract"] != "payload not accepted" || row["llm_boundary"] != "no LLM" {
		t.Fatalf("system task execution boundary wrong: %v", row)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "poll too often",
		"interval_sec": float64(60),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd chatty intent: %v", err)
	}
	row = scheduleRowByID(t, c, ctx, add["id"].(string))
	if got, _ := row["frequency_warning"].(string); got != "agent wake schedule is very frequent" {
		t.Fatalf("intent frequency_warning = %q, want agent wake warning", got)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "catalog_sync",
		"interval_sec": float64(86400),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd quiet system task: %v", err)
	}
	row = scheduleRowByID(t, c, ctx, add["id"].(string))
	if got, _ := row["frequency_warning"].(string); got != "" {
		t.Fatalf("quiet system task frequency_warning = %q, want none", got)
	}
}

func TestScheduleAdd_ToolTarget(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("AgentAdd: %v", err)
	}

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "tool",
		"tool":         "shell",
		"agent":        "ops",
		"model":        "gpt-5-mini",
		"payload":      map[string]any{"command": "echo scheduled"},
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd tool target: %v", err)
	}
	if got, _ := add["target"].(string); got != "tool" {
		t.Fatalf("target = %q want tool", got)
	}
	if got, _ := add["tool"].(string); got != "shell" {
		t.Fatalf("tool = %q want shell", got)
	}
	if got, _ := add["agent"].(string); got != "ops" {
		t.Fatalf("tool schedule agent = %q want ops", got)
	}
	if got, _ := add["model"].(string); got != "" {
		t.Fatalf("tool schedule model = %q want cleared", got)
	}
	if add["executor"] != "tool" || add["uses_llm"] != false || add["execution_contract"] != "cron invokes tool shell as ops" {
		t.Fatalf("tool schedule execution metadata wrong: %v", add)
	}
	if add["execution_authority"] != "agent ops" || add["identity_owner"] != "agent ops tool policy" ||
		add["payload_contract"] != "cron passes JSON tool payload" || add["llm_boundary"] != "no LLM; direct tool invocation" {
		t.Fatalf("tool schedule execution boundary wrong: %v", add)
	}

	res, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList: %v", err)
	}
	list, _ := res["schedules"].([]any)
	if len(list) != 1 {
		t.Fatalf("schedules = %d want 1", len(list))
	}
	row, _ := list[0].(map[string]any)
	if got, _ := row["target"].(string); got != "tool" {
		t.Errorf("listed target = %q want tool", got)
	}
	if got, _ := row["tool"].(string); got != "shell" {
		t.Errorf("listed tool = %q want shell", got)
	}
	if got, _ := row["agent"].(string); got != "ops" {
		t.Errorf("listed agent = %q want ops", got)
	}
	if got, _ := row["model"].(string); got != "" {
		t.Errorf("listed model = %q want cleared", got)
	}
	if row["executor"] != "tool" || row["uses_llm"] != false || row["execution_contract"] != "cron invokes tool shell as ops" {
		t.Errorf("listed execution metadata wrong: %v", row)
	}
	if row["execution_authority"] != "agent ops" || row["identity_owner"] != "agent ops tool policy" ||
		row["payload_contract"] != "cron passes JSON tool payload" || row["llm_boundary"] != "no LLM; direct tool invocation" {
		t.Fatalf("listed tool execution boundary wrong: %v", row)
	}
	payload, _ := row["payload"].(map[string]any)
	if got, _ := payload["command"].(string); got != "echo scheduled" {
		t.Errorf("listed payload command = %q want echo scheduled", got)
	}
}

func TestScheduleEdit_ToolTarget(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "old task",
		"interval_sec": float64(3600),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd: %v", err)
	}
	id, _ := add["id"].(string)
	edit, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":      id,
		"target":  "tool",
		"tool":    "shell",
		"model":   "gpt-5-mini",
		"payload": map[string]any{"command": "echo edited"},
	})
	if err != nil {
		t.Fatalf("ScheduleEdit tool target: %v", err)
	}
	if got, _ := edit["target"].(string); got != "tool" {
		t.Fatalf("target = %q want tool", got)
	}
	if got, _ := edit["tool"].(string); got != "shell" {
		t.Fatalf("tool = %q want shell", got)
	}
	if edit["executor"] != "tool" || edit["uses_llm"] != false || edit["execution_contract"] != "cron invokes tool shell under system identity" {
		t.Fatalf("edited tool schedule execution metadata wrong: %v", edit)
	}
	if edit["execution_authority"] != "system identity" || edit["identity_owner"] != "none; tool call owns no agent soul" ||
		edit["payload_contract"] != "cron passes JSON tool payload" || edit["llm_boundary"] != "no LLM; direct tool invocation" {
		t.Fatalf("edited tool execution boundary wrong: %v", edit)
	}
	if got, _ := edit["model"].(string); got != "" {
		t.Fatalf("model = %q want cleared", got)
	}
	payload, _ := edit["payload"].(map[string]any)
	if got, _ := payload["command"].(string); got != "echo edited" {
		t.Errorf("payload command = %q want echo edited", got)
	}
}

func TestScheduleAdd_ToolTargetValidation(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "tool",
		"interval_sec": float64(60),
	}); err == nil || !strings.Contains(err.Error(), "args.tool required") {
		t.Fatalf("missing tool err = %v, want args.tool required", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "tool",
		"tool":         "missing",
		"interval_sec": float64(60),
	}); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("unknown tool err = %v, want unknown tool", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "system_task",
		"system_task":  "catalog_sync",
		"agent":        "ops",
		"interval_sec": float64(60),
	}); err == nil || !strings.Contains(err.Error(), "system task schedules cannot also set args.agent") {
		t.Fatalf("system_task+agent err = %v, want cannot also set args.agent", err)
	}
}

func TestScheduleToolTargetRespectsAgentToolPolicy(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "tool_allow": []any{"memory"}, "tool_deny": []any{"shell"}},
	}); err != nil {
		t.Fatalf("AgentAdd ops: %v", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"target":       "tool",
		"tool":         "shell",
		"agent":        "ops",
		"payload":      map[string]any{"command": "echo no"},
		"interval_sec": float64(60),
	}); err == nil || !strings.Contains(err.Error(), "agent tool denylist") {
		t.Fatalf("ScheduleAdd denied tool err = %v, want agent tool denylist", err)
	}
	list, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList: %v", err)
	}
	if got := len(list["schedules"].([]any)); got != 0 {
		t.Fatalf("denied tool schedule add left %d schedule(s)", got)
	}

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "check queue",
		"agent":        "ops",
		"interval_sec": float64(60),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd intent: %v", err)
	}
	id, _ := add["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{
		"id":      id,
		"target":  "tool",
		"tool":    "shell",
		"intent":  "changed",
		"payload": map[string]any{"command": "echo no"},
	}); err == nil || !strings.Contains(err.Error(), "agent tool denylist") {
		t.Fatalf("ScheduleEdit denied tool err = %v, want agent tool denylist", err)
	}
	row := scheduleRowByID(t, c, ctx, id)
	if row["intent"] != "check queue" || row["target"] == "tool" || row["tool"] == "shell" {
		t.Fatalf("denied tool edit mutated schedule: %+v", row)
	}

	stale, err := k.Schedules().Add("stale denied tool", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("seed stale schedule: %v", err)
	}
	if ok, err := k.Schedules().SetAgent(stale.ID, "ops"); err != nil || !ok {
		t.Fatalf("seed stale agent ok=%v err=%v", ok, err)
	}
	if ok, err := k.Schedules().SetToolTarget(stale.ID, "shell", json.RawMessage(`{"command":"echo no"}`)); err != nil || !ok {
		t.Fatalf("seed stale tool ok=%v err=%v", ok, err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": stale.ID, "enabled": true}); err == nil || !strings.Contains(err.Error(), "agent tool denylist") {
		t.Fatalf("ScheduleEnable denied tool err = %v, want agent tool denylist", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleRun, map[string]any{"id": stale.ID}); err == nil || !strings.Contains(err.Error(), "agent tool denylist") {
		t.Fatalf("ScheduleRun denied tool err = %v, want agent tool denylist", err)
	}
}

// TestScheduleFires_FilterByScheduleID — `--id` (args.id) restricts the listing
// to one schedule's firings (M55).
func TestScheduleFires_FilterByScheduleID(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	fire := func(corr, schedID string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
			CorrelationID: corr, Payload: map[string]any{"schedule_id": schedID, "intent": "i"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": "i"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	fire("a1", "sched-A")
	fire("a2", "sched-A")
	fire("b1", "sched-B")

	res, err := c.Call(context.Background(), controlplane.CmdScheduleFires,
		map[string]any{"id": "sched-A"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	fires, _ := res["fires"].([]any)
	if len(fires) != 2 {
		t.Fatalf("fires = %d want 2 (only sched-A's firings)", len(fires))
	}
	for _, raw := range fires {
		row, _ := raw.(map[string]any)
		if got, _ := row["schedule_id"].(string); got != "sched-A" {
			t.Errorf("filtered row schedule_id = %q want sched-A", got)
		}
	}
}

// TestScheduleStats_AggregatesFirings — `agt schedule stats` counts firings by
// outcome, computes success rate, sums spend, and counts distinct schedules
// (M57). Two completed (one with spend) + one failed across two schedules.
func TestScheduleStats_AggregatesFirings(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	fire := func(corr, schedID string, fail bool, spendMC int64) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
			CorrelationID: corr, Payload: map[string]any{"schedule_id": schedID, "intent": "i"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: corr,
		})
		if spendMC > 0 {
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
				CorrelationID: corr, Payload: map[string]any{"cost_microcents": spendMC},
			})
		}
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
	fire("c1", "sched-A", false, 100)
	fire("c2", "sched-A", false, 50)
	fire("c3", "sched-B", true, 0)

	res, err := c.Call(context.Background(), controlplane.CmdScheduleStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["total"]); got != 3 {
		t.Errorf("total = %d want 3", got)
	}
	if got := intOf(res["completed"]); got != 2 {
		t.Errorf("completed = %d want 2", got)
	}
	if got := intOf(res["failed"]); got != 1 {
		t.Errorf("failed = %d want 1", got)
	}
	if got := intOf(res["schedules"]); got != 2 {
		t.Errorf("schedules = %d want 2", got)
	}
	if got := int64(intOf(res["spent_microcents"])); got != 150 {
		t.Errorf("spent_microcents = %d want 150", got)
	}
	if rate, _ := res["success_rate"].(float64); rate < 0.66 || rate > 0.67 {
		t.Errorf("success_rate = %v want ~0.667", rate)
	}
}

// TestScheduleStats_FilterByScheduleID — `--id` scopes the stats to one
// schedule's firings (M57).
func TestScheduleStats_FilterByScheduleID(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	for _, f := range []struct {
		corr, sched string
	}{{"a1", "sched-A"}, {"a2", "sched-A"}, {"b1", "sched-B"}} {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
			CorrelationID: f.corr, Payload: map[string]any{"schedule_id": f.sched, "intent": "i"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: f.corr,
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a", CorrelationID: f.corr,
			Payload: map[string]any{"iters": 1},
		})
	}
	res, err := c.Call(context.Background(), controlplane.CmdScheduleStats,
		map[string]any{"id": "sched-A"})
	if err != nil {
		t.Fatal(err)
	}
	if got := intOf(res["total"]); got != 2 {
		t.Errorf("total = %d want 2 (only sched-A)", got)
	}
	if got := intOf(res["schedules"]); got != 1 {
		t.Errorf("schedules = %d want 1", got)
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

func TestScheduleAddList_WithRosterAgent(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":         "researcher",
			"name":         "Researcher",
			"soul":         "Research and report from your own memory and tools.",
			"model":        "mock-model",
			"memory_scope": "researcher",
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "prepare the research brief", "interval_sec": 7200, "agent": "researcher",
	})
	if err != nil {
		t.Fatalf("schedule add with agent: %v", err)
	}
	if got, _ := res["agent"].(string); got != "researcher" {
		t.Fatalf("agent = %q, want researcher; result=%v", got, res)
	}
	id, _ := res["id"].(string)

	res, err = c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	list, _ := res["schedules"].([]any)
	if len(list) != 1 {
		t.Fatalf("list count = %d, want 1", len(list))
	}
	row, _ := list[0].(map[string]any)
	if row["id"] != id || row["agent"] != "researcher" {
		t.Fatalf("listed schedule should carry structured agent binding: %v", row)
	}
}

func TestScheduleAgentValidation(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "x", "interval_sec": 60, "agent": "missing",
	}); err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("unknown schedule agent err = %v, want unknown agent", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "ops", "enabled": false}); err != nil {
		t.Fatalf("pause agent: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "x", "interval_sec": 60, "agent": "ops",
	}); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("paused schedule agent err = %v, want paused", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "dead", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("dead agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "dead"}); err != nil {
		t.Fatalf("retire dead: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "x", "interval_sec": 60, "agent": "dead",
	}); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("retired schedule agent err = %v, want retired", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "worker", "parent_agent": "lead", "direct_callable": false},
	}); err != nil {
		t.Fatalf("worker add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "x", "interval_sec": 60, "agent": "worker",
	}); err == nil || !strings.Contains(err.Error(), "managed sub-agent") || !strings.Contains(err.Error(), "wake lead") {
		t.Fatalf("managed schedule agent err = %v, want managed sub-agent with manager hint", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "sleeper", "model": "mock-model"},
	}); err != nil {
		t.Fatalf("sleeper add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "x", "interval_sec": 60, "agent": "sleeper",
	})
	if err != nil {
		t.Fatalf("sleeper schedule add: %v", err)
	}
	id, _ := res["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": false}); err != nil {
		t.Fatalf("disable sleeper schedule: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "sleeper"}); err != nil {
		t.Fatalf("retire sleeper: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": true}); err == nil ||
		!strings.Contains(err.Error(), "retired") {
		t.Fatalf("enable schedule bound to retired agent err = %v, want retired", err)
	}

	res, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "x", "interval_sec": 60, "agent": "lead",
	})
	if err != nil {
		t.Fatalf("lead schedule add: %v", err)
	}
	leadID, _ := res["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdScheduleEdit, map[string]any{"id": leadID, "agent": "worker"}); err == nil ||
		!strings.Contains(err.Error(), "managed sub-agent") || !strings.Contains(err.Error(), "wake lead") {
		t.Fatalf("edit schedule to managed agent err = %v, want managed sub-agent with manager hint", err)
	}
	if _, err := k.Schedules().SetAgent(leadID, "worker"); err != nil {
		t.Fatalf("force corrupt schedule agent: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleRun, map[string]any{"id": leadID}); err == nil ||
		!strings.Contains(err.Error(), "managed sub-agent") || !strings.Contains(err.Error(), "wake lead") {
		t.Fatalf("run schedule bound to managed agent err = %v, want managed sub-agent with manager hint", err)
	}
}

func TestScheduleRunAndEnableValidateTypedTargetsStillExist(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "stale workflow", "interval_sec": 60,
	})
	if err != nil {
		t.Fatalf("add workflow schedule shell: %v", err)
	}
	workflowID, _ := add["id"].(string)
	if _, err := k.Schedules().SetWorkflowTarget(workflowID, "missing-flow", nil); err != nil {
		t.Fatalf("force stale workflow target: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleRun, map[string]any{"id": workflowID}); err == nil ||
		!strings.Contains(err.Error(), "unknown workflow: missing-flow") {
		t.Fatalf("run stale workflow err = %v, want unknown workflow", err)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "stale tool", "interval_sec": 60,
	})
	if err != nil {
		t.Fatalf("add tool schedule shell: %v", err)
	}
	toolID, _ := add["id"].(string)
	if _, err := k.Schedules().SetToolTarget(toolID, "missing-tool", nil); err != nil {
		t.Fatalf("force stale tool target: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": toolID, "enabled": false}); err != nil {
		t.Fatalf("disable stale tool schedule: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": toolID, "enabled": true}); err == nil ||
		!strings.Contains(err.Error(), "unknown tool: missing-tool") {
		t.Fatalf("enable stale tool err = %v, want unknown tool", err)
	}

	add, err = c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "stale system task", "interval_sec": 60,
	})
	if err != nil {
		t.Fatalf("add system task schedule shell: %v", err)
	}
	systemTaskID, _ := add["id"].(string)
	if _, err := k.Schedules().SetSystemTaskTarget(systemTaskID, "missing_system_task"); err != nil {
		t.Fatalf("force stale system task target: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleRun, map[string]any{"id": systemTaskID}); err == nil ||
		!strings.Contains(err.Error(), "unknown system task: missing_system_task") {
		t.Fatalf("run stale system task err = %v, want unknown system task", err)
	}

	list, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("list stale schedules: %v", err)
	}
	rows, _ := list["schedules"].([]any)
	blocked := map[string]string{}
	for _, item := range rows {
		row, _ := item.(map[string]any)
		id, _ := row["id"].(string)
		if id == "" {
			continue
		}
		if row["target_status"] == "blocked" {
			blocked[id], _ = row["target_error"].(string)
		}
	}
	for id, want := range map[string]string{
		workflowID:   "unknown workflow: missing-flow",
		toolID:       "unknown tool: missing-tool",
		systemTaskID: "unknown system task: missing_system_task",
	} {
		if got := blocked[id]; !strings.Contains(got, want) {
			t.Fatalf("schedule list target_error[%s] = %q, want %q", id, got, want)
		}
	}
}

func TestScheduleAddDailyAndPauseResume(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
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
	var actions []string
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "schedule.enable" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) == nil && pl["id"] == id {
			if action, _ := pl["action"].(string); action != "" {
				actions = append(actions, action)
			}
		}
		return nil
	})
	if !reflect.DeepEqual(actions, []string{"paused", "resumed"}) {
		t.Fatalf("schedule enable audit actions = %v, want [paused resumed]", actions)
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

// TestScheduleFires_SinceWindow — args.since_ms windows the firing history
// (M65): a huge window includes a just-published firing; a tiny window after a
// sleep excludes it.
func TestScheduleFires_SinceWindow(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "schedule.fired", Kind: event.KindScheduleFired, Actor: "schedule",
		CorrelationID: "f1", Payload: map[string]any{"schedule_id": "s", "intent": "i"},
	})
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a", CorrelationID: "f1",
	})

	res, err := c.Call(context.Background(), controlplane.CmdScheduleFires,
		map[string]any{"since_ms": int64(3_600_000)})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := res["fires"].([]any); len(got) != 1 {
		t.Errorf("1h window fires = %d want 1", len(got))
	}

	time.Sleep(5 * time.Millisecond)
	res, err = c.Call(context.Background(), controlplane.CmdScheduleFires,
		map[string]any{"since_ms": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := res["fires"].([]any); len(got) != 0 {
		t.Errorf("1ms window fires = %d want 0", len(got))
	}
}

// TestScheduleFires_IntentFilter — `agt schedule fires --intent` keeps only
// firings whose intent contains the substring, case-insensitively (M80).
func TestScheduleFires_IntentFilter(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	fire := func(schedID, intent string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "schedule", Kind: event.KindScheduleFired, Actor: "cadence",
			CorrelationID: "run-" + schedID,
			Payload:       map[string]any{"schedule_id": schedID, "intent": intent, "model": "m"},
		})
	}
	fire("s1", "nightly DEPLOY")
	fire("s2", "hourly summary")
	fire("s3", "deploy canary")

	res, err := c.Call(context.Background(), controlplane.CmdScheduleFires,
		map[string]any{"intent": "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	fires, _ := res["fires"].([]any)
	if len(fires) != 2 {
		t.Fatalf("--intent deploy = %d want 2 (case-insensitive)", len(fires))
	}
}

func scheduleRowByID(t *testing.T, c *controlplane.Client, ctx context.Context, id string) map[string]any {
	t.Helper()
	list, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("ScheduleList: %v", err)
	}
	rows, _ := list["schedules"].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["id"] == id {
			return row
		}
	}
	t.Fatalf("schedule %q not found in %+v", id, rows)
	return nil
}

// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestAgent_CRUDRoundTrip drives the agent-roster management surface over the
// control plane: add → list → edit → pause → remove (by slug), asserting each
// step and that every mutation is journaled (roster.created/updated/removed).
func TestAgent_CRUDRoundTrip(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":             "researcher",
			"soul":             "You research deeply and cite sources.",
			"model":            "mock-model",
			"config_overrides": map[string]any{"AGEZT_X_MODE": "agent"},
		},
	})
	if err != nil {
		t.Fatalf("agent add: %v", err)
	}
	prof, _ := add["profile"].(map[string]any)
	if prof == nil || prof["slug"] != "researcher" || prof["enabled"] != true {
		t.Fatalf("add returned %v", add)
	}
	if cfg, _ := prof["config_overrides"].(map[string]any); cfg["AGEZT_X_MODE"] != "agent" {
		t.Fatalf("config overrides missing from add: %v", prof["config_overrides"])
	}
	if prof["name"] != "researcher" {
		t.Errorf("name should default to slug, got %v", prof["name"])
	}

	// A duplicate slug is refused.
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "researcher"},
	}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate slug: err = %v, want already-exists", err)
	}

	list, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	if n, _ := list["count"].(float64); n != 1 {
		t.Fatalf("count = %v, want 1", list["count"])
	}

	// Edit by slug: the soul changes, the slug cannot.
	edit, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{
		"ref": "researcher",
		"profile": map[string]any{
			"slug": "hijacked", "name": "The Researcher", "soul": "v2 soul", "model": "mock-model",
			"config_overrides": map[string]any{"AGEZT_X_MODE": "edited"},
		},
	})
	if err != nil {
		t.Fatalf("agent edit: %v", err)
	}
	ep, _ := edit["profile"].(map[string]any)
	if ep["slug"] != "researcher" || ep["soul"] != "v2 soul" || ep["name"] != "The Researcher" {
		t.Fatalf("edit result wrong: %v", ep)
	}
	if cfg, _ := ep["config_overrides"].(map[string]any); cfg["AGEZT_X_MODE"] != "edited" {
		t.Fatalf("config overrides missing from edit: %v", ep["config_overrides"])
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{
		"ref": "ghost", "profile": map[string]any{},
	}); err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("edit unknown: err = %v", err)
	}

	// Pause by slug; the webui string transport ("false") must also work.
	pause, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "researcher", "enabled": "false"})
	if err != nil {
		t.Fatalf("agent pause: %v", err)
	}
	if pp, _ := pause["profile"].(map[string]any); pp["enabled"] != false {
		t.Fatalf("pause result wrong: %v", pause)
	}

	rm, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{"ref": "researcher"})
	if err != nil {
		t.Fatalf("agent remove: %v", err)
	}
	if removed, _ := rm["removed"].(bool); !removed {
		t.Error("remove should report removed=true")
	}

	// Every mutation is journaled.
	for _, want := range []event.Kind{event.KindRosterCreated, event.KindRosterUpdated, event.KindRosterRemoved} {
		n := 0
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Kind == want {
				n++
			}
			return nil
		})
		if n == 0 {
			t.Errorf("no %s event journaled", want)
		}
	}
}

func TestAgentCapabilitiesPatch_PreservesIdentityAndTasks(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug": "lead",
			"soul": "Lead the builders.",
		},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":            "builder",
			"name":            "Builder",
			"soul":            "Build carefully.",
			"instructions":    []any{"ship cautiously"},
			"model":           "gpt-5",
			"fallbacks":       []any{"gpt-4.1"},
			"task_type":       "code",
			"owner_agent":     "lead",
			"parent_agent":    "lead",
			"direct_callable": false,
			"tool_allow":      []any{"memory"},
			"tool_deny":       []any{"notify"},
			"trust_ceiling":   "L2",
			"lifecycle":       map[string]any{"mode": "cycle", "max_cycles": 5, "completed_cycles": 2},
			"retry_policy":    map[string]any{"max_attempts": 3, "backoff": "exponential"},
			"health_policy":   map[string]any{"doctor_agent": "lead", "failure_threshold": 2},
			"self_repair":     map[string]any{"enabled": true, "max_attempts": 2, "escalate_to": "lead"},
			"tasklist": []any{
				map[string]any{"id": "task-1", "title": "ship capability center", "scope": "total", "status": "doing"},
			},
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	res, err := c.Call(ctx, controlplane.CmdAgentCapabilities, map[string]any{
		"ref":           "builder",
		"trust_ceiling": "L3",
		"memory_scope":  "agents/builder",
		"max_cost_mc":   int64(50_000_000),
		"max_daily_mc":  int64(100_000_000),
		"tool_allow":    []any{"memory", "shell"},
		"tool_deny":     []any{"browser"},
		"noise_policy": map[string]any{
			"silent_on_success":       true,
			"disable_memory_writes":   true,
			"min_notify_severity":     "warning",
			"min_notify_interval_sec": 3600,
		},
		"config_overrides": map[string]any{"AGEZT_PROVIDER": "openai"},
	})
	if err != nil {
		t.Fatalf("agent capabilities: %v", err)
	}
	prof, _ := res["profile"].(map[string]any)
	if prof["slug"] != "builder" || prof["soul"] != "Build carefully." || prof["model"] != "gpt-5" || prof["task_type"] != "code" {
		t.Fatalf("identity fields were not preserved: %v", prof)
	}
	if prof["owner_agent"] != "lead" || prof["parent_agent"] != "lead" || prof["direct_callable"] != false {
		t.Fatalf("hierarchy fields were not preserved: %v", prof)
	}
	instructions, _ := prof["instructions"].([]any)
	if len(instructions) != 1 || instructions[0] != "ship cautiously" {
		t.Fatalf("instructions were not preserved: %v", prof["instructions"])
	}
	lifecycle, _ := prof["lifecycle"].(map[string]any)
	if lifecycle["mode"] != "cycle" || lifecycle["max_cycles"] != float64(5) || lifecycle["completed_cycles"] != float64(2) {
		t.Fatalf("lifecycle was not preserved: %v", lifecycle)
	}
	retry, _ := prof["retry_policy"].(map[string]any)
	if retry["max_attempts"] != float64(3) || retry["backoff"] != "exponential" {
		t.Fatalf("retry policy was not preserved: %v", retry)
	}
	health, _ := prof["health_policy"].(map[string]any)
	if health["doctor_agent"] != "lead" || health["failure_threshold"] != float64(2) {
		t.Fatalf("health policy was not preserved: %v", health)
	}
	selfRepair, _ := prof["self_repair"].(map[string]any)
	if selfRepair["enabled"] != true || selfRepair["max_attempts"] != float64(2) || selfRepair["escalate_to"] != "lead" {
		t.Fatalf("self-repair policy was not preserved: %v", selfRepair)
	}
	if got, _ := prof["trust_ceiling"].(string); got != "L3" {
		t.Fatalf("trust ceiling = %q want L3", got)
	}
	if prof["memory_scope"] != "agents/builder" || prof["max_cost_mc"] != float64(50_000_000) || prof["max_daily_mc"] != float64(100_000_000) {
		t.Fatalf("resource governance fields missing from capabilities patch: %v", prof)
	}
	tasks, _ := prof["tasklist"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("tasklist rewritten: %v", prof["tasklist"])
	}
	noise, _ := prof["noise_policy"].(map[string]any)
	if noise["min_notify_severity"] != "warning" {
		t.Fatalf("noise policy missing: %v", noise)
	}
	cfg, _ := prof["config_overrides"].(map[string]any)
	if cfg["AGEZT_PROVIDER"] != "openai" {
		t.Fatalf("config overrides missing: %v", cfg)
	}
	allow, _ := res["tool_allow"].([]any)
	deny, _ := res["tool_deny"].([]any)
	if len(allow) != 1 || allow[0] != "shell" || len(deny) != 2 || deny[0] != "browser" || deny[1] != "memory" {
		t.Fatalf("permission snapshot missing allow/deny: allow=%v deny=%v", allow, deny)
	}
}

func TestManagedSubAgent_NotDirectlyCallable(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":            "lead",
			"soul":            "Lead the work.",
			"model":           "mock-model",
			"direct_callable": true,
		},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":            "worker",
			"soul":            "Do focused worker tasks.",
			"model":           "mock-model",
			"parent_agent":    "lead",
			"direct_callable": false,
			"retry_policy": map[string]any{
				"max_attempts":   3,
				"backoff":        "exponential",
				"base_delay_sec": 1,
			},
			"health_policy": map[string]any{
				"doctor_agent":      "lead",
				"failure_threshold": 5,
			},
			"self_repair": map[string]any{
				"enabled":      true,
				"max_attempts": 2,
				"escalate_to":  "lead",
			},
		},
	}); err != nil {
		t.Fatalf("worker add: %v", err)
	}

	list, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	profiles, _ := list["profiles"].([]any)
	var worker map[string]any
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if p["slug"] == "worker" {
			worker = p
		}
	}
	if worker == nil || worker["parent_agent"] != "lead" || worker["direct_callable"] != false {
		t.Fatalf("worker hierarchy fields not listed: %v", worker)
	}
	if worker["kind"] != "subagent" || worker["managed"] != true {
		t.Fatalf("worker identity class not listed: %v", worker)
	}
	if retry, _ := worker["retry_policy"].(map[string]any); intOf(retry["max_attempts"]) != 3 {
		t.Fatalf("worker retry_policy not listed: %v", worker["retry_policy"])
	}
	if health, _ := worker["health_policy"].(map[string]any); health["doctor_agent"] != "lead" {
		t.Fatalf("worker health_policy not listed: %v", worker["health_policy"])
	}
	if repair, _ := worker["self_repair"].(map[string]any); repair["enabled"] != true {
		t.Fatalf("worker self_repair not listed: %v", worker["self_repair"])
	}

	if _, err := c.Call(ctx, controlplane.CmdRun, map[string]any{
		"intent": "x", "agent": "worker", "dry_run": true,
	}); err == nil || !strings.Contains(err.Error(), "cannot be called directly") {
		t.Fatalf("direct run err = %v, want cannot be called directly", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "x", "interval_sec": 60, "agent": "worker",
	}); err == nil || !strings.Contains(err.Error(), "cannot be scheduled directly") {
		t.Fatalf("schedule worker err = %v, want cannot be scheduled directly", err)
	}
}

func TestAgentHierarchyReferencesRequireLiveAgents(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":            "ghost-worker",
			"parent_agent":    "ghost",
			"direct_callable": false,
		},
	}); err == nil || !strings.Contains(err.Error(), `parent_agent "ghost" does not exist`) {
		t.Fatalf("ghost parent add err = %v", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "Lead.", "model": "m"},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "old-lead", "soul": "Retired lead.", "model": "m"},
	}); err != nil {
		t.Fatalf("old lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "old-lead"}); err != nil {
		t.Fatalf("old lead retire: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":            "old-worker",
			"parent_agent":    "old-lead",
			"direct_callable": false,
		},
	}); err == nil || !strings.Contains(err.Error(), `parent_agent "old-lead" is retired`) {
		t.Fatalf("retired parent add err = %v", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":            "worker",
			"parent_agent":    "lead",
			"direct_callable": false,
		},
	}); err != nil {
		t.Fatalf("worker add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{
		"ref": "worker",
		"profile": map[string]any{
			"slug":            "ignored",
			"parent_agent":    "ghost",
			"direct_callable": false,
		},
	}); err == nil || !strings.Contains(err.Error(), `parent_agent "ghost" does not exist`) {
		t.Fatalf("ghost parent edit err = %v", err)
	}
	if worker, ok := k.Roster().Get("worker"); !ok || worker.ParentAgent != "lead" {
		t.Fatalf("worker parent mutated after failed edit: ok=%v profile=%+v", ok, worker)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{
		"ref":     "lead",
		"cascade": map[string]any{"subagents": true},
	}); err != nil {
		t.Fatalf("lead remove: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRevive, map[string]any{"ref": "worker"}); err == nil || !strings.Contains(err.Error(), `parent_agent "lead" does not exist`) {
		t.Fatalf("orphan revive err = %v", err)
	}
}

func TestAgentRemove_SubagentCascadeRetiresAndPausesChildTriggers(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "Lead."},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "worker", "parent_agent": "lead", "direct_callable": false},
	}); err != nil {
		t.Fatalf("worker add: %v", err)
	}
	workerStanding, err := k.AddStanding(standing.Order{
		Name:     "worker inbox",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "board.dm.worker"}},
		Agent:    "worker",
		Plan:     "handle worker inbox",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("worker standing add: %v", err)
	}
	workerSchedule, err := k.Schedules().Add("worker cycle", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("worker schedule add: %v", err)
	}
	if ok, err := k.Schedules().SetAgent(workerSchedule.ID, "worker"); err != nil || !ok {
		t.Fatalf("worker schedule set agent ok=%v err=%v", ok, err)
	}

	rm, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{
		"ref":     "lead",
		"cascade": map[string]any{"subagents": true},
	})
	if err != nil {
		t.Fatalf("lead remove: %v", err)
	}
	if rm["removed"] != true || intOf(rm["subagents_retired"]) != 1 || intOf(rm["standing_removed"]) != 0 || intOf(rm["schedules_removed"]) != 0 {
		t.Fatalf("remove result should retire subagent without deleting triggers: %v", rm)
	}
	if got := anyStrings(rm["subagents_retired_slugs"]); !reflect.DeepEqual(got, []string{"worker"}) {
		t.Fatalf("retired subagent slugs = %v, want [worker]", got)
	}
	worker, ok := k.Roster().Get("worker")
	if !ok || !worker.Retired {
		t.Fatalf("worker should remain as retired dependent sub-agent: ok=%v profile=%+v", ok, worker)
	}
	for _, o := range k.Standing().List() {
		if o.ID == workerStanding.ID && o.Enabled {
			t.Fatalf("retired subagent standing trigger stayed enabled: %+v", o)
		}
	}
	for _, e := range k.Schedules().List() {
		if e.ID == workerSchedule.ID && e.Enabled {
			t.Fatalf("retired subagent schedule stayed enabled: %+v", e)
		}
	}
}

func TestAgentTaskUpdate_AddUpdateRemove(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Run operations."},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	add, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, map[string]any{
		"ref":    "ops",
		"op":     "add",
		"title":  "check queue",
		"scope":  "cycle",
		"status": "todo",
	})
	if err != nil {
		t.Fatalf("task add: %v", err)
	}
	task, _ := add["task"].(map[string]any)
	id, _ := task["id"].(string)
	if id == "" || task["title"] != "check queue" || task["scope"] != "cycle" {
		t.Fatalf("task add result wrong: %v", task)
	}

	upd, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, map[string]any{
		"ref":    "ops",
		"id":     id,
		"status": "done",
	})
	if err != nil {
		t.Fatalf("task update: %v", err)
	}
	task, _ = upd["task"].(map[string]any)
	if task["status"] != "done" {
		t.Fatalf("task status = %v want done", task["status"])
	}
	prof, _ := upd["profile"].(map[string]any)
	list, _ := prof["tasklist"].([]any)
	if len(list) != 1 {
		t.Fatalf("profile tasklist len = %d want 1", len(list))
	}

	rm, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, map[string]any{
		"ref": "ops",
		"op":  "remove",
		"id":  id,
	})
	if err != nil {
		t.Fatalf("task remove: %v", err)
	}
	prof, _ = rm["profile"].(map[string]any)
	if list, _ := prof["tasklist"].([]any); len(list) != 0 {
		t.Fatalf("tasklist after remove = %v want empty", list)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, map[string]any{
		"ref": "ops", "id": "missing", "status": "done",
	}); err == nil || !strings.Contains(err.Error(), "unknown agent task") {
		t.Fatalf("missing task err = %v, want unknown agent task", err)
	}

	updates := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindRosterUpdated {
			updates++
		}
		return nil
	})
	if updates < 3 {
		t.Fatalf("roster.updated events = %d want at least 3 task mutations", updates)
	}
}

func TestAgentTaskUpdate_RejectsInvalidTaskMutations(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Run operations."},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, map[string]any{
		"ref": "ops",
		"op":  "add",
	}); err == nil || !strings.Contains(err.Error(), "args.title required") {
		t.Fatalf("blank task add err = %v, want title required", err)
	}
	add, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, map[string]any{
		"ref":   "ops",
		"op":    "add",
		"title": "check queue",
	})
	if err != nil {
		t.Fatalf("task add: %v", err)
	}
	task, _ := add["task"].(map[string]any)
	id, _ := task["id"].(string)
	if id == "" {
		t.Fatalf("task add result missing id: %v", task)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "blank title update",
			args: map[string]any{"ref": "ops", "id": id, "title": ""},
			want: "args.title required",
		},
		{
			name: "blank nested title update",
			args: map[string]any{"ref": "ops", "task": map[string]any{"id": id, "title": ""}},
			want: "args.title required",
		},
		{
			name: "invalid scope",
			args: map[string]any{"ref": "ops", "id": id, "scope": "daily"},
			want: "args.scope must be cycle or total",
		},
		{
			name: "invalid status",
			args: map[string]any{"ref": "ops", "id": id, "status": "lost"},
			want: "args.status must be todo, doing, done, blocked, or retired",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("task update err = %v, want %q", err, tc.want)
			}
		})
	}
	prof, ok := k.Roster().Get("ops")
	if !ok || len(prof.TaskList) != 1 {
		t.Fatalf("profile tasklist after invalid mutations = ok:%v list:%v", ok, prof.TaskList)
	}
	if prof.TaskList[0].ID != id || prof.TaskList[0].Title != "check queue" {
		t.Fatalf("profile task changed after invalid mutations: %+v", prof.TaskList[0])
	}
}

func TestAgentList_IncludesDynamicHealthAndRepairStatus(t *testing.T) {
	k, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	st, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	srv.SetBoard(st, nil)
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":             "builder",
			"model":            "mock-model",
			"task_type":        "code",
			"config_overrides": map[string]any{"AGEZT_MAX_ITER": "abc"},
			"self_repair":      map[string]any{"enabled": true},
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"phase":            "queued",
			"agent":            "builder",
			"fingerprint":      "fp-1",
			"reason":           "invalid runtime override",
			"issues":           []string{"AGEZT_MAX_ITER: must be an integer"},
			"incident_id":      "inc-builder-1",
			"root_incident_id": "inc-root-1",
			"root_agent":       "builder",
			"chain_depth":      0,
		},
	}); err != nil {
		t.Fatalf("publish repair event: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "governor.fallback",
		Kind:    event.KindProviderFallback,
		Actor:   "governor",
		Payload: map[string]any{
			"failed_model": "mock-model",
			"next_model":   "mock-fallback",
			"reason":       "provider timeout",
			"scope":        "model-chain",
			"task_type":    "code",
		},
	}); err != nil {
		t.Fatalf("publish routing fallback: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "agent.builder.retry",
		Kind:    event.KindAgentRetry,
		Actor:   "agent-retry",
		Payload: map[string]any{
			"agent":        "builder",
			"next_attempt": 2,
			"max_attempts": 3,
			"reason":       "provider timeout",
		},
	}); err != nil {
		t.Fatalf("publish agent retry: %v", err)
	}
	if _, err := st.HelpRequest("guardian-doctor", "builder", "needs ownership", time.Now().UnixMilli()); err != nil {
		t.Fatalf("help request: %v", err)
	}

	list, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	profiles, _ := list["profiles"].([]any)
	var builder map[string]any
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if p["slug"] == "builder" {
			builder = p
		}
	}
	if builder == nil {
		t.Fatal("builder not found in list")
	}
	status, _ := builder["status"].(map[string]any)
	if status["health_state"] != "misconfigured" || status["repair_state"] != "queued" {
		t.Fatalf("dynamic status missing: %+v", status)
	}
	if intOf(status["invalid_runtime_overrides"]) != 1 || intOf(status["repair_inflight"]) != 1 {
		t.Fatalf("dynamic counts wrong: %+v", status)
	}
	issues, _ := status["config_issues"].([]any)
	if len(issues) != 1 || !strings.Contains(issues[0].(string), "AGEZT_MAX_ITER") {
		t.Fatalf("config issues missing: %+v", status)
	}
	if status["repair_incident_id"] != "inc-builder-1" || status["repair_root_incident_id"] != "inc-root-1" || status["repair_root_agent"] != "builder" {
		t.Fatalf("repair incident lineage missing: %+v", status)
	}
	if intOf(status["routing_fallback_count"]) != 1 || status["routing_last_failed"] != "mock-model" || status["routing_last_next"] != "mock-fallback" {
		t.Fatalf("routing pressure missing: %+v", status)
	}
	if status["routing_last_reason"] != "provider timeout" {
		t.Fatalf("routing reason missing: %+v", status)
	}
	if intOf(status["retry_count"]) != 1 || status["retry_last_reason"] != "provider timeout" ||
		intOf(status["retry_next_attempt"]) != 2 || intOf(status["retry_max_attempts"]) != 3 {
		t.Fatalf("retry pressure missing: %+v", status)
	}
	if intOf(status["escalation_open_count"]) != 1 || intOf(status["escalation_acked_count"]) != 0 {
		t.Fatalf("escalation load missing: %+v", status)
	}
}

func TestAgentRemove_CascadeDeletesPrivateOwnedResourcesOnly(t *testing.T) {
	k, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	st, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	srv.SetBoard(st, nil)

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "researcher", "memory_scope": "agent/researcher", "workdir": "researcher"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "research-worker", "kind": "subagent", "owner_agent": "researcher", "parent_agent": "researcher", "direct_callable": false, "workdir": "researcher/worker"},
	}); err != nil {
		t.Fatalf("subagent add: %v", err)
	}
	workspaceRoot := filepath.Join(k.BaseDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "researcher", "worker"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "researcher", "notes.md"), []byte("parent notes"), 0o644); err != nil {
		t.Fatalf("parent workspace file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "researcher", "worker", "task.md"), []byte("worker notes"), 0o644); err != nil {
		t.Fatalf("worker workspace file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "shared"), 0o755); err != nil {
		t.Fatalf("mkdir shared workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "shared", "keep.md"), []byte("shared"), 0o644); err != nil {
		t.Fatalf("shared workspace file: %v", err)
	}
	if _, err := k.AddStanding(standing.Order{
		Name:     "Research standing",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "x.y"}},
		Agent:    "researcher",
		Plan:     "do research",
	}); err != nil {
		t.Fatalf("standing add: %v", err)
	}
	if _, err := k.AddStanding(standing.Order{
		Name:     "Worker standing",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "worker.y"}},
		Agent:    "research-worker",
		Plan:     "do worker research",
	}); err != nil {
		t.Fatalf("worker standing add: %v", err)
	}
	sched, err := k.Schedules().Add("refresh", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("schedule add: %v", err)
	}
	if ok, err := k.Schedules().SetAgent(sched.ID, "researcher"); err != nil || !ok {
		t.Fatalf("schedule set agent ok=%v err=%v", ok, err)
	}
	workerSched, err := k.Schedules().Add("worker refresh", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("worker schedule add: %v", err)
	}
	if ok, err := k.Schedules().SetAgent(workerSched.ID, "research-worker"); err != nil || !ok {
		t.Fatalf("worker schedule set agent ok=%v err=%v", ok, err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "private", Content: "researcher private durable fact", Tags: map[string]string{"scope": "agent/researcher"},
	}); err != nil {
		t.Fatalf("private memory: %v", err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "worker private", Content: "worker private durable fact", Tags: map[string]string{"scope": "research-worker"},
	}); err != nil {
		t.Fatalf("worker private memory: %v", err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "shared", Content: "shared durable fact about project",
	}); err != nil {
		t.Fatalf("shared memory: %v", err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "authored shared", Content: "researcher wrote this shared durable fact", Actor: "researcher",
	}); err != nil {
		t.Fatalf("authored shared memory: %v", err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "worker authored shared", Content: "worker wrote this shared durable fact", Actor: "research-worker",
	}); err != nil {
		t.Fatalf("worker authored shared memory: %v", err)
	}
	privateSkill, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name: "research-private", Body: "Do private research workflow.", Agent: "researcher",
	})
	if err != nil {
		t.Fatalf("private skill: %v", err)
	}
	workerSkill, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name: "worker-private", Body: "Do worker workflow.", Agent: "research-worker",
	})
	if err != nil {
		t.Fatalf("worker private skill: %v", err)
	}
	sharedSkill, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name: "shared-skill", Body: "Do shared workflow.",
	})
	if err != nil {
		t.Fatalf("shared skill: %v", err)
	}
	if err := k.ConfigCenter().Set(&configcenter.ConfigEntry{
		Key:       "agent/researcher/runtime",
		Value:     "mode=private",
		Rating:    configcenter.RatingInternal,
		CreatedBy: "researcher",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Metadata:  map[string]string{"agent": "researcher"},
	}); err != nil {
		t.Fatalf("private config: %v", err)
	}
	if err := k.ConfigCenter().Set(&configcenter.ConfigEntry{
		Key:       "agent/research-worker/runtime",
		Value:     "mode=worker",
		Rating:    configcenter.RatingInternal,
		CreatedBy: "research-worker",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Metadata:  map[string]string{"agent": "research-worker"},
	}); err != nil {
		t.Fatalf("worker private config: %v", err)
	}
	if err := k.ConfigCenter().Set(&configcenter.ConfigEntry{
		Key:            "shared/api-key",
		Value:          "shared",
		Rating:         configcenter.RatingRestricted,
		AllowedAgents:  []string{"researcher", "ops"},
		ExcludedAgents: []string{"research-worker", "blocked"},
		CreatedAt:      time.Now().Unix(),
		UpdatedAt:      time.Now().Unix(),
	}); err != nil {
		t.Fatalf("shared config: %v", err)
	}
	if _, _, err := k.SaveWorkflow("", workflow.Workflow{
		Name: "research-agent-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "handoff", Type: workflow.NodeTool, Label: "handoff researcher", Config: json.RawMessage(`{"tool":"agent","args":{"agent":"researcher","intent":"review findings"}}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "handoff"}},
	}); err != nil {
		t.Fatalf("parent workflow: %v", err)
	}
	if _, _, err := k.SaveWorkflow("", workflow.Workflow{
		Name: "worker-agent-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "delegate", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"board","args":{"target_agent":"research-worker","text":"wake worker"}}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "delegate"}},
	}); err != nil {
		t.Fatalf("worker workflow: %v", err)
	}
	if _, err := st.Send(board.Message{Topic: "dm", From: "operator", To: "researcher", Text: "private ask"}, 1000); err != nil {
		t.Fatalf("board dm: %v", err)
	}
	if _, err := st.Send(board.Message{Topic: "dm", From: "researcher", To: "operator", Text: "private reply"}, 1001); err != nil {
		t.Fatalf("board sent: %v", err)
	}
	if _, err := st.Broadcast("ops", "broadcast ask", 1002); err != nil {
		t.Fatalf("board broadcast: %v", err)
	}
	if _, err := st.Send(board.Message{Topic: "dm", From: "operator", To: "research-worker", Text: "worker ask"}, 1003); err != nil {
		t.Fatalf("board worker dm: %v", err)
	}

	impact, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": "researcher"})
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if intOf(impact["standing_count"]) != 1 || intOf(impact["schedule_count"]) != 1 ||
		intOf(impact["memory_count"]) != 1 || intOf(impact["skill_count"]) != 1 ||
		intOf(impact["config_count"]) != 1 || intOf(impact["workspace_count"]) != 1 ||
		intOf(impact["subagent_count"]) != 1 {
		t.Fatalf("impact counts wrong: %v", impact)
	}
	if intOf(impact["authored_shared_memory_count"]) != 1 {
		t.Fatalf("authored shared memory impact count wrong: %v", impact)
	}
	if intOf(impact["workflow_ref_count"]) != 1 {
		t.Fatalf("workflow reference impact count wrong: %v", impact)
	}
	if got := strings.Join(anyStrings(impact["workflow_refs"]), "\n"); !strings.Contains(got, "research-agent-flow/handoff") || !strings.Contains(got, "handoff researcher") {
		t.Fatalf("workflow reference impact labels wrong: %v", impact["workflow_refs"])
	}
	if intOf(impact["mailbox_message_count"]) != 3 {
		t.Fatalf("mailbox history impact count wrong: %v", impact)
	}
	mailboxImpact, _ := impact["mailbox_messages"].([]any)
	joinedMailboxImpact := strings.Join(anyStrings(mailboxImpact), "\n")
	if !strings.Contains(joinedMailboxImpact, "dm received") ||
		!strings.Contains(joinedMailboxImpact, "dm sent") ||
		!strings.Contains(joinedMailboxImpact, "broadcast broadcast") {
		t.Fatalf("mailbox history impact labels wrong: %v", mailboxImpact)
	}
	if intOf(impact["subagent_standing_count"]) != 1 || intOf(impact["subagent_schedule_count"]) != 1 {
		t.Fatalf("subagent impact counts wrong: %v", impact)
	}
	if intOf(impact["subagent_memory_count"]) != 1 || intOf(impact["subagent_skill_count"]) != 1 ||
		intOf(impact["subagent_config_count"]) != 1 || intOf(impact["subagent_workspace_count"]) != 1 {
		t.Fatalf("subagent private impact counts wrong: %v", impact)
	}
	if intOf(impact["subagent_authored_shared_memory_count"]) != 1 {
		t.Fatalf("subagent authored shared memory impact count wrong: %v", impact)
	}
	if intOf(impact["subagent_workflow_ref_count"]) != 1 {
		t.Fatalf("subagent workflow reference impact count wrong: %v", impact)
	}
	if got := strings.Join(anyStrings(impact["subagent_workflow_refs"]), "\n"); !strings.Contains(got, "research-worker: worker-agent-flow/delegate") {
		t.Fatalf("subagent workflow reference impact labels wrong: %v", impact["subagent_workflow_refs"])
	}
	if intOf(impact["subagent_mailbox_message_count"]) != 2 {
		t.Fatalf("subagent mailbox/audit impact count wrong: %v", impact)
	}
	ret, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "researcher", "reason": "cleanup drill"})
	if err != nil {
		t.Fatalf("agent retire with impact: %v", err)
	}
	summary, _ := ret["impact_summary"].(map[string]any)
	if intOf(summary["standing_count"]) != 1 || intOf(summary["schedule_count"]) != 1 ||
		intOf(summary["memory_count"]) != 1 || intOf(summary["skill_count"]) != 1 ||
		intOf(summary["config_count"]) != 1 || intOf(summary["workspace_count"]) != 1 ||
		intOf(summary["subagent_count"]) != 1 {
		t.Fatalf("retire impact summary wrong: %v", ret)
	}
	if intOf(summary["authored_shared_memory_count"]) != 1 {
		t.Fatalf("retire authored shared memory impact summary wrong: %v", ret)
	}
	if intOf(summary["workflow_ref_count"]) != 1 {
		t.Fatalf("retire workflow reference impact summary wrong: %v", ret)
	}
	if intOf(summary["subagent_standing_count"]) != 1 || intOf(summary["subagent_schedule_count"]) != 1 {
		t.Fatalf("retire subagent impact summary wrong: %v", ret)
	}
	if intOf(summary["subagent_memory_count"]) != 1 || intOf(summary["subagent_skill_count"]) != 1 ||
		intOf(summary["subagent_config_count"]) != 1 || intOf(summary["subagent_workspace_count"]) != 1 {
		t.Fatalf("retire subagent private impact summary wrong: %v", ret)
	}
	if intOf(summary["subagent_authored_shared_memory_count"]) != 1 {
		t.Fatalf("retire subagent authored shared memory impact summary wrong: %v", ret)
	}
	if intOf(summary["subagent_workflow_ref_count"]) != 1 {
		t.Fatalf("retire subagent workflow reference impact summary wrong: %v", ret)
	}
	if intOf(summary["subagent_mailbox_message_count"]) != 2 {
		t.Fatalf("retire subagent mailbox/audit impact summary wrong: %v", ret)
	}
	if intOf(ret["standing_paused"]) != 1 || intOf(ret["schedules_paused"]) != 1 ||
		intOf(summary["standing_paused"]) != 1 || intOf(summary["schedules_paused"]) != 1 {
		t.Fatalf("retire should pause bound triggers: %v", ret)
	}
	var retireAudit map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "agent.retire" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) == nil && pl["agent"] == "researcher" {
			retireAudit = pl
		}
		return nil
	})
	if retireAudit == nil || retireAudit["reason"] != "cleanup drill" ||
		intOf(retireAudit["standing_paused"]) != 1 || intOf(retireAudit["schedules_paused"]) != 1 {
		t.Fatalf("agent.retire audit event wrong: %v", retireAudit)
	}
	retireImpact, _ := retireAudit["impact_summary"].(map[string]any)
	if intOf(retireImpact["standing_count"]) != 1 || intOf(retireImpact["schedule_count"]) != 1 {
		t.Fatalf("agent.retire audit impact wrong: %v", retireAudit)
	}
	if intOf(retireImpact["workflow_ref_count"]) != 1 || intOf(retireImpact["subagent_workflow_ref_count"]) != 1 {
		t.Fatalf("agent.retire audit workflow impact wrong: %v", retireAudit)
	}
	for _, o := range k.Standing().List() {
		if o.Agent == "researcher" && o.Enabled {
			t.Fatalf("retired agent standing order stayed armed: %+v", o)
		}
		if o.Agent == "research-worker" && !o.Enabled {
			t.Fatalf("sub-agent standing order should not be paused by parent retire: %+v", o)
		}
	}
	for _, e := range k.Schedules().List() {
		if e.Agent == "researcher" && e.Enabled {
			t.Fatalf("retired agent schedule stayed armed: %+v", e)
		}
		if e.Agent == "research-worker" && !e.Enabled {
			t.Fatalf("sub-agent schedule should not be paused by parent retire: %+v", e)
		}
	}
	rev, err := c.Call(ctx, controlplane.CmdAgentRevive, map[string]any{"ref": "researcher"})
	if err != nil {
		t.Fatalf("agent revive with paused triggers: %v", err)
	}
	if intOf(rev["standing_paused"]) != 1 || intOf(rev["schedules_paused"]) != 1 {
		t.Fatalf("revive should report still-paused bound triggers: %v", rev)
	}
	var reviveAudit map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "agent.revive" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) == nil && pl["agent"] == "researcher" {
			reviveAudit = pl
		}
		return nil
	})
	if reviveAudit == nil || intOf(reviveAudit["standing_paused"]) != 1 || intOf(reviveAudit["schedules_paused"]) != 1 {
		t.Fatalf("agent.revive audit event wrong: %v", reviveAudit)
	}
	rp, _ := rev["profile"].(map[string]any)
	if rp["retired"] == true || rp["enabled"] == true {
		t.Fatalf("revive should clear graveyard but keep agent paused: %v", rp)
	}
	for _, o := range k.Standing().List() {
		if o.Agent == "researcher" && o.Enabled {
			t.Fatalf("revive should not re-arm standing order: %+v", o)
		}
	}
	for _, e := range k.Schedules().List() {
		if e.Agent == "researcher" && e.Enabled {
			t.Fatalf("revive should not re-arm schedule: %+v", e)
		}
	}
	resume, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "researcher", "enabled": true})
	if err != nil {
		t.Fatalf("agent resume after revive: %v", err)
	}
	if intOf(resume["standing_paused"]) != 1 || intOf(resume["schedules_paused"]) != 1 {
		t.Fatalf("resume should report still-paused bound triggers: %v", resume)
	}
	for _, o := range k.Standing().List() {
		if o.Agent == "researcher" && o.Enabled {
			t.Fatalf("resume should not re-arm standing order: %+v", o)
		}
	}
	for _, e := range k.Schedules().List() {
		if e.Agent == "researcher" && e.Enabled {
			t.Fatalf("resume should not re-arm schedule: %+v", e)
		}
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{
		"ref": "researcher",
		"cascade": map[string]any{
			"standing":        true,
			"schedules":       true,
			"memory":          true,
			"authored_memory": true,
			"skills":          true,
		},
	}); err == nil {
		t.Fatal("agent remove without subagent cascade unexpectedly succeeded")
	}

	rm, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{
		"ref": "researcher",
		"cascade": map[string]any{
			"standing":        true,
			"schedules":       true,
			"memory":          true,
			"authored_memory": true,
			"skills":          true,
			"config":          true,
			"workspace":       true,
			"subagents":       true,
		},
	})
	if err != nil {
		t.Fatalf("agent remove cascade: %v", err)
	}
	if rm["removed"] != true || intOf(rm["standing_removed"]) != 2 || intOf(rm["schedules_removed"]) != 2 ||
		intOf(rm["memories_forgotten"]) != 2 || intOf(rm["skills_archived"]) != 2 ||
		intOf(rm["authored_memories_forgotten"]) != 2 || intOf(rm["configs_deleted"]) != 2 ||
		intOf(rm["configs_access_pruned"]) != 2 || intOf(rm["workspaces_deleted"]) != 1 || intOf(rm["subagents_retired"]) != 1 ||
		intOf(rm["mailbox_messages_retained"]) != 4 || intOf(rm["workflow_refs_retained"]) != 1 ||
		intOf(rm["subagent_workflow_refs_retained"]) != 1 {
		t.Fatalf("remove cascade result wrong: %v", rm)
	}
	if got := anyStrings(rm["subagents_retired_slugs"]); !reflect.DeepEqual(got, []string{"research-worker"}) {
		t.Fatalf("remove cascade retired subagent slugs = %v, want [research-worker]", got)
	}
	if got := strings.Join(anyStrings(rm["mailbox_messages_retained_refs"]), "\n"); !strings.Contains(got, "dm received") ||
		!strings.Contains(got, "dm sent") || !strings.Contains(got, "broadcast broadcast") {
		t.Fatalf("remove cascade retained mailbox refs wrong: %v", rm["mailbox_messages_retained_refs"])
	}
	if got := strings.Join(anyStrings(rm["workflow_refs_retained_labels"]), "\n"); !strings.Contains(got, "research-agent-flow/handoff") {
		t.Fatalf("remove cascade retained workflow refs wrong: %v", rm["workflow_refs_retained_labels"])
	}
	if got := strings.Join(anyStrings(rm["subagent_workflow_refs_retained_labels"]), "\n"); !strings.Contains(got, "research-worker: worker-agent-flow/delegate") {
		t.Fatalf("remove cascade retained subagent workflow refs wrong: %v", rm["subagent_workflow_refs_retained_labels"])
	}
	var removeAudit map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "agent.remove" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) == nil && pl["agent"] == "researcher" {
			removeAudit = pl
		}
		return nil
	})
	if removeAudit == nil {
		t.Fatal("agent.remove audit event missing")
	}
	if intOf(removeAudit["standing_removed"]) != 2 || intOf(removeAudit["schedules_removed"]) != 2 ||
		intOf(removeAudit["memories_forgotten"]) != 2 || intOf(removeAudit["authored_memories_forgotten"]) != 2 ||
		intOf(removeAudit["skills_archived"]) != 2 || intOf(removeAudit["configs_deleted"]) != 2 ||
		intOf(removeAudit["configs_access_pruned"]) != 2 || intOf(removeAudit["workspaces_deleted"]) != 1 || intOf(removeAudit["subagents_retired"]) != 1 ||
		intOf(removeAudit["mailbox_messages_retained"]) != 4 || intOf(removeAudit["workflow_refs_retained"]) != 1 ||
		intOf(removeAudit["subagent_workflow_refs_retained"]) != 1 {
		t.Fatalf("agent.remove audit cleanup counts wrong: %v", removeAudit)
	}
	if got := anyStrings(removeAudit["subagents_retired_slugs"]); !reflect.DeepEqual(got, []string{"research-worker"}) {
		t.Fatalf("agent.remove audit retired subagent slugs = %v, want [research-worker]", got)
	}
	if got := strings.Join(anyStrings(removeAudit["mailbox_messages_retained_refs"]), "\n"); !strings.Contains(got, "dm received") ||
		!strings.Contains(got, "dm sent") || !strings.Contains(got, "broadcast broadcast") {
		t.Fatalf("agent.remove audit retained mailbox refs wrong: %v", removeAudit["mailbox_messages_retained_refs"])
	}
	if got := strings.Join(anyStrings(removeAudit["workflow_refs_retained_labels"]), "\n"); !strings.Contains(got, "research-agent-flow/handoff") {
		t.Fatalf("agent.remove audit retained workflow refs wrong: %v", removeAudit["workflow_refs_retained_labels"])
	}
	if got := strings.Join(anyStrings(removeAudit["subagent_workflow_refs_retained_labels"]), "\n"); !strings.Contains(got, "research-worker: worker-agent-flow/delegate") {
		t.Fatalf("agent.remove audit retained subagent workflow refs wrong: %v", removeAudit["subagent_workflow_refs_retained_labels"])
	}
	cascadeAudit, _ := removeAudit["cascade"].(map[string]any)
	if cascadeAudit["standing"] != true || cascadeAudit["schedules"] != true || cascadeAudit["memory"] != true ||
		cascadeAudit["authored_memory"] != true || cascadeAudit["skills"] != true || cascadeAudit["config"] != true ||
		cascadeAudit["workspace"] != true || cascadeAudit["subagents"] != true {
		t.Fatalf("agent.remove audit cascade wrong: %v", removeAudit)
	}
	if _, ok := k.Roster().Get("researcher"); ok {
		t.Fatal("agent still exists")
	}
	if child, ok := k.Roster().Get("research-worker"); !ok || !child.Retired {
		t.Fatalf("dependent subagent should remain retired, got ok=%v profile=%+v", ok, child)
	}
	if got := k.Standing().Count(); got != 0 {
		t.Fatalf("standing count = %d, want 0", got)
	}
	if got := len(k.Schedules().List()); got != 0 {
		t.Fatalf("schedule count = %d, want 0", got)
	}
	if got := k.Workflows().Count(); got != 2 {
		t.Fatalf("workflow references should remain as operator-owned chains, count=%d want 2", got)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "researcher")); !os.IsNotExist(err) {
		t.Fatalf("agent workspace should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "shared", "keep.md")); err != nil {
		t.Fatalf("shared workspace file should remain: %v", err)
	}
	active, _ := k.Memory().Active()
	subjects := map[string]bool{}
	for _, r := range active {
		subjects[r.Subject] = true
	}
	if len(active) != 1 || !subjects["shared"] {
		t.Fatalf("only unrelated shared memory should remain after parent+subagent cleanup: %+v", active)
	}
	priv, _, _ := k.Forge().Get(privateSkill.ID)
	workerPriv, _, _ := k.Forge().Get(workerSkill.ID)
	shared, _, _ := k.Forge().Get(sharedSkill.ID)
	if priv.Status != skill.StatusArchived {
		t.Fatalf("private skill status = %s, want archived", priv.Status)
	}
	if workerPriv.Status != skill.StatusArchived {
		t.Fatalf("retired subagent skill status = %s, want archived when skills+subagents cascades are selected", workerPriv.Status)
	}
	if shared.Status == skill.StatusArchived {
		t.Fatalf("shared skill should remain unarchived: %+v", shared)
	}
	if _, err := k.ConfigCenter().GetEntry("agent/researcher/runtime"); err == nil {
		t.Fatal("parent private config should be deleted")
	}
	if _, err := k.ConfigCenter().GetEntry("agent/research-worker/runtime"); err == nil {
		t.Fatal("subagent private config should be deleted when config+subagents cascades are selected")
	}
	sharedConfig, err := k.ConfigCenter().GetEntry("shared/api-key")
	if err != nil {
		t.Fatalf("shared config should remain even if allowed_agents mentioned removed agent: %v", err)
	}
	if configAgentListHas(sharedConfig.AllowedAgents, "researcher") || configAgentListHas(sharedConfig.ExcludedAgents, "research-worker") {
		t.Fatalf("shared config retained removed agent access refs: allowed=%v excluded=%v", sharedConfig.AllowedAgents, sharedConfig.ExcludedAgents)
	}
	if !configAgentListHas(sharedConfig.AllowedAgents, "ops") || !configAgentListHas(sharedConfig.ExcludedAgents, "blocked") {
		t.Fatalf("shared config should preserve unrelated agent access refs: allowed=%v excluded=%v", sharedConfig.AllowedAgents, sharedConfig.ExcludedAgents)
	}
}

func TestAgentRemove_CascadeIncludesNestedSubagents(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	for _, profile := range []map[string]any{
		{"slug": "lead", "workdir": "lead"},
		{"slug": "worker", "kind": "subagent", "parent_agent": "lead", "owner_agent": "lead", "direct_callable": false, "workdir": "lead/worker"},
		{"slug": "scout", "kind": "subagent", "parent_agent": "worker", "owner_agent": "worker", "direct_callable": false, "workdir": "lead/worker/scout"},
	} {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{"profile": profile}); err != nil {
			t.Fatalf("agent add %v: %v", profile["slug"], err)
		}
	}
	workspaceRoot := filepath.Join(k.BaseDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "lead", "worker", "scout"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "lead", "worker", "scout", "notes.md"), []byte("scout notes"), 0o644); err != nil {
		t.Fatalf("scout workspace file: %v", err)
	}
	if _, err := k.AddStanding(standing.Order{
		Name:     "Scout standing",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "scout.wake"}},
		Agent:    "scout",
		Plan:     "scan nested work",
	}); err != nil {
		t.Fatalf("standing add: %v", err)
	}
	sched, err := k.Schedules().Add("scout refresh", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("schedule add: %v", err)
	}
	if ok, err := k.Schedules().SetAgent(sched.ID, "scout"); err != nil || !ok {
		t.Fatalf("schedule set agent ok=%v err=%v", ok, err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "scout private", Content: "nested scout fact", Tags: map[string]string{"scope": "scout"},
	}); err != nil {
		t.Fatalf("scout memory: %v", err)
	}
	scoutSkill, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name: "scout-private", Body: "Nested scout routine.", Agent: "scout",
	})
	if err != nil {
		t.Fatalf("scout skill: %v", err)
	}
	if err := k.ConfigCenter().Set(&configcenter.ConfigEntry{
		Key:       "agent/scout/runtime",
		Value:     "mode=scout",
		Rating:    configcenter.RatingInternal,
		CreatedBy: "scout",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Metadata:  map[string]string{"agent": "scout"},
	}); err != nil {
		t.Fatalf("scout config: %v", err)
	}

	impact, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": "lead"})
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if intOf(impact["subagent_count"]) != 2 || intOf(impact["subagent_standing_count"]) != 1 ||
		intOf(impact["subagent_schedule_count"]) != 1 || intOf(impact["subagent_memory_count"]) != 1 ||
		intOf(impact["subagent_skill_count"]) != 1 || intOf(impact["subagent_config_count"]) != 1 ||
		intOf(impact["subagent_workspace_count"]) != 2 {
		t.Fatalf("nested subagent impact counts wrong: %v", impact)
	}

	rm, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{
		"ref": "lead",
		"cascade": map[string]any{
			"standing":  true,
			"schedules": true,
			"memory":    true,
			"skills":    true,
			"config":    true,
			"workspace": true,
			"subagents": true,
		},
	})
	if err != nil {
		t.Fatalf("remove cascade: %v", err)
	}
	if rm["removed"] != true || intOf(rm["subagents_retired"]) != 2 || intOf(rm["standing_removed"]) != 1 ||
		intOf(rm["schedules_removed"]) != 1 || intOf(rm["memories_forgotten"]) != 1 ||
		intOf(rm["skills_archived"]) != 1 || intOf(rm["configs_deleted"]) != 1 ||
		intOf(rm["workspaces_deleted"]) != 1 {
		t.Fatalf("nested remove cascade result wrong: %v", rm)
	}
	if got := anyStrings(rm["subagents_retired_slugs"]); !reflect.DeepEqual(got, []string{"scout", "worker"}) {
		t.Fatalf("nested retired subagent slugs = %v, want [scout worker]", got)
	}
	if worker, ok := k.Roster().Get("worker"); !ok || !worker.Retired {
		t.Fatalf("worker should remain retired, ok=%v profile=%+v", ok, worker)
	}
	if scout, ok := k.Roster().Get("scout"); !ok || !scout.Retired {
		t.Fatalf("scout should remain retired, ok=%v profile=%+v", ok, scout)
	}
	if got := k.Standing().Count(); got != 0 {
		t.Fatalf("standing count = %d, want 0", got)
	}
	if got := len(k.Schedules().List()); got != 0 {
		t.Fatalf("schedule count = %d, want 0", got)
	}
	if _, err := k.ConfigCenter().GetEntry("agent/scout/runtime"); err == nil {
		t.Fatal("scout private config should be deleted")
	}
	if got, _, _ := k.Forge().Get(scoutSkill.ID); got.Status != skill.StatusArchived {
		t.Fatalf("scout skill status = %s, want archived", got.Status)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "lead")); !os.IsNotExist(err) {
		t.Fatalf("lead workspace should be removed with nested workspace, err=%v", err)
	}
}

func configAgentListHas(values []string, slug string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), slug) {
			return true
		}
	}
	return false
}

func TestAgentRemove_RejectsSystemAgents(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := k.AddProfile(roster.Profile{
		Slug:    "guardian-health",
		Name:    "Guardian Health",
		Model:   "m",
		Enabled: true,
		System:  true,
	}); err != nil {
		t.Fatalf("seed system agent: %v", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{"ref": "guardian-health"}); err == nil || !strings.Contains(err.Error(), "system agent guardian-health cannot be removed") {
		t.Fatalf("system remove err = %v, want protected system-agent error", err)
	}
	if p, ok := k.Roster().Get("guardian-health"); !ok || !p.System {
		t.Fatalf("system agent should remain after rejected remove: ok=%v profile=%+v", ok, p)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "guardian-health", "reason": "operator maintenance"}); err != nil {
		t.Fatalf("system agent should still be retireable: %v", err)
	}
	if p, ok := k.Roster().Get("guardian-health"); !ok || !p.Retired {
		t.Fatalf("system agent should be in graveyard after retire: ok=%v profile=%+v", ok, p)
	}
}

// TestRun_AsAgent proves the run seam: --agent resolves the profile and applies
// its model + soul as the run's defaults (visible in the dry-run plan, which is
// built from the SAME locals the real run uses); explicit overrides still win;
// unknown and paused agents are usage errors.
func TestRun_AsAgent(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":  "researcher",
			"soul":  "You research deeply.",
			"model": "agent-model",
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	list, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	first := list["profiles"].([]any)[0].(map[string]any)
	if first["kind"] != "custom" || first["managed"] != false {
		t.Fatalf("custom identity class not listed: %v", first)
	}

	// The profile fills the gaps: model + system come from the agent.
	plan, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "researcher", "dry_run": true})
	if err != nil {
		t.Fatalf("dry-run as agent: %v", err)
	}
	if plan["model"] != "agent-model" {
		t.Errorf("model = %v, want agent-model", plan["model"])
	}
	if src, _ := plan["system_source"].(string); !strings.Contains(src, "per-run") {
		t.Errorf("system_source = %v, want per-run (the soul was applied)", src)
	}

	// Explicit per-run flags still win over the profile.
	plan2, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "researcher", "model": "explicit-model", "dry_run": true})
	if err != nil {
		t.Fatalf("dry-run with explicit model: %v", err)
	}
	if plan2["model"] != "explicit-model" {
		t.Errorf("model = %v, want explicit-model (explicit flag must win)", plan2["model"])
	}

	// Unknown agent → usage error, nothing runs.
	if _, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "ghost"}); err == nil ||
		!strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("unknown agent: err = %v", err)
	}

	// Paused agent → refused with a hint, nothing runs.
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled,
		map[string]any{"ref": "researcher", "enabled": false}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "researcher"}); err == nil ||
		!strings.Contains(err.Error(), "paused") {
		t.Fatalf("paused agent: err = %v", err)
	}

	ret, err := c.Call(ctx, controlplane.CmdAgentRetire,
		map[string]any{"ref": "researcher", "reason": "superseded by analyst"})
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if rp, _ := ret["profile"].(map[string]any); rp["retired_reason"] != "superseded by analyst" {
		t.Fatalf("retire reason not returned: %v", rp)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled,
		map[string]any{"ref": "researcher", "enabled": true}); err == nil ||
		!strings.Contains(err.Error(), "retired") {
		t.Fatalf("resume retired agent: err = %v, want retired", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "researcher"}); err == nil ||
		!strings.Contains(err.Error(), "retired") {
		t.Fatalf("retired agent: err = %v, want retired", err)
	}
}

// TestAgentActivity_ShowsRuns runs a real task AS a named agent and asserts the
// per-agent activity timeline (M854) attributes the run to it.
func TestAgentActivity_ShowsRuns(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("done")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "scout", "soul": "You scout.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	// A real run (not dry) as the agent — journals task.received tagged agent=scout.
	// CmdRun streams events before its terminal result, so drive it via Stream.
	if _, err := c.Stream(ctx, controlplane.CmdRun,
		map[string]any{"intent": "find the thing", "agent": "scout"},
		func(*event.Event) {}); err != nil {
		t.Fatalf("run as agent: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.scout.retry",
		Kind:          event.KindAgentRetry,
		Actor:         "agent-retry",
		CorrelationID: "corr-retry",
		Payload: map[string]any{
			"agent":        "scout",
			"attempt":      1,
			"next_attempt": 2,
			"max_attempts": 3,
			"reason":       "timeout",
			"error":        "provider timed out",
			"delay_ms":     2000,
			"backoff":      "exponential",
			"retry_on":     []string{"error", "timeout"},
		},
	}); err != nil {
		t.Fatalf("publish retry: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "roster.scout",
		Kind:          event.KindRosterUpdated,
		Actor:         "roster",
		CorrelationID: "corr-cycle",
		Payload: map[string]any{
			"slug":             "scout",
			"action":           "lifecycle_cycle_completed",
			"completed_cycles": 2,
			"max_cycles":       5,
		},
	}); err != nil {
		t.Fatalf("publish cycle completion: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "roster.scout",
		Kind:          event.KindRosterUpdated,
		Actor:         "roster",
		CorrelationID: "corr-auto-retire",
		Payload: map[string]any{
			"slug":    "scout",
			"action":  "retired",
			"retired": true,
			"reason":  "completed 5/5 cycles on run corr-auto-retire",
		},
	}); err != nil {
		t.Fatalf("publish automatic lifecycle retirement: %v", err)
	}
	for _, spec := range []event.Spec{
		{
			Subject: "agent.retire",
			Kind:    event.KindInfo,
			Actor:   "controlplane",
			Payload: map[string]any{
				"agent":            "scout",
				"reason":           "mission complete",
				"standing_paused":  1,
				"schedules_paused": 2,
			},
		},
		{
			Subject: "agent.revive",
			Kind:    event.KindInfo,
			Actor:   "controlplane",
			Payload: map[string]any{
				"agent":            "scout",
				"standing_paused":  1,
				"schedules_paused": 2,
			},
		},
		{
			Subject: "agent.remove",
			Kind:    event.KindInfo,
			Actor:   "controlplane",
			Payload: map[string]any{
				"agent":                           "scout",
				"standing_removed":                1,
				"schedules_removed":               2,
				"memories_forgotten":              3,
				"authored_memories_forgotten":     4,
				"skills_archived":                 5,
				"configs_deleted":                 6,
				"configs_access_pruned":           7,
				"workspaces_deleted":              8,
				"subagents_retired":               9,
				"mailbox_messages_retained":       10,
				"workflow_refs_retained":          11,
				"subagent_workflow_refs_retained": 12,
			},
		},
	} {
		if _, err := k.Bus().Publish(spec); err != nil {
			t.Fatalf("publish %s: %v", spec.Subject, err)
		}
	}

	res, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{"ref": "scout"})
	if err != nil {
		t.Fatalf("agent activity: %v", err)
	}
	if res["slug"] != "scout" {
		t.Errorf("slug = %v, want scout", res["slug"])
	}
	acts, _ := res["activity"].([]any)
	if len(acts) == 0 {
		t.Fatalf("no activity recorded for the agent's run")
	}
	var sawStart, sawRetry, sawCycle, sawLifecycleRetire, sawRetire, sawRevive, sawRemove bool
	for _, raw := range acts {
		m, _ := raw.(map[string]any)
		if s, _ := m["summary"].(string); strings.Contains(s, "started a run") && strings.Contains(s, "find the thing") {
			sawStart = true
		}
		if s, _ := m["summary"].(string); strings.Contains(s, "retrying run") && strings.Contains(s, "attempt 2/3") &&
			strings.Contains(s, "timeout") && strings.Contains(s, "delay 2000ms") &&
			strings.Contains(s, "backoff exponential") && strings.Contains(s, "retry_on error,timeout") {
			sawRetry = true
		}
		if s, _ := m["summary"].(string); strings.Contains(s, "completed lifecycle cycle 2/5") {
			sawCycle = true
		}
		if s, _ := m["summary"].(string); strings.Contains(s, "lifecycle retired the agent") &&
			strings.Contains(s, "completed 5/5 cycles") {
			sawLifecycleRetire = true
		}
		if s, _ := m["summary"].(string); strings.Contains(s, "operator retired the agent") &&
			strings.Contains(s, "reason: mission complete") && strings.Contains(s, "1 standing paused") &&
			strings.Contains(s, "2 schedules paused") {
			sawRetire = true
		}
		if s, _ := m["summary"].(string); strings.Contains(s, "operator revived the agent") &&
			strings.Contains(s, "1 standing paused") && strings.Contains(s, "2 schedules paused") {
			sawRevive = true
		}
		if s, _ := m["summary"].(string); strings.Contains(s, "operator removed the agent") &&
			strings.Contains(s, "1 standing removed") && strings.Contains(s, "2 schedules removed") &&
			strings.Contains(s, "3 private memories forgotten") && strings.Contains(s, "4 authored memories forgotten") &&
			strings.Contains(s, "5 skills archived") && strings.Contains(s, "6 configs deleted") &&
			strings.Contains(s, "7 shared config access pruned") && strings.Contains(s, "8 workspaces deleted") &&
			strings.Contains(s, "9 sub-agents retired") && strings.Contains(s, "10 mailbox/audit messages retained") &&
			strings.Contains(s, "11 workflow refs retained") && strings.Contains(s, "12 sub-agent workflow refs retained") {
			sawRemove = true
		}
	}
	if !sawStart {
		t.Errorf("activity did not attribute the run to the agent: %+v", acts)
	}
	if !sawRetry {
		t.Errorf("activity did not show the agent retry decision: %+v", acts)
	}
	if !sawCycle {
		t.Errorf("activity did not show lifecycle cycle completion: %+v", acts)
	}
	if !sawLifecycleRetire {
		t.Errorf("activity did not show automatic lifecycle retirement: %+v", acts)
	}
	if !sawRetire || !sawRevive || !sawRemove {
		t.Errorf("activity did not show lifecycle audit events (retire=%v revive=%v remove=%v): %+v", sawRetire, sawRevive, sawRemove, acts)
	}

	// An unrelated agent has no activity from scout's run.
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "idle", "soul": "."},
	}); err != nil {
		t.Fatalf("add idle: %v", err)
	}
	res2, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{"ref": "idle"})
	if err != nil {
		t.Fatalf("idle activity: %v", err)
	}
	if acts2, _ := res2["activity"].([]any); len(acts2) != 0 {
		t.Errorf("idle agent should have no activity, got %+v", acts2)
	}
}

func TestAgentActivity_ShowsAutoRepairEvents(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("done")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "builder", "self_repair": map[string]any{"enabled": true}},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	publish := func(payload map[string]any, corr string) {
		if _, err := k.Bus().Publish(event.Spec{
			Subject:       "doctor.auto_repair",
			Kind:          event.KindInfo,
			Actor:         "kernel",
			CorrelationID: corr,
			Payload:       payload,
		}); err != nil {
			t.Fatalf("publish repair event: %v", err)
		}
	}
	publish(map[string]any{
		"phase":       "queued",
		"agent":       "builder",
		"mode":        "misconfigured",
		"fingerprint": "fp-1",
		"issues":      []string{"AGEZT_MAX_ITER: must be an integer"},
		"reason":      "invalid runtime override",
	}, "")
	publish(map[string]any{
		"phase":       "completed",
		"agent":       "builder",
		"mode":        "degraded",
		"fingerprint": "fp-2",
		"reason":      "too many failures",
		"applied":     []string{"model", "config_overrides"},
	}, "corr-doctor-1")

	res, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{"ref": "builder", "limit": 10})
	if err != nil {
		t.Fatalf("agent activity: %v", err)
	}
	acts, _ := res["activity"].([]any)
	if len(acts) < 2 {
		t.Fatalf("activity len = %d, want at least 2", len(acts))
	}
	var sawQueued, sawCompleted bool
	for _, raw := range acts {
		row, _ := raw.(map[string]any)
		summary, _ := row["summary"].(string)
		switch {
		case strings.Contains(summary, "repair queued for 1 config issue"):
			sawQueued = true
		case strings.Contains(summary, "doctor applied 2 profile change"):
			sawCompleted = true
		}
	}
	if !sawQueued || !sawCompleted {
		t.Fatalf("auto-repair activity missing queued/completed rows: %+v", acts)
	}
}

func TestAgentRepairStatus_ShowsHistoryAndInflight(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("done")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":         "builder",
			"soul":         "You build.",
			"retry_policy": map[string]any{"max_attempts": 3, "backoff": "exponential", "retry_on": []string{"error", "timeout"}},
			"health_policy": map[string]any{
				"doctor_agent":      "guardian-doctor",
				"failure_threshold": 2,
			},
			"self_repair": map[string]any{"enabled": true, "max_attempts": 2, "escalate_to": "lead"},
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	publish := func(payload map[string]any, corr string) {
		if _, err := k.Bus().Publish(event.Spec{
			Subject:       "doctor.auto_repair",
			Kind:          event.KindInfo,
			Actor:         "kernel",
			CorrelationID: corr,
			Payload:       payload,
		}); err != nil {
			t.Fatalf("publish repair event: %v", err)
		}
	}
	publish(map[string]any{
		"phase":       "queued",
		"agent":       "builder",
		"mode":        "misconfigured",
		"fingerprint": "fp-1",
		"reason":      "invalid runtime override",
		"issues":      []string{"AGEZT_MAX_ITER: must be an integer"},
	}, "")
	publish(map[string]any{
		"phase":       "completed",
		"agent":       "builder",
		"mode":        "misconfigured",
		"fingerprint": "fp-1",
		"reason":      "invalid runtime override",
		"issues":      []string{"AGEZT_MAX_ITER: must be an integer"},
		"applied":     []string{"config_overrides"},
		"answer":      "fixed",
	}, "corr-repair-1")
	publish(map[string]any{
		"phase":       "queued",
		"agent":       "builder",
		"mode":        "degraded",
		"fingerprint": "fp-2",
		"reason":      "blank model override",
		"issues":      []string{"AGEZT_MODEL: value is blank"},
	}, "")
	publish(map[string]any{
		"phase":       "queued",
		"agent":       "writer",
		"fingerprint": "fp-other",
		"reason":      "other agent",
	}, "")

	res, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{"ref": "builder", "limit": 10})
	if err != nil {
		t.Fatalf("agent repair status: %v", err)
	}
	if res["slug"] != "builder" {
		t.Fatalf("slug = %v, want builder", res["slug"])
	}
	history, _ := res["history"].([]any)
	if len(history) != 3 {
		t.Fatalf("history len = %d, want 3", len(history))
	}
	latest, _ := res["latest"].(map[string]any)
	if latest["phase"] != "queued" || latest["fingerprint"] != "fp-2" || latest["mode"] != "degraded" {
		t.Fatalf("latest = %+v, want queued fp-2", latest)
	}
	inflight, _ := res["inflight"].([]any)
	if len(inflight) != 1 {
		t.Fatalf("inflight len = %d, want 1", len(inflight))
	}
	row, _ := inflight[0].(map[string]any)
	if row["fingerprint"] != "fp-2" {
		t.Fatalf("inflight row = %+v, want fp-2", row)
	}
	if _, ok := res["next_eligible_ms"].(float64); !ok {
		t.Fatalf("next_eligible_ms missing: %+v", res)
	}
	contract, _ := res["contract"].(map[string]any)
	if intOf(contract["retry_attempts"]) != 3 || contract["retry_backoff"] != "exponential" ||
		contract["doctor_agent"] != "guardian-doctor" || contract["self_repair_enabled"] != true ||
		intOf(contract["self_repair_attempts"]) != 2 || contract["escalate_to"] != "lead" {
		t.Fatalf("repair contract wrong: %+v", contract)
	}
	if got := strings.Join(anyStrings(contract["retry_on"]), ","); got != "error,timeout" {
		t.Fatalf("repair contract retry_on = %q", got)
	}
	if got, _ := contract["authority_boundary"].(string); !strings.Contains(got, "agent identity owns retry") {
		t.Fatalf("repair contract authority boundary = %q", got)
	}
	nextAction, _ := res["next_action"].(map[string]any)
	if nextAction["action"] != "wait_inflight" || nextAction["label"] != "repair in flight" ||
		nextAction["tone"] != "accent" || nextAction["fingerprint"] != "fp-2" {
		t.Fatalf("repair next_action wrong: %+v", nextAction)
	}
	if got, _ := nextAction["detail"].(string); !strings.Contains(got, "doctor/self-repair run is already queued") || !strings.Contains(got, "phase queued") {
		t.Fatalf("repair next_action detail = %q", got)
	}
}

func TestAgentRepair_AcceptsAndJournalsLifecycle(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("repair complete")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "builder", "soul": "You build.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentRepair, map[string]any{
		"ref":              "builder",
		"reason":           "operator rerun",
		"incident_id":      "inc-child-1",
		"root_incident_id": "inc-root-1",
	})
	if err != nil {
		t.Fatalf("agent repair: %v", err)
	}
	if res["accepted"] != true || res["agent"] != "builder" {
		t.Fatalf("repair result = %+v", res)
	}
	corr, _ := res["correlation_id"].(string)
	if corr == "" {
		t.Fatalf("repair correlation missing: %+v", res)
	}
	var sawRequested, sawCompleted bool
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Subject != "agent.repair" || e.CorrelationID != corr {
				return nil
			}
			var pl map[string]any
			_ = json.Unmarshal(e.Payload, &pl)
			switch pl["phase"] {
			case "requested":
				sawRequested = true
			case "completed":
				sawCompleted = true
				if pl["root_incident_id"] != "inc-root-1" {
					t.Fatalf("repair lineage missing: %+v", pl)
				}
			}
			return nil
		})
		if sawRequested && sawCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawRequested || !sawCompleted {
		t.Fatalf("repair lifecycle incomplete: requested=%v completed=%v", sawRequested, sawCompleted)
	}
}

func TestAgentWake_AcceptsAndRuns(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("wake handled")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "You lead.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentWake, map[string]any{
		"ref":              "lead",
		"reason":           "incident ownership required",
		"incident_id":      "inc-child-2",
		"root_incident_id": "inc-root-2",
	})
	if err != nil {
		t.Fatalf("agent wake: %v", err)
	}
	if res["accepted"] != true || res["agent"] != "lead" {
		t.Fatalf("wake result = %+v", res)
	}
	corr, _ := res["correlation_id"].(string)
	if corr == "" {
		t.Fatalf("wake correlation missing: %+v", res)
	}
	var sawRequested, sawCompleted, sawTask bool
	var taskPayload map[string]any
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.CorrelationID != corr {
				return nil
			}
			if e.Subject == "agent.wake" {
				var pl map[string]any
				_ = json.Unmarshal(e.Payload, &pl)
				switch pl["phase"] {
				case "requested":
					sawRequested = true
					runbook, _ := pl["autonomy_runbook"].(map[string]any)
					if runbook["trigger_contract"] != "operator_schedule_channel" ||
						runbook["route_contract"] != "self_owned" ||
						runbook["recovery_contract"] != "manual" ||
						runbook["sleep_contract"] != "persistent" ||
						runbook["identity_kind"] != "custom" {
						t.Fatalf("wake requested autonomy runbook = %+v", runbook)
					}
				case "completed":
					sawCompleted = true
					if pl["root_incident_id"] != "inc-root-2" {
						t.Fatalf("wake lineage missing: %+v", pl)
					}
					runbook, _ := pl["autonomy_runbook"].(map[string]any)
					if runbook["trigger_contract"] != "operator_schedule_channel" || runbook["route_contract"] != "self_owned" {
						t.Fatalf("wake completed autonomy runbook = %+v", runbook)
					}
				}
			}
			if e.Kind == event.KindTaskReceived {
				sawTask = true
				_ = json.Unmarshal(e.Payload, &taskPayload)
			}
			return nil
		})
		if sawRequested && sawCompleted && sawTask {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawRequested || !sawCompleted || !sawTask {
		t.Fatalf("wake lifecycle incomplete: requested=%v completed=%v task=%v", sawRequested, sawCompleted, sawTask)
	}
	if taskPayload["wake_source"] != "operator" || taskPayload["wake_reason"] != "incident ownership required" {
		t.Fatalf("wake task provenance = %+v", taskPayload)
	}
	act, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{"ref": "lead", "limit": 10})
	if err != nil {
		t.Fatalf("agent activity: %v", err)
	}
	acts, _ := act["activity"].([]any)
	found := false
	foundContract := false
	for _, raw := range acts {
		item, _ := raw.(map[string]any)
		if summary, _ := item["summary"].(string); strings.Contains(summary, "operator wake requested") {
			found = true
			if strings.Contains(summary, "contract operator_schedule_channel/self_owned/manual/persistent") {
				foundContract = true
			}
			break
		}
	}
	if !found {
		t.Fatalf("lead activity missing operator wake row: %+v", acts)
	}
	if !foundContract {
		t.Fatalf("lead activity missing autonomy contract summary: %+v", acts)
	}
	list, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	profiles, _ := list["profiles"].([]any)
	var status map[string]any
	for _, raw := range profiles {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "lead" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	runbook, _ := status["last_autonomy_runbook"].(map[string]any)
	if runbook["trigger_contract"] != "operator_schedule_channel" ||
		runbook["route_contract"] != "self_owned" ||
		runbook["phase"] != "completed" ||
		runbook["correlation_id"] != corr {
		t.Fatalf("agent status last autonomy runbook = %+v", runbook)
	}
}

func TestAgentWake_AdvancesCycleLifecycleOnce(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("cycle handled")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":      "cycler",
			"soul":      "You cycle.",
			"model":     "m",
			"lifecycle": map[string]any{"mode": "cycle", "max_cycles": 3},
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentWake, map[string]any{"ref": "cycler", "reason": "cycle wake"})
	if err != nil {
		t.Fatalf("agent wake: %v", err)
	}
	corr, _ := res["correlation_id"].(string)
	if corr == "" {
		t.Fatalf("wake correlation missing: %+v", res)
	}
	// Wait for the async wake run to complete.
	waitForWakePhase(t, k, corr, "completed")
	got, ok := k.Roster().Get("cycler")
	if !ok {
		t.Fatal("cycler missing after wake")
	}
	if got.Lifecycle.CompletedCycles != 1 {
		t.Fatalf("operator wake advanced cycle to %d, want exactly 1", got.Lifecycle.CompletedCycles)
	}
	if got.Lifecycle.LastCompletedRun != corr {
		t.Fatalf("last completed run = %q, want wake correlation %q", got.Lifecycle.LastCompletedRun, corr)
	}
	if got.Retired {
		t.Fatalf("cycler should stay active before max cycles: %+v", got.Lifecycle)
	}
}

func TestAgentWake_FailedWakeDoesNotAdvanceLifecycle(t *testing.T) {
	// Exhausted mock provider → the wake run errors, so lifecycle must not advance.
	k, _, c, _ := startPair(t, mock.New())
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":      "cycler",
			"soul":      "You cycle.",
			"model":     "m",
			"lifecycle": map[string]any{"mode": "cycle", "max_cycles": 3},
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentWake, map[string]any{"ref": "cycler", "reason": "cycle wake"})
	if err != nil {
		t.Fatalf("agent wake: %v", err)
	}
	corr, _ := res["correlation_id"].(string)
	waitForWakePhase(t, k, corr, "failed")
	got, ok := k.Roster().Get("cycler")
	if !ok {
		t.Fatal("cycler missing after failed wake")
	}
	if got.Lifecycle.CompletedCycles != 0 {
		t.Fatalf("failed wake advanced cycle to %d, want 0", got.Lifecycle.CompletedCycles)
	}
	if got.Retired || !got.Enabled {
		t.Fatalf("failed wake must not retire or pause the agent: %+v", got)
	}
}

// waitForWakePhase blocks until an agent.wake event with the given phase is
// journaled under corr, or fails the test after a short deadline.
func waitForWakePhase(t *testing.T, k *runtime.Kernel, corr, phase string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var saw bool
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.CorrelationID != corr || e.Subject != "agent.wake" {
				return nil
			}
			var pl map[string]any
			_ = json.Unmarshal(e.Payload, &pl)
			if pl["phase"] == phase {
				saw = true
			}
			return nil
		})
		if saw {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for agent.wake phase %q on %s", phase, corr)
}

func TestAgentWake_RejectsManagedSubAgent(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "Lead."},
	}); err != nil {
		t.Fatalf("agent add lead: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "worker", "parent_agent": "lead", "direct_callable": false},
	}); err != nil {
		t.Fatalf("agent add worker: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentWake, map[string]any{
		"ref":    "worker",
		"reason": "manual operator wake",
	}); err == nil || !strings.Contains(err.Error(), "managed sub-agent") || !strings.Contains(err.Error(), "cannot be called directly") || !strings.Contains(err.Error(), "wake lead") {
		t.Fatalf("managed sub-agent wake err = %v, want direct-call rejection with manager hint", err)
	}
}

func TestAgentList_FoldsScheduleWakeAutonomyRunbook(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Ops agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const corr = "sched-wake-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "schedule.fired",
		Kind:          event.KindScheduleFired,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": "sched-ops",
			"intent":      "check disks",
			"target":      "intent",
			"agent":       "ops",
			"autonomy_runbook": map[string]any{
				"trigger_contract":  "operator_schedule_channel",
				"route_contract":    "self_owned",
				"recovery_contract": "retry",
				"sleep_contract":    "cycle",
				"retry_attempts":    3,
			},
		},
	}); err != nil {
		t.Fatalf("publish schedule fired: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var status map[string]any
	for _, raw := range res["profiles"].([]any) {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "ops" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	runbook, _ := status["last_autonomy_runbook"].(map[string]any)
	if runbook["phase"] != "schedule_fired" ||
		runbook["source"] != "schedule" ||
		runbook["schedule_id"] != "sched-ops" ||
		runbook["correlation_id"] != corr ||
		runbook["trigger_contract"] != "operator_schedule_channel" ||
		runbook["recovery_contract"] != "retry" {
		t.Fatalf("last autonomy runbook = %+v", runbook)
	}
	summary, _ := status["last_activity_summary"].(string)
	if !strings.Contains(summary, "schedule wake fired: sched-ops") ||
		!strings.Contains(summary, "contract operator_schedule_channel/self_owned/retry/cycle") {
		t.Fatalf("last activity summary = %q", summary)
	}
}

func TestAgentList_FoldsStandingWakeAutonomyRunbook(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Ops agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const corr = "standing-wake-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "standing.standing-ops-events",
		Kind:          event.KindStandingFired,
		Actor:         "standing",
		CorrelationID: corr,
		Payload: map[string]any{
			"id":              "standing-ops-events",
			"name":            "ops events",
			"trigger_subject": "ops.alert",
			"agent":           "ops",
			"autonomy_runbook": map[string]any{
				"trigger_contract":  "operator_schedule_channel",
				"route_contract":    "self_owned",
				"recovery_contract": "retry",
				"sleep_contract":    "persistent",
				"retry_attempts":    2,
			},
		},
	}); err != nil {
		t.Fatalf("publish standing fired: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var status map[string]any
	for _, raw := range res["profiles"].([]any) {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "ops" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	runbook, _ := status["last_autonomy_runbook"].(map[string]any)
	if runbook["phase"] != "standing_fired" ||
		runbook["source"] != "standing" ||
		runbook["standing_id"] != "standing-ops-events" ||
		runbook["standing_name"] != "ops events" ||
		runbook["trigger_subject"] != "ops.alert" ||
		runbook["correlation_id"] != corr ||
		runbook["trigger_contract"] != "operator_schedule_channel" ||
		runbook["recovery_contract"] != "retry" {
		t.Fatalf("last autonomy runbook = %+v", runbook)
	}
	summary, _ := status["last_activity_summary"].(string)
	if !strings.Contains(summary, "standing wake fired: standing-ops-events") ||
		!strings.Contains(summary, "contract operator_schedule_channel/self_owned/retry/persistent") {
		t.Fatalf("last activity summary = %q", summary)
	}
}

func TestAgentList_FoldsMailboxWakeAutonomyRunbook(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Ops agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const corr = "mailbox-wake-001"
	// A standing order armed for mailbox wake fires when a board.dm.<slug>
	// message is posted: the standing.fired carries the matched board.posted
	// payload as trigger_payload.
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "standing.dm-watch",
		Kind:          event.KindStandingFired,
		Actor:         "standing",
		CorrelationID: corr,
		Payload: map[string]any{
			"id":              "dm-watch",
			"name":            "ops inbox",
			"trigger_subject": "board.dm.ops",
			"agent":           "ops",
			"trigger_payload": map[string]any{
				"topic":    "ops",
				"id":       "msg-42",
				"from":     "planner",
				"to":       "ops",
				"reply_to": "msg-7",
			},
			"autonomy_runbook": map[string]any{
				"trigger_contract":  "operator_schedule_channel",
				"route_contract":    "self_owned",
				"recovery_contract": "manual",
				"sleep_contract":    "persistent",
				"retry_attempts":    1,
			},
		},
	}); err != nil {
		t.Fatalf("publish standing fired: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var status map[string]any
	for _, raw := range res["profiles"].([]any) {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "ops" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	runbook, _ := status["last_autonomy_runbook"].(map[string]any)
	if runbook["phase"] != "standing_fired" ||
		runbook["source"] != "standing" ||
		runbook["wake_via"] != "mailbox" ||
		runbook["mailbox_message_id"] != "msg-42" ||
		runbook["mailbox_from"] != "planner" ||
		runbook["mailbox_to"] != "ops" ||
		runbook["mailbox_reply_to"] != "msg-7" ||
		runbook["trigger_subject"] != "board.dm.ops" ||
		runbook["correlation_id"] != corr {
		t.Fatalf("last autonomy runbook = %+v", runbook)
	}
	summary, _ := status["last_activity_summary"].(string)
	if !strings.Contains(summary, "mailbox wake fired: from planner") ||
		!strings.Contains(summary, "contract operator_schedule_channel/self_owned/manual/persistent") {
		t.Fatalf("last activity summary = %q", summary)
	}
}

func TestAgentList_FoldsDelegatedWakeAutonomyRunbook(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "worker", "soul": "Worker agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const parentCorr = "lead-run-001"
	const childCorr = "worker-run-001"
	// A leader delegating AS a named sub-agent journals a sub-agent spawn under the
	// parent correlation, carrying the child's wake contract and provenance.
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.subagent-" + childCorr + ".subagent",
		Kind:          event.KindSubAgentSpawned,
		Actor:         "subagent-" + childCorr,
		CorrelationID: parentCorr,
		Payload: map[string]any{
			"task":              "summarize the logs",
			"child_correlation": childCorr,
			"parent":            parentCorr,
			"agent":             "worker",
			"wake_source":       "delegated",
			"delegated_by":      "lead",
			"autonomy_runbook": map[string]any{
				"trigger_contract":  "operator_schedule_channel",
				"route_contract":    "self_owned",
				"recovery_contract": "manual",
				"sleep_contract":    "persistent",
				"retry_attempts":    1,
			},
		},
	}); err != nil {
		t.Fatalf("publish subagent spawned: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var status map[string]any
	for _, raw := range res["profiles"].([]any) {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "worker" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	runbook, _ := status["last_autonomy_runbook"].(map[string]any)
	if runbook["phase"] != "delegated_wake" ||
		runbook["source"] != "delegated" ||
		runbook["delegated_by"] != "lead" ||
		runbook["parent_correlation_id"] != parentCorr ||
		runbook["correlation_id"] != childCorr ||
		runbook["trigger_contract"] != "operator_schedule_channel" {
		t.Fatalf("last autonomy runbook = %+v", runbook)
	}
	summary, _ := status["last_activity_summary"].(string)
	if !strings.Contains(summary, "delegated wake fired: by lead") ||
		!strings.Contains(summary, "contract operator_schedule_channel/self_owned/manual/persistent") {
		t.Fatalf("last activity summary = %q", summary)
	}
}

func TestAgentList_FoldsDoctorWakeAutonomyRunbook(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	// owner is the agent doctor WAKES to handle builder's incident.
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "owner", "soul": "Owner agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add owner: %v", err)
	}
	const corr = "owner-wake-001"
	// doctor.auto_repair escalation_woke attributes the WOKEN agent via target_agent
	// (the "agent" field is the agent being repaired).
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "doctor.auto_repair",
		Kind:          event.KindInfo,
		Actor:         "controlplane",
		CorrelationID: corr,
		Payload: map[string]any{
			"phase":        "escalation_woke",
			"agent":        "builder",
			"target_agent": "owner",
			"mode":         "degraded",
			"incident_id":  "inc-9",
			"autonomy_runbook": map[string]any{
				"trigger_contract":  "operator_schedule_channel",
				"route_contract":    "self_owned",
				"recovery_contract": "manual",
				"sleep_contract":    "persistent",
				"retry_attempts":    1,
			},
		},
	}); err != nil {
		t.Fatalf("publish doctor wake: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var status map[string]any
	for _, raw := range res["profiles"].([]any) {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "owner" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	runbook, _ := status["last_autonomy_runbook"].(map[string]any)
	if runbook["phase"] != "escalation_woke" ||
		runbook["source"] != "doctor" ||
		runbook["doctor_for"] != "builder" ||
		runbook["doctor_mode"] != "degraded" ||
		runbook["incident_id"] != "inc-9" ||
		runbook["correlation_id"] != corr ||
		runbook["trigger_contract"] != "operator_schedule_channel" {
		t.Fatalf("last autonomy runbook = %+v", runbook)
	}
	summary, _ := status["last_activity_summary"].(string)
	if !strings.Contains(summary, "accepted escalation wake for builder") ||
		!strings.Contains(summary, "contract operator_schedule_channel/self_owned/manual/persistent") {
		t.Fatalf("last activity summary = %q", summary)
	}
}

func TestAgentList_ExposesMailboxWakeCausality(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Ops agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const corr = "ops-mailbox-wake-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "standing.dm-watch",
		Kind:          event.KindStandingFired,
		Actor:         "standing",
		CorrelationID: corr,
		Payload: map[string]any{
			"id":              "dm-watch",
			"name":            "ops inbox",
			"trigger_subject": "board.dm.ops",
			"agent":           "ops",
			"trigger_payload": map[string]any{"id": "msg-77", "from": "planner", "to": "ops"},
		},
	}); err != nil {
		t.Fatalf("publish standing fired: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var status map[string]any
	for _, raw := range res["profiles"].([]any) {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "ops" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	wakes, _ := status["mailbox_wakes"].(map[string]any)
	ref, _ := wakes["msg-77"].(map[string]any)
	if ref == nil {
		t.Fatalf("mailbox_wakes did not record the waking message: %+v", status["mailbox_wakes"])
	}
	if ref["correlation_id"] != corr || ref["trigger_subject"] != "board.dm.ops" {
		t.Fatalf("mailbox wake ref = %+v", ref)
	}
}

func TestAgentList_FoldsPolicyDenialsByRunCorrelation(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Ops agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const corr = "ops-run-001"
	// A run owned by ops…
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.ops.task",
		Kind:          event.KindTaskReceived,
		Actor:         "agent-ops",
		CorrelationID: corr,
		Payload:       map[string]any{"agent": "ops", "intent": "do the thing"},
	}); err != nil {
		t.Fatalf("publish task received: %v", err)
	}
	// …has a tool refused by policy (allow=false) and one allowed (allow=true).
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "policy",
		Kind:          event.KindPolicyDecision,
		Actor:         "policy",
		CorrelationID: corr,
		Payload:       map[string]any{"tool": "shell", "capability": "process.exec", "allow": false, "hard_denied": true, "reason": "tool denied for this agent"},
	}); err != nil {
		t.Fatalf("publish policy deny: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "policy",
		Kind:          event.KindPolicyDecision,
		Actor:         "policy",
		CorrelationID: corr,
		Payload:       map[string]any{"tool": "file", "capability": "fs.read", "allow": true},
	}); err != nil {
		t.Fatalf("publish policy allow: %v", err)
	}
	// A denial under an UNRELATED correlation must not count against ops.
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "policy",
		Kind:          event.KindPolicyDecision,
		Actor:         "policy",
		CorrelationID: "stranger-run",
		Payload:       map[string]any{"tool": "shell", "allow": false},
	}); err != nil {
		t.Fatalf("publish stranger deny: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var status map[string]any
	for _, raw := range res["profiles"].([]any) {
		prof, _ := raw.(map[string]any)
		if prof["slug"] == "ops" {
			status, _ = prof["status"].(map[string]any)
			break
		}
	}
	if testIntNumber(status["policy_denied_count"]) != 1 {
		t.Fatalf("policy_denied_count = %v, want 1", status["policy_denied_count"])
	}
	if status["policy_denied_last_tool"] != "shell" ||
		status["policy_denied_last_capability"] != "process.exec" ||
		status["policy_denied_last_hard"] != true ||
		status["policy_denied_last_reason"] != "tool denied for this agent" {
		t.Fatalf("policy denial summary = %+v", status)
	}
}

func TestAgentTombstone_ReturnsIdentityAndFootprint(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Ops agent.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "ops", "reason": "obsolete"}); err != nil {
		t.Fatalf("agent retire: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentTombstone, map[string]any{"ref": "ops"})
	if err != nil {
		t.Fatalf("agent tombstone: %v", err)
	}
	tomb, _ := res["tombstone"].(map[string]any)
	if tomb == nil {
		t.Fatalf("tombstone missing: %+v", res)
	}
	if tomb["slug"] != "ops" || tomb["retired"] != true || tomb["retired_reason"] != "obsolete" {
		t.Fatalf("tombstone identity = %+v", tomb)
	}
	if _, ok := tomb["footprint"].(map[string]any); !ok {
		t.Fatalf("tombstone missing footprint: %+v", tomb)
	}
	// Read-only: the agent still exists in the graveyard after a tombstone read.
	if _, err := c.Call(ctx, controlplane.CmdAgentTombstone, map[string]any{"ref": "ops"}); err != nil {
		t.Fatalf("tombstone is read-only and repeatable, second call failed: %v", err)
	}
}

func TestAgentTombstone_UnknownAgentErrors(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	if _, err := c.Call(context.Background(), controlplane.CmdAgentTombstone, map[string]any{"ref": "ghost"}); err == nil {
		t.Fatal("tombstone for unknown agent should error")
	}
}

func TestAgentGraveyard_ListsOnlyRetired(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	for _, slug := range []string{"live", "dead"} {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
			"profile": map[string]any{"slug": slug, "soul": "x", "model": "m"},
		}); err != nil {
			t.Fatalf("agent add %s: %v", slug, err)
		}
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "dead", "reason": "obsolete"}); err != nil {
		t.Fatalf("retire: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentGraveyard, nil)
	if err != nil {
		t.Fatalf("graveyard: %v", err)
	}
	rows, _ := res["graveyard"].([]any)
	if len(rows) != 1 {
		t.Fatalf("graveyard should list exactly the retired agent, got %d: %+v", len(rows), rows)
	}
	row, _ := rows[0].(map[string]any)
	if row["slug"] != "dead" || row["retired_reason"] != "obsolete" {
		t.Fatalf("graveyard row = %+v", row)
	}
	// A far-future retention window excludes the just-retired agent (reports only).
	res2, err := c.Call(ctx, controlplane.CmdAgentGraveyard, map[string]any{"older_than_days": 3650})
	if err != nil {
		t.Fatalf("graveyard older_than: %v", err)
	}
	if rows2, _ := res2["graveyard"].([]any); len(rows2) != 0 {
		t.Fatalf("older_than_days=3650 should exclude a fresh retirement, got %+v", rows2)
	}
}

func TestAgentKindSubagentNormalizesDirectCallPolicy(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("unused")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "Lead."},
	}); err != nil {
		t.Fatalf("agent add lead: %v", err)
	}
	add, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":         "worker",
			"kind":         "subagent",
			"parent_agent": "lead",
			"soul":         "Do worker tasks.",
		},
	})
	if err != nil {
		t.Fatalf("agent add worker: %v", err)
	}
	prof, _ := add["profile"].(map[string]any)
	if prof["kind"] != "subagent" || prof["managed"] != true || prof["direct_callable"] != false {
		t.Fatalf("kind-only subagent did not normalize direct-call policy: %v", prof)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentWake, map[string]any{
		"ref":    "worker",
		"reason": "manual operator wake",
	}); err == nil || !strings.Contains(err.Error(), "managed sub-agent") {
		t.Fatalf("kind-only subagent wake err = %v, want managed sub-agent rejection", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "assistant", "soul": "Assist directly."},
	}); err != nil {
		t.Fatalf("agent add assistant: %v", err)
	}
	edit, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{
		"ref": "assistant",
		"profile": map[string]any{
			"kind":         "subagent",
			"parent_agent": "lead",
			"soul":         "Assist only through lead.",
		},
	})
	if err != nil {
		t.Fatalf("agent edit assistant to subagent: %v", err)
	}
	edited, _ := edit["profile"].(map[string]any)
	if edited["kind"] != "subagent" || edited["managed"] != true || edited["direct_callable"] != false {
		t.Fatalf("kind-only edit did not normalize direct-call policy: %v", edited)
	}
}

func TestAgentList_IncludesWakeStatus(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "You operate.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent":       "check disks",
		"agent":        "ops",
		"interval_sec": float64(3600),
	}); err != nil {
		t.Fatalf("schedule add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":     "ops events",
			"agent":    "ops",
			"plan":     "triage ops event",
			"triggers": []any{map[string]any{"type": "event", "subject": "ops.alert"}},
		},
	}); err != nil {
		t.Fatalf("standing add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var ops map[string]any
	for _, raw := range res["profiles"].([]any) {
		p := raw.(map[string]any)
		if p["slug"] == "ops" {
			ops = p
			break
		}
	}
	if ops == nil {
		t.Fatalf("ops missing from list: %+v", res)
	}
	st := ops["status"].(map[string]any)
	if testIntNumber(st["wake_schedule_count"]) != 1 || testIntNumber(st["wake_standing_count"]) != 1 {
		t.Fatalf("wake counts = %+v", st)
	}
	if testIntNumber(st["next_wake_ms"]) <= time.Now().UnixMilli() {
		t.Fatalf("next wake missing/not future: %+v", st)
	}
	if st["next_wake_label"] != "check disks" {
		t.Fatalf("next wake label = %v", st["next_wake_label"])
	}
	rawSubjects, _ := st["wake_event_subjects"].([]any)
	if len(rawSubjects) != 1 || rawSubjects[0] != "ops.alert" {
		t.Fatalf("event subjects = %#v", st["wake_event_subjects"])
	}
}

func TestAgentList_IncludesLiveRunStatus(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "You operate.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const corr = "live-run-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "schedule.fired",
		Kind:          event.KindScheduleFired,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload:       map[string]any{"schedule_id": "sched-ops", "intent": "check disks", "target": "intent", "agent": "ops"},
	}); err != nil {
		t.Fatalf("publish schedule fired: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.ops.task",
		Kind:          event.KindTaskReceived,
		Actor:         "agent-ops",
		CorrelationID: corr,
		Payload:       map[string]string{"intent": "check disks", "agent": "ops"},
	}); err != nil {
		t.Fatalf("publish task received: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.ops.tool",
		Kind:          event.KindToolInvoked,
		Actor:         "agent-ops",
		CorrelationID: corr,
		Payload:       map[string]any{"tool": "shell", "iter": 2},
	}); err != nil {
		t.Fatalf("publish tool invoked: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var ops map[string]any
	for _, raw := range res["profiles"].([]any) {
		p := raw.(map[string]any)
		if p["slug"] == "ops" {
			ops = p
			break
		}
	}
	if ops == nil {
		t.Fatalf("ops missing from list: %+v", res)
	}
	st := ops["status"].(map[string]any)
	if testIntNumber(st["active_run_count"]) != 1 {
		t.Fatalf("active run count = %+v", st)
	}
	if st["active_correlation_id"] != corr {
		t.Fatalf("active correlation = %v", st["active_correlation_id"])
	}
	if st["active_intent"] != "check disks" {
		t.Fatalf("active intent = %v", st["active_intent"])
	}
	if testIntNumber(st["active_started_ms"]) <= 0 {
		t.Fatalf("active started missing: %+v", st)
	}
	if st["active_phase"] != "using tool" || st["active_tool"] != "shell" || st["active_detail"] != "shell" {
		t.Fatalf("active phase/tool = %+v", st)
	}
	if st["active_last_event_kind"] != string(event.KindToolInvoked) {
		t.Fatalf("active event kind = %v", st["active_last_event_kind"])
	}
	if testIntNumber(st["active_last_event_ms"]) <= 0 || testIntNumber(st["active_iter"]) != 2 {
		t.Fatalf("active heartbeat fields = %+v", st)
	}
	if st["active_wake_source"] != "schedule" || st["active_wake_reason"] != "intent" || st["active_schedule_id"] != "sched-ops" {
		t.Fatalf("active wake context = %+v", st)
	}
	if st["operational_state"] != "running" || st["operational_label"] != "using tool" {
		t.Fatalf("operational state = %+v", st)
	}
	if testIntNumber(st["last_activity_ms"]) <= 0 || st["last_activity_kind"] != string(event.KindTaskReceived) {
		t.Fatalf("last activity = %+v", st)
	}
	if st["last_activity_summary"] != "started a run: check disks" {
		t.Fatalf("last activity summary = %v", st["last_activity_summary"])
	}
}

func TestAgentList_IncludesStandingWakeNameInLiveRunStatus(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "You operate.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	const corr = "live-standing-001"
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "standing.fired",
		Kind:          event.KindStandingFired,
		Actor:         "standing",
		CorrelationID: corr,
		Payload: map[string]any{
			"id":              "standing-ops-events",
			"name":            "ops events",
			"trigger_subject": "ops.alert",
			"agent":           "ops",
		},
	}); err != nil {
		t.Fatalf("publish standing fired: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "agent.ops.task",
		Kind:          event.KindTaskReceived,
		Actor:         "agent-ops",
		CorrelationID: corr,
		Payload:       map[string]string{"intent": "triage ops event", "agent": "ops"},
	}); err != nil {
		t.Fatalf("publish task received: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	var ops map[string]any
	for _, raw := range res["profiles"].([]any) {
		p := raw.(map[string]any)
		if p["slug"] == "ops" {
			ops = p
			break
		}
	}
	if ops == nil {
		t.Fatalf("ops missing from list: %+v", res)
	}
	st := ops["status"].(map[string]any)
	if st["active_wake_source"] != "standing" || st["active_wake_reason"] != "event" {
		t.Fatalf("active standing wake source = %+v", st)
	}
	if st["active_standing_id"] != "standing-ops-events" || st["active_standing_name"] != "ops events" {
		t.Fatalf("active standing identity = %+v", st)
	}
	if st["active_trigger_subject"] != "ops.alert" {
		t.Fatalf("active trigger subject = %+v", st)
	}
}

func TestAgentList_IncludesSleepingAndPausedOperationalState(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "sleeper", "soul": "You wait.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add sleeper: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "paused", "soul": "You wait.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add paused: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{
		"ref":     "paused",
		"enabled": false,
	}); err != nil {
		t.Fatalf("agent pause: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	statusBySlug := map[string]map[string]any{}
	for _, raw := range res["profiles"].([]any) {
		p := raw.(map[string]any)
		statusBySlug[p["slug"].(string)] = p["status"].(map[string]any)
	}
	if statusBySlug["sleeper"]["operational_state"] != "sleeping" || statusBySlug["sleeper"]["operational_label"] != "sleeping" {
		t.Fatalf("sleeper state = %+v", statusBySlug["sleeper"])
	}
	if statusBySlug["paused"]["operational_state"] != "paused" || statusBySlug["paused"]["operational_label"] != "paused" {
		t.Fatalf("paused state = %+v", statusBySlug["paused"])
	}
}

func testIntNumber(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func TestAgentResolve_PausesAndJournalsLifecycle(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "builder", "soul": "You build.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":              "builder",
		"resolution":       "paused",
		"summary":          "pause exhausted routing incident",
		"incident_id":      "inc-child-3",
		"root_incident_id": "inc-root-3",
	})
	if err != nil {
		t.Fatalf("agent resolve: %v", err)
	}
	if res["applied"] != true || res["agent"] != "builder" || res["resolution"] != "paused" {
		t.Fatalf("resolve result = %+v", res)
	}
	corr, _ := res["correlation_id"].(string)
	if corr == "" {
		t.Fatalf("resolve correlation missing: %+v", res)
	}
	var sawRequested, sawCompleted bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "agent.resolve" || e.CorrelationID != corr {
			return nil
		}
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		switch pl["phase"] {
		case "requested":
			sawRequested = true
		case "completed":
			sawCompleted = true
			if pl["root_incident_id"] != "inc-root-3" || pl["resolution"] != "paused" {
				t.Fatalf("resolve payload missing lineage: %+v", pl)
			}
		}
		return nil
	})
	if !sawRequested || !sawCompleted {
		t.Fatalf("resolve lifecycle incomplete: requested=%v completed=%v", sawRequested, sawCompleted)
	}
	got, ok := k.Roster().Get("builder")
	if !ok || got.Enabled {
		t.Fatalf("profile after resolve = %+v", got)
	}
}

func TestAgentResolve_DelegatesViaBoard(t *testing.T) {
	_, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	st, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	srv.SetBoard(st, nil)
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "builder", "soul": "You build.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "infra-lead", "soul": "You own infra incidents.", "model": "m"},
	}); err != nil {
		t.Fatalf("infra-lead add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":              "builder",
		"resolution":       "delegated",
		"delegate_to":      "infra-lead",
		"summary":          "delegate exhausted routing incident",
		"incident_id":      "inc-child-4",
		"root_incident_id": "inc-root-4",
	})
	if err != nil {
		t.Fatalf("agent resolve delegated: %v", err)
	}
	if res["applied"] != true || res["resolution"] != "delegated" {
		t.Fatalf("delegated resolve result = %+v", res)
	}
	msgs := st.OpenHelp(10)
	if len(msgs) != 1 || msgs[0].To != "infra-lead" || !strings.Contains(msgs[0].Text, "delegate exhausted routing incident") {
		t.Fatalf("open help = %+v", msgs)
	}
}

func TestAgentResolve_DelegatedRejectsSelfAndCurrentOwner(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "You lead.", "model": "m"},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":         "builder",
			"soul":         "You build.",
			"model":        "m",
			"parent_agent": "lead",
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":         "builder",
		"resolution":  "delegated",
		"delegate_to": "builder",
	}); err == nil || !strings.Contains(err.Error(), "points back to the root agent") {
		t.Fatalf("delegate to self err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":         "builder",
		"resolution":  "delegated",
		"delegate_to": "lead",
	}); err == nil || !strings.Contains(err.Error(), "points back to the current owner") {
		t.Fatalf("delegate to owner err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":         "builder",
		"resolution":  "delegated",
		"delegate_to": "ghost",
	}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("delegate to unknown err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "graveyard-owner", "soul": "You are retired.", "model": "m"},
	}); err != nil {
		t.Fatalf("graveyard-owner add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{
		"ref": "graveyard-owner",
	}); err != nil {
		t.Fatalf("graveyard-owner retire: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":         "builder",
		"resolution":  "delegated",
		"delegate_to": "graveyard-owner",
	}); err == nil || !strings.Contains(err.Error(), "is retired") {
		t.Fatalf("delegate to retired err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":            "worker-owner",
			"soul":            "You are managed.",
			"model":           "m",
			"parent_agent":    "lead",
			"direct_callable": false,
		},
	}); err != nil {
		t.Fatalf("worker-owner add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":         "builder",
		"resolution":  "delegated",
		"delegate_to": "worker-owner",
	}); err == nil || !strings.Contains(err.Error(), "managed sub-agent") {
		t.Fatalf("delegate to managed err = %v", err)
	}
}

func TestAgentResolve_ForceChainUpdatesRoutingAndGeneration(t *testing.T) {
	prov := newRosterRoutingProvider(mock.New(mock.FinalText("ok")), map[string][]string{
		"code": {"gpt-5", "gpt-4.1"},
	})
	k, _, c, _ := startPair(t, prov)
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "builder", "soul": "You build.", "model": "m", "task_type": "code"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":              "builder",
		"resolution":       "force_chain",
		"task_type":        "code",
		"task_model_chain": []any{"gpt-4.1", "deepseek-chat"},
		"summary":          "force a new stable code chain",
		"incident_id":      "inc-child-5",
		"root_incident_id": "inc-root-5",
	})
	if err != nil {
		t.Fatalf("agent resolve force_chain: %v", err)
	}
	if res["applied"] != true || res["resolution"] != "force_chain" {
		t.Fatalf("force resolve result = %+v", res)
	}
	if got := prov.TaskModelChainsView()["code"]; strings.Join(got, ",") != "gpt-4.1,deepseek-chat" {
		t.Fatalf("routing chain = %v", got)
	}
	corr, _ := res["correlation_id"].(string)
	var sawCompleted bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "agent.resolve" || e.CorrelationID != corr {
			return nil
		}
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		if pl["phase"] == "completed" {
			sawCompleted = true
			if pl["routing_task_type"] != "code" || pl["routing_force_generation"] != float64(1) {
				t.Fatalf("force resolve payload = %+v", pl)
			}
		}
		return nil
	})
	if !sawCompleted {
		t.Fatal("force resolve missing completed event")
	}
}

func TestAgentResolve_ForceChainRejectsExhaustedChain(t *testing.T) {
	prov := newRosterRoutingProvider(mock.New(mock.FinalText("ok")), map[string][]string{
		"code": {"gpt-5", "gpt-4.1"},
	})
	k, _, c, _ := startPair(t, prov)
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "builder", "soul": "You build.", "model": "m", "task_type": "code"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"phase":                    "routing_force_exhausted_detected",
			"agent":                    "builder",
			"mode":                     "routing_forced_exhausted",
			"routing_task_type":        "code",
			"routing_task_model_chain": []string{"gpt-5", "gpt-4.1"},
			"incident_id":              "inc-child-6",
			"root_incident_id":         "inc-root-6",
		},
	}); err != nil {
		t.Fatalf("publish exhausted routing: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentResolve, map[string]any{
		"ref":              "builder",
		"resolution":       "force_chain",
		"task_type":        "code",
		"task_model_chain": []any{"gpt-5", "gpt-4.1"},
		"incident_id":      "inc-child-6",
		"root_incident_id": "inc-root-6",
	}); err == nil || !strings.Contains(err.Error(), "must choose a new chain") {
		t.Fatalf("force exhausted chain err = %v", err)
	}
	if got := prov.TaskModelChainsView()["code"]; strings.Join(got, ",") != "gpt-5,gpt-4.1" {
		t.Fatalf("routing chain mutated on rejected force_chain: %v", got)
	}
}

type rosterRoutingProvider struct {
	agent.Provider
	mu     sync.Mutex
	chains map[string][]string
}

func newRosterRoutingProvider(base agent.Provider, chains map[string][]string) *rosterRoutingProvider {
	cp := make(map[string][]string, len(chains))
	for k, v := range chains {
		cp[k] = append([]string(nil), v...)
	}
	return &rosterRoutingProvider{Provider: base, chains: cp}
}

func (p *rosterRoutingProvider) TaskModelChainsView() map[string][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string][]string, len(p.chains))
	for k, v := range p.chains {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func (p *rosterRoutingProvider) SetTaskModelChains(chains map[string][]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chains = make(map[string][]string, len(chains))
	for k, v := range chains {
		p.chains[k] = append([]string(nil), v...)
	}
}

func TestAgentEscalations_ShowsOpenDoctorResponsibilities(t *testing.T) {
	k, srv, c, dir := startPair(t, mock.New(mock.FinalText("done")))
	st, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	srv.SetBoard(st, nil)
	ctx := context.Background()
	for _, slug := range []string{"builder", "lead"} {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
			"profile": map[string]any{"slug": slug, "soul": "You work."},
		}); err != nil {
			t.Fatalf("agent add %s: %v", slug, err)
		}
	}
	sent, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from": "guardian-doctor",
		"to":   "lead",
		"help": true,
		"text": "Doctor recovery failed for agent builder. Reason: degraded.",
	})
	if err != nil {
		t.Fatalf("board send: %v", err)
	}
	msg, _ := sent["sent"].(map[string]any)
	msgID, _ := msg["id"].(string)
	if msgID == "" {
		t.Fatalf("board send missing id: %+v", sent)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"phase":              "escalation_woke",
			"agent":              "builder",
			"mode":               "degraded",
			"target_agent":       "lead",
			"target_correlation": "corr-owner-1",
			"mailbox_message_id": msgID,
			"fingerprint":        "fp-esc-1",
			"root_agent":         "builder",
			"chain_depth":        0,
			"incident_id":        "inc-root-1",
			"root_incident_id":   "inc-root-1",
		},
	}); err != nil {
		t.Fatalf("publish escalation: %v", err)
	}

	res, err := c.Call(ctx, controlplane.CmdAgentEscalations, map[string]any{"ref": "lead", "limit": 10})
	if err != nil {
		t.Fatalf("agent escalations: %v", err)
	}
	if res["open_count"] != float64(1) {
		t.Fatalf("open_count = %v, want 1", res["open_count"])
	}
	rows, _ := res["escalations"].([]any)
	if len(rows) != 1 {
		t.Fatalf("escalations len = %d, want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if row["source_agent"] != "builder" || row["wake_phase"] != "escalation_woke" || row["wake_correlation_id"] != "corr-owner-1" {
		t.Fatalf("escalation row = %+v", row)
	}
	if row["origin_kind"] != "doctor" || row["origin_agent"] != "guardian-doctor" {
		t.Fatalf("escalation origin = %+v", row)
	}
	if row["root_agent"] != "builder" || row["chain_depth"] != float64(0) {
		t.Fatalf("escalation chain = %+v", row)
	}
	if row["incident_id"] != "inc-root-1" || row["root_incident_id"] != "inc-root-1" {
		t.Fatalf("escalation incident ids = %+v", row)
	}

	act, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{"ref": "lead", "limit": 10})
	if err != nil {
		t.Fatalf("agent activity: %v", err)
	}
	acts, _ := act["activity"].([]any)
	found := false
	for _, raw := range acts {
		item, _ := raw.(map[string]any)
		if summary, _ := item["summary"].(string); strings.Contains(summary, "accepted escalation wake for builder") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("lead activity missing escalation wake: %+v", acts)
	}

	// Acknowledge the help request and make sure the row downgrades from open → acked.
	if _, err := c.Call(ctx, controlplane.CmdBoardAck, map[string]any{"id": msgID, "by": "lead"}); err != nil {
		t.Fatalf("board ack: %v", err)
	}
	res2, err := c.Call(ctx, controlplane.CmdAgentEscalations, map[string]any{"ref": "lead", "limit": 10})
	if err != nil {
		t.Fatalf("agent escalations after ack: %v", err)
	}
	rows2, _ := res2["escalations"].([]any)
	row2, _ := rows2[0].(map[string]any)
	if row2["status"] != "acked" || row2["acked"] != true {
		t.Fatalf("acked escalation row = %+v", row2)
	}

	// A threaded reply closes the escalation, but it should remain visible as answered history.
	if _, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from":     "lead",
		"reply_to": msgID,
		"text":     "I inspected builder and handled it.",
	}); err != nil {
		t.Fatalf("board reply: %v", err)
	}
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"phase":              "escalation_answered",
			"agent":              "builder",
			"mode":               "degraded",
			"target_agent":       "lead",
			"target_correlation": "corr-owner-1",
			"mailbox_message_id": msgID,
			"fingerprint":        "fp-esc-1",
			"resolution":         "delegated",
			"resolution_summary": "needs infra owner review",
			"delegate_to":        "infra-lead",
			"root_agent":         "builder",
			"chain_depth":        0,
			"incident_id":        "inc-root-1",
			"root_incident_id":   "inc-root-1",
		},
	}); err != nil {
		t.Fatalf("publish escalation answered: %v", err)
	}
	res3, err := c.Call(ctx, controlplane.CmdAgentEscalations, map[string]any{"ref": "lead", "limit": 10})
	if err != nil {
		t.Fatalf("agent escalations after reply: %v", err)
	}
	rows3, _ := res3["escalations"].([]any)
	row3, _ := rows3[0].(map[string]any)
	if row3["status"] != "answered" || row3["wake_phase"] != "escalation_answered" {
		t.Fatalf("answered escalation row = %+v", row3)
	}
	if row3["resolution"] != "delegated" || row3["resolution_summary"] != "needs infra owner review" || row3["delegate_to"] != "infra-lead" {
		t.Fatalf("answered escalation resolution = %+v", row3)
	}
	if row3["incident_id"] != "inc-root-1" || row3["root_incident_id"] != "inc-root-1" {
		t.Fatalf("answered escalation incident ids = %+v", row3)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "infra-lead", "soul": "You own infra escalations."},
	}); err != nil {
		t.Fatalf("agent add infra-lead: %v", err)
	}
	delegatedMsg, err := c.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from": "lead",
		"to":   "infra-lead",
		"text": "Escalated responsibility for agent builder.",
		"help": true,
	})
	if err != nil {
		t.Fatalf("delegated board send: %v", err)
	}
	delegatedSent, _ := delegatedMsg["sent"].(map[string]any)
	delegatedID, _ := delegatedSent["id"].(string)
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"phase":              "delegation_queued",
			"agent":              "builder",
			"mode":               "degraded",
			"target_agent":       "infra-lead",
			"delegated_by":       "lead",
			"root_agent":         "builder",
			"chain_depth":        1,
			"target_correlation": "corr-owner-2",
			"mailbox_message_id": delegatedID,
			"fingerprint":        "fp-esc-2",
			"resolution":         "delegated",
			"resolution_summary": "needs infra owner review",
			"delegate_to":        "infra-lead",
			"incident_id":        "inc-child-1",
			"root_incident_id":   "inc-root-1",
			"parent_incident_id": "inc-root-1",
		},
	}); err != nil {
		t.Fatalf("publish delegation queued: %v", err)
	}
	infra, err := c.Call(ctx, controlplane.CmdAgentEscalations, map[string]any{"ref": "infra-lead", "limit": 10})
	if err != nil {
		t.Fatalf("infra escalations: %v", err)
	}
	infraRows, _ := infra["escalations"].([]any)
	if len(infraRows) != 1 {
		t.Fatalf("infra escalations len = %d, want 1", len(infraRows))
	}
	infraRow, _ := infraRows[0].(map[string]any)
	if infraRow["origin_kind"] != "delegated" || infraRow["origin_agent"] != "lead" {
		t.Fatalf("infra escalation origin = %+v", infraRow)
	}
	if infraRow["root_agent"] != "builder" || infraRow["chain_depth"] != float64(1) {
		t.Fatalf("infra escalation chain = %+v", infraRow)
	}
	if infraRow["incident_id"] != "inc-child-1" || infraRow["root_incident_id"] != "inc-root-1" || infraRow["parent_incident_id"] != "inc-root-1" {
		t.Fatalf("infra escalation incident ids = %+v", infraRow)
	}

	// Directed help should stay private; an unrelated agent sees nothing.
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "writer", "soul": "You write."},
	}); err != nil {
		t.Fatalf("agent add writer: %v", err)
	}
	other, err := c.Call(ctx, controlplane.CmdAgentEscalations, map[string]any{"ref": "writer"})
	if err != nil {
		t.Fatalf("writer escalations: %v", err)
	}
	if rows, _ := other["escalations"].([]any); len(rows) != 0 {
		t.Fatalf("writer should not see lead's escalation, got %+v", rows)
	}
}

// addAgentWithCreatedMS was an early-attempt helper that turned out
// unnecessary: roster.Add sets CreatedMS via time.Now().UnixMilli() but the
// (CreatedMS, Slug) cursor breaks ties on slug, so back-to-back adds in the
// same ms still paginate deterministically.

func TestAgentList_CursorPaginatesByCreatedMSSlugDesc(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Five agents. Add them sequentially; CreatedMS may collide on the same
	// millisecond, but the slug tie-break in the cursor keeps ordering
	// deterministic.
	slugs := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	for _, s := range slugs {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
			"profile": map[string]any{"slug": s, "model": "mock-model", "task_type": "code"},
		}); err != nil {
			t.Fatalf("agent add %s: %v", s, err)
		}
	}

	// Page 1 — limit 2, no cursor. Expect next_cursor present (more pages).
	p1, err := c.Call(ctx, controlplane.CmdAgentList, map[string]any{"limit": 2})
	if err != nil {
		t.Fatalf("agent list p1: %v", err)
	}
	p1Profiles, _ := p1["profiles"].([]any)
	if len(p1Profiles) != 2 {
		t.Fatalf("page 1 should have 2 profiles, got %d", len(p1Profiles))
	}
	if p1["next_cursor"] == "" || p1["next_cursor"] == nil {
		t.Fatal("page 1 should have next_cursor (more pages exist)")
	}
	if intOf(p1["count"]) != 2 {
		t.Fatalf("page 1 count wrong: %v", p1["count"])
	}
	if intOf(p1["total"]) != 5 {
		t.Fatalf("page 1 total wrong: %v", p1["total"])
	}

	// Page 2 — limit 2 with cursor from page 1.
	p2, err := c.Call(ctx, controlplane.CmdAgentList, map[string]any{
		"limit":  2,
		"cursor": p1["next_cursor"],
	})
	if err != nil {
		t.Fatalf("agent list p2: %v", err)
	}
	p2Profiles, _ := p2["profiles"].([]any)
	if len(p2Profiles) != 2 {
		t.Fatalf("page 2 should have 2 profiles, got %d", len(p2Profiles))
	}
	if p2["next_cursor"] == "" || p2["next_cursor"] == nil {
		t.Fatal("page 2 should have next_cursor (one more page exists)")
	}
	// No duplicate slugs across page 1 and page 2.
	p1Set := map[string]bool{}
	for _, raw := range p1Profiles {
		if v, _ := raw.(map[string]any); v != nil {
			p1Set[v["slug"].(string)] = true
		}
	}
	for _, raw := range p2Profiles {
		if v, _ := raw.(map[string]any); v != nil {
			if p1Set[v["slug"].(string)] {
				t.Fatalf("slug %s appeared on both pages", v["slug"])
			}
		}
	}

	// Page 3 — terminal. One agent left, no next_cursor.
	p3, err := c.Call(ctx, controlplane.CmdAgentList, map[string]any{
		"limit":  2,
		"cursor": p2["next_cursor"],
	})
	if err != nil {
		t.Fatalf("agent list p3: %v", err)
	}
	p3Profiles, _ := p3["profiles"].([]any)
	if len(p3Profiles) != 1 {
		t.Fatalf("page 3 should have 1 profile, got %d", len(p3Profiles))
	}
	if _, hasNext := p3["next_cursor"]; hasNext {
		t.Fatalf("page 3 should NOT have next_cursor, got %v", p3["next_cursor"])
	}

	// Combined: all 5 distinct slugs across the three pages.
	seen := map[string]bool{}
	for _, raw := range append(append(p1Profiles, p2Profiles...), p3Profiles...) {
		if v, _ := raw.(map[string]any); v != nil {
			seen[v["slug"].(string)] = true
		}
	}
	for _, s := range slugs {
		if !seen[s] {
			t.Fatalf("slug %s missing from paginated union", s)
		}
	}
}

func TestAgentList_DescendingOrderAcrossPages(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	for _, s := range []string{"alpha", "bravo", "charlie"} {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
			"profile": map[string]any{"slug": s, "model": "mock-model", "task_type": "code"},
		}); err != nil {
			t.Fatalf("agent add %s: %v", s, err)
		}
	}

	// Single page of 3 — expect DESC order: (latest CreatedMS first; within
	// the same ms, DESC by slug). Because all three were added back-to-back,
	// they likely share CreatedMS, so DESC by slug: charlie, bravo, alpha.
	res, err := c.Call(ctx, controlplane.CmdAgentList, map[string]any{"limit": 3})
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	profiles, _ := res["profiles"].([]any)
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	gotSlugs := []string{}
	for _, raw := range profiles {
		if v, _ := raw.(map[string]any); v != nil {
			gotSlugs = append(gotSlugs, v["slug"].(string))
		}
	}
	// DESC by slug when CreatedMS is equal (within-ms ordering is DESC by
	// slug lexicographically).
	want := []string{"charlie", "bravo", "alpha"}
	for i := range want {
		if gotSlugs[i] != want[i] {
			t.Fatalf("order: got %v want %v", gotSlugs, want)
		}
	}
}

func TestAgentList_UnparseableCursorFallsBackToFirstPage(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	for _, s := range []string{"alpha", "bravo"} {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
			"profile": map[string]any{"slug": s, "model": "mock-model", "task_type": "code"},
		}); err != nil {
			t.Fatalf("agent add: %v", err)
		}
	}
	// Garbage cursor — server should treat as no cursor, return first page.
	res, err := c.Call(ctx, controlplane.CmdAgentList, map[string]any{
		"limit":  10,
		"cursor": "not-a-cursor",
	})
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	profiles, _ := res["profiles"].([]any)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestAgentList_LimitZeroReturnsAll(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	for _, s := range []string{"alpha", "bravo", "charlie"} {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
			"profile": map[string]any{"slug": s, "model": "mock-model", "task_type": "code"},
		}); err != nil {
			t.Fatalf("agent add: %v", err)
		}
	}
	// limit=0 (or absent) — return everything; no next_cursor.
	res, err := c.Call(ctx, controlplane.CmdAgentList, map[string]any{"limit": 0})
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	profiles, _ := res["profiles"].([]any)
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	if _, has := res["next_cursor"]; has {
		t.Fatalf("limit=0 should not emit next_cursor, got %v", res["next_cursor"])
	}
	if intOf(res["total"]) != 3 {
		t.Fatalf("total wrong: %v", res["total"])
	}
}

// publishActivityEvents seeds the journal with N doctor.auto_repair events
// attributable to slug, returning their assigned seqs in the order they were
// published (oldest first). Backs the cursor pagination tests for
// handleAgentActivity / handleAgentRepairStatus / handleAgentEscalations —
// all three handlers attribute events by their journal subject + agent slug,
// and doctor.auto_repair is the broadest "this happened to <agent>" signal.
func publishActivityEvents(t *testing.T, k *runtime.Kernel, slug string, n int) []int64 {
	t.Helper()
	seqs := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		ev, err := k.Bus().Publish(event.Spec{
			Subject: "doctor.auto_repair",
			Kind:    event.KindInfo,
			Actor:   "kernel",
			Payload: map[string]any{
				"agent":       slug,
				"fingerprint": "fp-" + strconv.Itoa(i),
				"phase":       "completed",
				"reason":      "test event " + strconv.Itoa(i),
			},
		})
		if err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
		if ev != nil {
			seqs = append(seqs, ev.Seq)
		}
	}
	return seqs
}

func TestAgentActivity_CursorPaginatesBySeqDesc(t *testing.T) {
	k, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "audited", "model": "mock-model", "task_type": "code"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	publishActivityEvents(t, k, "audited", 5)
	_ = srv

	// Page 1 — limit 2, no cursor.
	p1, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{
		"ref": "audited", "limit": 2,
	})
	if err != nil {
		t.Fatalf("agent activity p1: %v", err)
	}
	activity1, _ := p1["activity"].([]any)
	if len(activity1) != 2 {
		t.Fatalf("page 1 should have 2 activity items, got %d", len(activity1))
	}
	if p1["next_cursor"] == "" || p1["next_cursor"] == nil {
		t.Fatal("page 1 should have next_cursor")
	}
	if intOf(p1["total"]) < 5 {
		t.Fatalf("page 1 total should be >= 5, got %v", p1["total"])
	}

	// Page 2 — limit 2 with cursor from page 1.
	p2, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{
		"ref": "audited", "limit": 2, "cursor": p1["next_cursor"],
	})
	if err != nil {
		t.Fatalf("agent activity p2: %v", err)
	}
	activity2, _ := p2["activity"].([]any)
	if len(activity2) != 2 {
		t.Fatalf("page 2 should have 2 activity items, got %d", len(activity2))
	}
	// No duplicate seqs across page 1 and page 2.
	seen := map[int64]bool{}
	for _, raw := range activity1 {
		if v, _ := raw.(map[string]any); v != nil {
			seen[int64(intOf(v["seq"]))] = true
		}
	}
	for _, raw := range activity2 {
		if v, _ := raw.(map[string]any); v != nil {
			seq := int64(intOf(v["seq"]))
			if seen[seq] {
				t.Fatalf("seq %d appeared on both pages", seq)
			}
		}
	}

	// Page 3 — terminal; no next_cursor.
	p3, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{
		"ref": "audited", "limit": 2, "cursor": p2["next_cursor"],
	})
	if err != nil {
		t.Fatalf("agent activity p3: %v", err)
	}
	activity3, _ := p3["activity"].([]any)
	if len(activity3) < 1 {
		t.Fatalf("page 3 should have at least 1 activity item, got %d", len(activity3))
	}
	if _, has := p3["next_cursor"]; has {
		t.Fatalf("page 3 should not have next_cursor, got %v", p3["next_cursor"])
	}
}

func TestAgentActivity_UnparseableCursorFallsBackToFirstPage(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "audited", "model": "mock-model", "task_type": "code"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	publishActivityEvents(t, k, "audited", 2)
	res, err := c.Call(ctx, controlplane.CmdAgentActivity, map[string]any{
		"ref": "audited", "limit": 10, "cursor": "garbage",
	})
	if err != nil {
		t.Fatalf("agent activity: %v", err)
	}
	activity, _ := res["activity"].([]any)
	if len(activity) != 2 {
		t.Fatalf("expected 2 activity items, got %d", len(activity))
	}
}

func TestAgentRepairStatus_CursorPaginatesBySeqDesc(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("repair complete")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "audited", "model": "mock-model", "task_type": "code"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	// Publish 5 doctor.auto_repair events.
	for i := 0; i < 5; i++ {
		_, err := k.Bus().Publish(event.Spec{
			Subject: "doctor.auto_repair",
			Kind:    event.KindInfo,
			Actor:   "kernel",
			Payload: map[string]any{
				"agent":       "audited",
				"fingerprint": "fp-" + strconv.Itoa(i),
				"phase":       "completed",
				"reason":      "test event " + strconv.Itoa(i),
			},
		})
		if err != nil {
			t.Fatalf("publish repair %d: %v", i, err)
		}
	}

	p1, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{
		"ref": "audited", "limit": 2,
	})
	if err != nil {
		t.Fatalf("agent repair status p1: %v", err)
	}
	history1, _ := p1["history"].([]any)
	if len(history1) != 2 {
		t.Fatalf("page 1 history should have 2, got %d", len(history1))
	}
	if p1["next_cursor"] == "" || p1["next_cursor"] == nil {
		t.Fatal("page 1 should have next_cursor")
	}

	p2, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{
		"ref": "audited", "limit": 2, "cursor": p1["next_cursor"],
	})
	if err != nil {
		t.Fatalf("agent repair status p2: %v", err)
	}
	history2, _ := p2["history"].([]any)
	if len(history2) != 2 {
		t.Fatalf("page 2 history should have 2, got %d", len(history2))
	}

	// No duplicate seqs.
	seen := map[int64]bool{}
	for _, raw := range history1 {
		if v, _ := raw.(map[string]any); v != nil {
			seen[int64(intOf(v["seq"]))] = true
		}
	}
	for _, raw := range history2 {
		if v, _ := raw.(map[string]any); v != nil {
			seq := int64(intOf(v["seq"]))
			if seen[seq] {
				t.Fatalf("seq %d appeared on both pages", seq)
			}
		}
	}

	// Page 3 — terminal.
	p3, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{
		"ref": "audited", "limit": 2, "cursor": p2["next_cursor"],
	})
	if err != nil {
		t.Fatalf("agent repair status p3: %v", err)
	}
	history3, _ := p3["history"].([]any)
	if len(history3) != 1 {
		t.Fatalf("page 3 history should have 1, got %d", len(history3))
	}
	if _, has := p3["next_cursor"]; has {
		t.Fatalf("page 3 should not have next_cursor, got %v", p3["next_cursor"])
	}
}

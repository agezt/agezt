// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
)

// fakeStore records calls so the tool's mapping (op → store method + args) is
// asserted without a real on-disk schedule store.
type fakeStore struct {
	added               []cadence.Entry
	lastOnce            time.Time
	lastIntv            time.Duration
	lastAtMin, lastDays int
	removed             string
	removeOK            bool
	entries             []cadence.Entry
	lastAssureID        string
	lastAssureN         int
	lastAgentID         string
	lastAgent           string
	lastWorkflowID      string
	lastWorkflow        string
	lastSystemID        string
	lastSystem          string
	lastToolID          string
	lastTool            string
	lastPayload         json.RawMessage
}

func (f *fakeStore) Add(intent string, interval time.Duration, model, source string, now time.Time) (cadence.Entry, error) {
	f.lastIntv = interval
	e := cadence.Entry{ID: "ev1", Intent: intent, Mode: cadence.ModeInterval, IntervalSec: int64(interval / time.Second), Model: strings.TrimSpace(model), Source: source, Enabled: true, NextRunUnix: now.Add(interval).Unix()}
	f.added = append(f.added, e)
	return e, nil
}
func (f *fakeStore) AddDaily(intent string, atMinutes, days int, tz, model, source string, now time.Time) (cadence.Entry, error) {
	f.lastAtMin, f.lastDays = atMinutes, days
	e := cadence.Entry{ID: "day1", Intent: intent, Mode: cadence.ModeDaily, AtMinutes: atMinutes, Days: days, Model: strings.TrimSpace(model), Source: source, Enabled: true}
	f.added = append(f.added, e)
	return e, nil
}
func (f *fakeStore) AddOnce(intent string, at time.Time, model, source string, now time.Time) (cadence.Entry, error) {
	f.lastOnce = at
	e := cadence.Entry{ID: "once1", Intent: intent, Mode: cadence.ModeOnce, Model: strings.TrimSpace(model), Source: source, Enabled: true, NextRunUnix: at.Unix()}
	f.added = append(f.added, e)
	return e, nil
}
func (f *fakeStore) AddContinuous(intent string, cooldown time.Duration, model, source string, now time.Time) (cadence.Entry, error) {
	f.lastIntv = cooldown
	e := cadence.Entry{ID: "cont1", Intent: intent, Mode: cadence.ModeContinuous, IntervalSec: int64(cooldown / time.Second), Model: strings.TrimSpace(model), Source: source, Enabled: true, NextRunUnix: now.Unix()}
	f.added = append(f.added, e)
	return e, nil
}
func (f *fakeStore) Remove(id string) (bool, error) {
	f.removed = id
	for i := range f.added {
		if f.added[i].ID == id {
			f.added = append(append([]cadence.Entry{}, f.added[:i]...), f.added[i+1:]...)
			return true, nil
		}
	}
	return f.removeOK, nil
}
func (f *fakeStore) List() []cadence.Entry { return f.entries }

func (f *fakeStore) SetAssure(id string, n int) (bool, error) {
	f.lastAssureID, f.lastAssureN = id, n
	for i := range f.added {
		if f.added[i].ID == id {
			f.added[i].Assure = n
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeStore) SetAgent(id, slug string) (bool, error) {
	f.lastAgentID, f.lastAgent = id, slug
	for i := range f.added {
		if f.added[i].ID == id {
			f.added[i].Agent = slug
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeStore) SetWorkflowTarget(id, ref string, payload json.RawMessage) (bool, error) {
	f.lastWorkflowID, f.lastWorkflow, f.lastPayload = id, ref, append(json.RawMessage(nil), payload...)
	for i := range f.added {
		if f.added[i].ID == id {
			f.added[i].Target, f.added[i].Workflow, f.added[i].Payload = cadence.TargetWorkflow, ref, payload
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeStore) SetSystemTaskTarget(id, task string) (bool, error) {
	f.lastSystemID, f.lastSystem = id, task
	for i := range f.added {
		if f.added[i].ID == id {
			f.added[i].Target, f.added[i].SystemTask = cadence.TargetSystemTask, task
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeStore) SetToolTarget(id, tool string, payload json.RawMessage) (bool, error) {
	f.lastToolID, f.lastTool, f.lastPayload = id, tool, append(json.RawMessage(nil), payload...)
	for i := range f.added {
		if f.added[i].ID == id {
			f.added[i].Target, f.added[i].Tool, f.added[i].Payload = cadence.TargetTool, tool, payload
			return true, nil
		}
	}
	return false, nil
}

// fixedNow pins the clock so the one-shot's absolute fire time is assertable.
var fixedNow = time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

func newTool(f *fakeStore) *Tool {
	t := New()
	t.store = f
	t.now = func() time.Time { return fixedNow }
	return t
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	return invokeCtx(t, context.Background(), tool, in)
}

func invokeCtx(t *testing.T, ctx context.Context, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	return out, res.IsError
}

func TestDefinitionValid(t *testing.T) {
	d := New().Definition()
	if d.Name != "schedule" {
		t.Fatalf("name = %q", d.Name)
	}
	if !json.Valid(d.InputSchema) {
		t.Fatal("schema invalid")
	}
	if !strings.Contains(d.Description, "typed targets") || !strings.Contains(d.Description, "instead of embedding execution instructions") {
		t.Fatalf("description should steer agents toward typed cron targets, got %q", d.Description)
	}
}

func TestOpIn_OneShotAtDelay(t *testing.T) {
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "in", "delay": "30m", "intent": "check the deploy"})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if len(f.added) != 1 || f.added[0].Intent != "check the deploy" {
		t.Fatalf("AddOnce not called as expected: %+v", f.added)
	}
	if f.added[0].Source != "agent" {
		t.Errorf("source = %q, want agent", f.added[0].Source)
	}
	if want := fixedNow.Add(30 * time.Minute); !f.lastOnce.Equal(want) {
		t.Errorf("fire time = %v, want now+30m = %v", f.lastOnce, want)
	}
}

func TestOpEvery_Interval(t *testing.T) {
	f := &fakeStore{}
	_, isErr := invoke(t, newTool(f), map[string]any{"op": "every", "interval": "1h", "intent": "hourly digest"})
	if isErr {
		t.Fatal("unexpected error")
	}
	if f.lastIntv != time.Hour {
		t.Errorf("interval = %v, want 1h", f.lastIntv)
	}
}

func TestAgentTargetRequiresTaskButTypedTargetCanBeLabeless(t *testing.T) {
	f := &fakeStore{}
	raw, _ := json.Marshal(map[string]any{"op": "every", "interval": "1h", "target": "agent"})
	res, err := newTool(f).Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("empty agent task should error: %+v", res)
	}
	if res.Output != "schedule: target=agent needs agent task text in the intent field" {
		t.Fatalf("error = %q", res.Output)
	}

	out, isErr := invoke(t, newTool(f), map[string]any{
		"op":          "every",
		"interval":    "1h",
		"target":      "system_task",
		"system_task": cadence.SystemTaskMemoryClean,
	})
	if isErr {
		t.Fatalf("typed target should not need an intent label: %+v", out)
	}
	if out["target"] != "system_task" || out["system_task"] != cadence.SystemTaskMemoryClean {
		t.Fatalf("entry view missing system task: %+v", out)
	}
}

func TestOpContinuous_Loop(t *testing.T) {
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "continuous", "cooldown": "30s", "intent": "watch the world"})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if len(f.added) != 1 || f.added[0].Mode != cadence.ModeContinuous {
		t.Fatalf("AddContinuous not called: %+v", f.added)
	}
	if f.lastIntv != 30*time.Second {
		t.Errorf("cooldown = %v, want 30s", f.lastIntv)
	}
	if f.added[0].Source != "agent" {
		t.Errorf("source = %q, want agent", f.added[0].Source)
	}
}

func TestAssure_StampedOnCreate(t *testing.T) {
	// op=continuous with assure=2 must SetAssure on the created entry (M654).
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "continuous", "cooldown": "30s", "intent": "be sure", "assure": 2})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.lastAssureID != f.added[0].ID || f.lastAssureN != 2 {
		t.Errorf("SetAssure(id=%q,n=%d), want (%q,2)", f.lastAssureID, f.lastAssureN, f.added[0].ID)
	}
	if out["assure"].(float64) != 2 {
		t.Errorf("entry view should report assure=2, got %v", out["assure"])
	}
}

func TestAssure_OmittedWhenZero(t *testing.T) {
	f := &fakeStore{}
	invoke(t, newTool(f), map[string]any{"op": "every", "interval": "1h", "intent": "x"})
	if f.lastAssureN != 0 || f.lastAssureID != "" {
		t.Errorf("no assure arg should not call SetAssure, got (%q,%d)", f.lastAssureID, f.lastAssureN)
	}
}

func TestCreatedScheduleBindsActingAgent(t *testing.T) {
	f := &fakeStore{}
	ctx := agent.WithAgent(context.Background(), "researcher")
	out, isErr := invokeCtx(t, ctx, newTool(f), map[string]any{"op": "every", "interval": "1h", "intent": "hourly digest"})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.lastAgentID != f.added[0].ID || f.lastAgent != "researcher" {
		t.Fatalf("SetAgent(id=%q,agent=%q), want (%q,researcher)", f.lastAgentID, f.lastAgent, f.added[0].ID)
	}
	if out["agent"] != "researcher" {
		t.Errorf("entry view agent = %v, want researcher", out["agent"])
	}
}

func TestManagedSubAgentCannotScheduleDirectWake(t *testing.T) {
	f := &fakeStore{}
	tool := newTool(f)
	no := false
	tool.BindAgentLookup(func(ref string) (roster.Profile, bool) {
		if ref != "worker" {
			return roster.Profile{}, false
		}
		return roster.Profile{Slug: "worker", Enabled: true, ParentAgent: "lead", DirectCallable: &no}, true
	})
	ctx := agent.WithAgent(context.Background(), "worker")
	raw, _ := json.Marshal(map[string]any{"op": "every", "interval": "1h", "intent": "wake me"})
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("invoke schedule: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "managed sub-agent") || !strings.Contains(res.Output, "wake lead") {
		t.Fatalf("managed sub-agent direct schedule error = %+v, want manager hint", res)
	}
	if len(f.added) != 0 {
		t.Fatalf("schedule persisted despite managed sub-agent rejection: %+v", f.added)
	}

	_, isErr := invokeCtx(t, ctx, tool, map[string]any{"op": "every", "interval": "1h", "target": "tool", "tool": "shell"})
	if !isErr {
		t.Fatalf("managed sub-agent tool schedule should error because it would bind the sub-agent identity")
	}

	_, isErr = invokeCtx(t, ctx, tool, map[string]any{"op": "every", "interval": "1h", "target": "system_task", "system_task": cadence.SystemTaskMemoryClean})
	if isErr {
		t.Fatalf("system task schedule should still be allowed because it does not bind the managed actor")
	}
}

func TestWorkflowTargetBindsActingAgent(t *testing.T) {
	f := &fakeStore{}
	ctx := agent.WithAgent(context.Background(), "researcher")
	out, isErr := invokeCtx(t, ctx, newTool(f), map[string]any{
		"op":       "every",
		"interval": "1h",
		"target":   "workflow",
		"workflow": "nightly-sync",
		"payload":  map[string]any{"force": true},
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.lastWorkflowID != f.added[0].ID || f.lastWorkflow != "nightly-sync" {
		t.Fatalf("SetWorkflowTarget(id=%q,workflow=%q), want (%q,nightly-sync)", f.lastWorkflowID, f.lastWorkflow, f.added[0].ID)
	}
	if f.lastAgentID != f.added[0].ID || f.lastAgent != "researcher" {
		t.Fatalf("workflow schedule should bind acting agent, got (%q,%q)", f.lastAgentID, f.lastAgent)
	}
	if out["target"] != "workflow" || out["workflow"] != "nightly-sync" {
		t.Fatalf("entry view missing workflow target: %v", out)
	}
	if out["agent"] != "researcher" {
		t.Fatalf("entry view agent = %v, want researcher", out["agent"])
	}
	payload, _ := out["payload"].(map[string]any)
	if payload["force"] != true {
		t.Fatalf("payload = %v, want force=true", payload)
	}
}

func TestWorkflowTargetPreservesModelOverride(t *testing.T) {
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op":       "every",
		"interval": "1h",
		"target":   "workflow",
		"workflow": "nightly-sync",
		"model":    "schedule-model",
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.added[0].Model != "schedule-model" {
		t.Fatalf("stored model = %q, want schedule-model", f.added[0].Model)
	}
	if out["model"] != "schedule-model" {
		t.Fatalf("entry view model = %v, want schedule-model", out["model"])
	}
}

func TestToolTarget(t *testing.T) {
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op":      "in",
		"delay":   "5m",
		"target":  "tool",
		"tool":    "shell",
		"payload": map[string]any{"command": "echo hi"},
		"intent":  "shell echo label",
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.lastToolID != f.added[0].ID || f.lastTool != "shell" {
		t.Fatalf("SetToolTarget(id=%q,tool=%q), want (%q,shell)", f.lastToolID, f.lastTool, f.added[0].ID)
	}
	if out["target"] != "tool" || out["tool"] != "shell" {
		t.Fatalf("entry view missing tool target: %v", out)
	}
	payload, _ := out["payload"].(map[string]any)
	if payload["command"] != "echo hi" {
		t.Fatalf("payload = %v, want command", payload)
	}
}

func TestToolTargetBindsActingAgent(t *testing.T) {
	f := &fakeStore{}
	ctx := agent.WithAgent(context.Background(), "researcher")
	out, isErr := invokeCtx(t, ctx, newTool(f), map[string]any{
		"op":      "in",
		"delay":   "5m",
		"target":  "tool",
		"tool":    "shell",
		"payload": map[string]any{"command": "echo hi"},
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.lastAgentID != f.added[0].ID || f.lastAgent != "researcher" {
		t.Fatalf("tool schedule should bind acting agent, got (%q,%q)", f.lastAgentID, f.lastAgent)
	}
	if out["agent"] != "researcher" {
		t.Fatalf("entry view agent = %v, want researcher", out["agent"])
	}
}

func TestToolTargetPreservesModelOverrideMetadata(t *testing.T) {
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op":     "in",
		"delay":  "5m",
		"target": "tool",
		"tool":   "shell",
		"model":  "schedule-model",
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.added[0].Model != "schedule-model" {
		t.Fatalf("stored model = %q, want schedule-model", f.added[0].Model)
	}
	if out["model"] != "schedule-model" {
		t.Fatalf("entry view model = %v, want schedule-model", out["model"])
	}
}

func TestSystemTaskTarget(t *testing.T) {
	for _, task := range cadence.SystemTasks() {
		t.Run(task, func(t *testing.T) {
			f := &fakeStore{}
			out, isErr := invoke(t, newTool(f), map[string]any{
				"op":          "daily",
				"at":          "03:00",
				"target":      "system_task",
				"system_task": task,
			})
			if isErr {
				t.Fatalf("unexpected error: %v", out)
			}
			if f.lastSystemID != f.added[0].ID || f.lastSystem != task {
				t.Fatalf("SetSystemTaskTarget(id=%q,task=%q), want (%q,%s)", f.lastSystemID, f.lastSystem, f.added[0].ID, task)
			}
			if out["target"] != "system_task" || out["system_task"] != task {
				t.Fatalf("entry view missing system task target: %v", out)
			}
		})
	}
}

func TestSystemTaskTargetRejectsPayload(t *testing.T) {
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op":          "daily",
		"at":          "03:00",
		"target":      "system_task",
		"system_task": cadence.SystemTaskCatalogSync,
		"payload":     map[string]any{"instructions": "also do this"},
	})
	if !isErr {
		t.Fatalf("system task payload should be rejected: %v", out)
	}
	if len(f.added) != 0 || f.lastSystemID != "" {
		t.Fatalf("system task target should not be applied after payload rejection: added=%v lastSystem=%q", f.added, f.lastSystemID)
	}
}

func TestDefinitionSystemTaskEnumMatchesCadence(t *testing.T) {
	def := New().Definition()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
		t.Fatalf("schema parse: %v", err)
	}
	if got, want := schema.Properties["system_task"].Enum, cadence.SystemTasks(); !reflect.DeepEqual(got, want) {
		t.Fatalf("system_task enum = %v want %v", got, want)
	}
}

func TestTypedTargetValidation(t *testing.T) {
	f := &fakeStore{}
	cases := []map[string]any{
		{"op": "every", "interval": "1h", "target": "workflow"},
		{"op": "every", "interval": "1h", "target": "tool"},
		{"op": "every", "interval": "1h", "target": "system_task", "system_task": "missing"},
		{"op": "every", "interval": "1h", "target": "agent", "workflow": "x", "intent": "x"},
		{"op": "every", "interval": "1h", "workflow": "x", "tool": "shell"},
	}
	for _, c := range cases {
		if _, isErr := invoke(t, newTool(f), c); !isErr {
			t.Errorf("expected error result for %v", c)
		}
	}
}

func TestOpContinuous_BadCooldown(t *testing.T) {
	f := &fakeStore{}
	if _, isErr := invoke(t, newTool(f), map[string]any{"op": "continuous", "cooldown": "nope", "intent": "x"}); !isErr {
		t.Error("a bad cooldown should be an error")
	}
}

func TestOpDaily_ParsesTimeAndDays(t *testing.T) {
	f := &fakeStore{}
	_, isErr := invoke(t, newTool(f), map[string]any{"op": "daily", "at": "09:30", "days": "mon-fri", "intent": "standup"})
	if isErr {
		t.Fatal("unexpected error")
	}
	if f.lastAtMin != 9*60+30 {
		t.Errorf("atMinutes = %d, want %d", f.lastAtMin, 9*60+30)
	}
	if f.lastDays == 0 {
		t.Error("days should be a non-zero weekday mask for mon-fri")
	}
}

func TestOpRemove(t *testing.T) {
	f := &fakeStore{removeOK: true}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "remove", "id": "once1"})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.removed != "once1" {
		t.Errorf("removed = %q, want once1", f.removed)
	}
}

func TestOpRemove_NotFound(t *testing.T) {
	f := &fakeStore{removeOK: false}
	_, isErr := invoke(t, newTool(f), map[string]any{"op": "remove", "id": "nope"})
	if !isErr {
		t.Error("removing an unknown id should be an error result")
	}
}

func TestOpList(t *testing.T) {
	f := &fakeStore{entries: []cadence.Entry{
		{ID: "a", Intent: "x", Mode: cadence.ModeInterval, IntervalSec: 3600, Enabled: true, Source: "agent"},
	}}
	out, _ := invoke(t, newTool(f), map[string]any{"op": "list"})
	if out["count"].(float64) != 1 {
		t.Fatalf("count = %v, want 1", out["count"])
	}
}

func TestBadInputs(t *testing.T) {
	f := &fakeStore{}
	cases := []map[string]any{
		{"op": "in", "delay": "nope", "intent": "x"},
		{"op": "in", "delay": "-5m", "intent": "x"},
		{"op": "every", "interval": "", "intent": "x"},
		{"op": "daily", "at": "25:00", "intent": "x"},
		{"op": "daily", "at": "notime", "intent": "x"},
		{"op": "bogus"},
		{"op": ""},
	}
	for _, c := range cases {
		if _, isErr := invoke(t, newTool(f), c); !isErr {
			t.Errorf("expected error result for %v", c)
		}
	}
}

func TestUnboundStoreIsSafe(t *testing.T) {
	tool := New() // never Bound
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("an unbound tool should return an error result, not succeed")
	}
}

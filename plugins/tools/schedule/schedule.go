// SPDX-License-Identifier: MIT

// Package schedule is the in-process cronjob tool. It lets an agent create
// typed future work — wake this agent task, run a stored workflow, run daemon
// maintenance, or invoke a registered tool — by writing to the daemon's
// persistent cadence store, the same store the `agt schedule` CLI and the
// AGEZT_SCHEDULE env jobs use. A scheduled job fires through its target
// executor at its due time (M634).
//
// This is the autonomy primitive that turns a reactive assistant into a
// proactive one: an agent can install a visible future job without embedding
// identity instructions inside the schedule. Schedules it creates are tagged
// source="agent" so an operator can see and prune them (`agt schedule list`).
//
// The tool is created unbound and Bound to the live store after the kernel
// opens (the store is the kernel's), mirroring the notify tool's lifecycle.
package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
)

// store is the subset of *cadence.Store the tool needs — an interface so tests
// can inject a fake without a real on-disk store.
type store interface {
	Add(intent string, interval time.Duration, model, source string, now time.Time) (cadence.Entry, error)
	AddDaily(intent string, atMinutes, days int, tz, model, source string, now time.Time) (cadence.Entry, error)
	AddOnce(intent string, at time.Time, model, source string, now time.Time) (cadence.Entry, error)
	AddContinuous(intent string, cooldown time.Duration, model, source string, now time.Time) (cadence.Entry, error)
	SetAssure(id string, n int) (bool, error)
	SetAgent(id, agent string) (bool, error)
	SetWorkflowTarget(id, ref string, payload json.RawMessage) (bool, error)
	SetSystemTaskTarget(id, task string) (bool, error)
	SetToolTarget(id, tool string, payload json.RawMessage) (bool, error)
	Remove(id string) (bool, error)
	List() []cadence.Entry
}

// Tool implements agent.Tool. Created unbound via New(); Bind wires the store.
type Tool struct {
	mu          sync.RWMutex
	store       store
	now         func() time.Time
	agentLookup func(string) (roster.Profile, bool)
}

// New returns an unbound schedule tool (no store until Bind).
func New() *Tool { return &Tool{now: time.Now} }

// Bind wires the live cadence store. Called once after the kernel opens.
func (t *Tool) Bind(s *cadence.Store) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s != nil {
		t.store = s
	}
}

func (t *Tool) BindAgentLookup(lookup func(string) (roster.Profile, bool)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agentLookup = lookup
}

func (t *Tool) current() (store, func() time.Time, func(string) (roster.Profile, bool)) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := t.now
	if now == nil {
		now = time.Now
	}
	return t.store, now, t.agentLookup
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	systemTaskEnum, _ := json.Marshal(cadence.SystemTasks())
	return agent.ToolDef{
		Name: "schedule",
		Description: "Schedule future work: run your own agent task later, wake a workflow, " +
			"run a system task, or invoke a registered tool on a cadence. Use typed targets " +
			"instead of embedding execution instructions in the task/label.",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"create, list, or remove future scheduled jobs",
				"created schedules will launch typed cron jobs later until removed",
			},
			AffectedResources: []string{"cadence schedule store"},
			RollbackNotes:     "Created schedules can be removed by id with op=remove or via the operator schedule UI/CLI.",
			Confidence:        0.9,
		},
		InputSchema: json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":       {"type":"string", "enum":["in","every","daily","continuous","list","remove"], "description":"in=one-shot after a delay; every=recurring interval; daily=at a wall-clock time; continuous=a never-ending loop that re-runs after each run finishes; list; remove."},
    "intent":   {"type":"string", "description":"For target=agent/intent: the task to run at the scheduled time. For workflow/system_task/tool, optional label only, not instructions."},
    "target":   {"type":"string", "enum":["agent","intent","workflow","system_task","tool"], "description":"What this schedule fires. Default agent/intent runs this agent's own task. workflow runs a stored workflow. system_task runs daemon maintenance. tool invokes a registered tool."},
    "workflow": {"type":"string", "description":"Workflow name/id when target=workflow."},
    "system_task": {"type":"string", "enum":%s, "description":"System maintenance task when target=system_task."},
    "tool":     {"type":"string", "description":"Registered tool name when target=tool."},
    "payload":  {"description":"JSON payload for target=workflow or target=tool."},
    "delay":    {"type":"string", "description":"For op=in: how far out, e.g. \"30m\", \"2h\", \"24h\"."},
    "interval": {"type":"string", "description":"For op=every: the firing period, e.g. \"1h\", \"15m\"."},
    "cooldown": {"type":"string", "description":"For op=continuous: the breather between cycles, e.g. \"30s\", \"5m\". The loop runs forever; pause/remove it to stop."},
    "at":       {"type":"string", "description":"For op=daily: wall-clock time \"HH:MM\" (24h, daemon local time)."},
    "days":     {"type":"string", "description":"For op=daily (optional): which days, e.g. \"mon-fri\", \"weekends\". Default every day."},
    "model":    {"type":"string", "description":"Optional model override for the scheduled run."},
    "assure":   {"type":"integer", "description":"Optional do-it-for-sure budget for in/every/daily/continuous: if > 0, each firing runs, verifies it was actually completed, and retries the gap up to this many attempts. Use it for tasks that must definitely get done."},
    "id":       {"type":"string", "description":"For op=remove: the schedule id to delete."}
  }
}`, string(systemTaskEnum))),
	}
}

type input struct {
	Op       string `json:"op"`
	Intent   string `json:"intent"`
	Delay    string `json:"delay"`
	Interval string `json:"interval"`
	Cooldown string `json:"cooldown"`
	At       string `json:"at"`
	Days     string `json:"days"`
	Model    string `json:"model"`
	ID       string `json:"id"`
	Assure   int    `json:"assure"`
	Target   string `json:"target"`
	Workflow string `json:"workflow"`
	System   string `json:"system_task"`
	Tool     string `json:"tool"`
	Payload  any    `json:"payload"`
}

const source = "agent" // marks schedules the agent created, for operator visibility

// applyAssure stamps a do-it-for-sure budget onto a freshly created entry, so
// each firing runs-verifies-retries up to n attempts (M654). Best-effort: a
// SetAssure failure leaves the entry a single-pass schedule (no worse than
// assure being unset). Returns the entry with Assure reflected for display.
func applyAssure(st store, e cadence.Entry, n int) cadence.Entry {
	if n > 0 {
		if _, err := st.SetAssure(e.ID, n); err == nil {
			e.Assure = n
		}
	}
	return e
}

func applyActingAgent(ctx context.Context, st store, e cadence.Entry) cadence.Entry {
	if slug := agent.AgentFromContext(ctx); slug != "" {
		if _, err := st.SetAgent(e.ID, slug); err == nil {
			e.Agent = slug
		}
	}
	return e
}

func scheduleBindsActingAgent(in input) bool {
	target := strings.TrimSpace(in.Target)
	if target == "" {
		return strings.TrimSpace(in.Workflow) == "" && strings.TrimSpace(in.System) == "" && strings.TrimSpace(in.Tool) == ""
	}
	return target == "agent" || target == cadence.TargetIntent || target == cadence.TargetWorkflow || target == cadence.TargetTool
}

func validateActingAgentSchedule(ctx context.Context, in input, lookup func(string) (roster.Profile, bool)) agent.Result {
	if lookup == nil || !scheduleBindsActingAgent(in) {
		return agent.Result{}
	}
	slug := strings.TrimSpace(agent.AgentFromContext(ctx))
	if slug == "" {
		return agent.Result{}
	}
	p, ok := lookup(slug)
	if !ok {
		return errResult("acting agent " + slug + " is not in the roster")
	}
	if p.Retired {
		return errResult("agent " + p.Slug + " is retired and cannot schedule a direct wake")
	}
	if !p.Enabled {
		return errResult("agent " + p.Slug + " is paused and cannot schedule a direct wake")
	}
	if !p.AllowsDirectCall() {
		return errResult(managedSubAgentScheduleHint(p))
	}
	return agent.Result{}
}

func managedSubAgentScheduleHint(p roster.Profile) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot create independently firing schedules; " + hint
}

func scheduleTarget(in input) string {
	target := strings.TrimSpace(in.Target)
	if target == "" {
		switch {
		case strings.TrimSpace(in.Workflow) != "":
			target = cadence.TargetWorkflow
		case strings.TrimSpace(in.System) != "":
			target = cadence.TargetSystemTask
		case strings.TrimSpace(in.Tool) != "":
			target = cadence.TargetTool
		default:
			target = cadence.TargetIntent
		}
	}
	if target == "agent" {
		return cadence.TargetIntent
	}
	return target
}

func validateScheduledJob(in input) agent.Result {
	switch in.Op {
	case "in", "every", "daily", "continuous":
	default:
		return agent.Result{}
	}
	if scheduleTarget(in) == cadence.TargetIntent && strings.TrimSpace(in.Intent) == "" {
		return errResult("target=agent needs agent task text in the intent field")
	}
	return agent.Result{}
}

func applyTypedTarget(ctx context.Context, st store, e cadence.Entry, in input) (cadence.Entry, agent.Result, bool) {
	target := scheduleTarget(in)
	in.Workflow = strings.TrimSpace(in.Workflow)
	in.System = strings.TrimSpace(in.System)
	in.Tool = strings.TrimSpace(in.Tool)
	bindings := 0
	for _, s := range []string{in.Workflow, in.System, in.Tool} {
		if s != "" {
			bindings++
		}
	}
	if target == cadence.TargetIntent {
		if bindings > 0 {
			return e, errResult("target=agent/intent cannot also set workflow, system_task, or tool"), false
		}
		return applyActingAgent(ctx, st, e), agent.Result{}, true
	}
	if bindings > 1 {
		return e, errResult("choose only one of workflow, system_task, or tool"), false
	}
	var payload json.RawMessage
	if in.Payload != nil {
		b, err := json.Marshal(in.Payload)
		if err != nil {
			return e, errResult("payload must be JSON-serializable: " + err.Error()), false
		}
		payload = b
	}
	switch target {
	case cadence.TargetWorkflow:
		if in.Workflow == "" {
			return e, errResult("target=workflow needs workflow"), false
		}
		if _, err := st.SetWorkflowTarget(e.ID, in.Workflow, payload); err != nil {
			return e, errResult(err.Error()), false
		}
		e.Target, e.Workflow, e.Payload = cadence.TargetWorkflow, in.Workflow, payload
		e = applyActingAgent(ctx, st, e)
	case cadence.TargetSystemTask:
		if in.System == "" {
			return e, errResult("target=system_task needs system_task"), false
		}
		if in.Payload != nil {
			return e, errResult("target=system_task does not accept payload; choose a whitelisted system_task only"), false
		}
		if !cadence.IsSystemTask(in.System) {
			return e, errResult("unknown system task: " + in.System), false
		}
		if _, err := st.SetSystemTaskTarget(e.ID, in.System); err != nil {
			return e, errResult(err.Error()), false
		}
		e.Target, e.SystemTask = cadence.TargetSystemTask, in.System
		e.Agent, e.Model, e.Payload = "", "", nil
	case cadence.TargetTool:
		if in.Tool == "" {
			return e, errResult("target=tool needs tool"), false
		}
		if _, err := st.SetToolTarget(e.ID, in.Tool, payload); err != nil {
			return e, errResult(err.Error()), false
		}
		e.Target, e.Tool, e.Payload = cadence.TargetTool, in.Tool, payload
		e = applyActingAgent(ctx, st, e)
	default:
		return e, errResult("unknown target " + target + " (agent|workflow|system_task|tool)"), false
	}
	return e, agent.Result{}, true
}

func finalizeEntry(ctx context.Context, st store, e cadence.Entry, in input, msg string) agent.Result {
	e, res, ok := applyTypedTarget(ctx, st, e, in)
	if !ok {
		_, _ = st.Remove(e.ID)
		return res
	}
	return okEntry(msg, applyAssure(st, e, in.Assure))
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("schedule: parse input: %w", err)
	}
	st, nowFn, lookup := t.current()
	if st == nil {
		return errResult("scheduling is not available on this daemon"), nil
	}
	now := nowFn()
	if strings.TrimSpace(in.Intent) == "" {
		switch {
		case strings.TrimSpace(in.Workflow) != "":
			in.Intent = "workflow " + strings.TrimSpace(in.Workflow)
		case strings.TrimSpace(in.System) != "":
			in.Intent = "system task " + strings.TrimSpace(in.System)
		case strings.TrimSpace(in.Tool) != "":
			in.Intent = "tool " + strings.TrimSpace(in.Tool)
		}
	}
	if res := validateActingAgentSchedule(ctx, in, lookup); res.Output != "" {
		return res, nil
	}
	if res := validateScheduledJob(in); res.Output != "" {
		return res, nil
	}

	switch in.Op {
	case "in":
		d, err := time.ParseDuration(in.Delay)
		if err != nil || d <= 0 {
			return errResult(`op=in needs a positive "delay" duration like "30m" or "2h"`), nil
		}
		e, err := st.AddOnce(in.Intent, now.Add(d), in.Model, source, now)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return finalizeEntry(ctx, st, e, in, "scheduled once"), nil

	case "every":
		d, err := time.ParseDuration(in.Interval)
		if err != nil || d <= 0 {
			return errResult(`op=every needs a positive "interval" like "1h" or "15m"`), nil
		}
		e, err := st.Add(in.Intent, d, in.Model, source, now)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return finalizeEntry(ctx, st, e, in, "scheduled recurring"), nil

	case "continuous":
		d, err := time.ParseDuration(in.Cooldown)
		if err != nil || d <= 0 {
			return errResult(`op=continuous needs a positive "cooldown" like "30s" or "5m"`), nil
		}
		e, err := st.AddContinuous(in.Intent, d, in.Model, source, now)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return finalizeEntry(ctx, st, e, in, "started a continuous loop (runs forever; pause/remove to stop)"), nil

	case "daily":
		mins, ok := parseHHMM(in.At)
		if !ok {
			return errResult(`op=daily needs an "at" time in HH:MM (24h)`), nil
		}
		days := 0 // 0 = every day
		if in.Days != "" {
			d, err := cadence.ParseDays(in.Days)
			if err != nil {
				return errResult("bad days spec: " + err.Error()), nil
			}
			days = d
		}
		e, err := st.AddDaily(in.Intent, mins, days, "", in.Model, source, now)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return finalizeEntry(ctx, st, e, in, "scheduled daily"), nil

	case "remove":
		if in.ID == "" {
			return errResult(`op=remove needs an "id"`), nil
		}
		removed, err := st.Remove(in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if !removed {
			return errResult("no schedule with id " + in.ID), nil
		}
		return okJSON(map[string]any{"removed": in.ID}), nil

	case "list":
		entries := st.List()
		out := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			out = append(out, entryView(e))
		}
		return okJSON(map[string]any{"count": len(out), "schedules": out}), nil

	case "":
		return errResult("op required (in|every|daily|list|remove)"), nil
	default:
		return errResult("unknown op " + in.Op + " (in|every|daily|list|remove)"), nil
	}
}

// parseHHMM parses "HH:MM" (24h) into minutes since midnight. Returns ok=false
// on any malformed input.
func parseHHMM(s string) (int, bool) {
	var h, m int
	if n, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || n != 2 {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func entryView(e cadence.Entry) map[string]any {
	v := map[string]any{
		"id":      e.ID,
		"intent":  e.Intent,
		"cadence": e.Cadence(),
		"enabled": e.Enabled,
		"source":  e.Source,
	}
	if e.NextRunUnix > 0 {
		v["next_run"] = time.Unix(e.NextRunUnix, 0).Format(time.RFC3339)
	}
	if e.Fires > 0 {
		v["fires"] = e.Fires
	}
	if e.Assure > 0 {
		v["assure"] = e.Assure
	}
	if e.Model != "" {
		v["model"] = e.Model
	}
	if e.Agent != "" {
		v["agent"] = e.Agent
	}
	if e.Target != "" {
		v["target"] = e.Target
	}
	if e.Workflow != "" {
		v["workflow"] = e.Workflow
	}
	if e.SystemTask != "" {
		v["system_task"] = e.SystemTask
	}
	if e.Tool != "" {
		v["tool"] = e.Tool
	}
	if len(e.Payload) > 0 {
		var payload any
		if err := json.Unmarshal(e.Payload, &payload); err == nil {
			v["payload"] = payload
		}
	}
	return v
}

func okEntry(msg string, e cadence.Entry) agent.Result {
	view := entryView(e)
	view["message"] = msg
	return okJSON(view)
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "schedule: " + msg, IsError: true}
}

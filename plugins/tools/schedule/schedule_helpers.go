// SPDX-License-Identifier: MIT

// Schedule tool: mutation pipeline (apply/validate/finalize) + parseHHMM.
// Code extracted from schedule.go during the Day-100 god-file split.
// Public API unchanged.
package schedule


import (
	"context"
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
)

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

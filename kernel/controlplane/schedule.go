// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
)

// Schedule handlers (autonomy) — the control-plane surface behind `agt
// schedule`. Writes go to the kernel's persistent cadence.Store; the cadence
// resident fires due entries as agent wakes, workflow runs, daemon tasks, or
// tool invocations. Operators manage only operator-sourced entries here;
// env-seeded ones come from AGEZT_SCHEDULE.

func (s *Server) handleScheduleAdd(conn net.Conn, req Request) {
	intent, _ := req.Args["intent"].(string)
	model, _ := req.Args["model"].(string)
	agent, _ := req.Args["agent"].(string)
	agent = strings.TrimSpace(agent)
	target, _ := req.Args["target"].(string)
	workflowRef, _ := req.Args["workflow"].(string)
	systemTask, _ := req.Args["system_task"].(string)
	toolName, _ := req.Args["tool"].(string)
	target, workflowRef, systemTask, toolName = strings.TrimSpace(target), strings.TrimSpace(workflowRef), strings.TrimSpace(systemTask), strings.TrimSpace(toolName)
	if workflowRef != "" {
		target = cadence.TargetWorkflow
	}
	if systemTask != "" {
		target = cadence.TargetSystemTask
	}
	if toolName != "" {
		target = cadence.TargetTool
	}
	var workflowPayload json.RawMessage
	var toolPayload json.RawMessage
	switch target {
	case cadence.TargetWorkflow:
		if systemTask != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workflow schedules cannot also set args.system_task"})
			return
		}
		if toolName != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workflow schedules cannot also set args.tool"})
			return
		}
		w, ok := s.k.Workflows().Get(workflowRef)
		if !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + workflowRef})
			return
		}
		workflowRef = w.Name
		if strings.TrimSpace(intent) == "" {
			intent = "workflow " + w.Name
		}
		if payload, ok := req.Args["payload"]; ok {
			b, err := json.Marshal(payload)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "payload must be JSON-serializable: " + err.Error()})
				return
			}
			workflowPayload = b
		}
	case cadence.TargetSystemTask:
		if agent != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.agent"})
			return
		}
		if workflowRef != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.workflow"})
			return
		}
		if toolName != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.tool"})
			return
		}
		if !cadence.IsSystemTask(systemTask) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown system task: " + systemTask})
			return
		}
		if _, ok := req.Args["payload"]; ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules do not accept args.payload"})
			return
		}
		if strings.TrimSpace(intent) == "" {
			intent = "system task " + systemTask
		}
	case cadence.TargetTool:
		if toolName == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool required for tool schedules"})
			return
		}
		if workflowRef != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "tool schedules cannot also set args.workflow"})
			return
		}
		if systemTask != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "tool schedules cannot also set args.system_task"})
			return
		}
		if _, ok := s.k.Tools()[toolName]; !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown tool: " + toolName})
			return
		}
		if strings.TrimSpace(intent) == "" {
			intent = "tool " + toolName
		}
		if payload, ok := req.Args["payload"]; ok {
			b, err := json.Marshal(payload)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "payload must be JSON-serializable: " + err.Error()})
				return
			}
			toolPayload = b
		}
	case cadence.TargetIntent:
		if strings.TrimSpace(intent) == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent task text required for target=intent schedules"})
			return
		}
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown schedule target: " + target})
		return
	}
	if agent != "" {
		p, ok := s.k.Roster().Get(agent)
		if !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + agent})
			return
		}
		if p.Retired {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
			return
		}
		if !p.Enabled {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
			return
		}
		if !p.AllowsDirectCall() {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "scheduled")})
			return
		}
		agent = p.Slug
	}
	if target == cadence.TargetTool && agent != "" {
		p, _ := s.k.Roster().Get(agent)
		if err := validateAgentScheduledTool(p, toolName); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}

	var e cadence.Entry
	var err error
	// One-shot when once_at_unix is present; daily when at_minutes is present;
	// interval otherwise.
	if _, ok := req.Args["once_at_unix"]; ok {
		at, _, parseErr := scheduleArgNumber(req.Args, "once_at_unix")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		e, err = s.k.Schedules().AddOnce(intent, time.Unix(int64(at), 0), model, cadence.SourceOperator, time.Now())
	} else if _, ok := req.Args["cooldown_sec"]; ok {
		sec, _, parseErr := scheduleArgNumber(req.Args, "cooldown_sec")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		if sec < 1 {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.cooldown_sec must be >= 1"})
			return
		}
		e, err = s.k.Schedules().AddContinuous(intent, time.Duration(sec)*time.Second, model, cadence.SourceOperator, time.Now())
	} else if _, ok := req.Args["window_start"]; ok {
		start, _, parseErr := scheduleArgNumber(req.Args, "window_start")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		end, _, parseErr := scheduleArgNumber(req.Args, "window_end")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		sec, _, parseErr := scheduleArgNumber(req.Args, "interval_sec")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		days, _, parseErr := scheduleArgNumber(req.Args, "days")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		tz, _ := req.Args["tz"].(string)
		e, err = s.k.Schedules().AddWindow(intent, time.Duration(sec)*time.Second, int(start), int(end), int(days), tz, model, cadence.SourceOperator, time.Now())
	} else if _, ok := req.Args["at_minutes"]; ok {
		at, _, parseErr := scheduleArgNumber(req.Args, "at_minutes")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		days, _, parseErr := scheduleArgNumber(req.Args, "days") // weekday bitmask; 0 = every day
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		tz, _ := req.Args["tz"].(string)
		e, err = s.k.Schedules().AddDaily(intent, int(at), int(days), tz, model, cadence.SourceOperator, time.Now())
	} else {
		sec, _, parseErr := scheduleArgNumber(req.Args, "interval_sec")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		if sec < 1 {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.interval_sec must be >= 1 (or pass at_minutes)"})
			return
		}
		e, err = s.k.Schedules().Add(intent, time.Duration(sec)*time.Second, model, cadence.SourceOperator, time.Now())
	}
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	failCreated := func(msg string) {
		_, _ = s.k.Schedules().Remove(e.ID)
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: msg})
	}
	if agent != "" {
		ok, err := s.k.Schedules().SetAgent(e.ID, agent)
		if err != nil {
			failCreated(err.Error())
			return
		}
		if !ok {
			failCreated("schedule disappeared before agent binding")
			return
		}
		e.Agent = agent
	}
	if target == cadence.TargetWorkflow {
		ok, err := s.k.Schedules().SetWorkflowTarget(e.ID, workflowRef, workflowPayload)
		if err != nil {
			failCreated(err.Error())
			return
		}
		if !ok {
			failCreated("schedule disappeared before workflow binding")
			return
		}
		e.Target, e.Workflow, e.Payload = cadence.TargetWorkflow, workflowRef, workflowPayload
	}
	if target == cadence.TargetSystemTask {
		ok, err := s.k.Schedules().SetSystemTaskTarget(e.ID, systemTask)
		if err != nil {
			failCreated(err.Error())
			return
		}
		if !ok {
			failCreated("schedule disappeared before system task binding")
			return
		}
		e.Target, e.SystemTask = cadence.TargetSystemTask, systemTask
	}
	if target == cadence.TargetTool {
		ok, err := s.k.Schedules().SetToolTarget(e.ID, toolName, toolPayload)
		if err != nil {
			failCreated(err.Error())
			return
		}
		if !ok {
			failCreated("schedule disappeared before tool binding")
			return
		}
		e.Target, e.Tool, e.Payload = cadence.TargetTool, toolName, toolPayload
	}
	if refreshed, ok := s.k.Schedules().Get(e.ID); ok {
		e = refreshed
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: scheduleEntryView(e),
	})
}

func (s *Server) handleScheduleEnable(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	// Accept enabled as a bool (CLI/JSON transport) or a "true"/"false"/"1"/"0"
	// string (webui query-arg transport, which carries every value as a string).
	enabled := false
	switch v := req.Args["enabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true") || v == "1"
	}
	current, found := s.k.Schedules().Get(id)
	if enabled {
		if found {
			if err := s.validateScheduleRunnable(current); err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
		}
	}
	ok, err := s.k.Schedules().SetEnabled(id, enabled)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	action := "paused"
	if enabled {
		action = "resumed"
	}
	if ok {
		publishOperatorAction(s.k, "schedule.enable", s.k.NewCorrelation(), map[string]any{
			"id":      id,
			"enabled": enabled,
			"action":  action,
			"target":  current.Target,
			"agent":   current.Agent,
			"cadence": current.Cadence(),
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": ok, "enabled": enabled, "id": id, "action": action}})
}

// handleScheduleTest previews a schedule's next fire times (M120) — a read-only
// dry-run. Uses the entry's own Forecast simulation so the result matches what
// the cadence engine will actually do.
func (s *Server) handleScheduleTest(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	count := 5
	switch v := req.Args["count"].(type) {
	case float64:
		count = int(v)
	case int:
		count = v
	case int64:
		count = int(v)
	}
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}

	e, ok := s.k.Schedules().Get(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"found": false}})
		return
	}
	fires := e.Forecast(time.Now(), count)
	out := make([]map[string]any, 0, len(fires))
	for _, f := range fires {
		out = append(out, map[string]any{"unix": f})
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"found":     true,
			"id":        e.ID,
			"mode":      e.Mode,
			"cadence":   e.Cadence(),
			"enabled":   e.Enabled,
			"forecasts": out,
			"count":     len(out),
		},
	})
}

func (s *Server) handleScheduleEdit(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	store := s.k.Schedules()
	current, ok := store.Get(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": false}})
		return
	}
	now := time.Now()
	target, _ := req.Args["target"].(string)
	workflowRef, _ := req.Args["workflow"].(string)
	systemTask, _ := req.Args["system_task"].(string)
	toolName, _ := req.Args["tool"].(string)
	target, workflowRef, systemTask, toolName = strings.TrimSpace(target), strings.TrimSpace(workflowRef), strings.TrimSpace(systemTask), strings.TrimSpace(toolName)
	targetSet := false
	if _, ok := req.Args["target"]; ok {
		targetSet = true
	}
	if workflowRef != "" {
		target = cadence.TargetWorkflow
		targetSet = true
	}
	if systemTask != "" {
		target = cadence.TargetSystemTask
		targetSet = true
	}
	if toolName != "" {
		target = cadence.TargetTool
		targetSet = true
	}
	effectiveTarget := current.Target
	if targetSet {
		effectiveTarget = target
	}
	if effectiveTarget == cadence.TargetSystemTask {
		if _, ok := req.Args["payload"]; ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules do not accept args.payload"})
			return
		}
	}
	editAgent := ""
	effectiveAgent := strings.TrimSpace(current.Agent)
	if v, ok := req.Args["agent"]; ok {
		agent, _ := v.(string)
		agent = strings.TrimSpace(agent)
		if agent != "" && effectiveTarget == cadence.TargetSystemTask {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.agent"})
			return
		}
		if agent != "" {
			p, found := s.k.Roster().Get(agent)
			if !found {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + agent})
				return
			}
			if p.Retired {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
				return
			}
			if !p.Enabled {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
				return
			}
			if !p.AllowsDirectCall() {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "scheduled")})
				return
			}
			agent = p.Slug
		}
		editAgent = agent
		effectiveAgent = agent
	}
	if target == cadence.TargetWorkflow {
		if systemTask != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workflow schedules cannot also set args.system_task"})
			return
		}
		if toolName != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workflow schedules cannot also set args.tool"})
			return
		}
		if _, found := s.k.Workflows().Get(workflowRef); !found {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + workflowRef})
			return
		}
		if p, ok := req.Args["payload"]; ok {
			if _, err := json.Marshal(p); err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "payload must be JSON-serializable: " + err.Error()})
				return
			}
		}
	} else if target != "" && target != cadence.TargetIntent {
		if target != cadence.TargetSystemTask {
			if target != cadence.TargetTool {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown schedule target: " + target})
				return
			}
			if workflowRef != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "tool schedules cannot also set args.workflow"})
				return
			}
			if systemTask != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "tool schedules cannot also set args.system_task"})
				return
			}
			if toolName == "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool required for tool schedules"})
				return
			}
			if _, ok := s.k.Tools()[toolName]; !ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown tool: " + toolName})
				return
			}
			if effectiveAgent != "" {
				p, found := s.k.Roster().Get(effectiveAgent)
				if !found {
					s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + effectiveAgent})
					return
				}
				if err := validateAgentScheduledTool(p, toolName); err != nil {
					s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
					return
				}
			}
			if p, ok := req.Args["payload"]; ok {
				if _, err := json.Marshal(p); err != nil {
					s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "payload must be JSON-serializable: " + err.Error()})
					return
				}
			}
		} else {
			if workflowRef != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.workflow"})
				return
			}
			if toolName != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.tool"})
				return
			}
			if !cadence.IsSystemTask(systemTask) {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown system task: " + systemTask})
				return
			}
		}
	}
	if err := validateScheduleEditCadenceArgs(req.Args, now); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Field edits (any subset). A failure on intent (empty) is reported.
	if v, ok := req.Args["intent"]; ok {
		intent, _ := v.(string)
		if _, err := store.SetIntent(id, intent); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}
	if v, ok := req.Args["model"]; ok {
		model, _ := v.(string)
		_, _ = store.SetModel(id, model)
	}
	if v, ok := req.Args["agent"]; ok {
		_ = v
		_, _ = store.SetAgent(id, editAgent)
	}
	if target == cadence.TargetWorkflow {
		if systemTask != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workflow schedules cannot also set args.system_task"})
			return
		}
		if toolName != "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workflow schedules cannot also set args.tool"})
			return
		}
		w, found := s.k.Workflows().Get(workflowRef)
		if !found {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + workflowRef})
			return
		}
		var payload json.RawMessage
		if p, ok := req.Args["payload"]; ok {
			b, err := json.Marshal(p)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "payload must be JSON-serializable: " + err.Error()})
				return
			}
			payload = b
		}
		if _, err := store.SetWorkflowTarget(id, w.Name, payload); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	} else if target != "" && target != cadence.TargetIntent {
		if target != cadence.TargetSystemTask {
			if target != cadence.TargetTool {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown schedule target: " + target})
				return
			}
			if workflowRef != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "tool schedules cannot also set args.workflow"})
				return
			}
			if systemTask != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "tool schedules cannot also set args.system_task"})
				return
			}
			if toolName == "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool required for tool schedules"})
				return
			}
			if _, ok := s.k.Tools()[toolName]; !ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown tool: " + toolName})
				return
			}
			var payload json.RawMessage
			if p, ok := req.Args["payload"]; ok {
				b, err := json.Marshal(p)
				if err != nil {
					s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "payload must be JSON-serializable: " + err.Error()})
					return
				}
				payload = b
			}
			if _, err := store.SetToolTarget(id, toolName, payload); err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
		} else {
			if workflowRef != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.workflow"})
				return
			}
			if toolName != "" {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules cannot also set args.tool"})
				return
			}
			if _, ok := req.Args["payload"]; ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system task schedules do not accept args.payload"})
				return
			}
			if !cadence.IsSystemTask(systemTask) {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown system task: " + systemTask})
				return
			}
			if _, err := store.SetSystemTaskTarget(id, systemTask); err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
		}
	} else if _, ok := req.Args["target"]; ok && target == cadence.TargetIntent {
		if _, err := store.SetIntentTarget(id); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}

	// At most one cadence change: once | continuous | window | daily | interval.
	var err error
	tz, _ := req.Args["tz"].(string)
	if _, ok := req.Args["once_at_unix"]; ok {
		at, _, parseErr := scheduleArgNumber(req.Args, "once_at_unix")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		_, err = store.Reschedule(id, cadence.ModeOnce, 0, 0, 0, 0, "", time.Unix(int64(at), 0), now)
	} else if _, ok := req.Args["cooldown_sec"]; ok {
		sec, _, parseErr := scheduleArgNumber(req.Args, "cooldown_sec")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		_, err = store.Reschedule(id, cadence.ModeContinuous, time.Duration(sec)*time.Second, 0, 0, 0, "", time.Time{}, now)
	} else if _, ok := req.Args["window_start"]; ok {
		start, _, parseErr := scheduleArgNumber(req.Args, "window_start")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		end, _, parseErr := scheduleArgNumber(req.Args, "window_end")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		sec, _, parseErr := scheduleArgNumber(req.Args, "interval_sec")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		days, _, parseErr := scheduleArgNumber(req.Args, "days")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		_, err = store.Reschedule(id, cadence.ModeWindow, time.Duration(sec)*time.Second, int(start), int(end), int(days), tz, time.Time{}, now)
	} else if _, ok := req.Args["at_minutes"]; ok {
		at, _, parseErr := scheduleArgNumber(req.Args, "at_minutes")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		days, _, parseErr := scheduleArgNumber(req.Args, "days")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		_, err = store.Reschedule(id, cadence.ModeDaily, 0, int(at), 0, int(days), tz, time.Time{}, now)
	} else if _, ok := req.Args["interval_sec"]; ok {
		sec, _, parseErr := scheduleArgNumber(req.Args, "interval_sec")
		if parseErr != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: parseErr.Error()})
			return
		}
		_, err = store.Reschedule(id, cadence.ModeInterval, time.Duration(sec)*time.Second, 0, 0, 0, "", time.Time{}, now)
	}
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	e, _ := store.Get(id)
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: mergeScheduleEntryView(scheduleEntryView(e), map[string]any{"updated": true}),
	})
}

func (s *Server) handleScheduleList(conn net.Conn, req Request) {
	entries := s.k.Schedules().List()
	// Per-schedule last-firing outcome (M56): annotate each row with how the
	// schedule last went (status + when), folded from schedule.fired events
	// (M54/M55). Best-effort — a journal-walk failure just omits the annotation.
	latest, _ := s.latestFiringBySchedule(s.k)
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		row := scheduleEntryView(e)
		if lf, ok := latest[e.ID]; ok {
			row["last_status"] = lf.status
			row["last_reason"] = lf.reason
			row["last_fired_unix_ms"] = lf.firedMS
		}
		if err := s.validateScheduleRunnable(e); err != nil {
			row["target_status"] = "blocked"
			row["target_error"] = err.Error()
		} else {
			row["target_status"] = "ready"
		}
		if warning := s.scheduleFrequencyWarning(e); warning != "" {
			row["frequency_warning"] = warning
		}
		out = append(out, row)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"schedules": out, "count": len(out)},
	})
}

func scheduleEntryView(e cadence.Entry) map[string]any {
	meta := scheduleExecutionMetadata(e)
	return map[string]any{
		"id": e.ID, "intent": e.Intent, "mode": e.Mode, "interval_sec": e.IntervalSec,
		"at_minutes": e.AtMinutes, "end_minutes": e.EndMinutes, "days": e.Days, "tz": e.TZ, "cadence": e.Cadence(),
		"model": e.Model, "agent": e.Agent, "target": e.Target, "workflow": e.Workflow, "system_task": e.SystemTask, "tool": e.Tool, "payload": e.Payload,
		"source": e.Source, "enabled": e.Enabled,
		"created_unix": e.CreatedUnix, "last_run_unix": e.LastRunUnix,
		"next_run_unix": e.NextRunUnix, "fires": e.Fires, "assure": e.Assure,
		"executor": meta.Executor, "uses_llm": meta.UsesLLM, "execution_contract": meta.Contract,
		"execution_authority": meta.Authority, "identity_owner": meta.IdentityOwner,
		"payload_contract": meta.PayloadContract, "llm_boundary": meta.LLMBoundary,
	}
}

func mergeScheduleEntryView(base map[string]any, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func scheduleArgNumber(args map[string]any, key string) (float64, bool, error) {
	v, ok := args[key]
	if !ok {
		return 0, false, nil
	}
	switch n := v.(type) {
	case float64:
		return n, true, nil
	case int:
		return float64(n), true, nil
	case int64:
		return float64(n), true, nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, true, fmt.Errorf("args.%s must be numeric", key)
		}
		return f, true, nil
	default:
		return 0, true, fmt.Errorf("args.%s must be numeric", key)
	}
}

func validateScheduleEditCadenceArgs(args map[string]any, now time.Time) error {
	tz, _ := args["tz"].(string)
	if strings.TrimSpace(tz) != "" {
		if _, err := time.LoadLocation(strings.TrimSpace(tz)); err != nil {
			return fmt.Errorf("cadence: unknown timezone %q: %w", strings.TrimSpace(tz), err)
		}
	}
	if at, ok, err := scheduleArgNumber(args, "once_at_unix"); err != nil {
		return err
	} else if ok {
		if !time.Unix(int64(at), 0).After(now) {
			return fmt.Errorf("cadence: one-shot time must be in the future")
		}
		return nil
	}
	if sec, ok, err := scheduleArgNumber(args, "cooldown_sec"); err != nil {
		return err
	} else if ok {
		if time.Duration(sec)*time.Second < cadence.MinInterval {
			return fmt.Errorf("cadence: cooldown %s is below the %s minimum", time.Duration(sec)*time.Second, cadence.MinInterval)
		}
		return nil
	}
	if start, ok, err := scheduleArgNumber(args, "window_start"); err != nil {
		return err
	} else if ok {
		end, _, err := scheduleArgNumber(args, "window_end")
		if err != nil {
			return err
		}
		sec, _, err := scheduleArgNumber(args, "interval_sec")
		if err != nil {
			return err
		}
		days, _, err := scheduleArgNumber(args, "days")
		if err != nil {
			return err
		}
		interval := time.Duration(sec) * time.Second
		if interval < cadence.MinInterval {
			return fmt.Errorf("cadence: interval %s is below the %s minimum", interval, cadence.MinInterval)
		}
		if int(start) < 0 || int(start) > 1439 || int(end) < 0 || int(end) > 1439 {
			return fmt.Errorf("cadence: window bounds must be 00:00..23:59")
		}
		if int(end) <= int(start) {
			return fmt.Errorf("cadence: window end must be after its start")
		}
		if int(days) < 0 || int(days) > cadence.AllDays {
			return fmt.Errorf("cadence: day-mask must be 0..%d", cadence.AllDays)
		}
		return nil
	}
	if at, ok, err := scheduleArgNumber(args, "at_minutes"); err != nil {
		return err
	} else if ok {
		days, _, err := scheduleArgNumber(args, "days")
		if err != nil {
			return err
		}
		if int(at) < 0 || int(at) > 1439 {
			return fmt.Errorf("cadence: time-of-day must be 00:00..23:59")
		}
		if int(days) < 0 || int(days) > cadence.AllDays {
			return fmt.Errorf("cadence: day-mask must be 0..%d", cadence.AllDays)
		}
		return nil
	}
	if sec, ok, err := scheduleArgNumber(args, "interval_sec"); err != nil {
		return err
	} else if ok && time.Duration(sec)*time.Second < cadence.MinInterval {
		return fmt.Errorf("cadence: interval %s is below the %s minimum", time.Duration(sec)*time.Second, cadence.MinInterval)
	}
	return nil
}

type scheduleExecutionMeta struct {
	Executor        string
	UsesLLM         bool
	Contract        string
	Authority       string
	IdentityOwner   string
	PayloadContract string
	LLMBoundary     string
}

func scheduleExecutionMetadata(e cadence.Entry) scheduleExecutionMeta {
	agentSlug := strings.TrimSpace(e.Agent)
	payloadContract := schedulePayloadContract(e)
	switch e.Target {
	case cadence.TargetWorkflow:
		workflowRef := strings.TrimSpace(e.Workflow)
		if agentSlug != "" {
			return scheduleExecutionMeta{
				Executor: "workflow", UsesLLM: true,
				Contract:  "cron runs workflow " + workflowRef + " as " + agentSlug,
				Authority: "agent " + agentSlug, IdentityOwner: "agent " + agentSlug,
				PayloadContract: payloadContract, LLMBoundary: "workflow may use LLM nodes under workflow policy and invoking agent authority",
			}
		}
		return scheduleExecutionMeta{
			Executor: "workflow", UsesLLM: true,
			Contract:  "cron runs workflow " + workflowRef + " under system identity",
			Authority: "system identity", IdentityOwner: "none; workflow is a reusable graph",
			PayloadContract: payloadContract, LLMBoundary: "workflow may use LLM nodes under workflow policy, but no agent identity is woken",
		}
	case cadence.TargetSystemTask:
		return scheduleExecutionMeta{
			Executor: "daemon", UsesLLM: false,
			Contract:  "cron runs daemon system task " + strings.TrimSpace(e.SystemTask),
			Authority: "daemon", IdentityOwner: "none; daemon task owns no agent soul",
			PayloadContract: payloadContract, LLMBoundary: "no LLM",
		}
	case cadence.TargetTool:
		toolName := strings.TrimSpace(e.Tool)
		if agentSlug != "" {
			return scheduleExecutionMeta{
				Executor: "tool", UsesLLM: false,
				Contract:  "cron invokes tool " + toolName + " as " + agentSlug,
				Authority: "agent " + agentSlug, IdentityOwner: "agent " + agentSlug + " tool policy",
				PayloadContract: payloadContract, LLMBoundary: "no LLM; direct tool invocation",
			}
		}
		return scheduleExecutionMeta{
			Executor: "tool", UsesLLM: false,
			Contract:  "cron invokes tool " + toolName + " under system identity",
			Authority: "system identity", IdentityOwner: "none; tool call owns no agent soul",
			PayloadContract: payloadContract, LLMBoundary: "no LLM; direct tool invocation",
		}
	default:
		if agentSlug != "" {
			return scheduleExecutionMeta{
				Executor: "agent", UsesLLM: true,
				Contract:  "cron wakes agent " + agentSlug,
				Authority: "agent " + agentSlug, IdentityOwner: "agent " + agentSlug,
				PayloadContract: payloadContract, LLMBoundary: "LLM runs inside the agent wake; schedule stores only cadence and task text",
			}
		}
		return scheduleExecutionMeta{
			Executor: "llm", UsesLLM: true,
			Contract:  "cron runs governed LLM task",
			Authority: "system identity", IdentityOwner: "none; ad-hoc governed task",
			PayloadContract: payloadContract, LLMBoundary: "LLM run has no durable agent identity",
		}
	}
}

func schedulePayloadContract(e cadence.Entry) string {
	switch e.Target {
	case cadence.TargetSystemTask:
		return "payload not accepted"
	case cadence.TargetTool:
		if len(e.Payload) == 0 || strings.TrimSpace(string(e.Payload)) == "" || strings.TrimSpace(string(e.Payload)) == "null" {
			return "cron passes no tool payload"
		}
		return "cron passes JSON tool payload"
	case cadence.TargetWorkflow:
		if len(e.Payload) == 0 || strings.TrimSpace(string(e.Payload)) == "" || strings.TrimSpace(string(e.Payload)) == "null" {
			return "cron passes no workflow payload"
		}
		return "cron passes JSON workflow payload"
	default:
		return "task text only"
	}
}

func (s *Server) handleScheduleSystemTasks(conn net.Conn, req Request) {
	tasks := cadence.SystemTasks()
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"system_tasks": tasks, "system_task_info": cadence.SystemTaskInfos(), "count": len(tasks)},
	})
}

func (s *Server) handleScheduleRemove(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	removed, err := s.k.Schedules().Remove(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed}})
}

func (s *Server) handleScheduleRun(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	if e, ok := s.k.Schedules().Get(id); ok {
		if err := s.validateScheduleRunnable(e); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}
	triggered, err := s.k.Schedules().RunNow(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"triggered": triggered}})
}

func (s *Server) validateScheduleRunnable(e cadence.Entry) error {
	if strings.TrimSpace(e.Agent) != "" {
		p, found := s.k.Roster().Get(e.Agent)
		if !found {
			return errString("unknown agent: " + e.Agent)
		}
		if p.Retired {
			return errString("agent " + p.Slug + " is retired — revive it first")
		}
		if !p.Enabled {
			return errString("agent " + p.Slug + " is paused")
		}
		if !p.AllowsDirectCall() {
			return errString(managedSubagentDirectCallError(p, "scheduled"))
		}
	}
	switch e.Target {
	case cadence.TargetIntent:
		return nil
	case cadence.TargetWorkflow:
		if strings.TrimSpace(e.Workflow) == "" {
			return errString("workflow schedule missing workflow target")
		}
		if _, ok := s.k.Workflows().Get(e.Workflow); !ok {
			return errString("unknown workflow: " + e.Workflow)
		}
	case cadence.TargetSystemTask:
		if !cadence.IsSystemTask(e.SystemTask) {
			return errString("unknown system task: " + e.SystemTask)
		}
	case cadence.TargetTool:
		if strings.TrimSpace(e.Tool) == "" {
			return errString("tool schedule missing tool target")
		}
		if _, ok := s.k.Tools()[e.Tool]; !ok {
			return errString("unknown tool: " + e.Tool)
		}
		if strings.TrimSpace(e.Agent) != "" {
			p, found := s.k.Roster().Get(e.Agent)
			if !found {
				return errString("unknown agent: " + e.Agent)
			}
			if err := validateAgentScheduledTool(p, e.Tool); err != nil {
				return err
			}
		}
	default:
		return errString("unknown schedule target: " + e.Target)
	}
	return nil
}

func validateAgentScheduledTool(p roster.Profile, toolName string) error {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if name == "" {
		return nil
	}
	if stringSet(p.ToolDeny)[name] {
		return errString("agent " + p.Slug + " cannot schedule tool " + toolName + ": agent tool denylist")
	}
	allow := stringSet(p.ToolAllow)
	if len(allow) > 0 && !allow[name] {
		return errString("agent " + p.Slug + " cannot schedule tool " + toolName + ": not in agent tool allowlist")
	}
	return nil
}

func (s *Server) scheduleFrequencyWarning(e cadence.Entry) string {
	if e.IntervalSec <= 0 || (e.Mode != "" && e.Mode != cadence.ModeInterval && e.Mode != cadence.ModeWindow && e.Mode != cadence.ModeContinuous) {
		return ""
	}
	if e.Target == cadence.TargetSystemTask {
		for _, info := range cadence.SystemTaskInfos() {
			if info.Name == e.SystemTask && info.RecommendedIntervalSec > 0 && e.IntervalSec < info.RecommendedIntervalSec {
				return "system task runs more frequently than its recommended cadence"
			}
		}
		return ""
	}
	if strings.TrimSpace(e.Agent) != "" {
		if p, ok := s.k.Roster().Get(e.Agent); ok && p.System && e.IntervalSec < 8*3600 {
			return "system agent schedule is more frequent than the guardian quiet window"
		}
	}
	if e.Target == cadence.TargetIntent && e.IntervalSec < 15*60 {
		return "agent wake schedule is very frequent"
	}
	return ""
}

type errString string

func (e errString) Error() string { return string(e) }

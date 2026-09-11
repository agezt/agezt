// SPDX-License-Identifier: MIT

package controlplane

// Server handleScheduleAdd (the biggest single handler, 282 lines): the
// `schedule add` command that creates a new recurring/one-shot/daily/
// continuous/window schedule entry. Carved out of schedule.go during
// the Day 32 god file split #1.

import (
	"encoding/json"
	"net"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/cadence"
)

func (s *Server) handleScheduleAdd(conn net.Conn, req Request) {
	sa, err := argStrings(req.Args, "intent", "model", "agent", "target", "workflow", "system_task", "tool")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	intent, model := sa["intent"], sa["model"]
	agent := strings.TrimSpace(sa["agent"])
	target := strings.TrimSpace(sa["target"])
	workflowRef := strings.TrimSpace(sa["workflow"])
	systemTask := strings.TrimSpace(sa["system_task"])
	toolName := strings.TrimSpace(sa["tool"])
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
			s.fail(conn, req, err)
			return
		}
	}

	var e cadence.Entry
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
		tz, _, tzErr := argString(req.Args, "tz")
		if tzErr != nil {
			s.fail(conn, req, tzErr)
			return
		}
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
		tz, _, tzErr := argString(req.Args, "tz")
		if tzErr != nil {
			s.fail(conn, req, tzErr)
			return
		}
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
		s.fail(conn, req, err)
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


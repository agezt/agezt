// SPDX-License-Identifier: MIT

package controlplane

// Server handleScheduleEdit (340 lines): the `schedule edit` command that
// reschedules an existing entry (interval / at / endMinutes / days / tz /
// onceAt / model). Carved out of schedule.go during the Day 32 god
// file split #1.

import (
	"encoding/json"
	"net"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/cadence"
)

func (s *Server) handleScheduleEdit(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	store := s.k.Schedules()
	current, ok := store.Get(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": false}})
		return
	}
	now := time.Now()
	sa, err := argStrings(req.Args, "target", "workflow", "system_task", "tool")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	target := strings.TrimSpace(sa["target"])
	workflowRef := strings.TrimSpace(sa["workflow"])
	systemTask := strings.TrimSpace(sa["system_task"])
	toolName := strings.TrimSpace(sa["tool"])
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
					s.fail(conn, req, err)
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
		s.fail(conn, req, err)
		return
	}

	// Field edits (any subset). A failure on intent (empty) is reported.
	if v, ok := req.Args["intent"]; ok {
		intent, _ := v.(string)
		if _, err := store.SetIntent(id, intent); err != nil {
			s.fail(conn, req, err)
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
			s.fail(conn, req, err)
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
				s.fail(conn, req, err)
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
				s.fail(conn, req, err)
				return
			}
		}
	} else if _, ok := req.Args["target"]; ok && target == cadence.TargetIntent {
		if _, err := store.SetIntentTarget(id); err != nil {
			s.fail(conn, req, err)
			return
		}
	}

	// At most one cadence change: once | continuous | window | daily | interval.
	tz, _, err := argString(req.Args, "tz")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
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
		s.fail(conn, req, err)
		return
	}

	e, _ := store.Get(id)
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: mergeScheduleEntryView(scheduleEntryView(e), map[string]any{"updated": true}),
	})
}


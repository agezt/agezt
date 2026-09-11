// SPDX-License-Identifier: MIT

package controlplane

// Schedule misc handlers + validation: handleScheduleSystemTasks +
// handleScheduleRemove + handleScheduleRun + validateScheduleRunnable +
// validateAgentScheduledTool + scheduleFrequencyWarning. Carved out of
// schedule.go during the Day 32 god file split #1.

import (
	"net"
	"strings"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
)

func (s *Server) handleScheduleSystemTasks(conn net.Conn, req Request) {
	tasks := cadence.SystemTasks()
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"system_tasks": tasks, "system_task_info": cadence.SystemTaskInfos(), "count": len(tasks)},
	})
}

func (s *Server) handleScheduleRemove(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removed, err := s.k.Schedules().Remove(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed}})
}

func (s *Server) handleScheduleRun(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if e, ok := s.k.Schedules().Get(id); ok {
		if err := s.validateScheduleRunnable(e); err != nil {
			s.fail(conn, req, err)
			return
		}
	}
	triggered, err := s.k.Schedules().RunNow(id)
	if err != nil {
		s.fail(conn, req, err)
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


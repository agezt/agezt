// SPDX-License-Identifier: MIT

package controlplane

// Schedule read-side + view helpers: handleScheduleList + scheduleEntryView
// + mergeScheduleEntryView + scheduleArgNumber + validateScheduleEditCadenceArgs
// + scheduleExecutionMetadata + schedulePayloadContract. Carved out of
// schedule.go during the Day 32 god file split #1.

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/cadence"
)

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


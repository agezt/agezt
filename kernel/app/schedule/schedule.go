// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/cadence"
	"strings"
	"time"
)

type Reader interface {
	List() []cadence.Entry
	Get(string) (cadence.Entry, bool)
}
type LastFiring struct {
	Status  string
	Reason  string
	FiredMS int64
}
type Host struct {
	LastFirings func() (map[string]LastFiring, error)
	Validate    func(cadence.Entry) error
	Warning     func(cadence.Entry) string
}
type Service struct {
	reader Reader
	host   Host
	now    func() time.Time
}

func New(reader Reader, host Host, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{reader: reader, host: host, now: now}
}

type Record struct {
	ID                 string          `json:"id"`
	Intent             string          `json:"intent"`
	Mode               string          `json:"mode"`
	IntervalSec        int64           `json:"interval_sec"`
	AtMinutes          int             `json:"at_minutes"`
	EndMinutes         int             `json:"end_minutes"`
	Days               int             `json:"days"`
	TZ                 string          `json:"tz"`
	Cadence            string          `json:"cadence"`
	Model              string          `json:"model"`
	Agent              string          `json:"agent"`
	Target             string          `json:"target"`
	Workflow           string          `json:"workflow"`
	SystemTask         string          `json:"system_task"`
	Tool               string          `json:"tool"`
	Payload            json.RawMessage `json:"payload"`
	Source             string          `json:"source"`
	Enabled            bool            `json:"enabled"`
	CreatedUnix        int64           `json:"created_unix"`
	LastRunUnix        int64           `json:"last_run_unix"`
	NextRunUnix        int64           `json:"next_run_unix"`
	Fires              int64           `json:"fires"`
	Assure             int             `json:"assure"`
	Executor           string          `json:"executor"`
	UsesLLM            bool            `json:"uses_llm"`
	ExecutionContract  string          `json:"execution_contract"`
	ExecutionAuthority string          `json:"execution_authority"`
	IdentityOwner      string          `json:"identity_owner"`
	PayloadContract    string          `json:"payload_contract"`
	LLMBoundary        string          `json:"llm_boundary"`
	LastStatus         *string         `json:"last_status,omitempty"`
	LastReason         *string         `json:"last_reason,omitempty"`
	LastFiredMS        *int64          `json:"last_fired_unix_ms,omitempty"`
	TargetStatus       string          `json:"target_status,omitempty"`
	TargetError        string          `json:"target_error,omitempty"`
	FrequencyWarning   string          `json:"frequency_warning,omitempty"`
}

func Project(e cadence.Entry) Record {
	meta := ExecutionMetadata(e)
	return Record{ID: e.ID, Intent: e.Intent, Mode: e.Mode, IntervalSec: e.IntervalSec, AtMinutes: e.AtMinutes, EndMinutes: e.EndMinutes, Days: e.Days, TZ: e.TZ, Cadence: e.Cadence(), Model: e.Model, Agent: e.Agent, Target: e.Target, Workflow: e.Workflow, SystemTask: e.SystemTask, Tool: e.Tool, Payload: e.Payload, Source: e.Source, Enabled: e.Enabled, CreatedUnix: e.CreatedUnix, LastRunUnix: e.LastRunUnix, NextRunUnix: e.NextRunUnix, Fires: e.Fires, Assure: e.Assure, Executor: meta.Executor, UsesLLM: meta.UsesLLM, ExecutionContract: meta.Contract, ExecutionAuthority: meta.Authority, IdentityOwner: meta.IdentityOwner, PayloadContract: meta.PayloadContract, LLMBoundary: meta.LLMBoundary}
}

type ListInput struct{}
type ListOutput struct {
	Schedules []Record `json:"schedules"`
	Count     int      `json:"count"`
}

func (s *Service) List(_ context.Context, _ ListInput) (ListOutput, error) {
	entries := s.reader.List()
	latest := map[string]LastFiring{}
	if s.host.LastFirings != nil {
		latest, _ = s.host.LastFirings()
	}
	out := make([]Record, 0, len(entries))
	for _, entry := range entries {
		row := Project(entry)
		if firing, found := latest[entry.ID]; found {
			row.LastStatus = &firing.Status
			row.LastReason = &firing.Reason
			row.LastFiredMS = &firing.FiredMS
		}
		row.TargetStatus = "ready"
		if s.host.Validate != nil {
			if err := s.host.Validate(entry); err != nil {
				row.TargetStatus = "blocked"
				row.TargetError = err.Error()
			}
		}
		if s.host.Warning != nil {
			row.FrequencyWarning = s.host.Warning(entry)
		}
		out = append(out, row)
	}
	return ListOutput{Schedules: out, Count: len(out)}, nil
}

type SystemTasksInput struct{}
type SystemTasksOutput struct {
	SystemTasks    []string                 `json:"system_tasks"`
	SystemTaskInfo []cadence.SystemTaskInfo `json:"system_task_info"`
	Count          int                      `json:"count"`
}

func (s *Service) SystemTasks(_ context.Context, _ SystemTasksInput) (SystemTasksOutput, error) {
	tasks := cadence.SystemTasks()
	return SystemTasksOutput{SystemTasks: tasks, SystemTaskInfo: cadence.SystemTaskInfos(), Count: len(tasks)}, nil
}

type TestInput struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}
type Fire struct {
	Unix int64 `json:"unix"`
}
type TestOutput struct {
	Found     bool    `json:"found"`
	ID        *string `json:"id,omitempty"`
	Mode      *string `json:"mode,omitempty"`
	Cadence   *string `json:"cadence,omitempty"`
	Enabled   *bool   `json:"enabled,omitempty"`
	Forecasts *[]Fire `json:"forecasts,omitempty"`
	Count     *int    `json:"count,omitempty"`
}

func (s *Service) Test(_ context.Context, in TestInput) (TestOutput, error) {
	entry, found := s.reader.Get(in.ID)
	if !found {
		return TestOutput{Found: false}, nil
	}
	fires := entry.Forecast(s.now(), in.Count)
	out := make([]Fire, 0, len(fires))
	for _, fire := range fires {
		out = append(out, Fire{Unix: fire})
	}
	id, mode, cadenceText, enabled, count := entry.ID, entry.Mode, entry.Cadence(), entry.Enabled, len(out)
	return TestOutput{Found: true, ID: &id, Mode: &mode, Cadence: &cadenceText, Enabled: &enabled, Forecasts: &out, Count: &count}, nil
}

type ExecutionMeta struct {
	Executor        string
	UsesLLM         bool
	Contract        string
	Authority       string
	IdentityOwner   string
	PayloadContract string
	LLMBoundary     string
}

func ExecutionMetadata(e cadence.Entry) ExecutionMeta {
	agentSlug := strings.TrimSpace(e.Agent)
	payloadContract := PayloadContract(e)
	switch e.Target {
	case cadence.TargetWorkflow:
		workflowRef := strings.TrimSpace(e.Workflow)
		if agentSlug != "" {
			return ExecutionMeta{
				Executor: "workflow", UsesLLM: true,
				Contract:  "cron runs workflow " + workflowRef + " as " + agentSlug,
				Authority: "agent " + agentSlug, IdentityOwner: "agent " + agentSlug,
				PayloadContract: payloadContract, LLMBoundary: "workflow may use LLM nodes under workflow policy and invoking agent authority",
			}
		}
		return ExecutionMeta{
			Executor: "workflow", UsesLLM: true,
			Contract:  "cron runs workflow " + workflowRef + " under system identity",
			Authority: "system identity", IdentityOwner: "none; workflow is a reusable graph",
			PayloadContract: payloadContract, LLMBoundary: "workflow may use LLM nodes under workflow policy, but no agent identity is woken",
		}
	case cadence.TargetSystemTask:
		return ExecutionMeta{
			Executor: "daemon", UsesLLM: false,
			Contract:  "cron runs daemon system task " + strings.TrimSpace(e.SystemTask),
			Authority: "daemon", IdentityOwner: "none; daemon task owns no agent soul",
			PayloadContract: payloadContract, LLMBoundary: "no LLM",
		}
	case cadence.TargetTool:
		toolName := strings.TrimSpace(e.Tool)
		if agentSlug != "" {
			return ExecutionMeta{
				Executor: "tool", UsesLLM: false,
				Contract:  "cron invokes tool " + toolName + " as " + agentSlug,
				Authority: "agent " + agentSlug, IdentityOwner: "agent " + agentSlug + " tool policy",
				PayloadContract: payloadContract, LLMBoundary: "no LLM; direct tool invocation",
			}
		}
		return ExecutionMeta{
			Executor: "tool", UsesLLM: false,
			Contract:  "cron invokes tool " + toolName + " under system identity",
			Authority: "system identity", IdentityOwner: "none; tool call owns no agent soul",
			PayloadContract: payloadContract, LLMBoundary: "no LLM; direct tool invocation",
		}
	default:
		if agentSlug != "" {
			return ExecutionMeta{
				Executor: "agent", UsesLLM: true,
				Contract:  "cron wakes agent " + agentSlug,
				Authority: "agent " + agentSlug, IdentityOwner: "agent " + agentSlug,
				PayloadContract: payloadContract, LLMBoundary: "LLM runs inside the agent wake; schedule stores only cadence and task text",
			}
		}
		return ExecutionMeta{
			Executor: "llm", UsesLLM: true,
			Contract:  "cron runs governed LLM task",
			Authority: "system identity", IdentityOwner: "none; ad-hoc governed task",
			PayloadContract: payloadContract, LLMBoundary: "LLM run has no durable agent identity",
		}
	}
}

func PayloadContract(e cadence.Entry) string {
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

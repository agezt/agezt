// SPDX-License-Identifier: MIT

package schedule

import (
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/cadence"
	"strings"
)

// AddTargetInput carries target admission data before cadence parsing or writes.
type AddTargetInput struct {
	Intent, Model, Agent, Target, Workflow, SystemTask, Tool string
	Payload                                                  any
	PayloadPresent                                           bool
}

// AddTarget is the admitted and canonical target binding for schedule creation.
type AddTarget struct {
	Intent, Model, Agent, Target, Workflow, SystemTask, Tool string
	WorkflowPayload, ToolPayload                             json.RawMessage
}

func (s *Admission) AddTarget(in AddTargetInput) (AddTarget, error) {
	intent, model := in.Intent, in.Model
	agent := strings.TrimSpace(in.Agent)
	target := strings.TrimSpace(in.Target)
	workflowRef := strings.TrimSpace(in.Workflow)
	systemTask := strings.TrimSpace(in.SystemTask)
	toolName := strings.TrimSpace(in.Tool)
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
			return AddTarget{}, errors.New("workflow schedules cannot also set args.system_task")
		}
		if toolName != "" {
			return AddTarget{}, errors.New("workflow schedules cannot also set args.tool")
		}
		w, ok := s.host.WorkflowName(workflowRef)
		if !ok {
			return AddTarget{}, errors.New("unknown workflow: " + workflowRef)
		}
		workflowRef = w
		if strings.TrimSpace(intent) == "" {
			intent = "workflow " + w
		}
		if payload := in.Payload; in.PayloadPresent {
			b, err := json.Marshal(payload)
			if err != nil {
				return AddTarget{}, errors.New("payload must be JSON-serializable: " + err.Error())
			}
			workflowPayload = b
		}
	case cadence.TargetSystemTask:
		if agent != "" {
			return AddTarget{}, errors.New("system task schedules cannot also set args.agent")
		}
		if workflowRef != "" {
			return AddTarget{}, errors.New("system task schedules cannot also set args.workflow")
		}
		if toolName != "" {
			return AddTarget{}, errors.New("system task schedules cannot also set args.tool")
		}
		if !cadence.IsSystemTask(systemTask) {
			return AddTarget{}, errors.New("unknown system task: " + systemTask)
		}
		if in.PayloadPresent {
			return AddTarget{}, errors.New("system task schedules do not accept args.payload")
		}
		if strings.TrimSpace(intent) == "" {
			intent = "system task " + systemTask
		}
	case cadence.TargetTool:
		if toolName == "" {
			return AddTarget{}, errors.New("args.tool required for tool schedules")
		}
		if workflowRef != "" {
			return AddTarget{}, errors.New("tool schedules cannot also set args.workflow")
		}
		if systemTask != "" {
			return AddTarget{}, errors.New("tool schedules cannot also set args.system_task")
		}
		if !s.host.Tool(toolName) {
			return AddTarget{}, errors.New("unknown tool: " + toolName)
		}
		if strings.TrimSpace(intent) == "" {
			intent = "tool " + toolName
		}
		if payload := in.Payload; in.PayloadPresent {
			b, err := json.Marshal(payload)
			if err != nil {
				return AddTarget{}, errors.New("payload must be JSON-serializable: " + err.Error())
			}
			toolPayload = b
		}
	case cadence.TargetIntent:
		if strings.TrimSpace(intent) == "" {
			return AddTarget{}, errors.New("agent task text required for target=intent schedules")
		}
	default:
		return AddTarget{}, errors.New("unknown schedule target: " + target)
	}
	if agent != "" {
		p, ok := s.host.Agent(agent)
		if !ok {
			return AddTarget{}, errors.New("unknown agent: " + agent)
		}
		if p.Retired {
			return AddTarget{}, errors.New("agent " + p.Slug + " is retired — revive it first")
		}
		if !p.Enabled {
			return AddTarget{}, errors.New("agent " + p.Slug + " is paused")
		}
		if !p.AllowsDirectCall() {
			return AddTarget{}, errors.New(s.host.ManagedDirectError(p, "scheduled"))
		}
		agent = p.Slug
	}
	if target == cadence.TargetTool && agent != "" {
		p, _ := s.host.Agent(agent)
		if err := ValidateAgentTool(p, toolName); err != nil {
			return AddTarget{}, err
		}
	}

	return AddTarget{Intent: intent, Model: model, Agent: agent, Target: target, Workflow: workflowRef, SystemTask: systemTask, Tool: toolName, WorkflowPayload: workflowPayload, ToolPayload: toolPayload}, nil
}

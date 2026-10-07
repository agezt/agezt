// SPDX-License-Identifier: MIT

package schedule

import (
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/cadence"
	"strings"
)

// EditTargetInput retains field presence separately from selected string values.
type EditTargetInput struct {
	Current                                     cadence.Entry
	Target, Workflow, SystemTask, Tool, Agent   string
	TargetPresent, AgentPresent, PayloadPresent bool
	Payload                                     any
}

// EditTarget is the admitted target/agent change, before cadence checks or writes.
type EditTarget struct{ Target, Workflow, SystemTask, Tool, Agent string }

func (s *Admission) EditTarget(in EditTargetInput) (EditTarget, error) {
	target := strings.TrimSpace(in.Target)
	workflowRef := strings.TrimSpace(in.Workflow)
	systemTask := strings.TrimSpace(in.SystemTask)
	toolName := strings.TrimSpace(in.Tool)
	targetSet := false
	if in.TargetPresent {
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
	effectiveTarget := in.Current.Target
	if targetSet {
		effectiveTarget = target
	}
	if effectiveTarget == cadence.TargetSystemTask {
		if in.PayloadPresent {
			return EditTarget{}, errors.New("system task schedules do not accept args.payload")
		}
	}
	editAgent := ""
	effectiveAgent := strings.TrimSpace(in.Current.Agent)
	if in.AgentPresent {
		agent := in.Agent
		agent = strings.TrimSpace(agent)
		if agent != "" && effectiveTarget == cadence.TargetSystemTask {
			return EditTarget{}, errors.New("system task schedules cannot also set args.agent")
		}
		if agent != "" {
			p, found := s.host.Agent(agent)
			if !found {
				return EditTarget{}, errors.New("unknown agent: " + agent)
			}
			if p.Retired {
				return EditTarget{}, errors.New("agent " + p.Slug + " is retired — revive it first")
			}
			if !p.Enabled {
				return EditTarget{}, errors.New("agent " + p.Slug + " is paused")
			}
			if !p.AllowsDirectCall() {
				return EditTarget{}, errors.New(s.host.ManagedDirectError(p, "scheduled"))
			}
			agent = p.Slug
		}
		editAgent = agent
		effectiveAgent = agent
	}
	if target == cadence.TargetWorkflow {
		if systemTask != "" {
			return EditTarget{}, errors.New("workflow schedules cannot also set args.system_task")
		}
		if toolName != "" {
			return EditTarget{}, errors.New("workflow schedules cannot also set args.tool")
		}
		if !s.host.Workflow(workflowRef) {
			return EditTarget{}, errors.New("unknown workflow: " + workflowRef)
		}
		if p := in.Payload; in.PayloadPresent {
			if _, err := json.Marshal(p); err != nil {
				return EditTarget{}, errors.New("payload must be JSON-serializable: " + err.Error())
			}
		}
	} else if target != "" && target != cadence.TargetIntent {
		if target != cadence.TargetSystemTask {
			if target != cadence.TargetTool {
				return EditTarget{}, errors.New("unknown schedule target: " + target)
			}
			if workflowRef != "" {
				return EditTarget{}, errors.New("tool schedules cannot also set args.workflow")
			}
			if systemTask != "" {
				return EditTarget{}, errors.New("tool schedules cannot also set args.system_task")
			}
			if toolName == "" {
				return EditTarget{}, errors.New("args.tool required for tool schedules")
			}
			if !s.host.Tool(toolName) {
				return EditTarget{}, errors.New("unknown tool: " + toolName)
			}
			if effectiveAgent != "" {
				p, found := s.host.Agent(effectiveAgent)
				if !found {
					return EditTarget{}, errors.New("unknown agent: " + effectiveAgent)
				}
				if err := ValidateAgentTool(p, toolName); err != nil {
					return EditTarget{}, err
				}
			}
			if p := in.Payload; in.PayloadPresent {
				if _, err := json.Marshal(p); err != nil {
					return EditTarget{}, errors.New("payload must be JSON-serializable: " + err.Error())
				}
			}
		} else {
			if workflowRef != "" {
				return EditTarget{}, errors.New("system task schedules cannot also set args.workflow")
			}
			if toolName != "" {
				return EditTarget{}, errors.New("system task schedules cannot also set args.tool")
			}
			if !cadence.IsSystemTask(systemTask) {
				return EditTarget{}, errors.New("unknown system task: " + systemTask)
			}
		}
	}
	return EditTarget{Target: target, Workflow: workflowRef, SystemTask: systemTask, Tool: toolName, Agent: editAgent}, nil
}

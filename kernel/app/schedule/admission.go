// SPDX-License-Identifier: MIT

package schedule

import (
	"errors"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"strings"
)

// AdmissionHost supplies selected live target and agent views.
type AdmissionHost struct {
	Agent              func(string) (agentapi.Profile, bool)
	Workflow           func(string) bool
	WorkflowName       func(string) (string, bool)
	Tool               func(string) bool
	ManagedDirectError func(agentapi.Profile, string) string
}
type Admission struct{ host AdmissionHost }

func NewAdmission(host AdmissionHost) *Admission { return &Admission{host: host} }

func (s *Admission) Runnable(e cadenceapi.Entry) error {
	if strings.TrimSpace(e.Agent) != "" {
		p, found := s.host.Agent(e.Agent)
		if !found {
			return errors.New("unknown agent: " + e.Agent)
		}
		if p.Retired {
			return errors.New("agent " + p.Slug + " is retired — revive it first")
		}
		if !p.Enabled {
			return errors.New("agent " + p.Slug + " is paused")
		}
		if !p.AllowsDirectCall() {
			return errors.New(s.host.ManagedDirectError(p, "scheduled"))
		}
	}
	switch e.Target {
	case cadenceapi.TargetIntent:
		return nil
	case cadenceapi.TargetWorkflow:
		if strings.TrimSpace(e.Workflow) == "" {
			return errors.New("workflow schedule missing workflow target")
		}
		if !s.host.Workflow(e.Workflow) {
			return errors.New("unknown workflow: " + e.Workflow)
		}
	case cadenceapi.TargetSystemTask:
		if !cadenceapi.IsSystemTask(e.SystemTask) {
			return errors.New("unknown system task: " + e.SystemTask)
		}
	case cadenceapi.TargetTool:
		if strings.TrimSpace(e.Tool) == "" {
			return errors.New("tool schedule missing tool target")
		}
		if !s.host.Tool(e.Tool) {
			return errors.New("unknown tool: " + e.Tool)
		}
		if strings.TrimSpace(e.Agent) != "" {
			p, found := s.host.Agent(e.Agent)
			if !found {
				return errors.New("unknown agent: " + e.Agent)
			}
			if err := ValidateAgentTool(p, e.Tool); err != nil {
				return err
			}
		}
	default:
		return errors.New("unknown schedule target: " + e.Target)
	}
	return nil
}

func ValidateAgentTool(p agentapi.Profile, toolName string) error {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if name == "" {
		return nil
	}
	if stringSet(p.ToolDeny)[name] {
		return errors.New("agent " + p.Slug + " cannot schedule tool " + toolName + ": agent tool denylist")
	}
	allow := stringSet(p.ToolAllow)
	if len(allow) > 0 && !allow[name] {
		return errors.New("agent " + p.Slug + " cannot schedule tool " + toolName + ": not in agent tool allowlist")
	}
	return nil
}

func (s *Admission) Warning(e cadenceapi.Entry) string {
	if e.IntervalSec <= 0 || (e.Mode != "" && e.Mode != cadenceapi.ModeInterval && e.Mode != cadenceapi.ModeWindow && e.Mode != cadenceapi.ModeContinuous) {
		return ""
	}
	if e.Target == cadenceapi.TargetSystemTask {
		for _, info := range cadenceapi.SystemTaskInfos() {
			if info.Name == e.SystemTask && info.RecommendedIntervalSec > 0 && e.IntervalSec < info.RecommendedIntervalSec {
				return "system task runs more frequently than its recommended cadence"
			}
		}
		return ""
	}
	if strings.TrimSpace(e.Agent) != "" {
		if p, ok := s.host.Agent(e.Agent); ok && p.System && e.IntervalSec < 8*3600 {
			return "system agent schedule is more frequent than the guardian quiet window"
		}
	}
	if e.Target == cadenceapi.TargetIntent && e.IntervalSec < 15*60 {
		return "agent wake schedule is very frequent"
	}
	return ""
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		if item = strings.ToLower(strings.TrimSpace(item)); item != "" {
			out[item] = true
		}
	}
	return out
}

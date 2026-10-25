// SPDX-License-Identifier: MIT

package schedule

import (
	"strings"

	"github.com/agezt/agezt/kernel/contract/cadenceapi"
)

func scheduleFiredSystemTaskInfo(name string) (cadenceapi.SystemTaskInfo, bool) {
	name = strings.TrimSpace(name)
	for _, info := range cadenceapi.SystemTaskInfos() {
		if info.Name == name {
			return info, true
		}
	}
	return cadenceapi.SystemTaskInfo{}, false
}

func scheduleFiredExecutor(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.Executor) != "" {
		return strings.TrimSpace(p.Executor)
	}
	if p.Target == cadenceapi.TargetSystemTask {
		if info, ok := scheduleFiredSystemTaskInfo(p.SystemTask); ok && strings.TrimSpace(info.Executor) != "" {
			return info.Executor
		}
		return "daemon"
	}
	if p.Target == cadenceapi.TargetWorkflow {
		return "workflow"
	}
	if p.Target == cadenceapi.TargetTool {
		return "tool"
	}
	return "agent"
}

func scheduleFiredCategory(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.Category) != "" {
		return strings.TrimSpace(p.Category)
	}
	if p.Target == cadenceapi.TargetSystemTask {
		if info, ok := scheduleFiredSystemTaskInfo(p.SystemTask); ok {
			return strings.TrimSpace(info.Category)
		}
	}
	return ""
}

func scheduleFiredEffectClass(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.EffectClass) != "" {
		return strings.TrimSpace(p.EffectClass)
	}
	if p.Target == cadenceapi.TargetSystemTask {
		if info, ok := scheduleFiredSystemTaskInfo(p.SystemTask); ok {
			return strings.TrimSpace(info.EffectClass)
		}
	}
	return ""
}

func scheduleFiredUsesLLM(p scheduleFiredPayload) bool {
	if p.UsesLLM != nil {
		return *p.UsesLLM
	}
	return p.Target == "" || p.Target == cadenceapi.TargetWorkflow
}

func scheduleFiredAction(p scheduleFiredPayload) string {
	switch p.Target {
	case cadenceapi.TargetWorkflow:
		if p.Workflow != "" {
			return "run workflow " + p.Workflow
		}
	case cadenceapi.TargetSystemTask:
		if p.SystemTask != "" {
			return "run system task " + p.SystemTask
		}
	case cadenceapi.TargetTool:
		if p.Tool != "" {
			return "run tool " + p.Tool
		}
	}
	if p.Agent != "" && p.Intent != "" {
		return "wake " + p.Agent + ": " + p.Intent
	}
	if p.Intent != "" {
		return p.Intent
	}
	return p.ScheduleID
}

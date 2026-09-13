// SPDX-License-Identifier: MIT

// Schedule-fired classifiers: scheduleFiredSystemTaskInfo +
// scheduleFiredExecutor + scheduleFiredCategory +
// scheduleFiredEffectClass + scheduleFiredUsesLLM +
// scheduleFiredAction. Carved out of schedule_fires.go during the
// Day 198 god-file split so the main file can stay focused on the
// handleScheduleFires dispatcher and the payload file can stay
// focused on the scheduleFiredPayload type + extraction.
// Public API unchanged.
package controlplane


import (
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
)

func scheduleFiredSystemTaskInfo(name string) (cadence.SystemTaskInfo, bool) {
	name = strings.TrimSpace(name)
	for _, info := range cadence.SystemTaskInfos() {
		if info.Name == name {
			return info, true
		}
	}
	return cadence.SystemTaskInfo{}, false
}

func scheduleFiredExecutor(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.Executor) != "" {
		return strings.TrimSpace(p.Executor)
	}
	if p.Target == cadence.TargetSystemTask {
		if info, ok := scheduleFiredSystemTaskInfo(p.SystemTask); ok && strings.TrimSpace(info.Executor) != "" {
			return info.Executor
		}
		return "daemon"
	}
	if p.Target == cadence.TargetWorkflow {
		return "workflow"
	}
	if p.Target == cadence.TargetTool {
		return "tool"
	}
	return "agent"
}

func scheduleFiredCategory(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.Category) != "" {
		return strings.TrimSpace(p.Category)
	}
	if p.Target == cadence.TargetSystemTask {
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
	if p.Target == cadence.TargetSystemTask {
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
	return p.Target == "" || p.Target == cadence.TargetWorkflow
}

func scheduleFiredAction(p scheduleFiredPayload) string {
	switch p.Target {
	case cadence.TargetWorkflow:
		if p.Workflow != "" {
			return "run workflow " + p.Workflow
		}
	case cadence.TargetSystemTask:
		if p.SystemTask != "" {
			return "run system task " + p.SystemTask
		}
	case cadence.TargetTool:
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

// handleScheduleStats aggregates scheduled-run firings (M57) — the autonomy
// analogue of handleRunsStats. Folds the journal's schedule.fired events, joins
// each with its run outcome (collectRuns), and reports counts, success rate, and
// total spend over scheduled runs. Optional args.id scopes to one schedule;
// args.since_ms windows by firing time.


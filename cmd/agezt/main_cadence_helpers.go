// SPDX-License-Identifier: MIT

// Cadence/scheduled-task payload + context helpers extracted from main_cadence.go
// during Day 211 god-file refactor (#52). Public API unchanged.
package main

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/cadence/systemtasks"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func scheduledRunContext(runCtx context.Context, model string, prof *roster.Profile) context.Context {
	mctx := runCtx
	if prof != nil {
		mctx = kernelruntime.WithAgentProfile(mctx, *prof)
		if prof.MaxCostMc > 0 {
			mctx = kernelruntime.WithMaxCost(mctx, prof.MaxCostMc)
		}
	}
	model = strings.TrimSpace(model)
	if model != "" {
		mctx = kernelruntime.WithModel(mctx, model)
		mctx = kernelruntime.WithModelChain(mctx, []string{model})
	}
	return mctx
}
func scheduleFiredEventPayload(id, intent, model string, ent cadence.Entry, profs ...*roster.Profile) map[string]any {
	payload := map[string]any{
		"schedule_id": id,
		"intent":      intent,
		"model":       model,
		"target":      ent.Target,
		"agent":       ent.Agent,
	}
	if len(profs) > 0 && profs[0] != nil {
		payload["autonomy_runbook"] = agentAutonomyRunbookPayload(*profs[0])
	}
	switch ent.Target {
	case cadence.TargetWorkflow:
		payload["workflow"] = ent.Workflow
		payload["executor"] = "workflow"
		payload["uses_llm"] = true
	case cadence.TargetSystemTask:
		payload["system_task"] = ent.SystemTask
		if info, ok := systemtasks.Info(ent.SystemTask); ok {
			payload["executor"] = info.Executor
			payload["category"] = info.Category
			payload["effect_class"] = info.EffectClass
			payload["uses_llm"] = info.UsesLLM
		} else {
			payload["executor"] = "daemon"
			payload["uses_llm"] = false
		}
	case cadence.TargetTool:
		payload["tool"] = ent.Tool
		payload["executor"] = "tool"
		payload["uses_llm"] = false
	default:
		payload["executor"] = "agent"
		payload["uses_llm"] = true
	}
	return payload
}
// agentAutonomyRunbookPayload attaches the machine-readable wake contract to
// autonomous wake evidence (schedule.fired and standing.fired) when the firing
// resolves a concrete agent profile. It delegates to the canonical roster builder
// so operator, schedule, standing, and delegated wakes all carry an
// identically-shaped runbook through the journal.
func agentAutonomyRunbookPayload(p roster.Profile) map[string]any {
	return roster.AutonomyRunbook(p)
}

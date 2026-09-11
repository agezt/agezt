// SPDX-License-Identifier: MIT

package selfrepair

// Auto-repair resolution parsing + sanitization (M846): the pure-logic
// half of the resolution pipeline that lives independently of the
// coordinator's bus/board side. Carved out of selfrepair.go during the
// Day 25 god file split #2 so the main file can focus on the
// coordinator state machine.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/strutil"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/board"
)

func parseAutoRepairResolution(finalText string) *autoRepairResolution {
	if strings.TrimSpace(finalText) == "" {
		return nil
	}
	var candidates []string
	for _, block := range strings.Split(finalText, "```") {
		b := strings.TrimSpace(block)
		if b == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(b), "json") {
			b = strings.TrimSpace(b[4:])
		}
		if strings.HasPrefix(b, "{") && strings.Contains(b, "}") {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		last := strings.LastIndex(finalText, "{")
		end := strings.LastIndex(finalText, "}")
		if last >= 0 && end > last {
			candidates = append(candidates, finalText[last:end+1])
		}
	}
	for i := len(candidates) - 1; i >= 0; i-- {
		var out autoRepairResolution
		if err := json.Unmarshal([]byte(strings.TrimSpace(candidates[i])), &out); err != nil {
			continue
		}
		cleanAutoRepairResolution(&out)
		if out.Resolution != "" {
			return &out
		}
	}
	return nil
}

func sanitizeTaskModelChain(models []string) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			out = append(out, model)
		}
	}
	return out
}

func cleanAutoRepairResolution(out *autoRepairResolution) {
	if out == nil {
		return
	}
	out.Resolution = strings.TrimSpace(strings.ToLower(out.Resolution))
	switch out.Resolution {
	case "handled", "paused", "retired", "delegated", "blocked", "force_chain":
	default:
		out.Resolution = ""
	}
	out.Summary = strings.TrimSpace(strings.Join(strings.Fields(out.Summary), " "))
	out.DelegateTo = strings.TrimSpace(out.DelegateTo)
	out.TaskType = strings.TrimSpace(out.TaskType)
	out.TaskModelChain = sanitizeTaskModelChain(out.TaskModelChain)
	if out.Resolution != "delegated" {
		out.DelegateTo = ""
	}
	if out.Resolution != "force_chain" {
		out.TaskType = ""
		out.TaskModelChain = nil
	}
	if out.Resolution == "force_chain" && (out.TaskType == "" || len(out.TaskModelChain) == 0) {
		out.Resolution = ""
		out.TaskType = ""
		out.TaskModelChain = nil
	}
}

func (c *autoRepairCoordinator) applyAutoRepairResolution(ctx context.Context, k *kernelruntime.Kernel, src autoRepairSource, mailbox Mailbox, postNotify func(board.Message, string), cand autoRepairCandidate, wake autoRepairWakeResult) (*autoRepairResolutionOutcome, error) {
	if wake.Resolution == nil || wake.Resolution.Resolution == "" {
		return nil, nil
	}
	if err := validateAutoRepairResolution(cand, wake); err != nil {
		return nil, err
	}
	switch wake.Resolution.Resolution {
	case "handled", "blocked":
		return nil, nil
	case "paused":
		if k == nil {
			return nil, fmt.Errorf("paused resolution requires kernel access")
		}
		_, err := k.SetProfileEnabled(cand.Slug, false)
		if err != nil {
			return nil, err
		}
		return &autoRepairResolutionOutcome{Phase: "resolution_applied"}, nil
	case "retired":
		if k == nil {
			return nil, fmt.Errorf("retired resolution requires kernel access")
		}
		reason := strings.TrimSpace(wake.Resolution.Summary)
		if reason == "" {
			reason = "retired by escalation resolution"
		}
		_, err := k.SetProfileRetired(cand.Slug, true, reason)
		if err != nil {
			return nil, err
		}
		return &autoRepairResolutionOutcome{Phase: "resolution_applied"}, nil
	case "force_chain":
		return c.applyForcedRoutingResolution(k, src, cand, wake)
	case "delegated":
		if k == nil {
			return nil, fmt.Errorf("delegated resolution requires kernel access")
		}
		return nil, c.applyDelegatedResolution(ctx, k, k.Bus(), mailbox, postNotify, cand, wake)
	default:
		return nil, nil
	}
}

func (c *autoRepairCoordinator) applyForcedRoutingResolution(k *kernelruntime.Kernel, src autoRepairSource, cand autoRepairCandidate, wake autoRepairWakeResult) (*autoRepairResolutionOutcome, error) {
	if wake.Resolution == nil {
		return nil, nil
	}
	if src == nil {
		return nil, fmt.Errorf("force_chain resolution requires an active repair source")
	}
	applier, ok := src.(autoRepairRoutingChainApplier)
	if !ok {
		return nil, fmt.Errorf("force_chain resolution is not supported by the active repair source")
	}
	taskType := strings.TrimSpace(wake.Resolution.TaskType)
	if taskType == "" || len(wake.Resolution.TaskModelChain) == 0 {
		return nil, fmt.Errorf("force_chain resolution requires task_type and task_model_chain")
	}
	if cand.Mode == "routing_forced_exhausted" &&
		taskType == strings.TrimSpace(cand.RoutingRollbackTaskType) &&
		equalStringSlices(wake.Resolution.TaskModelChain, cand.RoutingRollbackToChain) {
		return nil, fmt.Errorf("force_chain resolution must choose a new chain for exhausted routing policy")
	}
	prevGeneration := autoRepairLatestForceGeneration(k, cand.Slug, taskType)
	nextGeneration := prevGeneration + 1
	if nextGeneration <= 0 {
		nextGeneration = 1
	}
	reason := strings.TrimSpace(wake.Resolution.Summary)
	if reason == "" {
		reason = "forced by escalation resolution"
	}
	res, err := applier.ApplyRoutingChain(cand.Slug, taskType, wake.Resolution.TaskModelChain, reason)
	if err != nil {
		return nil, err
	}
	return &autoRepairResolutionOutcome{
		Phase:                          "resolution_applied",
		RoutingTaskType:                strutil.FirstNonEmpty(res.RoutingTaskType, taskType),
		RoutingTaskModelChain:          strutil.FirstNonEmptySlice(res.RoutingTaskModelChain, wake.Resolution.TaskModelChain),
		PreviousRoutingTaskModelChain:  res.PreviousRoutingTaskModelChain,
		RoutingForceGeneration:         nextGeneration,
		PreviousRoutingForceGeneration: prevGeneration,
	}, nil
}

func validateAutoRepairResolution(cand autoRepairCandidate, wake autoRepairWakeResult) error {
	if wake.Resolution == nil {
		return nil
	}
	if strings.TrimSpace(cand.Mode) != "routing_forced_exhausted" {
		return nil
	}
	switch strings.TrimSpace(wake.Resolution.Resolution) {
	case "paused", "retired", "delegated", "force_chain":
		return nil
	case "handled", "blocked":
		return fmt.Errorf("%s resolution is not allowed for exhausted routing policy", strings.TrimSpace(wake.Resolution.Resolution))
	default:
		return nil
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}


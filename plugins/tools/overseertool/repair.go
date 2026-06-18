// SPDX-License-Identifier: MIT

package overseertool

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

type repairProposal struct {
	Soul            string            `json:"soul"`
	Model           string            `json:"model"`
	Fallbacks       []string          `json:"fallbacks"`
	TaskType        string            `json:"task_type"`
	TaskModelChain  []string          `json:"task_model_chain"`
	ConfigOverrides map[string]string `json:"config_overrides"`
}

func buildRepairBrief(p roster.Profile, rep kernelruntime.ReaperReport, reason string, currentTaskChain []string) string {
	var out []string
	out = append(out,
		`You are the agent "`+p.Slug+`". This is an autonomous self-repair run requested by the overseer.`,
	)
	if reason = strings.TrimSpace(reason); reason != "" {
		out = append(out, "Repair request context: "+reason)
	}
	var cfg []string
	if p.Model != "" {
		cfg = append(cfg, "model="+p.Model)
	}
	if len(p.Fallbacks) > 0 {
		cfg = append(cfg, "fallbacks="+strings.Join(p.Fallbacks, " -> "))
	}
	if p.TaskType != "" {
		cfg = append(cfg, "task_type="+p.TaskType)
	}
	if p.Workdir != "" {
		cfg = append(cfg, "workdir="+p.Workdir)
	}
	if p.MemoryScope != "" {
		cfg = append(cfg, "memory_scope="+p.MemoryScope)
	}
	if len(currentTaskChain) > 0 {
		cfg = append(cfg, "task_model_chain="+strings.Join(currentTaskChain, " -> "))
	}
	if len(cfg) > 0 {
		out = append(out, "Your current configuration: "+strings.Join(cfg, ", ")+".")
	}
	out = append(out, "", "## Evidence")
	if row := findMisconfigured(rep, p.Slug); row != nil {
		out = append(out, "- Invalid runtime overrides:")
		for _, issue := range row.Issues {
			out = append(out, "  • "+clip(issue, 220))
		}
	}
	if row := findDegraded(rep, p.Slug); row != nil {
		line := "- Recent failure pressure: " + itoa(row.Failures) + " failed run(s)"
		if row.Window > 0 {
			line += " in the last " + itoa(row.Window) + " judged run(s)"
		}
		if row.LastReason != "" {
			line += " — last reason: " + clip(row.LastReason, 220)
		}
		out = append(out, line)
	}
	if row := findRoutingPressure(rep, p.Slug); row != nil {
		line := "- Model-chain fallback pressure: " + itoa(row.Count) + " fallback hop(s)"
		if row.WindowSec > 0 {
			line += " in the last " + itoa(row.WindowSec) + " second(s)"
		}
		if row.Threshold > 0 {
			line += " (threshold " + itoa(row.Threshold) + ")"
		}
		if row.TaskType != "" {
			line += " for task_type=" + row.TaskType
		}
		if row.LastFailedModel != "" || row.LastNextModel != "" {
			line += " — last hop: " + strings.TrimSpace(row.LastFailedModel)
			if row.LastNextModel != "" {
				line += " -> " + row.LastNextModel
			}
		}
		if row.LastReason != "" {
			line += " — last reason: " + clip(row.LastReason, 220)
		}
		out = append(out, line)
	}
	if len(out) == 2 || (len(out) == 3 && strings.HasPrefix(out[2], "Your current configuration:")) {
		out = append(out, "- No explicit reaper failure signal was attached — look for latent weaknesses instead.")
	}
	out = append(out,
		"",
		"## Your task",
		"1. Diagnose the root cause from the evidence above.",
		"2. FIX IT YOURSELF using your tools: edit your own files, repair your own agent-scoped config, and remove invalid runtime overrides where appropriate.",
		"3. If your durable profile or routing is the problem (soul/model/fallbacks/task_type/task_model_chain/config_overrides), end your answer with EXACTLY ONE fenced code block tagged json containing only the fields you want changed:",
		"```json",
		`{ "soul": "<full revised system prompt>", "model": "<primary model>", "fallbacks": ["<m1>", "<m2>"], "task_type": "<task type>", "task_model_chain": ["<m1>", "<m2>"], "config_overrides": { "AGEZT_MAX_ITER": "6" } }`,
		"```",
		"Include only the keys you are actually changing. Use task_model_chain only when you are intentionally changing the durable task routing for this agent's task type. Omit the block entirely if no durable change is needed. That block will be applied automatically.",
	)
	return strings.Join(out, "\n")
}

func findMisconfigured(rep kernelruntime.ReaperReport, slug string) *kernelruntime.MisconfiguredAgent {
	for i := range rep.MisconfiguredAgents {
		if rep.MisconfiguredAgents[i].Slug == slug {
			return &rep.MisconfiguredAgents[i]
		}
	}
	return nil
}

func findDegraded(rep kernelruntime.ReaperReport, slug string) *kernelruntime.DegradedAgent {
	for i := range rep.DegradedAgents {
		if rep.DegradedAgents[i].Slug == slug {
			return &rep.DegradedAgents[i]
		}
	}
	return nil
}

func findRoutingPressure(rep kernelruntime.ReaperReport, slug string) *kernelruntime.RoutingPressureAgent {
	for i := range rep.RoutingPressure {
		if rep.RoutingPressure[i].Slug == slug {
			return &rep.RoutingPressure[i]
		}
	}
	return nil
}

func parseRepairProposal(finalText string) *repairProposal {
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
		var p repairProposal
		if err := json.Unmarshal([]byte(strings.TrimSpace(candidates[i])), &p); err != nil {
			continue
		}
		cleanRepairProposal(&p)
		if p.Soul != "" || p.Model != "" || len(p.Fallbacks) > 0 || p.TaskType != "" || len(p.TaskModelChain) > 0 || len(p.ConfigOverrides) > 0 {
			return &p
		}
	}
	return nil
}

func cleanRepairProposal(p *repairProposal) {
	if p == nil {
		return
	}
	p.Soul = strings.TrimSpace(p.Soul)
	p.Model = strings.TrimSpace(p.Model)
	p.TaskType = strings.TrimSpace(p.TaskType)
	fb := p.Fallbacks[:0]
	for _, s := range p.Fallbacks {
		if s = strings.TrimSpace(s); s != "" {
			fb = append(fb, s)
		}
	}
	p.Fallbacks = fb
	chain := p.TaskModelChain[:0]
	for _, s := range p.TaskModelChain {
		if s = strings.TrimSpace(s); s != "" {
			chain = append(chain, s)
		}
	}
	p.TaskModelChain = chain
	if len(p.ConfigOverrides) > 0 {
		out := map[string]string{}
		for k, v := range p.ConfigOverrides {
			k = strings.TrimSpace(strings.ToUpper(k))
			if k == "" {
				continue
			}
			out[k] = strings.TrimSpace(v)
		}
		p.ConfigOverrides = out
	}
}

func applyRepairProposal(dst *roster.Profile, prop *repairProposal) []string {
	if dst == nil || prop == nil {
		return nil
	}
	var applied []string
	if prop.Soul != "" {
		dst.Soul = prop.Soul
		applied = append(applied, "soul")
	}
	if prop.Model != "" {
		dst.Model = prop.Model
		applied = append(applied, "model")
	}
	if len(prop.Fallbacks) > 0 {
		dst.Fallbacks = append([]string(nil), prop.Fallbacks...)
		applied = append(applied, "fallbacks")
	}
	if prop.TaskType != "" {
		dst.TaskType = prop.TaskType
		applied = append(applied, "task_type")
	}
	if len(prop.ConfigOverrides) > 0 {
		dst.ConfigOverrides = map[string]string{}
		for k, v := range prop.ConfigOverrides {
			dst.ConfigOverrides[k] = v
		}
		applied = append(applied, "config_overrides")
	}
	return applied
}

func repairTaskType(p roster.Profile, rep kernelruntime.ReaperReport) string {
	if taskType := strings.TrimSpace(p.TaskType); taskType != "" {
		return taskType
	}
	if row := findRoutingPressure(rep, p.Slug); row != nil {
		return strings.TrimSpace(row.TaskType)
	}
	return ""
}

func repairProposalTaskType(current roster.Profile, prop *repairProposal) string {
	if prop == nil {
		return ""
	}
	if taskType := strings.TrimSpace(prop.TaskType); taskType != "" {
		return taskType
	}
	return strings.TrimSpace(current.TaskType)
}

func clip(s string, max int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// SPDX-License-Identifier: MIT

// Sub-agent prompt/budget injection: subAgentInjectedSystem + subAgentSpendMicrocents.
// Code extracted from subagent.go during the Day-40 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/delegation"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/skill"
)


func (k *Kernel) subAgentInjectedSystem(ctx context.Context, corr, actor, intent, system string) (string, []string, string) {
	directive := skill.ParseActivationDirective(intent)
	if directive.Explicit && directive.CleanIntent != "" {
		intent = directive.CleanIntent
	}
	if systemAgentFromCtx(ctx) {
		return system, nil, intent
	}
	if k.cfg.MemoryInject && k.memory != nil {
		topK := k.cfg.MemoryTopK
		if topK <= 0 {
			topK = 5
		}
		if hits, err := k.memory.RecallScoped(corr, intent, topK, memory.ScopeFrom(ctx)); err == nil && len(hits) > 0 {
			system = injectMemory(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Record.ID)
			}
			chosen, rejected := contextselect.SplitCandidates(memoryContextCandidates(hits, time.Now().UnixMilli()), contextselect.ChosenIDSet(ids), "subagent_memory_injection")
			summary := contextselect.Summary(chosen, rejected)
			summary["source"] = "subagent_memory_injection"
			k.publishContextSelection(corr, actor, contextselect.Manifest{
				Phase:    "memory",
				Query:    intent,
				Chosen:   chosen,
				Rejected: rejected,
				Summary:  summary,
			})
		}
	}
	var activatedSkillIDs []string
	if k.cfg.SkillInject && k.forge != nil {
		topK := k.cfg.SkillTopK
		if topK <= 0 {
			topK = 3
		}
		agentSlug := agentSlugFromCtx(ctx)
		if directive.Explicit {
			hits, missing, err := k.forge.ActivateExplicitFor(corr, agentSlug, intent, directive.Refs, topK)
			if err == nil {
				if len(hits) > 0 {
					system = injectSkills(system, hits)
					for _, h := range hits {
						activatedSkillIDs = appendUniqueString(activatedSkillIDs, h.Skill.ID)
					}
				}
				chosen := skillContextCandidates(hits, time.Now().UnixMilli())
				for i := range chosen {
					chosen[i].Chosen = true
					chosen[i].Reason = "selected:subagent_skill_explicit_activation"
				}
				summary := contextselect.Summary(chosen, nil)
				summary["source"] = "subagent_skill_explicit_activation"
				summary["activation"] = "explicit"
				summary["refs"] = directive.Refs
				if len(missing) > 0 {
					summary["missing"] = missing
				}
				k.publishContextSelection(corr, actor, contextselect.Manifest{
					Phase:   "skill",
					Query:   intent,
					Chosen:  chosen,
					Summary: summary,
				})
			}
		} else {
			if hits, err := k.forge.ActivateFor(corr, agentSlug, intent, topK); err == nil && len(hits) > 0 {
				system = injectSkills(system, hits)
				for _, h := range hits {
					activatedSkillIDs = appendUniqueString(activatedSkillIDs, h.Skill.ID)
				}
				chosen, rejected := contextselect.SplitCandidates(skillContextCandidates(hits, time.Now().UnixMilli()), contextselect.ChosenIDSet(activatedSkillIDs), "subagent_skill_injection")
				summary := contextselect.Summary(chosen, rejected)
				summary["source"] = "subagent_skill_injection"
				k.publishContextSelection(corr, actor, contextselect.Manifest{
					Phase:    "skill",
					Query:    intent,
					Chosen:   chosen,
					Rejected: rejected,
					Summary:  summary,
				})
			}
		}
	}
	return system, activatedSkillIDs, intent
}

// subAgentSpendMicrocents sums the spend (budget.consumed cost_microcents, M47)
// of every run descended from parentCorr — its sub-agents and their sub-agents,
// transitively — excluding parentCorr's own direct spend. It backs the M48
// spend cap: a single forward journal pass builds the parent→children links
// (from subagent.spawned) and the per-run spend (from budget.consumed), then
// totals the spend over parentCorr's transitive descendants. Stateless and
// race-free: every prior delegation's spend is already durably journaled by the
// time the next delegate calls this. Only invoked when the cap is enabled.
func (k *Kernel) subAgentSpendMicrocents(parentCorr string) int64 {
	childrenOf := map[string][]string{}
	spendOf := map[string]int64{}
	_ = k.journal.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindSubAgentSpawned:
			if child, parent := delegation.SpawnLink(e.Payload); child != "" && parent != "" {
				childrenOf[parent] = append(childrenOf[parent], child)
			}
		case event.KindBudgetConsumed:
			if e.CorrelationID != "" {
				spendOf[e.CorrelationID] += delegation.BudgetCostMicrocents(e.Payload)
			}
		}
		return nil
	})

	// Sum spend over the transitive descendants of parentCorr (BFS over the
	// links), excluding parentCorr itself — the cap bounds sub-agent spend, not
	// the lead's own. A `seen` set guards against a malformed cyclic link.
	var total int64
	seen := map[string]bool{parentCorr: true}
	queue := append([]string{}, childrenOf[parentCorr]...)
	head := 0
	for head < len(queue) {
		corr := queue[head]
		head++
		if seen[corr] {
			continue
		}
		seen[corr] = true
		total += spendOf[corr]
		queue = append(queue, childrenOf[corr]...)
	}
	return total
}

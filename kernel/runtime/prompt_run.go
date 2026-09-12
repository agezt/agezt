// SPDX-License-Identifier: MIT

// Prompt assembly: buildRunPrompt (the big 170-line function that stitches together all inject helpers).
// Code extracted from prompt.go during the Day-73 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/skill"
	"time"
)


// world / skill injection below still layer on top.
func (k *Kernel) buildRunPrompt(runCtx context.Context, corr, actor, intent string, systemAgent bool, skillDirective skill.ActivationDirective) (string, []string) {
	system := k.System() // live daemon default identity (M710), editable at runtime
	if s := systemFromCtx(runCtx); s != "" {
		system = s
	}
	// Operator profile (M1000): prepend what AGEZT has learned about the operator
	// so every (non-system) run knows WHO it works for — distinct from the per-agent
	// persona above (identity) and applied before the ephemeral task injections
	// below, so it sits adjacent to the persona. Always-on (not intent-driven);
	// a no-op until DistillProfile has synthesized a profile.
	if k.cfg.ProfileInject && !systemAgent {
		if p := k.memory.ProfileText(); p != "" {
			system = injectUserProfile(system, p)
		}
	}
	// Taste overlay: prepend curated "what good looks like" exemplars scoped to
	// this run so output quality is anchored to concrete examples. Sits adjacent
	// to the operator profile (both shape HOW the agent works) and before the
	// factual memory recall below. Journaled as taste.injected under corr.
	if k.cfg.TasteInject && !systemAgent && k.taste != nil {
		topK := k.cfg.TasteTopK
		if topK <= 0 {
			topK = 3
		}
		if ex := k.taste.ForScope(agentSlugFromCtx(runCtx), topK); len(ex) > 0 {
			system = injectTaste(system, ex)
			ids := make([]string, 0, len(ex))
			for _, e := range ex {
				ids = append(ids, e.ID)
			}
			_, _ = k.bus.Publish(event.Spec{
				Subject:       "agent.agent-" + corr + ".taste",
				Kind:          event.KindTasteInjected,
				Actor:         "taste",
				CorrelationID: corr,
				Payload:       map[string]any{"count": len(ex), "ids": ids, "scope": agentSlugFromCtx(runCtx)},
			})
		}
	}
	if k.cfg.MemoryInject && !systemAgent {
		topK := k.cfg.MemoryTopK
		if topK <= 0 {
			topK = 5
		}
		var candidates []contextselect.Candidate
		if scored, err := k.memory.SearchScoped(intent, contextselect.CandidateLimit, memory.ScopeFrom(runCtx)); err == nil {
			candidates = memoryContextCandidates(scored, time.Now().UnixMilli())
		}
		// Scoped to the run's agent identity (M786): a named agent's private
		// notes surface in its injected context; an unscoped run sees shared
		// memory only (RecallScoped with "" ≡ the previous Recall behaviour).
		if hits, err := k.memory.RecallScoped(corr, intent, topK, memory.ScopeFrom(runCtx)); err == nil && len(hits) > 0 {
			system = injectMemory(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Record.ID)
			}
			chosen, rejected := contextselect.SplitCandidates(candidates, contextselect.ChosenIDSet(ids), "memory_recall")
			k.publishContextSelection(corr, actor, contextselect.Manifest{
				Phase:    "memory",
				Query:    intent,
				Chosen:   chosen,
				Rejected: rejected,
				Summary:  contextselect.Summary(chosen, rejected),
			})
		}
	}

	// World-model injection: resolve the entities the intent refers to and
	// prepend them, so the model starts knowing what "the portfolio" means
	// (SPEC-05 §7 step 1). Resolve journals worldmodel.retrieved under corr,
	// so `agt why` shows what references were grounded.
	if k.cfg.WorldInject && !systemAgent {
		topK := k.cfg.WorldTopK
		if topK <= 0 {
			topK = 5
		}
		var candidates []contextselect.Candidate
		if scored, err := k.world.ResolveQuiet(intent, contextselect.CandidateLimit); err == nil {
			candidates = worldContextCandidates(scored, time.Now().UnixMilli())
		}
		if hits, err := k.world.Resolve(corr, intent, topK); err == nil && len(hits) > 0 {
			system = injectWorld(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Entity.ID)
			}
			chosen, rejected := contextselect.SplitCandidates(candidates, contextselect.ChosenIDSet(ids), "world_resolve")
			k.publishContextSelection(corr, actor, contextselect.Manifest{
				Phase:    "world",
				Query:    intent,
				Chosen:   chosen,
				Rejected: rejected,
				Summary:  contextselect.Summary(chosen, rejected),
			})
		}
	}

	// Skill activation: retrieve matching ACTIVE skills and prepend their
	// bodies so the model plans with learned procedures (SPEC-05 §4.2, §7
	// step 4). The pool is scoped to the acting agent (M932): shared skills
	// plus its own private ones. Activate journals skill.activated under corr
	// for `agt why`.
	var activatedSkillIDs []string
	if k.cfg.SkillInject && !systemAgent {
		topK := k.cfg.SkillTopK
		if topK <= 0 {
			topK = 3
		}
		agentSlug := agentSlugFromCtx(runCtx)
		if skillDirective.Explicit {
			hits, missing, err := k.forge.ActivateExplicitFor(corr, agentSlug, intent, skillDirective.Refs, topK)
			if err == nil {
				if len(hits) > 0 {
					system = injectSkills(system, hits)
					for _, h := range hits {
						activatedSkillIDs = append(activatedSkillIDs, h.Skill.ID)
					}
				}
				chosen := skillContextCandidates(hits, time.Now().UnixMilli())
				for i := range chosen {
					chosen[i].Chosen = true
					chosen[i].Reason = "selected:skill_explicit_activation"
				}
				summary := contextselect.Summary(chosen, nil)
				summary["activation"] = "explicit"
				summary["refs"] = skillDirective.Refs
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
			var candidates []contextselect.Candidate
			if all, err := k.forge.List(); err == nil {
				pool := all[:0:0]
				for _, sk := range all {
					if sk.Agent == "" || sk.Agent == agentSlug {
						pool = append(pool, sk)
					}
				}
				candidates = skillContextCandidates(skill.Retrieve(pool, intent, contextselect.CandidateLimit, time.Now().UnixMilli()), time.Now().UnixMilli())
			}
			if hits, err := k.forge.ActivateFor(corr, agentSlug, intent, topK); err == nil && len(hits) > 0 {
				system = injectSkills(system, hits)
				for _, h := range hits {
					activatedSkillIDs = append(activatedSkillIDs, h.Skill.ID)
				}
				chosen, rejected := contextselect.SplitCandidates(candidates, contextselect.ChosenIDSet(activatedSkillIDs), "skill_activation")
				k.publishContextSelection(corr, actor, contextselect.Manifest{
					Phase:    "skill",
					Query:    intent,
					Chosen:   chosen,
					Rejected: rejected,
					Summary:  contextselect.Summary(chosen, rejected),
				})
			}
		}
	}
	return system, activatedSkillIDs
}

// injectHostEnvironment prepends the host-environment preamble (M609):
// OS/arch, the shell the shell tool uses, the shared workspace dir, the
// date, and THIS run's tools — so the model acts correctly on this host
// instead of guessing. Called last (after memory/world/skills and after
// the run's tool set is resolved) so it sits at the top of the system
// prompt and reflects any per-run tool restriction. No-op unless
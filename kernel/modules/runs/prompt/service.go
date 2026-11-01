// SPDX-License-Identifier: MIT

package prompt

import (
	"context"

	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// Service assembles the system prompt of a run.
//
// It is the run engine's first mile: before a run asks a model anything, the
// service decides what the model already knows. The order is fixed and it is
// the order the prompt reads best in: identity first (the daemon's live
// system, or the run's own override), then the operator profile (who this
// agent works for), then the taste exemplars (what good looks like), then
// the recalled memory (what we know), then the resolved world entities (what
// the words refer to), and last the activated skills (the procedures to
// follow).
type Service struct {
	cfg  Config
	host Host
	deps Deps
}

// New builds the service over a configuration snapshot, a host and the
// stores it reads.
func New(cfg Config, host Host, deps Deps) *Service {
	return &Service{cfg: cfg, host: host, deps: deps}
}

// Build assembles the system prompt of a ROOT run and answers the activated
// skill ids alongside it.
//
// A system agent (a roster profile that IS the system) skips every injection:
// the daemon's identity is its identity, and a system run carries no operator
// profile, no exemplars, no memory and no skills.
func (s *Service) Build(runCtx context.Context, corr, actor, intent string, systemAgent bool, skillDirective skillapi.ActivationDirective) (string, []string) {
	// The daemon's live default identity (M710), editable at runtime; the
	// run's own override (the agent profile's soul) wins over it.
	system := s.host.System()
	if v := runctx.SystemFromCtx(runCtx); v != "" {
		system = v
	}

	// Operator profile (M1000): prepend what AGEZT has learned about the
	// operator so every (non-system) run knows WHO it works for — distinct
	// from the per-agent persona above (identity) and applied before the
	// ephemeral task injections below, so it sits adjacent to the persona.
	// Always-on (not intent-driven); a no-op until DistillProfile has
	// synthesized a profile.
	if s.cfg.ProfileInject && !systemAgent && s.deps.Memory != nil {
		if p := s.deps.Memory.ProfileText(); p != "" {
			system = injectUserProfile(system, p)
		}
	}

	// Taste overlay: prepend curated "what good looks like" exemplars scoped
	// to this run so output quality is anchored to concrete examples. Sits
	// adjacent to the operator profile (both shape HOW the agent works) and
	// before the factual memory recall below. Journaled as taste.injected
	// under corr.
	if s.cfg.TasteInject && !systemAgent && s.deps.Taste != nil {
		topK := s.cfg.TasteTopK
		if topK <= 0 {
			topK = 3
		}
		if ex := s.deps.Taste.ForScope(runctx.AgentSlugFromCtx(runCtx), topK); len(ex) > 0 {
			system = injectTaste(system, ex)
			ids := make([]string, 0, len(ex))
			for _, e := range ex {
				ids = append(ids, e.ID)
			}
			_, _ = s.host.Publish(event.Spec{
				Subject:       "agent.agent-" + corr + ".taste",
				Kind:          event.KindTasteInjected,
				Actor:         "taste",
				CorrelationID: corr,
				Payload:       map[string]any{"count": len(ex), "ids": ids, "scope": runctx.AgentSlugFromCtx(runCtx)},
			})
		}
	}

	if s.cfg.MemoryInject && !systemAgent && s.deps.Memory != nil {
		topK := s.cfg.MemoryTopK
		if topK <= 0 {
			topK = 5
		}
		var candidates []contextselect.Candidate
		if scored, err := s.deps.Memory.SearchScoped(intent, contextselect.CandidateLimit, memoryapi.ScopeFrom(runCtx)); err == nil {
			candidates = memoryContextCandidates(scored, s.host.Now().UnixMilli())
		}
		// Scoped to the run's agent identity (M786): a named agent's private
		// notes surface in its injected context; an unscoped run sees shared
		// memory only (RecallScoped with "" ≡ the previous Recall behaviour).
		if hits, err := s.deps.Memory.RecallScoped(corr, intent, topK, memoryapi.ScopeFrom(runCtx)); err == nil && len(hits) > 0 {
			system = injectMemory(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Record.ID)
			}
			chosen, rejected := contextselect.SplitCandidates(candidates, contextselect.ChosenIDSet(ids), "memory_recall")
			s.PublishSelection(corr, actor, contextselect.Manifest{
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
	if s.cfg.WorldInject && !systemAgent {
		topK := s.cfg.WorldTopK
		if topK <= 0 {
			topK = 5
		}
		var candidates []contextselect.Candidate
		if scored, err := s.deps.World.ResolveQuiet(intent, contextselect.CandidateLimit); err == nil {
			candidates = worldContextCandidates(scored, s.host.Now().UnixMilli())
		}
		if hits, err := s.deps.World.Resolve(corr, intent, topK); err == nil && len(hits) > 0 {
			system = injectWorld(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Entity.ID)
			}
			chosen, rejected := contextselect.SplitCandidates(candidates, contextselect.ChosenIDSet(ids), "world_resolve")
			s.PublishSelection(corr, actor, contextselect.Manifest{
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
	//
	// A daemon with NO skill library holds a nil port (W3.55): the run simply
	// carries no skill, because a port that calls into nothing would answer
	// the agent with a panic.
	var activatedSkillIDs []string
	if s.cfg.SkillInject && !systemAgent && s.deps.Skills != nil {
		topK := s.cfg.SkillTopK
		if topK <= 0 {
			topK = 3
		}
		agentSlug := runctx.AgentSlugFromCtx(runCtx)
		if skillDirective.Explicit {
			hits, missing, err := s.deps.Skills.ActivateExplicitFor(corr, agentSlug, intent, skillDirective.Refs, topK)
			if err == nil {
				if len(hits) > 0 {
					system = injectSkills(system, hits)
					for _, h := range hits {
						activatedSkillIDs = append(activatedSkillIDs, h.Skill.ID)
					}
				}
				chosen := skillContextCandidates(hits, s.host.Now().UnixMilli())
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
				s.PublishSelection(corr, actor, contextselect.Manifest{
					Phase:   "skill",
					Query:   intent,
					Chosen:  chosen,
					Summary: summary,
				})
			}
		} else {
			var candidates []contextselect.Candidate
			if all, err := s.deps.Skills.List(); err == nil {
				pool := all[:0:0]
				for _, sk := range all {
					if sk.Agent == "" || sk.Agent == agentSlug {
						pool = append(pool, sk)
					}
				}
				candidates = skillContextCandidates(skillapi.Retrieve(pool, intent, contextselect.CandidateLimit, s.host.Now().UnixMilli()), s.host.Now().UnixMilli())
			}
			if hits, err := s.deps.Skills.ActivateFor(corr, agentSlug, intent, topK); err == nil && len(hits) > 0 {
				system = injectSkills(system, hits)
				for _, h := range hits {
					activatedSkillIDs = append(activatedSkillIDs, h.Skill.ID)
				}
				chosen, rejected := contextselect.SplitCandidates(candidates, contextselect.ChosenIDSet(activatedSkillIDs), "skill_activation")
				s.PublishSelection(corr, actor, contextselect.Manifest{
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

// BuildEnvironment prepends the host-environment preamble (M609) when the
// configuration turns it on. It is called last — after memory, world and
// skills, and after the run's tool set is resolved — so it sits at the top of
// the system prompt and reflects any per-run tool restriction.
func (s *Service) BuildEnvironment(system string, tools map[string]toolapi.Tool) string {
	if s.cfg.EnvironmentInject {
		system = injectEnvironment(system, s.cfg.WorkspaceRoot, tools, s.host.Now())
	}
	return system
}

// BuildSubagent assembles the system prompt of a CHILD run (a delegated
// sub-agent) and answers the activated skill ids and the CLEANED intent
// alongside it.
//
// The child inherits less than the root run: no operator profile and no taste
// exemplars — a sub-agent is given the task, not the operator's persona —
// but it does get the memory and the skills, and it publishes its own
// selection manifests under its own correlation.
//
// The intent is CLEANED here: an explicit activation directive is a
// directive, not part of the task, so the child receives the intent without
// it.
func (s *Service) BuildSubagent(ctx context.Context, corr, actor, intent, system string) (string, []string, string) {
	directive := skillapi.ParseActivationDirective(intent)
	if directive.Explicit && directive.CleanIntent != "" {
		intent = directive.CleanIntent
	}
	if runctx.SystemAgentFromCtx(ctx) {
		return system, nil, intent
	}
	if s.cfg.MemoryInject && s.deps.Memory != nil {
		topK := s.cfg.MemoryTopK
		if topK <= 0 {
			topK = 5
		}
		if hits, err := s.deps.Memory.RecallScoped(corr, intent, topK, memoryapi.ScopeFrom(ctx)); err == nil && len(hits) > 0 {
			system = injectMemory(system, hits)
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.Record.ID)
			}
			chosen, rejected := contextselect.SplitCandidates(memoryContextCandidates(hits, s.host.Now().UnixMilli()), contextselect.ChosenIDSet(ids), "subagent_memory_injection")
			summary := contextselect.Summary(chosen, rejected)
			summary["source"] = "subagent_memory_injection"
			s.PublishSelection(corr, actor, contextselect.Manifest{
				Phase:    "memory",
				Query:    intent,
				Chosen:   chosen,
				Rejected: rejected,
				Summary:  summary,
			})
		}
	}
	var activatedSkillIDs []string
	if s.cfg.SkillInject && s.deps.Skills != nil {
		topK := s.cfg.SkillTopK
		if topK <= 0 {
			topK = 3
		}
		agentSlug := runctx.AgentSlugFromCtx(ctx)
		if directive.Explicit {
			hits, missing, err := s.deps.Skills.ActivateExplicitFor(corr, agentSlug, intent, directive.Refs, topK)
			if err == nil {
				if len(hits) > 0 {
					system = injectSkills(system, hits)
					for _, h := range hits {
						activatedSkillIDs = runctx.AppendUniqueString(activatedSkillIDs, h.Skill.ID)
					}
				}
				chosen := skillContextCandidates(hits, s.host.Now().UnixMilli())
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
				s.PublishSelection(corr, actor, contextselect.Manifest{
					Phase:   "skill",
					Query:   intent,
					Chosen:  chosen,
					Summary: summary,
				})
			}
		} else {
			if hits, err := s.deps.Skills.ActivateFor(corr, agentSlug, intent, topK); err == nil && len(hits) > 0 {
				system = injectSkills(system, hits)
				for _, h := range hits {
					activatedSkillIDs = runctx.AppendUniqueString(activatedSkillIDs, h.Skill.ID)
				}
				chosen, rejected := contextselect.SplitCandidates(skillContextCandidates(hits, s.host.Now().UnixMilli()), contextselect.ChosenIDSet(activatedSkillIDs), "subagent_skill_injection")
				summary := contextselect.Summary(chosen, rejected)
				summary["source"] = "subagent_skill_injection"
				s.PublishSelection(corr, actor, contextselect.Manifest{
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

// PublishSelection journals a context-selection manifest when there is data
// worth sharing. An empty manifest is not worth a journal entry.
func (s *Service) PublishSelection(corr, actor string, manifest contextselect.Manifest) {
	if len(manifest.Chosen) == 0 && len(manifest.Rejected) == 0 && len(manifest.Summary) == 0 {
		return
	}
	_, _ = s.host.Publish(event.Spec{
		Subject:       "context.selection",
		Kind:          event.KindContextSelection,
		Actor:         actor,
		CorrelationID: corr,
		Payload:       manifest,
	})
}

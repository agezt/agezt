// SPDX-License-Identifier: MIT

// Forge shadow lifecycle: shadow evaluation, record shadow outcome, auto-promote, and propose (LLM-driven skill creation).
// Code extracted from forge.go during the Day-36 god-file split. Public API unchanged.
package skill


import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)


func (f *Forge) ShadowEvaluate(ctx context.Context, corr string, provider agent.Provider, model, intent, outcome string, limit int) error {
	if provider == nil {
		return errors.New("skill: shadow eval requires a provider")
	}
	all, err := f.store.All()
	if err != nil {
		return err
	}
	for _, c := range RetrieveShadow(all, intent, limit, f.now().UnixMilli()) {
		user := fmt.Sprintf("Task intent:\n%s\n\nWhat actually happened:\n%s\n\nCandidate skill %q:\n%s",
			intent, outcome, c.Skill.Name, c.Skill.Body)
		resp, cerr := provider.Complete(ctx, agent.CompletionRequest{
			Model:         model,
			System:        shadowJudgeSystem,
			Messages:      []agent.Message{{Role: agent.RoleUser, Content: user}},
			CorrelationID: corr,
			TaskType:      "shadow-eval",
			MaxTokens:     16,
		})
		if cerr != nil {
			continue
		}
		f.RecordShadowOutcome(corr, c.Skill.ID, parseShadowVerdict(resp.Message.Content))
	}
	return nil
}

// RecordShadowOutcome bumps a shadow skill's evaluation counters and journals
// skill.shadow_evaluated under the run's correlation. Only shadow skills are
// affected. The shadow→active auto-promotion gate (M401) reads these counters.
func (f *Forge) RecordShadowOutcome(corr, id string, helped bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sk, found, err := f.store.Get(id)
	if err != nil || !found || sk.Status != StatusShadow {
		return
	}
	sk.Metrics.ShadowEvals++
	if helped {
		sk.Metrics.ShadowWins++
	}
	if err := f.store.Put(sk); err != nil {
		return
	}
	f.publish(event.KindSkillShadowEval, corr, map[string]any{
		"id": id, "name": sk.Name, "helped": helped,
		"evals": sk.Metrics.ShadowEvals, "wins": sk.Metrics.ShadowWins,
	})
	if helped {
		f.maybeAutoPromote(corr, sk)
	}
}

// maybeAutoPromote promotes a SHADOW skill to active when its shadow-evaluation
// record crosses the configured win threshold (SPEC-05 §5.2 shadow→active, "N
// successful real uses, gated"). Requires BOTH a minimum win count and a win
// rate, so a shadow skill judged unhelpful as often as not is not promoted. Only
// shadow skills are affected; the promotion is journaled with the gate reason and
// is reversible (auto-quarantine can later pull it if it regresses).
func (f *Forge) maybeAutoPromote(corr string, sk Skill) {
	if f.apMinWins <= 0 || sk.Status != StatusShadow {
		return
	}
	if sk.Metrics.ShadowEvals == 0 || sk.Metrics.ShadowWins < f.apMinWins {
		return
	}
	rate := float64(sk.Metrics.ShadowWins) / float64(sk.Metrics.ShadowEvals)
	if rate < f.apWinRate {
		return
	}
	reason := fmt.Sprintf("auto-promote: %d/%d shadow evals judged helpful (%.0f%%)",
		sk.Metrics.ShadowWins, sk.Metrics.ShadowEvals, rate*100)
	_, _ = f.promoteWithReason(corr, sk.ID, reason)
}

// Get returns a single skill by id.

// ----

func (f *Forge) Propose(ctx context.Context, corr string, provider agent.Provider, model, intent, transcript string) ([]string, error) {
	if provider == nil {
		return nil, errors.New("skill: propose requires a provider")
	}
	user := fmt.Sprintf("Task intent:\n%s\n\nWhat happened:\n%s", intent, transcript)
	resp, err := provider.Complete(ctx, agent.CompletionRequest{
		Model:    model,
		System:   proposeSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: user}},
		TaskType: "forge",
	})
	if err != nil {
		return nil, fmt.Errorf("skill: propose completion: %w", err)
	}
	parsed, ok := parsePropose(resp.Message.Content)
	if !ok || parsed.Skill == nil {
		// Non-JSON (e.g. the mock provider) or an explicit decline → nothing
		// to author. Not an error; proposal is opportunistic.
		return nil, nil
	}
	if strings.TrimSpace(parsed.Skill.Body) == "" || strings.TrimSpace(parsed.Skill.Name) == "" {
		return nil, nil
	}
	// A skill learned while a named agent acted belongs to that agent (M932):
	// private-by-default self-learning, mirroring per-agent memory. Default-
	// persona runs (no agent on ctx) author into the shared pool as before.
	sk, _, err := f.Create(corr, CreateSpec{
		Name:          parsed.Skill.Name,
		Description:   parsed.Skill.Description,
		Triggers:      parsed.Skill.Triggers,
		Body:          parsed.Skill.Body,
		ToolsRequired: parsed.Skill.Tools,
		Agent:         agent.AgentFromContext(ctx),
	})
	if err != nil {
		return nil, err
	}
	return []string{sk.ID}, nil
}

// parsePropose extracts the JSON object from a model response, tolerating
// surrounding prose or markdown fences by scanning for the outermost braces.
func parsePropose(s string) (proposeResult, bool) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return proposeResult{}, false
	}
	var r proposeResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return proposeResult{}, false
	}
	return r, true
}

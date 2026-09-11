// SPDX-License-Identifier: MIT

// Forge activation: visibleTo, activate (rank/return), record outcome, auto-quarantine, shadow verdict parse.
// Code extracted from forge.go during the Day-36 god-file split. Public API unchanged.
package skill


import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)


func visibleTo(sk Skill, agentSlug string) bool {
	return sk.Agent == "" || sk.Agent == agentSlug
}

// Activate ranks active skills against intent and journals skill.activated
// (under corr) when anything matched, bumping each matched skill's use metrics
// — so `agt why` shows which skills shaped a run. Returns the ranked results.
// Equivalent to ActivateFor with no acting agent (shared pool only).
func (f *Forge) Activate(corr, intent string, limit int) ([]Scored, error) {
	return f.ActivateFor(corr, "", intent, limit)
}

// ActivateFor is Activate scoped to the acting agent (M932): the retrieval
// pool is the shared skills plus the agent's own private ones, so an agent
// plans with what IT learned without leaking another agent's procedures.
func (f *Forge) ActivateFor(corr, agentSlug, intent string, limit int) ([]Scored, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all, err := f.store.All()
	if err != nil {
		return nil, err
	}
	pool := all[:0:0]
	for _, sk := range all {
		if visibleTo(sk, agentSlug) {
			pool = append(pool, sk)
		}
	}
	nowMS := f.now().UnixMilli()
	hits := Retrieve(pool, intent, limit, nowMS)
	if len(hits) == 0 {
		return hits, nil
	}
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.Skill.ID)
		sk := h.Skill
		sk.Metrics.Uses++
		sk.Metrics.LastUsedMS = nowMS
		_ = f.store.Put(sk)
	}
	f.publish(event.KindSkillActivated, corr, map[string]any{
		"intent": intent, "matched": len(hits), "ids": ids, "activation": "auto",
	})
	return hits, nil
}

// ActivateExplicitFor activates active skills by exact name, full id, or unique
// id prefix. It is the explicit counterpart to intent retrieval: a slash
// directive can name the procedure to load without relying on keyword overlap,
// but it still cannot bypass visibility or lifecycle state.
func (f *Forge) ActivateExplicitFor(corr, agentSlug, intent string, refs []string, limit int) ([]Scored, []string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all, err := f.store.All()
	if err != nil {
		return nil, nil, err
	}
	nowMS := f.now().UnixMilli()
	hits := make([]Scored, 0, len(refs))
	missing := make([]string, 0)
	seen := map[string]bool{}
	for _, ref := range refs {
		sk, ok := resolveActiveVisible(all, agentSlug, ref)
		if !ok {
			missing = append(missing, strings.TrimSpace(ref))
			continue
		}
		if seen[sk.ID] {
			continue
		}
		seen[sk.ID] = true
		if limit > 0 && len(hits) >= limit {
			break
		}
		sk.Metrics.Uses++
		sk.Metrics.LastUsedMS = nowMS
		if err := f.store.Put(sk); err != nil {
			return nil, missing, err
		}
		hits = append(hits, Scored{Skill: sk, Score: 1})
	}
	if len(hits) > 0 || len(missing) > 0 {
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.Skill.ID)
		}
		f.publish(event.KindSkillActivated, corr, map[string]any{
			"intent": intent, "matched": len(hits), "ids": ids,
			"activation": "explicit", "refs": refs, "missing": missing,
		})
	}
	return hits, missing, nil
}

func resolveActiveVisible(all []Skill, agentSlug, ref string) (Skill, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Skill{}, false
	}
	refLower := strings.ToLower(ref)
	var prefix Skill
	prefixMatches := 0
	for _, sk := range all {
		if !sk.Active() || !visibleTo(sk, agentSlug) {
			continue
		}
		switch {
		case sk.ID == ref || strings.EqualFold(sk.Name, ref):
			return sk, true
		case strings.HasPrefix(sk.ID, refLower):
			prefix = sk
			prefixMatches++
		}
	}
	if prefixMatches == 1 {
		return prefix, true
	}
	return Skill{}, false
}

// RecordOutcome bumps success/failure metrics for the given skill ids and, on a
// failure, auto-quarantines a skill whose record has crossed the threshold
// (SPEC-05 §5). The runtime calls it after a run with the skills that run
// activated; corr ties any resulting skill.quarantined event back to that run.
func (f *Forge) RecordOutcome(corr string, ids []string, success bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		sk, found, err := f.store.Get(id)
		if err != nil || !found {
			continue
		}
		if success {
			sk.Metrics.Successes++
		} else {
			sk.Metrics.Failures++
		}
		if err := f.store.Put(sk); err != nil {
			continue
		}
		if !success {
			f.maybeAutoQuarantine(corr, sk)
		}
	}
}

// maybeAutoQuarantine pulls an ACTIVE skill from production when its failure
// record crosses the configured threshold (SPEC-05 §5: "pulled from production by
// a regression or repeated failure"). Requires BOTH a minimum failure count and a
// failure rate, so a few failures amid many successes don't yank a good skill.
// Only ACTIVE skills are affected (shadow skills are still under evaluation); the
// action is journaled and reversible (`agt skill promote` re-activates).
func (f *Forge) maybeAutoQuarantine(corr string, sk Skill) {
	if f.aqMinFailures <= 0 || sk.Status != StatusActive {
		return
	}
	total := sk.Metrics.Successes + sk.Metrics.Failures
	if total == 0 || sk.Metrics.Failures < f.aqMinFailures {
		return
	}
	rate := float64(sk.Metrics.Failures) / float64(total)
	if rate < f.aqFailureRate {
		return
	}
	reason := fmt.Sprintf("auto-quarantine: %d/%d runs failed (%.0f%%)", sk.Metrics.Failures, total, rate*100)
	_ = f.quarantineLocked(corr, sk.ID, reason) // caller (RecordOutcome) holds f.mu
}

// shadowJudgeSystem instructs the model to decide whether a shadow skill would
// have helped a just-completed run (SPEC-05 §5.2). The verdict is a single word
// so it parses robustly across providers; "be conservative" biases toward NO.
const shadowJudgeSystem = `You evaluate whether a candidate "skill" (a reusable procedure) would have helped an agent complete a task it just finished.
You are given the task intent, what actually happened, and the skill's instructions.
Reply with exactly one word: YES if the skill's guidance would plausibly have improved or sped up the outcome, otherwise NO. Be conservative — reply NO if unsure.`

// parseShadowVerdict reads the model's YES/NO leniently: helped only when the
// first meaningful word is affirmative. Anything ambiguous or non-conforming
// (e.g. the offline mock's canned text) defaults to false — conservative, so a
// skill is never credited toward promotion on a vague answer.
func parseShadowVerdict(text string) bool {
	for _, line := range strings.Split(strings.ToLower(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		word := line
		if i := strings.IndexFunc(line, func(r rune) bool {
			return r == ' ' || r == '.' || r == ',' || r == ':' || r == '!' || r == ';'
		}); i >= 0 {
			word = line[:i]
		}
		return word == "yes" || word == "true"
	}
	return false
}

// ShadowEvaluate judges the shadow skills relevant to a just-completed run and
// records the verdicts (SPEC-05 §5.2: shadow "runs alongside real execution
// without affecting outcomes ... compared to what actually happened"). It runs
// NO tools — the shadow skill is never executed, so evaluation cannot affect
// outcomes; the model judges whether the skill WOULD have helped. Best-effort:
// a provider error on one candidate is skipped, not fatal. limit bounds how many
// shadow candidates are judged per run (cost control).
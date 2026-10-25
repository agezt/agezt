// SPDX-License-Identifier: MIT

package skillapi

// Provenance: the pure skill rules — retrieval ranking and the activation
//             directive parser. Code moved from the skills module
//             (kernel/modules/skills/skill) during the W3.55 slice.
//
// These are RULES, not plumbing: they decide which skill is offered to a run
// and what the run's intent really says once the control line is stripped. They
// live with the rows they judge (Skill, Scored, ActivationDirective) because a
// rule that stays in the implementation while its row moves to the contract
// cannot be reached by the layers above without naming the implementation.

import (
	"sort"
	"strings"
	"unicode"
)

// Retrieve ranks the ACTIVE skills in sks against an intent and returns the
// top `limit` by score (SPEC-05 §4.2, §7 step 4). A skill scores on keyword
// overlap between the intent tokens and the skill's name + description +
// triggers, weighted by recency. Only active skills participate — drafts,
// shadows, quarantined and archived skills are never injected into a run.
//
// Ranking is a pure function of (sks, intent, limit, nowMS): same inputs →
// same ordering, ties broken by LastUsedMS (more-used recently first) then id.
// Mirrors memory.Search / worldmodel.Resolve so retrieval is consistent across
// the three knowledge layers.
func Retrieve(sks []Skill, intent string, limit int, nowMS int64) []Scored {
	return retrieveMatching(sks, intent, limit, nowMS, Skill.Active)
}

// RetrieveShadow ranks the SHADOW skills in sks against an intent, using the same
// scoring as Retrieve. Shadow skills are never injected into runs; this is the
// candidate set for shadow-evaluation (M400) — the same relevance ranking
// decides which shadow skills to evaluate against a completed run.
func RetrieveShadow(sks []Skill, intent string, limit int, nowMS int64) []Scored {
	return retrieveMatching(sks, intent, limit, nowMS, func(s Skill) bool {
		return s.Status == StatusShadow
	})
}

// retrieveMatching is the shared ranking core: it scores every skill the include
// predicate admits — keyword overlap weighted by recency — and returns the top
// `limit` deterministically.
func retrieveMatching(sks []Skill, intent string, limit int, nowMS int64, include func(Skill) bool) []Scored {
	qTokens := tokenize(intent)
	out := make([]Scored, 0, len(sks))
	if len(qTokens) == 0 {
		return out
	}
	for _, sk := range sks {
		if !include(sk) {
			continue
		}
		overlap := keywordOverlap(qTokens, sk)
		if overlap == 0 {
			continue
		}
		score := float64(overlap) * recencyFactor(sk.LastSeenMS, nowMS)
		out = append(out, Scored{Skill: sk, Score: score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Skill.Metrics.LastUsedMS != out[j].Skill.Metrics.LastUsedMS {
			return out[i].Skill.Metrics.LastUsedMS > out[j].Skill.Metrics.LastUsedMS
		}
		return out[i].Skill.ID < out[j].Skill.ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// keywordOverlap counts how many distinct intent tokens appear anywhere in the
// skill's searchable text (name + description + triggers). The body is NOT
// searched — matching on instruction text would surface skills on incidental
// word overlap; description+triggers are the curated activation surface (§4.2).
func keywordOverlap(qTokens []string, sk Skill) int {
	var b strings.Builder
	b.WriteString(sk.Name)
	b.WriteByte(' ')
	b.WriteString(sk.Description)
	for _, t := range sk.Triggers {
		b.WriteByte(' ')
		b.WriteString(t)
	}
	haystack := make(map[string]struct{})
	for _, t := range tokenize(b.String()) {
		haystack[t] = struct{}{}
	}
	n := 0
	seen := make(map[string]struct{}, len(qTokens))
	for _, t := range qTokens {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		if _, ok := haystack[t]; ok {
			n++
		}
	}
	return n
}

// recencyFactor decays with age: 1.0 for "now", asymptoting toward 0 for old
// skills. A skill never drops out of retrieval on recency alone. Mirrors
// kernel/modules/knowledge/memory's recency model.
func recencyFactor(lastSeenMS, nowMS int64) float64 {
	ageMS := nowMS - lastSeenMS
	if ageMS <= 0 {
		return 1.0
	}
	const dayMS = 24 * 60 * 60 * 1000
	ageDays := float64(ageMS) / float64(dayMS)
	return 1.0 / (1.0 + ageDays)
}

// tokenize lowercases s and splits on any non-alphanumeric rune, dropping
// empties and 1-character tokens. Mirrors kernel/modules/knowledge/memory.tokenize so all three
// knowledge layers rank consistently.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := fields[:0]
	for _, f := range fields {
		if len(f) <= 1 {
			continue
		}
		out = append(out, f)
	}
	return out
}

// ParseActivationDirective recognizes leading slash directives:
//
//	/skill diagnose-ci
//	/skills diagnose-ci, webresearch
//
// Only leading lines are control lines; the first ordinary line begins the real
// task. If a directive is present but no task follows, CleanIntent is the
// original input so a bare "/skill foo" does not create an empty run.
func ParseActivationDirective(intent string) ActivationDirective {
	original := intent
	lines := strings.Split(intent, "\n")
	var refs []string
	i := 0
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" && len(refs) == 0 {
			continue
		}
		if !strings.HasPrefix(line, "/skill ") && !strings.HasPrefix(line, "/skills ") {
			break
		}
		var tail string
		if strings.HasPrefix(line, "/skills ") {
			tail = strings.TrimSpace(strings.TrimPrefix(line, "/skills "))
		} else {
			tail = strings.TrimSpace(strings.TrimPrefix(line, "/skill "))
		}
		refs = appendSkillRefs(refs, tail)
	}
	if len(refs) == 0 {
		return ActivationDirective{CleanIntent: original}
	}
	clean := strings.TrimSpace(strings.Join(lines[i:], "\n"))
	if clean == "" {
		clean = strings.TrimSpace(original)
	}
	return ActivationDirective{
		Explicit:    true,
		Refs:        refs,
		CleanIntent: clean,
	}
}

func appendSkillRefs(dst []string, raw string) []string {
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		ref := strings.TrimSpace(part)
		if ref == "" {
			continue
		}
		dup := false
		for _, existing := range dst {
			if strings.EqualFold(existing, ref) {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, ref)
		}
	}
	return dst
}

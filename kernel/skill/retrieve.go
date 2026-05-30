// SPDX-License-Identifier: MIT

package skill

import (
	"sort"
	"strings"
	"unicode"
)

// Scored is a skill paired with its retrieval score.
type Scored struct {
	Skill Skill   `json:"skill"`
	Score float64 `json:"score"`
}

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
	qTokens := tokenize(intent)
	out := make([]Scored, 0, len(sks))
	if len(qTokens) == 0 {
		return out
	}
	for _, sk := range sks {
		if !sk.Active() {
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
// kernel/memory's recency model.
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
// empties and 1-character tokens. Mirrors kernel/memory.tokenize so all three
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

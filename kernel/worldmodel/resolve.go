// SPDX-License-Identifier: MIT

package worldmodel

import (
	"sort"
	"strings"
	"unicode"
)

// ScoredEntity is an entity paired with its resolve score.
type ScoredEntity struct {
	Entity Entity  `json:"entity"`
	Score  float64 `json:"score"`
}

// Resolve ranks the active entities in es against a phrase and returns the top
// `limit` by score (descending). This is what answers "what does 'the
// portfolio' resolve to?" (SPEC-05 §3.4). Matching is, strongest first:
//
//   - exact (case-folded) name match,
//   - exact alias match,
//   - keyword overlap between the phrase tokens and the entity's
//     name + aliases + kind.
//
// The match strength is weighted by the entity's Weight (active-ness) and its
// recency, so a heavily-used recent project outranks a stale one that happens
// to share a word. Tombstoned and superseded entities are excluded.
//
// Ranking is a pure function of (es, phrase, limit, nowMS): same inputs →
// same ordering, ties broken by Weight then LastSeenMS (newer first) then id.
func Resolve(es []Entity, phrase string, limit int, nowMS int64) []ScoredEntity {
	qTokens := tokenize(phrase)
	folded := strings.ToLower(strings.TrimSpace(phrase))
	out := make([]ScoredEntity, 0, len(es))
	if folded == "" {
		return out
	}
	for _, e := range es {
		if !e.Active() {
			continue
		}
		strength := matchStrength(e, folded, qTokens)
		if strength == 0 {
			continue
		}
		w := e.Weight
		if w <= 0 {
			w = 0.5 // never zero-out a hit purely on a missing weight
		}
		score := strength * (0.5 + w) * recencyFactor(e.LastSeenMS, nowMS)
		out = append(out, ScoredEntity{Entity: e, Score: score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Entity.Weight != out[j].Entity.Weight {
			return out[i].Entity.Weight > out[j].Entity.Weight
		}
		if out[i].Entity.LastSeenMS != out[j].Entity.LastSeenMS {
			return out[i].Entity.LastSeenMS > out[j].Entity.LastSeenMS
		}
		return out[i].Entity.ID < out[j].Entity.ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// matchStrength scores how strongly entity e matches a folded phrase and its
// tokens. Exact name/alias matches dominate; otherwise the count of distinct
// phrase tokens that appear in the entity's searchable text.
func matchStrength(e Entity, folded string, qTokens []string) float64 {
	if strings.ToLower(strings.TrimSpace(e.Name)) == folded {
		return 4.0
	}
	for _, a := range e.Aliases {
		if strings.ToLower(strings.TrimSpace(a)) == folded {
			return 3.0
		}
	}
	overlap := keywordOverlapEntity(qTokens, e)
	return float64(overlap)
}

// keywordOverlapEntity counts how many distinct query tokens appear anywhere
// in the entity's searchable text (name + aliases + kind).
func keywordOverlapEntity(qTokens []string, e Entity) int {
	var b strings.Builder
	b.WriteString(e.Name)
	b.WriteByte(' ')
	b.WriteString(string(e.Kind))
	for _, a := range e.Aliases {
		b.WriteByte(' ')
		b.WriteString(a)
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

// Neighbor is one edge incident to a queried entity together with the entity
// on the other end (nil-safe: Other is the zero Entity if the adjacent node is
// missing/forgotten).
type Neighbor struct {
	Relation Relation `json:"relation"`
	// Outgoing is true when the queried entity is the edge's From.
	Outgoing bool   `json:"outgoing"`
	Other    Entity `json:"other"`
}

// Neighbors returns the active edges incident to entityID together with the
// adjacent entity for each, ordered deterministically (by relation CreatedMS
// then id). Only active relations are returned; an edge to a tombstoned entity
// is still returned (the relation itself is active) with whatever Other holds.
func Neighbors(entityID string, es []Entity, rs []Relation) []Neighbor {
	byID := make(map[string]Entity, len(es))
	for _, e := range es {
		byID[e.ID] = e
	}
	out := make([]Neighbor, 0)
	for _, r := range rs {
		if !r.Active() {
			continue
		}
		switch entityID {
		case r.From:
			out = append(out, Neighbor{Relation: r, Outgoing: true, Other: byID[r.To]})
		case r.To:
			out = append(out, Neighbor{Relation: r, Outgoing: false, Other: byID[r.From]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Relation.CreatedMS != out[j].Relation.CreatedMS {
			return out[i].Relation.CreatedMS < out[j].Relation.CreatedMS
		}
		return out[i].Relation.ID < out[j].Relation.ID
	})
	return out
}

// recencyFactor decays with age: 1.0 for "now", asymptoting toward 0 for old
// nodes. A node never drops out of resolve on recency alone — it only loses
// ranking weight. Mirrors kernel/memory's recency model.
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
// empties and 1-character tokens (noise). Stable and dependency-free; mirrors
// kernel/memory.tokenize so resolve and recall rank consistently.
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

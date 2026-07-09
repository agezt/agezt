// SPDX-License-Identifier: MIT

// Package contextselect provides context candidate scoring, selection, and
// failure analysis for agent runs. It is extracted from kernel/runtime to
// narrow the composition root and make selection logic independently testable.
package contextselect

import (
	"fmt"
	"sort"
	"strings"
	"time"

	kmemory "github.com/agezt/agezt/kernel/memory"
	kskill "github.com/agezt/agezt/kernel/skill"
	kworld "github.com/agezt/agezt/kernel/worldmodel"
)

const (
	CandidateLimit   = 12
	rejectedLimit    = 5
)

// Candidate describes one item considered for context inclusion.
type Candidate struct {
	Source     string   `json:"source"`
	ID         string   `json:"id"`
	Label      string   `json:"label,omitempty"`
	Score      float64  `json:"score,omitempty"`
	Tokens     int      `json:"tokens"`
	HardCost   int      `json:"hard_cost"`
	SoftCost   float64  `json:"soft_cost"`
	RiskCost   float64  `json:"risk_cost"`
	Freshness  float64  `json:"freshness"`
	Confidence float64  `json:"confidence"`
	Chosen     bool     `json:"chosen"`
	Reason     string   `json:"reason"`
	Signals    []string `json:"signals,omitempty"`
}

// Manifest is the full selection result published as a context.selection event.
type Manifest struct {
	Phase       string       `json:"phase"`
	Query       string       `json:"query,omitempty"`
	BudgetChars int          `json:"budget_chars,omitempty"`
	Chosen      []Candidate  `json:"chosen,omitempty"`
	Rejected    []Candidate  `json:"rejected,omitempty"`
	Summary     map[string]any `json:"summary,omitempty"`
}

// SplitCandidates divides candidates into chosen and rejected slices, sorted
// by descending score. Rejected is capped at rejectedLimit.
func SplitCandidates(all []Candidate, chosenIDs map[string]bool, reason string) (chosen, rejected []Candidate) {
	for _, c := range all {
		if chosenIDs[c.ID] {
			c.Chosen = true
			c.Reason = "selected:" + reason
			chosen = append(chosen, c)
			continue
		}
		c.Chosen = false
		c.Reason = rejectReason(c, len(chosenIDs))
		rejected = append(rejected, c)
	}
	sort.SliceStable(rejected, func(i, j int) bool {
		if rejected[i].Score != rejected[j].Score {
			return rejected[i].Score > rejected[j].Score
		}
		return rejected[i].ID < rejected[j].ID
	})
	if len(rejected) > rejectedLimit {
		rejected = rejected[:rejectedLimit]
	}
	return chosen, rejected
}

func rejectReason(c Candidate, chosenCount int) string {
	switch {
	case chosenCount > 0:
		return "budget"
	case c.Score <= 0:
		return "relevance"
	case c.Freshness < 0.25:
		return "freshness"
	default:
		return "relevance"
	}
}

// TokenCost estimates the token count of a text string (1 token ≈ 4 chars).
func TokenCost(text string) int {
	if strings.TrimSpace(text) == "" {
		return 1
	}
	n := len([]rune(text)) / 4
	if n < 1 {
		return 1
	}
	return n
}

// Freshness computes a 0–1 freshness score from last-seen time.
func Freshness(lastSeenMS, nowMS int64) float64 {
	if lastSeenMS <= 0 || nowMS <= 0 {
		return 0.5
	}
	age := nowMS - lastSeenMS
	if age <= 0 {
		return 1
	}
	ageDays := float64(age) / float64(24*time.Hour.Milliseconds())
	return 1 / (1 + ageDays)
}

// Risk computes a 0–2 risk score from confidence, freshness, and source.
func Risk(confidence, freshness float64, source string) float64 {
	if confidence <= 0 {
		confidence = 0.5
	}
	risk := (1 - confidence) + (1 - freshness)
	if source == "skill" {
		risk *= 0.75
	}
	if risk < 0 {
		return 0
	}
	if risk > 2 {
		return 2
	}
	return risk
}

// Summary builds the selection summary map.
func Summary(chosen, rejected []Candidate) map[string]any {
	sum := map[string]any{
		"chosen":        len(chosen),
		"rejected":      len(rejected),
		"chosen_tokens": totalTokens(chosen),
	}
	if len(rejected) > 0 {
		sum["top_rejected"] = rejected[0].ID
		sum["top_rejected_reason"] = rejected[0].Reason
		sum["top_rejected_score"] = rejected[0].Score
	}
	return sum
}

func totalTokens(cands []Candidate) int {
	total := 0
	for _, c := range cands {
		total += c.Tokens
	}
	return total
}

// ChosenIDSet builds a lookup set from chosen IDs.
func ChosenIDSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

// CandidateIDs extracts the ID field from a slice of candidates.
func CandidateIDs(cands []Candidate) []string {
	ids := make([]string, 0, len(cands))
	for _, c := range cands {
		ids = append(ids, c.ID)
	}
	return ids
}

// FailureAnalysisSuspects re-ranks rejected candidates from the last selection
// to identify likely context-omission suspects after a run failure.
func FailureAnalysisSuspects(rejected []Candidate) []Candidate {
	suspects := append([]Candidate(nil), rejected...)
	sort.SliceStable(suspects, func(i, j int) bool {
		li := suspects[i].Score - float64(suspects[i].Tokens)/1000 - suspects[i].RiskCost
		lj := suspects[j].Score - float64(suspects[j].Tokens)/1000 - suspects[j].RiskCost
		if li != lj {
			return li > lj
		}
		return suspects[i].ID < suspects[j].ID
	})
	if len(suspects) > 3 {
		suspects = suspects[:3]
	}
	return suspects
}

// MemoryCandidates converts memory search hits to context candidates.
func MemoryCandidates(hits []kmemory.Scored, nowMS int64) []Candidate {
	out := make([]Candidate, 0, len(hits))
	for _, h := range hits {
		r := h.Record
		text := r.Subject + " " + r.Content
		fresh := Freshness(r.LastSeenMS, nowMS)
		conf := r.Confidence
		if conf <= 0 {
			conf = 0.5
		}
		out = append(out, Candidate{
			Source:     "memory",
			ID:         r.ID,
			Label:      strings.TrimSpace(string(r.Type) + ":" + r.Subject),
			Score:      h.Score,
			Tokens:     TokenCost(text),
			HardCost:   TokenCost(text),
			SoftCost:   float64(TokenCost(text)) / 1000,
			RiskCost:   Risk(conf, fresh, "memory"),
			Freshness:  fresh,
			Confidence: conf,
			Signals:    []string{"provenance:" + emptyAs(r.SourceEvent, "unknown")},
		})
	}
	return out
}

// WorldCandidates converts world-model search hits to context candidates.
func WorldCandidates(hits []kworld.ScoredEntity, nowMS int64) []Candidate {
	out := make([]Candidate, 0, len(hits))
	for _, h := range hits {
		e := h.Entity
		text := string(e.Kind) + " " + e.Name + " " + strings.Join(e.Aliases, " ")
		fresh := Freshness(e.LastSeenMS, nowMS)
		conf := e.Weight
		if conf <= 0 {
			conf = 0.5
		}
		if conf > 1 {
			conf = 1
		}
		out = append(out, Candidate{
			Source:     "world",
			ID:         e.ID,
			Label:      strings.TrimSpace(string(e.Kind) + ":" + e.Name),
			Score:      h.Score,
			Tokens:     TokenCost(text),
			HardCost:   TokenCost(text),
			SoftCost:   float64(TokenCost(text)) / 1000,
			RiskCost:   Risk(conf, fresh, "world"),
			Freshness:  fresh,
			Confidence: conf,
			Signals:    []string{"provenance:" + emptyAs(e.SourceEvent, "unknown")},
		})
	}
	return out
}

// SkillCandidates converts skill search hits to context candidates.
func SkillCandidates(hits []kskill.Scored, nowMS int64) []Candidate {
	out := make([]Candidate, 0, len(hits))
	for _, h := range hits {
		s := h.Skill
		text := s.Name + " " + s.Description + " " + strings.Join(s.Triggers, " ") + " " + s.Body
		fresh := Freshness(s.Metrics.LastUsedMS, nowMS)
		if s.Metrics.LastUsedMS == 0 {
			fresh = Freshness(s.CreatedMS, nowMS)
		}
		conf := skillConfidence(s)
		out = append(out, Candidate{
			Source:     "skill",
			ID:         s.ID,
			Label:      s.Name,
			Score:      h.Score,
			Tokens:     TokenCost(text),
			HardCost:   TokenCost(text),
			SoftCost:   float64(TokenCost(text)) / 800,
			RiskCost:   Risk(conf, fresh, "skill"),
			Freshness:  fresh,
			Confidence: conf,
			Signals:    []string{"version:" + fmt.Sprint(s.Version)},
		})
	}
	return out
}

func skillConfidence(s kskill.Skill) float64 {
	total := s.Metrics.Successes + s.Metrics.Failures
	if total == 0 {
		return 0.65
	}
	return (float64(s.Metrics.Successes) + 1) / (float64(total) + 2)
}

func emptyAs(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// SPDX-License-Identifier: MIT
//
// Package contextselect: selection logic + types (Candidate, Manifest,
// SplitCandidates + rejectReason + Summary + FailureAnalysisSuspects +
// totalTokens + ChosenIDSet + CandidateIDs).
// Extracted from context.go during Day 211 god-file refactor (#71).
// Public API unchanged.
package contextselect

import (
	"sort"
)

const (
	CandidateLimit = 12
	rejectedLimit  = 5
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
	Phase       string         `json:"phase"`
	Query       string         `json:"query,omitempty"`
	BudgetChars int            `json:"budget_chars,omitempty"`
	Chosen      []Candidate    `json:"chosen,omitempty"`
	Rejected    []Candidate    `json:"rejected,omitempty"`
	Summary     map[string]any `json:"summary,omitempty"`
}
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
	// Per-candidate checks first: a candidate that the chooser would
	// have rejected anyway (score == 0 or very stale) keeps that label
	// even when the chosen set is non-empty. Only the leftover is
	// attributed to budget pressure.
	switch {
	case c.Score <= 0:
		return "relevance"
	case c.Freshness < 0.25:
		return "freshness"
	case chosenCount > 0:
		return "budget"
	default:
		return "relevance"
	}
}
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
func ChosenIDSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}
func CandidateIDs(cands []Candidate) []string {
	ids := make([]string, 0, len(cands))
	for _, c := range cands {
		ids = append(ids, c.ID)
	}
	return ids
}
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

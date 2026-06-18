// SPDX-License-Identifier: MIT

package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
	kmemory "github.com/agezt/agezt/kernel/memory"
	kskill "github.com/agezt/agezt/kernel/skill"
	kworld "github.com/agezt/agezt/kernel/worldmodel"
)

const (
	contextSelectionCandidateLimit = 12
	contextSelectionRejectedLimit  = 5
)

type contextCandidate struct {
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

type contextSelectionManifest struct {
	Phase       string             `json:"phase"`
	Query       string             `json:"query,omitempty"`
	BudgetChars int                `json:"budget_chars,omitempty"`
	Chosen      []contextCandidate `json:"chosen,omitempty"`
	Rejected    []contextCandidate `json:"rejected,omitempty"`
	Summary     map[string]any     `json:"summary,omitempty"`
}

func (k *Kernel) publishContextSelection(corr, actor string, manifest contextSelectionManifest) {
	if len(manifest.Chosen) == 0 && len(manifest.Rejected) == 0 && len(manifest.Summary) == 0 {
		return
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "context.selection",
		Kind:          event.KindContextSelection,
		Actor:         actor,
		CorrelationID: corr,
		Payload:       manifest,
	})
}

func splitContextCandidates(all []contextCandidate, chosenIDs map[string]bool, reason string) (chosen, rejected []contextCandidate) {
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
	if len(rejected) > contextSelectionRejectedLimit {
		rejected = rejected[:contextSelectionRejectedLimit]
	}
	return chosen, rejected
}

func rejectReason(c contextCandidate, chosenCount int) string {
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

func candidateTokenCost(text string) int {
	if strings.TrimSpace(text) == "" {
		return 1
	}
	n := len([]rune(text)) / 4
	if n < 1 {
		return 1
	}
	return n
}

func candidateFreshness(lastSeenMS, nowMS int64) float64 {
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

func candidateRisk(confidence, freshness float64, source string) float64 {
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

func candidateSummary(chosen, rejected []contextCandidate) map[string]any {
	sum := map[string]any{
		"chosen":        len(chosen),
		"rejected":      len(rejected),
		"chosen_tokens": totalCandidateTokens(chosen),
	}
	if len(rejected) > 0 {
		sum["top_rejected"] = rejected[0].ID
		sum["top_rejected_reason"] = rejected[0].Reason
		sum["top_rejected_score"] = rejected[0].Score
	}
	return sum
}

func totalCandidateTokens(cands []contextCandidate) int {
	total := 0
	for _, c := range cands {
		total += c.Tokens
	}
	return total
}

func chosenIDSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func (k *Kernel) publishContextFailureAnalysis(corr, actor string, runErr error) {
	if runErr == nil || k == nil || k.journal == nil {
		return
	}
	var latest *contextSelectionManifest
	_ = k.journal.Range(func(e *event.Event) error {
		if e.CorrelationID != corr || e.Kind != event.KindContextSelection {
			return nil
		}
		var m contextSelectionManifest
		if json.Unmarshal(e.Payload, &m) == nil && m.Phase != "failure_analysis" {
			latest = &m
		}
		return nil
	})
	if latest == nil || len(latest.Rejected) == 0 {
		return
	}
	suspects := append([]contextCandidate(nil), latest.Rejected...)
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
	k.publishContextSelection(corr, actor, contextSelectionManifest{
		Phase: "failure_analysis",
		Summary: map[string]any{
			"classifier":        "heuristic_external",
			"failure":           runErr.Error(),
			"suspect_omission":  len(suspects) > 0,
			"counterfactuals":   candidateIDs(suspects),
			"credit_assignment": fmt.Sprintf("replay with %d rejected candidate(s) from previous selection", len(suspects)),
		},
		Rejected: suspects,
	})
}

func candidateIDs(cands []contextCandidate) []string {
	ids := make([]string, 0, len(cands))
	for _, c := range cands {
		ids = append(ids, c.ID)
	}
	return ids
}

func memoryContextCandidates(hits []kmemory.Scored, nowMS int64) []contextCandidate {
	out := make([]contextCandidate, 0, len(hits))
	for _, h := range hits {
		r := h.Record
		text := r.Subject + " " + r.Content
		fresh := candidateFreshness(r.LastSeenMS, nowMS)
		conf := r.Confidence
		if conf <= 0 {
			conf = 0.5
		}
		out = append(out, contextCandidate{
			Source:     "memory",
			ID:         r.ID,
			Label:      strings.TrimSpace(string(r.Type) + ":" + r.Subject),
			Score:      h.Score,
			Tokens:     candidateTokenCost(text),
			HardCost:   candidateTokenCost(text),
			SoftCost:   float64(candidateTokenCost(text)) / 1000,
			RiskCost:   candidateRisk(conf, fresh, "memory"),
			Freshness:  fresh,
			Confidence: conf,
			Signals:    []string{"provenance:" + emptyAs(r.SourceEvent, "unknown")},
		})
	}
	return out
}

func worldContextCandidates(hits []kworld.ScoredEntity, nowMS int64) []contextCandidate {
	out := make([]contextCandidate, 0, len(hits))
	for _, h := range hits {
		e := h.Entity
		text := string(e.Kind) + " " + e.Name + " " + strings.Join(e.Aliases, " ")
		fresh := candidateFreshness(e.LastSeenMS, nowMS)
		conf := e.Weight
		if conf <= 0 {
			conf = 0.5
		}
		if conf > 1 {
			conf = 1
		}
		out = append(out, contextCandidate{
			Source:     "world",
			ID:         e.ID,
			Label:      strings.TrimSpace(string(e.Kind) + ":" + e.Name),
			Score:      h.Score,
			Tokens:     candidateTokenCost(text),
			HardCost:   candidateTokenCost(text),
			SoftCost:   float64(candidateTokenCost(text)) / 1000,
			RiskCost:   candidateRisk(conf, fresh, "world"),
			Freshness:  fresh,
			Confidence: conf,
			Signals:    []string{"provenance:" + emptyAs(e.SourceEvent, "unknown")},
		})
	}
	return out
}

func skillContextCandidates(hits []kskill.Scored, nowMS int64) []contextCandidate {
	out := make([]contextCandidate, 0, len(hits))
	for _, h := range hits {
		s := h.Skill
		text := s.Name + " " + s.Description + " " + strings.Join(s.Triggers, " ") + " " + s.Body
		fresh := candidateFreshness(s.Metrics.LastUsedMS, nowMS)
		if s.Metrics.LastUsedMS == 0 {
			fresh = candidateFreshness(s.CreatedMS, nowMS)
		}
		conf := skillConfidence(s)
		out = append(out, contextCandidate{
			Source:     "skill",
			ID:         s.ID,
			Label:      s.Name,
			Score:      h.Score,
			Tokens:     candidateTokenCost(text),
			HardCost:   candidateTokenCost(text),
			SoftCost:   float64(candidateTokenCost(text)) / 800,
			RiskCost:   candidateRisk(conf, fresh, "skill"),
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

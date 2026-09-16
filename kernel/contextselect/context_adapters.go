// SPDX-License-Identifier: MIT
//
// Package contextselect: per-source candidate adapters (MemoryCandidates +
// WorldCandidates + SkillCandidates) — convert kmemory/kworld/kskill Scored
// hits into Candidate rows.
// Extracted from context.go during Day 211 god-file refactor (#71).
// Public API unchanged.
package contextselect

import (
	"fmt"
	"strings"

	kmemory "github.com/agezt/agezt/kernel/memory"
	kworld "github.com/agezt/agezt/kernel/worldmodel"
	kskill "github.com/agezt/agezt/kernel/skill"
)

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

// SPDX-License-Identifier: MIT
//
// Package contextselect: scoring helpers (TokenCost + Freshness + Risk +
// skillConfidence + emptyAs).
// Extracted from context.go during Day 211 god-file refactor (#71).
// Public API unchanged.
package contextselect

import (
	"strings"
	"time"

	kskill "github.com/agezt/agezt/kernel/skill"
)

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

// SPDX-License-Identifier: MIT

// Package toolaudit renders the shared tool-policy journal representation.
package toolaudit

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
)

// PolicyDecisionPayload renders one gate decision for the journal. Split out so
// the 24-field map doesn't dominate the gating logic it belongs to.
func PolicyDecisionPayload(tc llm.ToolCall, v policyapi.PolicyVerdict) map[string]any {
	return map[string]any{
		"tool":                  tc.Name,
		"call_id":               tc.ID,
		"capability":            v.Capability,
		"allow":                 v.Allow,
		"reason":                v.Reason,
		"would_ask":             v.WouldAsk,
		"hard_denied":           v.HardDenied,
		"effect_class":          v.EffectClass,
		"affected_resources":    v.AffectedResources,
		"epistemic_action":      v.EpistemicAction,
		"epistemic_reason":      v.EpistemicReason,
		"epistemic_signals":     v.EpistemicSignals,
		"epistemic_confidence":  v.EpistemicConfidence,
		"failure_matches":       v.FailureMatches,
		"weighted_failures":     v.WeightedFailures,
		"schema_hash":           v.SchemaHash,
		"input_shape":           v.InputShape,
		"temporal_sensitive":    v.TemporalSensitive,
		"novel_tool":            v.NovelTool,
		"untrusted_observation": v.UntrustedObservation,
		"observation_sources":   v.ObservationSources,
		"directive_like":        v.ObservationDirectiveLike,
		"directive_matches":     v.ObservationDirectiveMatches,
	}
}

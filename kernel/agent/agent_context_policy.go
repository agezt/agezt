// SPDX-License-Identifier: MIT

// Package agent: loop ↔ policy-engine contract (PolicyVerdict + Policy).
// Extracted from agent_context.go during the Day-211 god-file split.
// Public API unchanged.
package agent


import (
	"context"
)

// PolicyVerdict is the contract between the loop and the policy engine
// (kernel/edict). The loop journals it as the payload of policy.decision.
type PolicyVerdict struct {
	// Allow is true → the tool will be invoked.
	Allow bool
	// Capability is the policy-engine's classification of this call
	// (e.g. "shell", "file.write", "http.post"). Free-form string so the
	// loop need not import kernel/edict.
	Capability string
	// Reason is human-readable; used in journal entries and (on deny) in
	// the synthetic tool result returned to the model.
	Reason string
	// WouldAsk indicates an approval would have been requested in a future
	// release with live HITL routing. Captured for audit.
	WouldAsk bool
	// HardDenied indicates a non-overridable rule fired.
	HardDenied bool
	// EffectClass is the runtime/tool classification used for governance
	// routing. Empty means unknown.
	EffectClass string
	// AffectedResources is a compact operator-facing resource list, when known.
	AffectedResources []string
	// EpistemicAction is the system-level calibration verdict for this proposed
	// tool call: allow, escalate, or deny. It is advisory unless the runtime
	// explicitly wires escalation into HITL.
	EpistemicAction string
	// EpistemicReason explains the calibration verdict in operator-facing text.
	EpistemicReason string
	// EpistemicSignals are structured reasons such as temporal_sensitive,
	// matched_failure_conditions:N, or low_effect_confidence:X.
	EpistemicSignals []string
	// EpistemicConfidence is the runtime confidence in the effect prediction.
	EpistemicConfidence float64
	// FailureMatches counts historical failures whose conditions match this call.
	FailureMatches int
	// WeightedFailures is FailureMatches with time decay applied.
	WeightedFailures float64
	// SchemaHash and InputShape identify the validated call condition used for
	// historical failure matching without storing raw schema/input twice.
	SchemaHash string
	InputShape string
	// TemporalSensitive marks calls whose correctness depends on fresh external
	// state, dates, versions, prices, schedules, or similar changing facts.
	TemporalSensitive bool
	// NovelTool marks calls whose tool/schema conditions have not appeared in the
	// journal window used by the runtime epistemic gate.
	NovelTool bool
	// UntrustedObservation marks proposals made after external-world data entered
	// the model context. This signal is produced by the loop, not by the model.
	UntrustedObservation bool
	// ObservationSources lists the external observation sources currently tainting
	// the run. Used for audit and HITL prompts.
	ObservationSources []string
	// ObservationDirectiveLike indicates that the external data contained text
	// resembling hidden instructions or social-engineering attempts.
	ObservationDirectiveLike bool
	// ObservationDirectiveMatches lists the directive-like patterns that fired.
	ObservationDirectiveMatches []string
}

// Policy is the signature the loop expects. Implementations are free to
// be pure functions (e.g. kernel/edict.Engine.Decide adapted by
// kernel/runtime).
type Policy func(ctx context.Context, tc ToolCall) PolicyVerdict

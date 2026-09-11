// SPDX-License-Identifier: MIT

package agent

// ToolCapability + Params accessor methods (IsZero / For). Carved out
// of agent.go during the Day 31 god file split #1 so the main file can
// focus on Run + context helpers.

import (
	"encoding/json"
	"strings"
)

func (c ToolCapability) IsZero() bool {
	return c.Name == "" && len(c.ByValue) == 0
}

// For resolves the capability one call exercises. Unparseable input resolves to
// Name — the tool author's declared fallback — rather than failing, because the
// policy layer must reach a decision for every call the model makes, including
// a malformed one.
func (c ToolCapability) For(input json.RawMessage) string {
	if c.Field == "" || len(c.ByValue) == 0 {
		return c.Name
	}
	var probe map[string]any
	if err := json.Unmarshal(input, &probe); err != nil {
		return c.Name
	}
	raw, ok := probe[c.Field].(string)
	if !ok {
		return c.Name
	}
	if cap, ok := c.ByValue[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return cap
	}
	return c.Name
}

// EffectClass classifies a tool call by operational reversibility. It is
// governance metadata, not provider-facing schema; runtime uses it to build HITL
// decision bundles and future compensation routing.
type EffectClass string

const (
	EffectUnknown      EffectClass = ""
	EffectReadOnly     EffectClass = "read_only"
	EffectReversible   EffectClass = "reversible"
	EffectCompensable  EffectClass = "compensable"
	EffectIrreversible EffectClass = "irreversible"
)

// ToolEffect is optional governance metadata supplied by a tool definition.
// Empty fields are filled by runtime capability defaults where possible.
type ToolEffect struct {
	Class             EffectClass
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
}

// Params carries optional per-request sampling / generation knobs that are
// universal across providers. Every field is a pointer (or a nil-able slice)
// so the zero value means "unset — send nothing, let the provider use its own
// default". An adapter MUST only emit a wire field when the corresponding
// pointer is non-nil, keeping an unset Params byte-for-byte identical to the
// pre-Params request (the same default-preserving contract as JSONMode).
//
// Provider-specific knobs that don't generalise (e.g. Anthropic's raw thinking
// config) ride CompletionRequest.ProviderOptions instead; ReasoningEffort is
// the one reasoning knob normalised here because every reasoning-capable family
// exposes some form of it (OpenAI reasoning_effort, Anthropic/Gemini thinking
// budget).
type Params struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	TopK             *int     `json:"top_k,omitempty"`
	Stop             []string `json:"stop,omitempty"`
	Seed             *int64   `json:"seed,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	// ReasoningEffort is the normalised reasoning/thinking knob: one of
	// "", "minimal", "low", "medium", "high". Empty leaves the provider's
	// construction-time default (e.g. AGEZT_ANTHROPIC_THINKING_BUDGET) in force.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// IsZero reports whether no per-request knob is set, so adapters can cheaply
// skip the whole apply path and guarantee an unchanged request.
func (p Params) IsZero() bool {
	return p.Temperature == nil && p.TopP == nil && p.TopK == nil &&
		len(p.Stop) == 0 && p.Seed == nil && p.FrequencyPenalty == nil &&
		p.PresencePenalty == nil && p.ReasoningEffort == ""
}

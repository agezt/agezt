// Package provopts holds shared helpers for applying per-request generation
// knobs (agent.Params) and provider-specific extras (CompletionRequest.
// ProviderOptions) onto an adapter's outbound wire request.
//
// The two helpers here are dialect-agnostic:
//
//   - Merge overlays a raw JSON object (the caller's ProviderOptions entry for
//     this dialect) onto an already-serialised request body. Empty extras leave
//     the body byte-for-byte unchanged, preserving the default-preserving
//     contract every adapter upholds.
//   - ThinkingBudget maps the normalised Params.ReasoningEffort enum onto a
//     concrete thinking/Reasoning token budget for the families that express
//     reasoning that way (Anthropic, Gemini). OpenAI-dialect adapters pass the
//     effort string through unchanged instead.
package provopts

import (
	"encoding/json"
	"maps"
	"strings"
)

// Merge overlays the top-level keys of the JSON object `extra` onto the JSON
// object `body`, with keys in `extra` winning. Both must encode JSON objects.
//
// When `extra` is empty/blank, `body` is returned unchanged (same bytes), so an
// unset ProviderOptions never alters the request. When `extra` is present the
// result is a re-marshalled object (key order may change, which is irrelevant
// over HTTP) carrying the overlay — an explicit opt-in by the caller.
func Merge(body []byte, extra json.RawMessage) ([]byte, error) {
	if len(strings.TrimSpace(string(extra))) == 0 {
		return body, nil
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(body, &base); err != nil {
		return nil, err
	}
	var over map[string]json.RawMessage
	if err := json.Unmarshal(extra, &over); err != nil {
		return nil, err
	}
	if base == nil {
		base = make(map[string]json.RawMessage, len(over))
	}
	maps.Copy(base, over)
	return json.Marshal(base)
}

// ThinkingBudget maps a normalised reasoning-effort level onto a thinking-token
// budget for families that express reasoning as a token allotment (Anthropic
// extended thinking, Gemini thinkingConfig). It returns ok=false when effort is
// empty (caller leaves its construction-time default in force) or unrecognised.
//
// maxTokens, when > 0, caps the budget below the response cap so the budget can
// never meet or exceed it (providers reject budget >= max_tokens). A floor of
// 1024 is applied because Anthropic rejects smaller budgets.
func ThinkingBudget(effort string, maxTokens int) (int, bool) {
	var b int
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal":
		b = 1024
	case "low":
		b = 2048
	case "medium":
		b = 8192
	case "high":
		b = 16384
	default:
		return 0, false
	}
	if b < 1024 {
		b = 1024
	}
	if maxTokens > 0 && b >= maxTokens {
		b = maxTokens - 1
		if b < 1024 {
			// max_tokens too small to carry any valid budget — disable.
			return 0, false
		}
	}
	return b, true
}

// NormalizeEffort lower-cases and validates a reasoning-effort string, returning
// "" for anything not in the supported set so adapters can cheaply gate on it.
func NormalizeEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal":
		return "minimal"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	default:
		return ""
	}
}

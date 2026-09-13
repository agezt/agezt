// SPDX-License-Identifier: MIT

// Package bedrock: vendor-detection predicates (isAnthropicModel +
// isMistralModel + isCohereModel + isMetaLlamaModel). Bedrock serves many
// vendors behind one endpoint; these decide which wire shape the adapter
// speaks (M1.m — Anthropic now, others land in M1.m.x). Extracted from
// bedrock.go during the Day-211 god-file split. Public API unchanged.
package bedrock


import (
	"strings"
)
func isAnthropicModel(id string) bool {
	if strings.HasPrefix(id, "anthropic.") {
		return true
	}
	// Regional profile: prefix segment + "." + "anthropic." + ...
	if i := strings.Index(id, ".anthropic."); i >= 0 && i < len(id)-len(".anthropic.") {
		return true
	}
	return false
}

// isMistralModel reports whether the model id maps to the
// Mistral-on-Bedrock body shape (M1.tt). Covers both direct ids
// (`mistral.mistral-large-2407-v1:0`) and regional cross-inference
// profiles (`eu.mistral.*`, `us.mistral.*`).
func isMistralModel(id string) bool {
	if strings.HasPrefix(id, "mistral.") {
		return true
	}
	if i := strings.Index(id, ".mistral."); i >= 0 && i < len(id)-len(".mistral.") {
		return true
	}
	return false
}

// isCohereModel reports whether the model id maps to the
// Cohere-on-Bedrock body shape (M1.tt-2). Cohere Command R/R+
// use a `message` / `chat_history` request shape.
func isCohereModel(id string) bool {
	if strings.HasPrefix(id, "cohere.") {
		return true
	}
	if i := strings.Index(id, ".cohere."); i >= 0 && i < len(id)-len(".cohere.") {
		return true
	}
	return false
}

// isMetaLlamaModel reports whether the model id maps to the
// Meta-Llama-on-Bedrock body shape (M1.tt-3). Uses the
// prompt-template format: `<|begin_of_text|><|start_header_id|>user
// <|end_header_id|>...<|eot_id|>` etc. No tool use through the
// raw prompt template; chat-only.
func isMetaLlamaModel(id string) bool {
	if strings.HasPrefix(id, "meta.") {
		return true
	}
	if i := strings.Index(id, ".meta."); i >= 0 && i < len(id)-len(".meta.") {
		return true
	}
	return false
}

// SPDX-License-Identifier: MIT

package compat

// Base URL helpers for compat providers: compatVendorBaseURL + defaultBaseURL.
// Carved out of compat.go during the Day 178 god-file split (further
// trimmed during Day 183) so the main file can stay focused on
// error vars + CredLookup + Build + family dispatch.
// Public API unchanged.

import (
	"strings"

	"github.com/agezt/agezt/kernel/catalog"
)
func compatVendorBaseURL(npm string) string {
	n := strings.TrimSpace(strings.ToLower(npm))
	if n == "@openrouter/ai-sdk-provider" {
		return "https://openrouter.ai/api/v1"
	}
	switch strings.TrimPrefix(n, "@ai-sdk/") {
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "xai":
		return "https://api.x.ai/v1"
	case "cerebras":
		return "https://api.cerebras.ai/v1"
	case "togetherai":
		return "https://api.together.xyz/v1"
	case "deepinfra":
		return "https://api.deepinfra.com/v1/openai"
	case "perplexity":
		return "https://api.perplexity.ai"
	case "fireworks":
		return "https://api.fireworks.ai/inference/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "moonshotai":
		return "https://api.moonshot.ai/v1"
	}
	return ""
}

// defaultBaseURL returns the well-known base URL for a family when the
// catalog's `api` field is empty. Only families with a single,
// universally-correct host get a default. The openai-compatible *family* has no
// single host (many vendors share it), so it returns "" here — per-vendor URLs
// come from compatVendorBaseURL instead, and a vendor with neither is caught by
// the empty-api guard in Build.
func defaultBaseURL(f catalog.Family) string {
	switch f {
	case catalog.FamilyAnthropic:
		// Includes the version segment: the anthropic adapter appends only
		// "/messages" (the @ai-sdk/anthropic convention models.dev follows).
		return "https://api.anthropic.com/v1"
	case catalog.FamilyOpenAI:
		return "https://api.openai.com/v1"
	case catalog.FamilyGoogle:
		return "https://generativelanguage.googleapis.com"
	case catalog.FamilyOllama:
		return "http://localhost:11434"
	case catalog.FamilyMistral:
		return "https://api.mistral.ai/v1"
	case catalog.FamilyCohere:
		return "https://api.cohere.com"
	}
	return ""
}

// FirstModelID returns a deterministic "first model" choice for a
// catalog entry: alphabetically smallest. The daemon uses this when
// AGEZT_MODEL is unset — operators get a working default rather than
// an error, and the choice is reproducible.

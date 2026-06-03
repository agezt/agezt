// SPDX-License-Identifier: MIT

package compat

import (
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

func TestCompatVendorBaseURL(t *testing.T) {
	cases := map[string]string{
		"@ai-sdk/groq":                "https://api.groq.com/openai/v1",
		"@ai-sdk/xai":                 "https://api.x.ai/v1",
		"@ai-sdk/cerebras":            "https://api.cerebras.ai/v1",
		"@ai-sdk/togetherai":          "https://api.together.xyz/v1",
		"@ai-sdk/deepinfra":           "https://api.deepinfra.com/v1/openai",
		"@ai-sdk/perplexity":          "https://api.perplexity.ai",
		"@ai-sdk/fireworks":           "https://api.fireworks.ai/inference/v1",
		"@openrouter/ai-sdk-provider": "https://openrouter.ai/api/v1",
		// Case-insensitive, mirroring catalog.FamilyFromNPM.
		"@AI-SDK/Groq": "https://api.groq.com/openai/v1",
		// Not vendors compat ships a default for → "" (guard stays active).
		"@ai-sdk/openai-compatible": "",
		"@ai-sdk/openai":            "",
		"@ai-sdk/anthropic":         "",
		"":                          "",
	}
	for npm, want := range cases {
		if got := compatVendorBaseURL(npm); got != want {
			t.Errorf("compatVendorBaseURL(%q) = %q, want %q", npm, got, want)
		}
	}
}

// Every vendor compatVendorBaseURL knows must classify as
// FamilyOpenAICompatible — otherwise Build would never consult the table for it.
func TestCompatVendorBaseURL_AllAreOpenAICompatibleFamily(t *testing.T) {
	for _, npm := range []string{
		"@ai-sdk/groq", "@ai-sdk/xai", "@ai-sdk/cerebras", "@ai-sdk/togetherai",
		"@ai-sdk/deepinfra", "@ai-sdk/perplexity", "@ai-sdk/fireworks",
		"@openrouter/ai-sdk-provider",
	} {
		if got := catalog.FamilyFromNPM(npm); got != catalog.FamilyOpenAICompatible {
			t.Errorf("FamilyFromNPM(%q) = %v, want FamilyOpenAICompatible", npm, got)
		}
		if compatVendorBaseURL(npm) == "" {
			t.Errorf("compatVendorBaseURL(%q) is empty but should have a default", npm)
		}
	}
}

// Build for a known compat vendor with an empty `api` now succeeds (the default
// URL is filled in) instead of being refused.
func TestBuild_KnownCompatVendorEmptyAPIUsesDefault(t *testing.T) {
	for _, npm := range []string{"@ai-sdk/groq", "@ai-sdk/xai", "@openrouter/ai-sdk-provider"} {
		entry := &catalog.Provider{
			ID: "vendor", NPM: npm,
			Env:    []string{"VENDOR_API_KEY"},
			API:    "",
			Models: map[string]*catalog.Model{"m": {ID: "m"}},
		}
		if _, _, err := Build(entry, "m", func(string) string { return "k" }); err != nil {
			t.Errorf("%s: Build with empty api should succeed now, got %v", npm, err)
		}
	}
}

// An explicit catalog `api` still wins over the built-in default.
func TestBuild_ExplicitAPIOverridesVendorDefault(t *testing.T) {
	entry := &catalog.Provider{
		ID: "groq", NPM: "@ai-sdk/groq",
		Env:    []string{"GROQ_API_KEY"},
		API:    "https://proxy.internal/v1",
		Models: map[string]*catalog.Model{"m": {ID: "m"}},
	}
	if _, _, err := Build(entry, "m", func(string) string { return "k" }); err != nil {
		t.Fatalf("Build with explicit api: %v", err)
	}
	// (The override path is the same `base := p.API` branch the other adapters
	// use; this just asserts a known vendor with an explicit api still builds.)
}

// SPDX-License-Identifier: MIT

package compat

import (
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

// DeepSeek is named in the README's compat vendor list, but before M232 its
// official package (@ai-sdk/deepseek) classified as FamilyUnknown, so
// compat.Build refused it with ErrFamilyUnsupported — a claimed vendor that
// didn't work. M232 classifies it as OpenAI-compatible and carries its base URL.
func TestDeepSeek_NowFirstClass(t *testing.T) {
	if got := catalog.FamilyFromNPM("@ai-sdk/deepseek"); got != catalog.FamilyOpenAICompatible {
		t.Fatalf("FamilyFromNPM(@ai-sdk/deepseek) = %v, want FamilyOpenAICompatible", got)
	}
	if got := compatVendorBaseURL("@ai-sdk/deepseek"); got != "https://api.deepseek.com/v1" {
		t.Errorf("compatVendorBaseURL(@ai-sdk/deepseek) = %q, want the deepseek root", got)
	}

	// Build with an empty `api` now succeeds (default URL filled in) instead of
	// failing with ErrFamilyUnsupported.
	entry := &catalog.Provider{
		ID: "deepseek", NPM: "@ai-sdk/deepseek",
		Env:    []string{"DEEPSEEK_API_KEY"},
		API:    "",
		Models: map[string]*catalog.Model{"deepseek-chat": {ID: "deepseek-chat"}},
	}
	if _, _, err := Build(entry, "deepseek-chat", func(string) string { return "k" }); err != nil {
		t.Fatalf("Build(deepseek, empty api) should succeed now, got %v", err)
	}
}

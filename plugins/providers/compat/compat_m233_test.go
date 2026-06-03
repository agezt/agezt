// SPDX-License-Identifier: MIT

package compat

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

// Moonshot AI (Kimi) ships an official @ai-sdk/moonshotai package and speaks the
// OpenAI dialect, so — like DeepSeek (M232) — it would otherwise classify as
// FamilyUnknown and be refused. M233 wires it.
func TestMoonshot_NowFirstClass(t *testing.T) {
	if got := catalog.FamilyFromNPM("@ai-sdk/moonshotai"); got != catalog.FamilyOpenAICompatible {
		t.Fatalf("FamilyFromNPM(@ai-sdk/moonshotai) = %v, want FamilyOpenAICompatible", got)
	}
	if got := compatVendorBaseURL("@ai-sdk/moonshotai"); got != "https://api.moonshot.ai/v1" {
		t.Errorf("compatVendorBaseURL(@ai-sdk/moonshotai) = %q, want the moonshot root", got)
	}
	entry := &catalog.Provider{
		ID: "moonshotai", NPM: "@ai-sdk/moonshotai",
		Env:    []string{"MOONSHOT_API_KEY"},
		API:    "",
		Models: map[string]*catalog.Model{"kimi-k2": {ID: "kimi-k2"}},
	}
	if _, _, err := Build(entry, "kimi-k2", func(string) string { return "k" }); err != nil {
		t.Fatalf("Build(moonshotai, empty api) should succeed now, got %v", err)
	}
}

// An unrecognised npm still fails — but with an actionable message pointing at
// the openai-compatible escape hatch, not the old "this branch is unreachable"
// claim that DeepSeek/Moonshot disproved (M233).
func TestBuild_UnknownNPMGivesActionableError(t *testing.T) {
	entry := &catalog.Provider{
		ID: "futurevendor", NPM: "@ai-sdk/some-future-vendor",
		Env:    []string{"FUTUREVENDOR_API_KEY"},
		API:    "",
		Models: map[string]*catalog.Model{"m": {ID: "m"}},
	}
	_, _, err := Build(entry, "m", func(string) string { return "k" })
	if err == nil {
		t.Fatal("expected ErrFamilyUnsupported for an unknown npm")
	}
	msg := err.Error()
	if !strings.Contains(msg, "openai-compatible") || !strings.Contains(msg, "custom.json") {
		t.Errorf("error should point at the openai-compatible/custom.json workaround: %q", msg)
	}
	if strings.Contains(msg, "unreachable") {
		t.Errorf("error still claims the branch is unreachable: %q", msg)
	}
}

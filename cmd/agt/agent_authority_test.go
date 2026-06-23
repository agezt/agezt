// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

func TestBuildAgentAuthority_MergesProfileAndPolicy(t *testing.T) {
	profile := map[string]any{
		"slug":             "researcher",
		"trust_ceiling":    "L2",
		"tool_allow":       []any{"file", "http", "memory"},
		"tool_deny":        []any{"shell"},
		"memory_scope":     "researcher/private",
		"workdir":          "researcher",
		"direct_callable":  true,
		"config_overrides": map[string]any{"AGEZT_MODEL": "sonnet"},
	}
	edict := map[string]any{
		"ask_policy": "prompt",
		"levels": map[string]any{
			"shell":      "L1",
			"file.write": "L3",
			"http.post":  "L4",
		},
		"hard_deny": []any{
			map[string]any{"name": "rm-root", "substring": "rm -rf /", "applies_to": []any{"shell"}},
		},
	}

	v := buildAgentAuthority(profile, edict)

	if v.Slug != "researcher" {
		t.Fatalf("slug: got %q want researcher", v.Slug)
	}
	if v.TrustCeiling != "L2" {
		t.Fatalf("trust ceiling: got %q want L2", v.TrustCeiling)
	}
	if !v.DirectCall {
		t.Fatal("direct_callable: got false want true")
	}
	if len(v.ToolAllow) != 3 {
		t.Fatalf("tool allow: got %d want 3", len(v.ToolAllow))
	}
	if len(v.ToolDeny) != 1 || v.ToolDeny[0] != "shell" {
		t.Fatalf("tool deny: got %v want [shell]", v.ToolDeny)
	}
	if v.ApprovalMode != "prompt" {
		t.Fatalf("approval mode: got %q want prompt", v.ApprovalMode)
	}
	if v.CapLevels["file.write"] != "L3" {
		t.Fatalf("cap level file.write: got %q want L3", v.CapLevels["file.write"])
	}
	if len(v.HardDeny) != 1 {
		t.Fatalf("hard deny: got %d want 1", len(v.HardDeny))
	}
	if v.HardDeny[0].Substring != "rm -rf /" {
		t.Fatalf("hard deny rule: got %q want 'rm -rf /'", v.HardDeny[0].Substring)
	}
	if v.ConfigCount != 1 {
		t.Fatalf("config count: got %d want 1", v.ConfigCount)
	}
}

func TestBuildAgentAuthority_EmptyEdict(t *testing.T) {
	// A daemon without CmdEdictShow should degrade gracefully — profile fields
	// still render, policy overlay is empty.
	profile := map[string]any{"slug": "minimal", "trust_ceiling": "L1"}
	v := buildAgentAuthority(profile, map[string]any{})
	if v.Slug != "minimal" {
		t.Fatalf("slug: got %q", v.Slug)
	}
	if v.ApprovalMode != "" {
		t.Fatalf("approval mode should be empty, got %q", v.ApprovalMode)
	}
	if len(v.CapLevels) != 0 {
		t.Fatalf("cap levels should be empty, got %d", len(v.CapLevels))
	}
}

func TestLevelRank(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"L0", 0}, {"deny", 0}, {"", 0},
		{"L1", 1}, {"ask", 1},
		{"L2", 2}, {"askfirst", 2},
		{"L3", 3}, {"askscoped", 3},
		{"L4", 4}, {"allow", 4},
		{"garbage", 0},
	}
	for _, tc := range cases {
		got := levelRank(tc.input)
		if got != tc.want {
			t.Errorf("levelRank(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestLevelExceeds(t *testing.T) {
	if !levelExceeds("L4", "L2") {
		t.Error("L4 should exceed L2")
	}
	if levelExceeds("L1", "L2") {
		t.Error("L1 should not exceed L2")
	}
	if levelExceeds("L2", "L2") {
		t.Error("equal levels should not exceed")
	}
}

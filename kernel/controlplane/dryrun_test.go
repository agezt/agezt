// SPDX-License-Identifier: MIT

package controlplane

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildRunPlan_ToolsModes(t *testing.T) {
	all := []string{"shell", "file", "http", "notify"}

	cases := []struct {
		name      string
		allowSet  bool
		allow     []string
		wantMode  string
		wantTools []string
		wantDrop  []string
	}{
		{"unrestricted = all", false, nil, "all", []string{"file", "http", "notify", "shell"}, nil},
		{"no-tools = none", true, []string{}, "none (--no-tools)", []string{}, nil},
		{"subset", true, []string{"shell", "file"}, "restricted", []string{"file", "shell"}, nil},
		{"unknown dropped", true, []string{"file", "ghost"}, "restricted", []string{"file"}, []string{"ghost"}},
		{"dedup", true, []string{"file", "file"}, "restricted", []string{"file"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := buildRunPlan(runPlanInput{
				Intent:       "hi",
				Model:        "mock",
				AllToolNames: all,
				AllowSet:     tc.allowSet,
				Allow:        tc.allow,
			})
			if got := plan["tools_mode"].(string); got != tc.wantMode {
				t.Errorf("tools_mode = %q, want %q", got, tc.wantMode)
			}
			if got := plan["tools"].([]string); !reflect.DeepEqual(got, tc.wantTools) {
				t.Errorf("tools = %v, want %v", got, tc.wantTools)
			}
			drop, _ := plan["tools_dropped"].([]string)
			if !reflect.DeepEqual(drop, tc.wantDrop) {
				t.Errorf("tools_dropped = %v, want %v", drop, tc.wantDrop)
			}
		})
	}
}

func TestBuildRunPlan_Sources(t *testing.T) {
	// Defaults: no overrides, no daemon system, no timeout.
	p := buildRunPlan(runPlanInput{Intent: "x", Model: "def"})
	if p["model_source"] != "daemon default" {
		t.Errorf("model_source = %v, want daemon default", p["model_source"])
	}
	if p["system_source"] != "none" {
		t.Errorf("system_source = %v, want none", p["system_source"])
	}
	if p["timeout"] != "none" {
		t.Errorf("timeout = %v, want none", p["timeout"])
	}
	if p["execution_profile"] != "(tool defaults)" || p["execution_profile_source"] != "tool defaults" || p["warden_profile"] != "tool defaults" {
		t.Errorf("execution defaults wrong: profile=%v source=%v warden=%v", p["execution_profile"], p["execution_profile_source"], p["warden_profile"])
	}
	if p["tenant"] != "(primary)" {
		t.Errorf("tenant = %v, want (primary)", p["tenant"])
	}
	// model_known false → capability keys omitted (not just false).
	if _, ok := p["supports_vision"]; ok {
		t.Error("supports_vision present for unknown model; want omitted")
	}

	// All overrides set.
	p = buildRunPlan(runPlanInput{
		Intent:           "x",
		Tenant:           "acme",
		Model:            "claude",
		ModelOverridden:  true,
		ModelKnown:       true,
		SupportsVision:   true,
		SupportsTools:    true,
		SystemSet:        true,
		SystemOverride:   true,
		Timeout:          "30s",
		DaemonTimeout:    5 * time.Minute,
		ExecutionProfile: "remote-agezt",
		WardenProfile:    "remote-agezt",
		RemotePeer:       "nodeB",
	})
	if p["model_source"] != "per-run (--model)" {
		t.Errorf("model_source = %v", p["model_source"])
	}
	if p["system_source"] != "per-run (--system)" {
		t.Errorf("system_source = %v", p["system_source"])
	}
	if p["timeout"] != "30s (per-run)" {
		t.Errorf("timeout = %v, want 30s (per-run)", p["timeout"])
	}
	if p["execution_profile"] != "remote-agezt" || p["execution_profile_source"] != "per-run (--exec-profile)" || p["warden_profile"] != "remote-agezt" {
		t.Errorf("execution override wrong: profile=%v source=%v warden=%v", p["execution_profile"], p["execution_profile_source"], p["warden_profile"])
	}
	if p["remote_peer"] != "nodeB" {
		t.Errorf("remote_peer = %v, want nodeB", p["remote_peer"])
	}
	if p["tenant"] != "acme" {
		t.Errorf("tenant = %v", p["tenant"])
	}
	if p["supports_vision"] != true || p["supports_tools"] != true {
		t.Errorf("caps not surfaced: %v / %v", p["supports_vision"], p["supports_tools"])
	}

	// Daemon default timeout (no per-run), daemon system set but no per-run override.
	p = buildRunPlan(runPlanInput{Intent: "x", Model: "def", SystemSet: true, DaemonTimeout: 2 * time.Minute})
	if p["timeout"] != "2m0s (daemon default)" {
		t.Errorf("timeout = %v, want 2m0s (daemon default)", p["timeout"])
	}
	if p["system_source"] != "daemon default" {
		t.Errorf("system_source = %v, want daemon default", p["system_source"])
	}
}

func warningsOf(p map[string]any) []string {
	w, _ := p["warnings"].([]string)
	return w
}

func hasWarningContaining(p map[string]any, substr string) bool {
	for _, w := range warningsOf(p) {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestBuildRunPlan_CostCap(t *testing.T) {
	// No cap → "none".
	p := buildRunPlan(runPlanInput{Model: "m", ModelKnown: true, ModelPriced: true})
	if p["cost_cap"] != "none" {
		t.Errorf("no cap: cost_cap = %v want none", p["cost_cap"])
	}

	// Cap on a priced model → formatted, no warning.
	p = buildRunPlan(runPlanInput{
		Model: "claude", ModelKnown: true, ModelPriced: true, MaxCostMC: 500_000_000,
	})
	if p["cost_cap"] != "$0.50 (per-run)" {
		t.Errorf("cost_cap = %v want $0.50 (per-run)", p["cost_cap"])
	}
	if hasWarningContaining(p, "will not bind") {
		t.Errorf("priced model should not warn about binding: %v", warningsOf(p))
	}

	// Cap on an unpriced (catalog-known but no Cost) model → warns it won't bind.
	p = buildRunPlan(runPlanInput{
		Model: "freebie", ModelKnown: true, ModelPriced: false, SupportsTools: true, MaxCostMC: 100_000_000,
	})
	if p["cost_cap"] != "$0.10 (per-run)" {
		t.Errorf("cost_cap = %v want $0.10 (per-run)", p["cost_cap"])
	}
	if !hasWarningContaining(p, "will not bind") {
		t.Errorf("unpriced model with a cap should warn; got %v", warningsOf(p))
	}
}

func TestBuildRunPlan_Warnings(t *testing.T) {
	all := []string{"shell", "file"}

	t.Run("unknown model warns", func(t *testing.T) {
		p := buildRunPlan(runPlanInput{Model: "mystery", AllToolNames: all})
		if !hasWarningContaining(p, "not in the catalog") {
			t.Errorf("want unknown-model warning, got %v", warningsOf(p))
		}
	})

	t.Run("tool_call=false with tools enabled warns", func(t *testing.T) {
		p := buildRunPlan(runPlanInput{
			Model: "notools", ModelKnown: true, SupportsTools: false, ContextLimit: 200000,
			AllToolNames: all, // unrestricted → tools effective
		})
		if !hasWarningContaining(p, "does not advertise tool-use") {
			t.Errorf("want tool-use warning, got %v", warningsOf(p))
		}
		if !hasWarningContaining(p, "AGEZT_MODEL_STRICT") {
			t.Errorf("tool-use warning should mention strict mode, got %v", warningsOf(p))
		}
	})

	t.Run("tool_call=false with --no-tools does NOT warn", func(t *testing.T) {
		p := buildRunPlan(runPlanInput{
			Model: "notools", ModelKnown: true, SupportsTools: false, ContextLimit: 200000,
			AllToolNames: all, AllowSet: true, Allow: []string{}, // --no-tools
		})
		if hasWarningContaining(p, "does not advertise tool-use") {
			t.Errorf("no tools enabled — should not warn about tool-use, got %v", warningsOf(p))
		}
	})

	t.Run("small context warns", func(t *testing.T) {
		p := buildRunPlan(runPlanInput{
			Model: "tiny", ModelKnown: true, SupportsTools: true, ContextLimit: 4096,
			AllToolNames: all,
		})
		if !hasWarningContaining(p, "small context window") {
			t.Errorf("want small-context warning, got %v", warningsOf(p))
		}
	})

	t.Run("healthy known model: no warnings, key omitted", func(t *testing.T) {
		p := buildRunPlan(runPlanInput{
			Model: "good", ModelKnown: true, SupportsTools: true, ContextLimit: 200000,
			AllToolNames: all,
		})
		if _, ok := p["warnings"]; ok {
			t.Errorf("healthy model should omit warnings key, got %v", p["warnings"])
		}
	})
}

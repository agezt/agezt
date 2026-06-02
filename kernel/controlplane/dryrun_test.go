// SPDX-License-Identifier: MIT

package controlplane

import (
	"reflect"
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
	if p["tenant"] != "(primary)" {
		t.Errorf("tenant = %v, want (primary)", p["tenant"])
	}
	// model_known false → capability keys omitted (not just false).
	if _, ok := p["supports_vision"]; ok {
		t.Error("supports_vision present for unknown model; want omitted")
	}

	// All overrides set.
	p = buildRunPlan(runPlanInput{
		Intent:          "x",
		Tenant:          "acme",
		Model:           "claude",
		ModelOverridden: true,
		ModelKnown:      true,
		SupportsVision:  true,
		SupportsTools:   true,
		SystemSet:       true,
		SystemOverride:  true,
		Timeout:         "30s",
		DaemonTimeout:   5 * time.Minute,
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

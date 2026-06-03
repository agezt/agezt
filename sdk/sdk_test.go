// SPDX-License-Identifier: MIT

package sdk

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildRunArgs_DefaultsToIntentOnly(t *testing.T) {
	args := buildRunArgs("hello", runConfig{})
	if args["intent"] != "hello" {
		t.Errorf("intent = %v", args["intent"])
	}
	for _, k := range []string{"model", "tenant", "system", "timeout", "tools", "images", "max_cost"} {
		if _, ok := args[k]; ok {
			t.Errorf("unset option leaked key %q into args", k)
		}
	}
}

func TestBuildRunArgs_AllOptions(t *testing.T) {
	var cfg runConfig
	for _, o := range []Option{
		WithModel("m1"),
		WithTenant("t1"),
		WithSystem("be terse"),
		WithTimeout(90 * time.Second),
		WithTools("file", "http"),
		WithImages("data:image/png;base64,AA"),
		WithMaxCostUSD(0.5),
	} {
		o(&cfg)
	}
	args := buildRunArgs("go", cfg)

	if args["model"] != "m1" || args["tenant"] != "t1" || args["system"] != "be terse" {
		t.Errorf("string options wrong: %+v", args)
	}
	if args["timeout"] != "1m30s" {
		t.Errorf("timeout = %v, want a Go duration string 1m30s", args["timeout"])
	}
	tools, ok := args["tools"].([]any)
	if !ok || len(tools) != 2 || tools[0] != "file" || tools[1] != "http" {
		t.Errorf("tools = %v", args["tools"])
	}
	imgs, ok := args["images"].([]any)
	if !ok || len(imgs) != 1 || imgs[0] != "data:image/png;base64,AA" {
		t.Errorf("images = %v", args["images"])
	}
	// $0.50 → 0.5e9 microcents, sent as float64.
	if mc, ok := args["max_cost"].(float64); !ok || mc != 0.5e9 {
		t.Errorf("max_cost = %v, want 5e8", args["max_cost"])
	}
}

func TestWithTools_ExplicitEmptyVsOmitted(t *testing.T) {
	// Omitted: no tools key (full default toolset).
	if _, ok := buildRunArgs("x", runConfig{})["tools"]; ok {
		t.Error("omitted WithTools should not set the tools key")
	}
	// Explicit empty: tools key present as an empty array (no tools).
	var cfg runConfig
	WithTools()(&cfg)
	ts, ok := buildRunArgs("x", cfg)["tools"].([]any)
	if !ok || len(ts) != 0 {
		t.Errorf("WithTools() should set an empty tools array, got %v", buildRunArgs("x", cfg)["tools"])
	}
}

func TestWithMaxCostUSD_NonPositiveIgnored(t *testing.T) {
	var cfg runConfig
	WithMaxCostUSD(0)(&cfg)
	WithMaxCostUSD(-1)(&cfg)
	if _, ok := buildRunArgs("x", cfg)["max_cost"]; ok {
		t.Error("non-positive max cost should be ignored")
	}
}

func TestParseResult(t *testing.T) {
	// JSON numbers arrive as float64.
	r := parseResult(map[string]any{
		"answer":         "the answer",
		"correlation_id": "corr-1",
		"model":          "m1",
		"iters":          float64(3),
		"spent_mc":       float64(5e8), // 0.5 USD
	})
	if r.Answer != "the answer" || r.CorrelationID != "corr-1" || r.Model != "m1" {
		t.Errorf("strings wrong: %+v", r)
	}
	if r.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", r.Iterations)
	}
	if r.CostUSD != 0.5 {
		t.Errorf("cost = %v, want 0.5", r.CostUSD)
	}
}

func TestParseResult_MissingFieldsAreZero(t *testing.T) {
	r := parseResult(map[string]any{"answer": "a"})
	if r.Answer != "a" || r.CorrelationID != "" || r.Model != "" || r.Iterations != 0 || r.CostUSD != 0 {
		t.Errorf("missing fields should be zero values: %+v", r)
	}
}

func TestDial_ReadsRuntimeFiles(t *testing.T) {
	base := t.TempDir()
	rt := filepath.Join(base, "runtime")
	if err := os.MkdirAll(rt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "control.addr"), []byte("127.0.0.1:9999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "control.token"), []byte("tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// AGEZT_TOKEN unset so the token file is the source of truth.
	t.Setenv("AGEZT_TOKEN", "")

	c, err := Dial(base)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if c == nil || c.cp == nil {
		t.Fatal("Dial returned a nil client")
	}
}

func TestDial_NoDaemonErrors(t *testing.T) {
	if _, err := Dial(t.TempDir()); err == nil {
		t.Error("Dial on a base with no runtime files should error")
	}
}

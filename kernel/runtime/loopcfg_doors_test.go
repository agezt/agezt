// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openLoopCfgKernel opens a daemon with one tool, so the loop configuration
// has a surface to shape.
func openLoopCfgKernel(t *testing.T, cfg runtime.Config) *runtime.Kernel {
	t.Helper()
	if cfg.Provider == nil {
		cfg.Provider = mock.New(mock.FinalText("done"))
	}
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestBuildLoopConfig_ReturnsTheGovernedShape pins the door's contract: the
// daemon hands the runs module's rule the state it needs, and the result is
// a governed loop — cost function, memo and rescue marker included.
func TestBuildLoopConfig_ReturnsTheGovernedShape(t *testing.T) {
	k := openLoopCfgKernel(t, runtime.Config{BaseDir: t.TempDir()})
	lc := k.BuildLoopConfig(context.Background(), "corr-1", "some-model")
	if lc.CostFn == nil {
		t.Fatal("CostFn = nil — the run's spend ceiling would be inert")
	}
	if lc.ToolMemo == nil {
		t.Fatal("ToolMemo = nil, want a memo")
	}
	if len(lc.ContextRescueMarkers) != 1 {
		t.Fatalf("ContextRescueMarkers = %v, want exactly one", lc.ContextRescueMarkers)
	}
	if lc.Model != "some-model" {
		t.Fatalf("Model = %q, want some-model", lc.Model)
	}
}

// TestBuildLoopConfig_ProjectsTheEffectiveConfiguration proves the
// projection is live: the daemon's own configuration lands on the loop,
// knob for knob.
func TestBuildLoopConfig_ProjectsTheEffectiveConfiguration(t *testing.T) {
	prov := mock.New(mock.FinalText("done"))
	catalog := &catalogapi.Catalog{
		Providers: map[string]*catalogapi.Provider{
			"mock": {
				ID: "mock",
				Models: map[string]*catalogapi.Model{
					"some-model": {ID: "some-model", Limit: catalogapi.Limit{Context: 100000}},
				},
			},
		},
	}
	k := openLoopCfgKernel(t, runtime.Config{
		BaseDir:             t.TempDir(),
		Provider:            prov,
		Catalog:             catalog,
		MaxIter:             11,
		MaxAutoContinue:     3,
		AutoContinueWait:    2 * time.Second,
		ToolTimeout:         7 * time.Second,
		MaxParallelTools:    5,
		ObservationDeltas:   true,
		ToolDiscoveryMax:    9,
		ContextBudget:       4321,
		ContextBudgetAuto:   true,
		ContextProtectFirst: 64,
		ContextSummarize:    true,
		ArtifactThreshold:   8192,
	})
	lc := k.BuildLoopConfig(context.Background(), "corr-1", "some-model")

	if lc.Provider == nil {
		t.Fatal("Provider = nil, want the daemon's provider")
	}
	if lc.MaxIter != 11 {
		t.Fatalf("MaxIter = %d, want 11", lc.MaxIter)
	}
	if lc.MaxAutoContinue != 3 {
		t.Fatalf("MaxAutoContinue = %d, want 3", lc.MaxAutoContinue)
	}
	if lc.AutoContinueWait != 2*time.Second {
		t.Fatalf("AutoContinueWait = %s, want 2s", lc.AutoContinueWait)
	}
	if lc.ToolTimeout != 7*time.Second {
		t.Fatalf("ToolTimeout = %s, want 7s", lc.ToolTimeout)
	}
	if lc.MaxParallelTools != 5 {
		t.Fatalf("MaxParallelTools = %d, want 5", lc.MaxParallelTools)
	}
	if !lc.ObservationDeltas {
		t.Fatal("ObservationDeltas = false, want true")
	}
	if lc.ContextBudget != 4321 {
		t.Fatalf("ContextBudget = %d, want 4321", lc.ContextBudget)
	}
	if lc.ContextProtectFirst != 64 {
		t.Fatalf("ContextProtectFirst = %d, want 64", lc.ContextProtectFirst)
	}
	if lc.ArtifactThreshold != 8192 {
		t.Fatalf("ArtifactThreshold = %d, want 8192", lc.ArtifactThreshold)
	}
	if lc.SummarizeElided == nil {
		t.Fatal("SummarizeElided = nil, want the summarizer (ContextSummarize is on)")
	}
}

// TestBuildLoopConfig_DerivesTheAutoContextBudgetFromTheCatalog proves the
// catalog crosses the port: the module asks the daemon for the live catalog
// and derives the run's context budget from the model's window.
func TestBuildLoopConfig_DerivesTheAutoContextBudgetFromTheCatalog(t *testing.T) {
	catalog := &catalogapi.Catalog{
		Providers: map[string]*catalogapi.Provider{
			"mock": {
				ID: "mock",
				Models: map[string]*catalogapi.Model{
					"some-model": {ID: "some-model", Limit: catalogapi.Limit{Context: 100000}},
				},
			},
		},
	}
	k := openLoopCfgKernel(t, runtime.Config{
		BaseDir:           t.TempDir(),
		Catalog:           catalog,
		ContextBudgetAuto: true,
	})
	lc := k.BuildLoopConfig(context.Background(), "corr-1", "some-model")
	want := agent.AutoContextBudgetChars(100000)
	if lc.ContextBudget != want {
		t.Fatalf("ContextBudget = %d, want %d (derived from the catalog)", lc.ContextBudget, want)
	}
}

// TestBuildLoopConfig_DefersDiscoveryAboveTheBound proves the discovery
// bound crosses the port: the daemon's bound reaches the module, and the
// module defers the catalog behind the search tool.
func TestBuildLoopConfig_DefersDiscoveryAboveTheBound(t *testing.T) {
	k := openLoopCfgKernel(t, runtime.Config{
		BaseDir: t.TempDir(),
		Tools: map[string]toolapi.Tool{
			"read":  stubLoopCfgTool{name: "read"},
			"write": stubLoopCfgTool{name: "write"},
			"shell": stubLoopCfgTool{name: "shell"},
		},
		ToolDiscoveryMax: 2,
	})
	lc := k.BuildLoopConfig(context.Background(), "corr-1", "some-model")
	if _, ok := lc.Tools["tool_search"]; !ok {
		t.Fatal("tool_search was not installed — the discovery bound did not cross the port")
	}
}

// TestBuildLoopConfig_HonoursTheAgentsOwnOverrides is the reason the
// configuration crosses as a PROJECTION and not a snapshot: a named agent's
// override is honoured for its own run.
func TestBuildLoopConfig_HonoursTheAgentsOwnOverrides(t *testing.T) {
	k := openLoopCfgKernel(t, runtime.Config{BaseDir: t.TempDir(), MaxIter: 11})
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:            "tuner",
		ConfigOverrides: map[string]string{"AGEZT_MAX_ITER": "42"},
	})
	lc := k.BuildLoopConfig(ctx, "corr-1", "some-model")
	if lc.MaxIter != 42 {
		t.Fatalf("MaxIter = %d, want 42 (the agent's own override)", lc.MaxIter)
	}
}

// TestBuildLoopConfig_AppliesTheRunAllowlist proves the daemon's per-run
// restriction still reaches the loop's tool surface.
func TestBuildLoopConfig_AppliesTheRunAllowlist(t *testing.T) {
	k := openLoopCfgKernel(t, runtime.Config{
		BaseDir: t.TempDir(),
		Tools:   map[string]toolapi.Tool{"read": stubLoopCfgTool{name: "read"}, "write": stubLoopCfgTool{name: "write"}},
	})
	ctx := runctx.WithTools(context.Background(), []string{"read"})
	lc := k.BuildLoopConfig(ctx, "corr-1", "some-model")
	if _, ok := lc.Tools["read"]; !ok {
		t.Fatal("Tools does not carry read")
	}
	if _, ok := lc.Tools["write"]; ok {
		t.Fatal("Tools carries write, want the run's allowlist to drop it")
	}
}

// TestBuildLoopConfig_CarriesTheDaemonsTools proves the daemon's live tool
// surface is the module's input — the merged MCP attachments and forged
// script tools are in, not the module's own catalogue.
func TestBuildLoopConfig_CarriesTheDaemonsTools(t *testing.T) {
	k := openLoopCfgKernel(t, runtime.Config{
		BaseDir: t.TempDir(),
		Tools:   map[string]toolapi.Tool{"echo": stubLoopCfgTool{name: "echo"}},
	})
	lc := k.BuildLoopConfig(context.Background(), "corr-1", "some-model")
	if _, ok := lc.Tools["echo"]; !ok {
		t.Fatal("Tools does not carry the daemon's echo tool")
	}
}

// TestBuildLoopConfig_CarriesTheDaemonsState pins the daemon's HALF of the
// move: the state the module is built from reaches the loop — the bus, the
// tool-phase service, the policy hook, the noise-policy hook and the
// artifact port.
func TestBuildLoopConfig_CarriesTheDaemonsState(t *testing.T) {
	k := openLoopCfgKernel(t, runtime.Config{BaseDir: t.TempDir()})
	lc := k.BuildLoopConfig(context.Background(), "corr-1", "some-model")
	if lc.Bus == nil {
		t.Fatal("Bus = nil, want the daemon's bus")
	}
	if lc.ToolPhases == nil {
		t.Fatal("ToolPhases = nil, want the daemon's tool-phase service")
	}
	if lc.Policy == nil {
		t.Fatal("Policy = nil, want the daemon's policy hook")
	}
	if lc.ToolResultHook == nil {
		t.Fatal("ToolResultHook = nil, want the daemon's noise-policy hook")
	}
	// The artifact port is the daemon's own: it is present only when the
	// daemon was given an artifact store, and the loop must carry whatever the
	// daemon has — never a substitute of its own.
}

type stubLoopCfgTool struct{ name string }

func (s stubLoopCfgTool) Definition() toolapi.ToolDef { return toolapi.ToolDef{Name: s.name} }
func (s stubLoopCfgTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: s.name}, nil
}

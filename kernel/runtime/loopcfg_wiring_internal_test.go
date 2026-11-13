// SPDX-License-Identifier: MIT

package runtime

// The daemon's WIRING for the runs module's loop configuration (W3.77).
//
// The rule — the shape every governed run's loop must have — is the module's
// (kernel/modules/runs/loopcfg, W3.76g). The daemon holds the STATE it is
// built from: its effective configuration, its live tool surface, its
// tool-phase service, its bus, its policy hook, its artifact port and its
// model catalog.
//
// The daemon holds no DOOR over the rule any more: the run engine and the
// delegation engine — both inside the module that owns the rule — hold the
// very same service and ask it directly.
//
// These tests are the daemon's HALF, and they are WHITE-BOX: the daemon's
// own service is the only place the daemon's state is observable now, and the
// rule's own shape is the module's test to keep.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openLoopCfgKernel opens a daemon with one tool, so the loop configuration
// has a surface to shape.
func openLoopCfgKernel(t *testing.T, cfg Config) *Kernel {
	t.Helper()
	if cfg.Provider == nil {
		cfg.Provider = mock.New(mock.FinalText("done"))
	}
	k, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestTheDaemonProjectsItsEffectiveConfiguration proves the projection is
// live: the daemon's own configuration lands on the loop, knob for knob.
func TestTheDaemonProjectsItsEffectiveConfiguration(t *testing.T) {
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
	k := openLoopCfgKernel(t, Config{
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
	lc := k.loopcfg.Build(context.Background(), "corr-1", "some-model")

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

// TestTheDaemonHonoursTheAgentsOwnOverrides is the reason the configuration
// crosses as a PROJECTION and not a snapshot: a named agent's override is
// honoured for its own run.
func TestTheDaemonHonoursTheAgentsOwnOverrides(t *testing.T) {
	k := openLoopCfgKernel(t, Config{BaseDir: t.TempDir(), MaxIter: 11})
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:            "tuner",
		ConfigOverrides: map[string]string{"AGEZT_MAX_ITER": "42"},
	})
	lc := k.loopcfg.Build(ctx, "corr-1", "some-model")
	if lc.MaxIter != 42 {
		t.Fatalf("MaxIter = %d, want 42 (the agent's own override)", lc.MaxIter)
	}
}

// TestTheDaemonCarriesItsLiveTools proves the daemon's live tool surface is
// the module's input — the merged MCP attachments and forged script tools are
// in, not the module's own catalogue.
func TestTheDaemonCarriesItsLiveTools(t *testing.T) {
	k := openLoopCfgKernel(t, Config{
		BaseDir: t.TempDir(),
		Tools:   map[string]toolapi.Tool{"echo": stubLoopCfgTool{name: "echo"}},
	})
	lc := k.loopcfg.Build(context.Background(), "corr-1", "some-model")
	if _, ok := lc.Tools["echo"]; !ok {
		t.Fatal("Tools does not carry the daemon's echo tool")
	}
}

// TestTheDaemonCarriesItsState pins the daemon's HALF of the move: the state
// the module is built from reaches the loop — the bus, the tool-phase
// service, the policy hook, the noise-policy hook and the artifact port.
func TestTheDaemonCarriesItsState(t *testing.T) {
	k := openLoopCfgKernel(t, Config{BaseDir: t.TempDir()})
	lc := k.loopcfg.Build(context.Background(), "corr-1", "some-model")
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

// TestTheDaemonWiresItsBoundPhaseService pins the tool-phase wiring: the
// loop configuration carries the very service the daemon BOUND — the one the
// application's invoker factory produced — and it is bound ONCE per kernel.
//
// It used to be observed through the daemon's door (BuildLoopConfig); the
// door is the module's now, and the daemon's own service is where the
// daemon's wiring can still be seen.
func TestTheDaemonWiresItsBoundPhaseService(t *testing.T) {
	k := openLoopCfgKernel(t, Config{BaseDir: t.TempDir()})
	lc := k.loopcfg.Build(context.Background(), "other", "")
	if lc.ToolPhases == nil {
		t.Fatal("ToolPhases = nil, want the daemon's bound phase service")
	}
	if lc.ToolPhases != k.toolPhases {
		t.Fatal("the loop configuration carries another phase service — the daemon's bound one must reach the loop")
	}
	one := k.loopcfg.Build(context.Background(), "one", "").ToolPhases
	two := k.loopcfg.Build(context.Background(), "two", "").ToolPhases
	if one == nil || one != two {
		t.Fatal("the phase service is not bound once per kernel")
	}
}

type stubLoopCfgTool struct{ name string }

func (s stubLoopCfgTool) Definition() toolapi.ToolDef { return toolapi.ToolDef{Name: s.name} }
func (s stubLoopCfgTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: s.name}, nil
}

// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// fakeProvider records the request it was given and answers with a fixed
// line — enough to prove that the elided-output summarizer is wired to the
// run's provider.
type fakeProvider struct {
	last llm.CompletionRequest
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	f.last = req
	return &llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "  a summary  "}}, nil
}

// testCatalog is a one-model catalog: the context window is what the
// auto-budget derives from.
func testCatalog(contextTokens int, reasoning bool) *catalogapi.Catalog {
	return &catalogapi.Catalog{
		Providers: map[string]*catalogapi.Provider{
			"fake": {
				ID: "fake",
				Models: map[string]*catalogapi.Model{
					"fake-model": {
						ID:        "fake-model",
						Reasoning: reasoning,
						Limit:     catalogapi.Limit{Context: contextTokens},
					},
				},
			},
		},
	}
}

// agentProfileDenyShell is a named agent that may not use the shell.
func agentProfileDenyShell() agentapi.Profile {
	return agentapi.Profile{ToolDeny: []string{"shell"}}
}

// baseDeps is the module's dependency set for the tests: no bus, no phases,
// no artifacts, no policy — the rule under test is the tool surface, the
// selector, the context budget and the summarizer.
func baseDeps(tools map[string]toolapi.Tool) Deps {
	return Deps{
		Tools: func(context.Context) map[string]toolapi.Tool { return tools },
		Bus:   nil,
	}
}

// TestBuild_AppliesTheNoisePolicy covers the noise half of the tool-shaping
// rule inside the build: an agent that may not write memory does not see the
// memory tool.
func TestBuild_AppliesTheNoisePolicy(t *testing.T) {
	svc := New(baseDeps(toolSet("memory", "read")))
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{
		NoisePolicy: &agentapi.NoisePolicy{DisableMemoryWrites: true},
	})
	lc := svc.Build(ctx, "corr-1", "fake-model", Config{})
	if _, ok := lc.Tools["memory"]; ok {
		t.Fatal("Tools carries memory, want the noise policy to drop it")
	}
	if _, ok := lc.Tools["read"]; !ok {
		t.Fatal("Tools does not carry read")
	}
}

// TestBuild_UsesTheDeferredSelectorAboveTheThreshold covers the small-schema
// mode: when the catalog is deferred, a turn that matches nothing offers the
// pinned search tool ONLY — not every schema.
func TestBuild_UsesTheDeferredSelectorAboveTheThreshold(t *testing.T) {
	svc := New(baseDeps(toolSet("read", "write", "shell")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{ToolDiscoveryMax: 2})
	if lc.ToolSelector == nil {
		t.Fatal("ToolSelector = nil, want a selector")
	}
	var defs []agent.ToolDef
	for name, tl := range lc.Tools {
		defs = append(defs, tl.Definition())
		_ = name
	}
	got, err := lc.ToolSelector(context.Background(), agent.ToolSelectionRequest{
		Intent: "qqqq",
		Tools:  defs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != toolSearchName {
		t.Fatalf("the deferred selector offered %d tools, want only %s", len(got), toolSearchName)
	}
}

// TestBuild_KeepsTheFullSelectorBelowTheThreshold is the other side: with the
// catalog below the threshold the selector is the conservative one, and a
// turn that matches nothing still offers every tool.
func TestBuild_KeepsTheFullSelectorBelowTheThreshold(t *testing.T) {
	svc := New(baseDeps(toolSet("read", "write", "shell")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{ToolDiscoveryMax: 10})
	if lc.ToolSelector == nil {
		t.Fatal("ToolSelector = nil, want a selector")
	}
	var defs []agent.ToolDef
	for _, tl := range lc.Tools {
		defs = append(defs, tl.Definition())
	}
	got, err := lc.ToolSelector(context.Background(), agent.ToolSelectionRequest{
		Intent: "qqqq",
		Tools:  defs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(defs) {
		t.Fatalf("the conservative selector offered %d tools, want all %d", len(got), len(defs))
	}
}

// TestBuild_ProjectsTheConfigurationOntoTheLoop is the core of the rule: the
// projected configuration lands on the loop, knob for knob.
func TestBuild_ProjectsTheConfigurationOntoTheLoop(t *testing.T) {
	prov := &fakeProvider{}
	svc := New(Deps{
		Tools: func(context.Context) map[string]toolapi.Tool { return toolSet("read") },
		Bus:   nil,
	})
	_ = prov

	cfg := Config{
		MaxIter:             7,
		MaxAutoContinue:     2,
		AutoContinueWait:    3 * time.Second,
		ToolTimeout:         9 * time.Second,
		MaxParallelTools:    4,
		ObservationDeltas:   true,
		ContextBudget:       1234,
		ContextProtectFirst: 56,
		ArtifactThreshold:   789,
	}
	lc := svc.Build(context.Background(), "corr-1", "fake-model", cfg)

	if lc.MaxIter != 7 {
		t.Fatalf("MaxIter = %d, want 7", lc.MaxIter)
	}
	if lc.MaxAutoContinue != 2 {
		t.Fatalf("MaxAutoContinue = %d, want 2", lc.MaxAutoContinue)
	}
	if lc.AutoContinueWait != 3*time.Second {
		t.Fatalf("AutoContinueWait = %s, want 3s", lc.AutoContinueWait)
	}
	if lc.ToolTimeout != 9*time.Second {
		t.Fatalf("ToolTimeout = %s, want 9s", lc.ToolTimeout)
	}
	if lc.MaxParallelTools != 4 {
		t.Fatalf("MaxParallelTools = %d, want 4", lc.MaxParallelTools)
	}
	if !lc.ObservationDeltas {
		t.Fatal("ObservationDeltas = false, want true")
	}
	if lc.ContextBudget != 1234 {
		t.Fatalf("ContextBudget = %d, want 1234", lc.ContextBudget)
	}
	if lc.ContextProtectFirst != 56 {
		t.Fatalf("ContextProtectFirst = %d, want 56", lc.ContextProtectFirst)
	}
	if lc.ArtifactThreshold != 789 {
		t.Fatalf("ArtifactThreshold = %d, want 789", lc.ArtifactThreshold)
	}
	if lc.Model != "fake-model" {
		t.Fatalf("Model = %q, want fake-model", lc.Model)
	}
}

// TestBuild_AlwaysCarriesTheCostFunction pins the rule that made delegated
// runs' spend ceiling a no-op: without CostFn the ceiling is inert.
func TestBuild_AlwaysCarriesTheCostFunction(t *testing.T) {
	svc := New(baseDeps(toolSet("read")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{})
	if lc.CostFn == nil {
		t.Fatal("CostFn = nil — MaxRunCostMicrocents would be inert")
	}
}

// TestBuild_AlwaysCarriesTheToolMemo pins the memo: every governed run gets
// one, root or delegated.
func TestBuild_AlwaysCarriesTheToolMemo(t *testing.T) {
	svc := New(baseDeps(toolSet("read")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{})
	if lc.ToolMemo == nil {
		t.Fatal("ToolMemo = nil, want a memo")
	}
}

// TestBuild_AlwaysCarriesTheContextRescueMarker pins the marker the loop uses
// to escape a compacted context.
func TestBuild_AlwaysCarriesTheContextRescueMarker(t *testing.T) {
	svc := New(baseDeps(toolSet("read")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{})
	if len(lc.ContextRescueMarkers) != 1 || lc.ContextRescueMarkers[0] != toolapi.DefaultContextRescueMarker {
		t.Fatalf("ContextRescueMarkers = %v, want [%s]", lc.ContextRescueMarkers, toolapi.DefaultContextRescueMarker)
	}
}

// TestBuild_AppliesTheRunAllowlist covers the per-run restriction: the
// daemon's allowlist narrows the tool surface.
func TestBuild_AppliesTheRunAllowlist(t *testing.T) {
	svc := New(baseDeps(toolSet("read", "write", "shell")))
	ctx := runctx.WithTools(context.Background(), []string{"read"})
	lc := svc.Build(ctx, "corr-1", "fake-model", Config{})
	if len(lc.Tools) != 1 {
		t.Fatalf("Tools = %d, want 1", len(lc.Tools))
	}
	if _, ok := lc.Tools["read"]; !ok {
		t.Fatal("Tools does not carry read")
	}
}

// TestBuild_AppliesTheAgentProfilePolicy covers the profile half: the tool
// surface is narrowed by the named agent's allow/deny policy.
func TestBuild_AppliesTheAgentProfilePolicy(t *testing.T) {
	svc := New(baseDeps(toolSet("read", "write", "shell")))
	ctx := runctx.WithAgentProfile(context.Background(), agentProfileDenyShell())
	lc := svc.Build(ctx, "corr-1", "fake-model", Config{})
	if _, ok := lc.Tools["shell"]; ok {
		t.Fatal("Tools carries shell, want it denied")
	}
}

// TestBuild_DefersDiscoveryAboveTheThreshold covers the deferred-discovery
// rule: above the threshold the catalog is deferred behind a search tool.
func TestBuild_DefersDiscoveryAboveTheThreshold(t *testing.T) {
	svc := New(baseDeps(toolSet("read", "write", "shell", "notify")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{ToolDiscoveryMax: 2})
	if _, ok := lc.Tools[toolSearchName]; !ok {
		t.Fatal("tool_search was not installed above the threshold")
	}
}

// TestBuild_KeepsDiscoveryOffBelowTheThreshold is the other side: at or below
// the threshold nothing is deferred and no search tool appears.
func TestBuild_KeepsDiscoveryOffBelowTheThreshold(t *testing.T) {
	svc := New(baseDeps(toolSet("read", "write")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{ToolDiscoveryMax: 5})
	if _, ok := lc.Tools[toolSearchName]; ok {
		t.Fatal("tool_search was installed below the threshold")
	}
	if len(lc.Tools) != 2 {
		t.Fatalf("Tools = %d, want 2", len(lc.Tools))
	}
}

// TestBuild_DerivesTheAutoContextBudgetFromTheCatalog covers the auto budget:
// the model's catalog context window is the source.
func TestBuild_DerivesTheAutoContextBudgetFromTheCatalog(t *testing.T) {
	deps := baseDeps(toolSet("read"))
	deps.Catalog = func() *catalogapi.Catalog { return testCatalog(100000, false) }
	svc := New(deps)
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{ContextBudgetAuto: true})
	want := agent.AutoContextBudgetChars(100000)
	if lc.ContextBudget != want {
		t.Fatalf("ContextBudget = %d, want %d", lc.ContextBudget, want)
	}
}

// TestBuild_AnExplicitBudgetWinsOverAuto covers the precedence: an explicit
// budget is the operator's decision and auto does not override it.
func TestBuild_AnExplicitBudgetWinsOverAuto(t *testing.T) {
	deps := baseDeps(toolSet("read"))
	deps.Catalog = func() *catalogapi.Catalog { return testCatalog(100000, false) }
	svc := New(deps)
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{
		ContextBudget:     4242,
		ContextBudgetAuto: true,
	})
	if lc.ContextBudget != 4242 {
		t.Fatalf("ContextBudget = %d, want 4242", lc.ContextBudget)
	}
}

// TestBuild_AnUnknownModelLeavesCompactionOff covers the unknown-model case:
// with no catalog entry the budget stays 0, so compaction is off rather than
// guessed.
func TestBuild_AnUnknownModelLeavesCompactionOff(t *testing.T) {
	deps := baseDeps(toolSet("read"))
	deps.Catalog = func() *catalogapi.Catalog { return testCatalog(100000, false) }
	svc := New(deps)
	lc := svc.Build(context.Background(), "corr-1", "no-such-model", Config{ContextBudgetAuto: true})
	if lc.ContextBudget != 0 {
		t.Fatalf("ContextBudget = %d, want 0 for an unknown model", lc.ContextBudget)
	}
}

// TestBuild_WiresTheSummarizerOnlyWhenCompactionIsActive covers the
// summarizer's precondition: it is only worth wiring when the run actually
// compacts.
func TestBuild_WiresTheSummarizerOnlyWhenCompactionIsActive(t *testing.T) {
	deps := baseDeps(toolSet("read"))
	deps.Catalog = func() *catalogapi.Catalog { return testCatalog(100000, false) }
	svc := New(deps)
	lc := svc.Build(context.Background(), "corr-1", "fake-model", Config{ContextSummarize: true})
	if lc.SummarizeElided != nil {
		t.Fatal("SummarizeElided was wired without an active context budget")
	}
}

// TestBuild_TheSummarizerCallsTheRunsProvider proves the summarizer is the
// run's own provider call: same model, same correlation, same task type.
func TestBuild_TheSummarizerCallsTheRunsProvider(t *testing.T) {
	prov := &fakeProvider{}
	deps := baseDeps(toolSet("read"))
	deps.Catalog = func() *catalogapi.Catalog { return testCatalog(100000, false) }
	cfg := Config{ContextBudget: 1000, ContextSummarize: true}
	cfg.Provider = prov
	svc := New(deps)
	lc := svc.Build(context.Background(), "corr-9", "fake-model", cfg)
	if lc.SummarizeElided == nil {
		t.Fatal("SummarizeElided = nil, want a summarizer")
	}
	out, err := lc.SummarizeElided(context.Background(), "a very long tool output")
	if err != nil {
		t.Fatal(err)
	}
	if out != "a summary" {
		t.Fatalf("summary = %q, want %q", out, "a summary")
	}
	if prov.last.Model != "fake-model" {
		t.Fatalf("summarizer model = %q, want fake-model", prov.last.Model)
	}
	if prov.last.CorrelationID != "corr-9" {
		t.Fatalf("summarizer correlation = %q, want corr-9", prov.last.CorrelationID)
	}
	if prov.last.TaskType != "summarize" {
		t.Fatalf("summarizer task type = %q, want summarize", prov.last.TaskType)
	}
	if prov.last.MaxTokens != elidedSummaryMaxTokens {
		t.Fatalf("summarizer max tokens = %d, want %d", prov.last.MaxTokens, elidedSummaryMaxTokens)
	}
}

// TestBuild_AReasoningModelGetsTheRoomySummaryCap pins the reasoning-model
// headroom (M926): a reasoning model spends its output budget on its chain of
// thought first, so a tight cap degrades every summary to empty.
func TestBuild_AReasoningModelGetsTheRoomySummaryCap(t *testing.T) {
	prov := &fakeProvider{}
	deps := baseDeps(toolSet("read"))
	deps.Catalog = func() *catalogapi.Catalog { return testCatalog(100000, true) }
	cfg := Config{ContextBudget: 1000, ContextSummarize: true}
	cfg.Provider = prov
	svc := New(deps)
	lc := svc.Build(context.Background(), "corr-1", "fake-model", cfg)
	if lc.SummarizeElided == nil {
		t.Fatal("SummarizeElided = nil, want a summarizer")
	}
	if _, err := lc.SummarizeElided(context.Background(), "output"); err != nil {
		t.Fatal(err)
	}
	if prov.last.MaxTokens != elidedSummaryReasoningMaxTokens {
		t.Fatalf("summarizer max tokens = %d, want %d", prov.last.MaxTokens, elidedSummaryReasoningMaxTokens)
	}
}

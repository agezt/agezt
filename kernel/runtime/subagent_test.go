// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// openSpendKernel wires a mock provider behind a real Governor so each scripted
// response (given token usage) journals a budget.consumed event — the substrate
// the M48 sub-agent spend cap reads. The governor shares the kernel's bus
// (SetBus, the daemon's pattern) so spend events land in the kernel's journal.
func openSpendKernel(t *testing.T, prov agent.Provider, spendCapMC int64) *runtime.Kernel {
	t.Helper()
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: prov.Name(), Provider: prov, AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	g, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:                    t.TempDir(),
		Provider:                   g,
		Tools:                      map[string]agent.Tool{"shell": shell.New()},
		Model:                      "claude-sonnet-4-6", // priced, so usage → non-zero spend
		SubAgentTool:               true,
		SubAgentMaxDepth:           1,
		SubAgentMaxSpendMicrocents: spendCapMC,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	g.SetBus(k.Bus()) // budget.consumed → the kernel's journal
	t.Cleanup(func() { k.Close() })
	return k
}

// withUsage stamps synthetic token usage on a response so the governor prices a
// non-zero cost. 2000/1000 tokens on claude-sonnet-4-6 = 2_100_000 microcents
// ($0.0021) per call.
func withUsage(r agent.CompletionResponse) agent.CompletionResponse {
	return mock.WithUsage(r, agent.Usage{InputTokens: 2000, OutputTokens: 1000, Model: "claude-sonnet-4-6"})
}

func TestSubAgent_SpendGuard(t *testing.T) {
	// Each sub-agent call costs $0.0021. With a $0.0030 cap the first delegation
	// (descendant spend $0) and the second ($0.0021 < $0.0030) are admitted, but
	// by the third the sub-agents have spent $0.0042 ≥ $0.0030, so it's refused —
	// the cost analogue of the fan-out guard. Two spawns; the lead still finishes.
	prov := mock.New(
		withUsage(mock.ToolUse("c1", "delegate", map[string]any{"task": "t1"})), // lead r1
		withUsage(mock.FinalText("child 1 done")),                               // child 1
		withUsage(mock.ToolUse("c2", "delegate", map[string]any{"task": "t2"})), // lead r2
		withUsage(mock.FinalText("child 2 done")),                               // child 2
		withUsage(mock.ToolUse("c3", "delegate", map[string]any{"task": "t3"})), // lead r3 — refused on spend
		withUsage(mock.FinalText("lead done")),                                  // lead final
	)
	k := openSpendKernel(t, prov, 3_000_000) // $0.0030
	col := &collector{}
	col.watch(k)

	ans, _, err := k.Run(context.Background(), "spend out")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "lead done" {
		t.Errorf("final answer = %q, want %q (lead completes despite the refusal)", ans, "lead done")
	}
	time.Sleep(50 * time.Millisecond)

	if n := len(col.ofKind(event.KindSubAgentSpawned)); n != 2 {
		t.Errorf("spend guard: expected 2 spawns (the 3rd refused on spend), got %d", n)
	}
}

func TestSubAgent_SpendUnboundedByDefault(t *testing.T) {
	// SubAgentMaxSpendMicrocents=0 keeps the historical behaviour: spend never
	// blocks a delegation. Three delegate rounds → three spawns.
	prov := mock.New(
		withUsage(mock.ToolUse("c1", "delegate", map[string]any{"task": "t1"})),
		withUsage(mock.FinalText("child 1")),
		withUsage(mock.ToolUse("c2", "delegate", map[string]any{"task": "t2"})),
		withUsage(mock.FinalText("child 2")),
		withUsage(mock.ToolUse("c3", "delegate", map[string]any{"task": "t3"})),
		withUsage(mock.FinalText("child 3")),
		withUsage(mock.FinalText("lead done")),
	)
	k := openSpendKernel(t, prov, 0) // no spend cap
	col := &collector{}
	col.watch(k)

	if _, _, err := k.Run(context.Background(), "spend out"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if n := len(col.ofKind(event.KindSubAgentSpawned)); n != 3 {
		t.Errorf("unbounded spend: expected 3 spawns, got %d", n)
	}
}

// collector drains the bus into a slice for post-run assertions.
type collector struct {
	mu sync.Mutex
	ev []*event.Event
}

func (c *collector) watch(k *runtime.Kernel) {
	sub, err := k.Bus().Subscribe(">", 1024)
	if err != nil {
		return
	}
	go func() {
		for e := range sub.C {
			c.mu.Lock()
			c.ev = append(c.ev, e)
			c.mu.Unlock()
		}
	}()
}

func (c *collector) ofKind(k event.Kind) []*event.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []*event.Event
	for _, e := range c.ev {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

func openSubAgentKernel(t *testing.T, prov agent.Provider, depth int) *runtime.Kernel {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:          t.TempDir(),
		Provider:         prov,
		Tools:            map[string]agent.Tool{"shell": shell.New()},
		SubAgentTool:     true,
		SubAgentMaxDepth: depth,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

func TestSubAgent_DelegationFlow(t *testing.T) {
	// Parent asks delegate → child answers → parent uses it.
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "find the module name"}),
		mock.FinalText("module github.com/agezt/agezt"),         // child's run
		mock.FinalText("The module is github.com/agezt/agezt."), // parent's final
	)
	k := openSubAgentKernel(t, prov, 1)

	// The delegate tool is advertised to the model.
	if _, ok := k.Tools()["delegate"]; !ok {
		t.Fatal("delegate tool should be registered when SubAgentTool is on")
	}

	col := &collector{}
	col.watch(k)

	ans, corr, err := k.Run(context.Background(), "what is this project's module?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "The module is github.com/agezt/agezt." {
		t.Errorf("final answer = %q", ans)
	}

	// Let the async bus drain.
	time.Sleep(50 * time.Millisecond)

	spawns := col.ofKind(event.KindSubAgentSpawned)
	if len(spawns) != 1 {
		t.Fatalf("expected exactly 1 subagent.spawned, got %d", len(spawns))
	}
	// The spawn is linked to the PARENT correlation (so `agt why <parent>`
	// surfaces it) and carries the child correlation to drill into.
	sp := spawns[0]
	if sp.CorrelationID != corr {
		t.Errorf("spawn correlation = %q, want parent %q", sp.CorrelationID, corr)
	}
	var pl struct {
		Task             string `json:"task"`
		ChildCorrelation string `json:"child_correlation"`
		Depth            int    `json:"depth"`
	}
	if err := json.Unmarshal(sp.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if pl.Task != "find the module name" {
		t.Errorf("spawn task = %q", pl.Task)
	}
	if pl.ChildCorrelation == "" || pl.ChildCorrelation == corr {
		t.Errorf("child_correlation = %q (must be a distinct fresh id)", pl.ChildCorrelation)
	}
	if pl.Depth != 1 {
		t.Errorf("spawn depth = %d, want 1", pl.Depth)
	}

	// The child correlation has its own task arc (drill-down works).
	childHasTask := false
	for _, e := range col.ofKind(event.KindTaskReceived) {
		if e.CorrelationID == pl.ChildCorrelation {
			childHasTask = true
		}
	}
	if !childHasTask {
		t.Error("child correlation should have its own task.received event")
	}
}

func TestSubAgent_DepthGuard(t *testing.T) {
	// maxDepth=1: the child's own attempt to delegate must be refused, so only
	// ONE spawn is ever journaled. The child still completes (the failed
	// delegation surfaces as a tool error it works around).
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "level 1"}), // parent delegates
		mock.ToolUse("c2", "delegate", map[string]any{"task": "level 2"}), // child tries to delegate
		mock.FinalText("child done"),                                      // child's final after the refusal
		mock.FinalText("parent done"),                                     // parent's final
	)
	k := openSubAgentKernel(t, prov, 1)
	col := &collector{}
	col.watch(k)

	ans, _, err := k.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "parent done" {
		t.Errorf("final answer = %q", ans)
	}
	time.Sleep(50 * time.Millisecond)

	if n := len(col.ofKind(event.KindSubAgentSpawned)); n != 1 {
		t.Errorf("depth guard: expected 1 spawn (the level-2 attempt refused), got %d", n)
	}
}

func TestSubAgent_FanoutGuard(t *testing.T) {
	// maxFanout=2: the lead delegates three times in three rounds, but only the
	// first TWO spawn a sub-agent — the third is refused with a tool error the
	// lead works around to still complete. Depth caps nesting; fan-out caps
	// breadth, independently. The script is consumed in call order: each lead
	// round produces one response, each spawned child consumes the next.
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "t1"}), // lead round 1
		mock.FinalText("child 1 done"),                               // child 1
		mock.ToolUse("c2", "delegate", map[string]any{"task": "t2"}), // lead round 2
		mock.FinalText("child 2 done"),                               // child 2
		mock.ToolUse("c3", "delegate", map[string]any{"task": "t3"}), // lead round 3 — refused, no child runs
		mock.FinalText("lead done"),                                  // lead final after the refusal
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:           t.TempDir(),
		Provider:          prov,
		Tools:             map[string]agent.Tool{"shell": shell.New()},
		SubAgentTool:      true,
		SubAgentMaxDepth:  1,
		SubAgentMaxFanout: 2,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	col := &collector{}
	col.watch(k)

	ans, _, err := k.Run(context.Background(), "fan out")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "lead done" {
		t.Errorf("final answer = %q, want %q (lead completes despite the refusal)", ans, "lead done")
	}
	time.Sleep(50 * time.Millisecond)

	if n := len(col.ofKind(event.KindSubAgentSpawned)); n != 2 {
		t.Errorf("fan-out guard: expected 2 spawns (the 3rd refused), got %d", n)
	}
}

func TestSubAgent_FanoutUnboundedByDefault(t *testing.T) {
	// SubAgentMaxFanout=0 (the default) keeps the historical behaviour: a lead
	// may spawn as many sub-agents as it asks for. Three delegate rounds → three
	// spawns, none refused.
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "t1"}),
		mock.FinalText("child 1"),
		mock.ToolUse("c2", "delegate", map[string]any{"task": "t2"}),
		mock.FinalText("child 2"),
		mock.ToolUse("c3", "delegate", map[string]any{"task": "t3"}),
		mock.FinalText("child 3"),
		mock.FinalText("lead done"),
	)
	k := openSubAgentKernel(t, prov, 1) // no fan-out cap
	col := &collector{}
	col.watch(k)

	if _, _, err := k.Run(context.Background(), "fan out"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if n := len(col.ofKind(event.KindSubAgentSpawned)); n != 3 {
		t.Errorf("unbounded fan-out: expected 3 spawns, got %d", n)
	}
}

func TestWithModel_OverridesPerRun(t *testing.T) {
	// A run started with runtime.WithModel makes the provider see that model in
	// CompletionRequest.Model — the basis for per-request model selection.
	prov := mock.New(mock.FinalText("ok"))
	var sawModel string
	prov.OnRequest = func(req agent.CompletionRequest) { sawModel = req.Model }

	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "default-model",
		Tools:    map[string]agent.Tool{"shell": shell.New()},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithModel(context.Background(), "requested-model")
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "hi"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if sawModel != "requested-model" {
		t.Errorf("provider saw model %q, want the per-run override", sawModel)
	}

	// Without an override, the configured default is used.
	sawModel = ""
	prov2 := mock.New(mock.FinalText("ok"))
	prov2.OnRequest = func(req agent.CompletionRequest) { sawModel = req.Model }
	k2, _ := runtime.Open(runtime.Config{
		BaseDir: t.TempDir(), Provider: prov2, Model: "default-model",
		Tools: map[string]agent.Tool{"shell": shell.New()},
	})
	t.Cleanup(func() { k2.Close() })
	if _, err := k2.RunWith(context.Background(), k2.NewCorrelation(), "hi"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if sawModel != "default-model" {
		t.Errorf("no override → provider should see default-model, got %q", sawModel)
	}
}

func TestSubAgent_DisabledByDefault(t *testing.T) {
	// Without SubAgentTool, the delegate tool is absent.
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{"shell": shell.New()},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	if _, ok := k.Tools()["delegate"]; ok {
		t.Error("delegate tool must not be present unless SubAgentTool is set")
	}
}

// openSpendKernelDepth is openSpendKernel with a caller-chosen max depth, so a
// sub-agent can itself delegate (depth >= 2) — needed to exercise the TRANSITIVE
// spend accounting (a grandchild's spend counting toward an ancestor's cap).
func openSpendKernelDepth(t *testing.T, prov agent.Provider, spendCapMC int64, depth int) *runtime.Kernel {
	t.Helper()
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: prov.Name(), Provider: prov, AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	g, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:                    t.TempDir(),
		Provider:                   g,
		Tools:                      map[string]agent.Tool{"shell": shell.New()},
		Model:                      "claude-sonnet-4-6",
		SubAgentTool:               true,
		SubAgentMaxDepth:           depth,
		SubAgentMaxSpendMicrocents: spendCapMC,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	g.SetBus(k.Bus())
	t.Cleanup(func() { k.Close() })
	return k
}

// TestSubAgent_SpendGuardCountsTransitiveDescendants proves the M48 spend cap
// sums spend over the FULL descendant tree, not just direct children — so the
// cap can't be evaded by nesting delegation one level deeper. Structure (depth
// 2): lead → child → grandchild. Each LLM call costs $0.0021.
//
//	child:      delegate-decision + final  = 2 calls = $0.0042
//	grandchild: final                       = 1 call  = $0.0021
//	lead's descendant total at its 2nd delegate = $0.0063
//
// With a $0.0050 cap the lead's 2nd delegation is REFUSED because the transitive
// total ($0.0063) ≥ cap. A buggy direct-children-only sum ($0.0042 < $0.0050)
// would have ADMITTED it — so the spawn count distinguishes correct from buggy:
// correct → 2 spawns (child + grandchild), buggy → 3.
func TestSubAgent_SpendGuardCountsTransitiveDescendants(t *testing.T) {
	prov := mock.New(
		withUsage(mock.ToolUse("c1", "delegate", map[string]any{"task": "t1"})),  // lead r1 → child
		withUsage(mock.ToolUse("g1", "delegate", map[string]any{"task": "t1a"})), // child r1 → grandchild
		withUsage(mock.FinalText("grandchild done")),                             // grandchild
		withUsage(mock.FinalText("child done")),                                  // child r2
		withUsage(mock.ToolUse("c2", "delegate", map[string]any{"task": "t2"})),  // lead r2 → REFUSED on transitive spend
		withUsage(mock.FinalText("lead done")),                                   // lead final
	)
	k := openSpendKernelDepth(t, prov, 5_000_000, 2) // $0.0050 cap, depth 2
	col := &collector{}
	col.watch(k)

	ans, _, err := k.Run(context.Background(), "nested spend")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "lead done" {
		t.Errorf("final answer = %q, want %q", ans, "lead done")
	}
	time.Sleep(50 * time.Millisecond)

	if n := len(col.ofKind(event.KindSubAgentSpawned)); n != 2 {
		t.Errorf("transitive spend guard: expected 2 spawns (child+grandchild; lead's 2nd "+
			"delegate refused once grandchild spend is counted), got %d", n)
	}
}

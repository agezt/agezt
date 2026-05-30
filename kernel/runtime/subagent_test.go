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
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

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

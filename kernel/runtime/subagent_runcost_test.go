// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDelegate_NamedAgentPerRunCostCeilingEnforced — a named agent's MaxCostMc
// (the documented per-run spend ceiling) must terminate a DELEGATED run that
// exceeds it. Regression for LD-1: executeSubAgent hand-built its own
// LoopConfig and never set CostFn, which per the LoopConfig contract makes
// MaxRunCostMicrocents inert — so the ceiling silently only applied to root
// runs. The shared buildLoopConfig now wires cost accounting for both.
func TestDelegate_NamedAgentPerRunCostCeilingEnforced(t *testing.T) {
	prov := mock.New(
		testToolUse("c1", "delegate", map[string]any{"task": "expensive work", "agent": "spender"}),
		// Child's one response: 2000/1000 tokens on the priced model =
		// 2_100_000 microcents, far above the 1000-mc ceiling → the child run
		// must fail with reason=cost_budget instead of returning the answer.
		withUsage(mock.FinalText("expensive answer")),
		mock.FinalText("wrapped up"), // parent absorbs the failed delegation
	)
	k := openSpendKernel(t, prov, 0) // 0 = no tree-wide spend cap; the per-run ceiling is under test

	if _, err := k.AddProfile(roster.Profile{
		Slug:      "spender",
		MaxCostMc: 1000, // $0.000001-scale ceiling: any priced usage exceeds it
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	col := &collector{}
	col.watch(k)

	if _, _, err := k.Run(context.Background(), "do the expensive thing"); err != nil {
		t.Fatalf("parent Run: %v", err)
	}

	// Let the async bus drain.
	time.Sleep(50 * time.Millisecond)

	// The child correlation must carry task.failed(reason=cost_budget).
	spawns := col.ofKind(event.KindSubAgentSpawned)
	if len(spawns) != 1 {
		t.Fatalf("expected 1 subagent.spawned, got %d", len(spawns))
	}
	var sp struct {
		ChildCorrelation string `json:"child_correlation"`
	}
	if err := json.Unmarshal(spawns[0].Payload, &sp); err != nil {
		t.Fatal(err)
	}
	childFailedReason := ""
	childCompleted := false
	for _, e := range col.ofKind(event.KindTaskFailed) {
		if e.CorrelationID == sp.ChildCorrelation {
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			childFailedReason = p.Reason
		}
	}
	for _, e := range col.ofKind(event.KindTaskCompleted) {
		if e.CorrelationID == sp.ChildCorrelation {
			childCompleted = true
		}
	}
	if childCompleted {
		t.Error("over-ceiling delegated run must not journal task.completed")
	}
	if childFailedReason != "cost_budget" {
		t.Errorf("child task.failed reason = %q, want cost_budget (per-run ceiling inert for delegations?)", childFailedReason)
	}
}

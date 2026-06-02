// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRun_PerRunCostCap_EndToEnd — `agt run --max-cost` enforced through the real
// control plane: a run billing against a priced model past its cap fails with the
// cost-budget reason, and an uncapped run completes (M166). This exercises the full
// wire: handleRun parses max_cost → runtime.WithMaxCost → agent.Run's cap →
// governor.CostMicrocents pricing.
func TestRun_PerRunCostCap_EndToEnd(t *testing.T) {
	// The mock bills 100k input tokens under a catalog-priced model
	// (claude-sonnet-4-6 at the fallback price = 300M microcents/MTok → 0.1 MTok ≈
	// 30M microcents ≈ $0.03), comfortably over the $0.01 cap below.
	mkProv := func() agent.Provider {
		return mock.New(mock.WithUsage(mock.FinalText("done"),
			agent.Usage{Model: "claude-sonnet-4-6", InputTokens: 100_000, OutputTokens: 0}))
	}

	t.Run("over cap → cost_budget failure", func(t *testing.T) {
		k, _, c, _ := startPair(t, mkProv())
		_, err := c.Stream(context.Background(), controlplane.CmdRun,
			map[string]any{"intent": "expensive", "max_cost": float64(10_000_000)}, // $0.01
			func(*event.Event) {})
		if err == nil {
			t.Fatal("expected a cost-budget failure, got nil")
		}
		if !strings.Contains(err.Error(), "cost budget") {
			t.Errorf("error = %q; want it to mention the cost budget", err.Error())
		}
		// Terminal event is task.failed(reason=cost_budget).
		var reason string
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Kind == event.KindTaskFailed {
				if strings.Contains(string(e.Payload), `"cost_budget"`) {
					reason = "cost_budget"
				}
			}
			return nil
		})
		if reason != "cost_budget" {
			t.Errorf("no task.failed(reason=cost_budget) journaled")
		}
	})

	t.Run("uncapped run completes", func(t *testing.T) {
		_, _, c, _ := startPair(t, mkProv())
		res, err := c.Stream(context.Background(), controlplane.CmdRun,
			map[string]any{"intent": "expensive but uncapped"}, func(*event.Event) {})
		if err != nil {
			t.Fatalf("uncapped run errored: %v", err)
		}
		if res["answer"] != "done" {
			t.Errorf("answer = %v want done", res["answer"])
		}
	})

	t.Run("malformed max_cost is a usage error", func(t *testing.T) {
		_, _, c, _ := startPair(t, mkProv())
		_, err := c.Stream(context.Background(), controlplane.CmdRun,
			map[string]any{"intent": "x", "max_cost": "lots"}, func(*event.Event) {})
		if err == nil || !strings.Contains(err.Error(), "max_cost must be a number") {
			t.Errorf("err = %v; want a max_cost type error", err)
		}
	})
}

// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// sumCost is a stand-in CostFn: 1 microcent per token. Deterministic and
// independent of the real price table, so the cap math in the test is obvious.
func sumCost(_ string, in, out int) int64 { return int64(in + out) }

// TestRun_PerRunCostCap_Terminates — a run whose first call's spend reaches the
// cap stops with ErrRunBudgetExceeded and journals task.failed(reason=cost_budget)
// instead of returning the answer (M166).
func TestRun_PerRunCostCap_Terminates(t *testing.T) {
	b, j := newTestBus(t)
	// One scripted final response carrying 2000 tokens of usage.
	prov := mock.New(mock.WithUsage(mock.FinalText("done"),
		agent.Usage{InputTokens: 1200, OutputTokens: 800, Model: "mock"}))

	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "agent-1", CorrelationID: "corr-cap",
		MaxRunCostMicrocents: 1500, // 2000 > 1500 → exceeded on the first call
		CostFn:               sumCost,
	}, "spendy task")
	if !errors.Is(err, agent.ErrRunBudgetExceeded) {
		t.Fatalf("err = %v, want ErrRunBudgetExceeded", err)
	}

	// The terminal event is task.failed(reason=cost_budget), and there is no
	// task.completed.
	var failedReason string
	var completed bool
	_ = j.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindTaskFailed:
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			failedReason = p.Reason
		case event.KindTaskCompleted:
			completed = true
		}
		return nil
	})
	if completed {
		t.Error("a capped run must not journal task.completed")
	}
	if failedReason != "cost_budget" {
		t.Errorf("task.failed reason = %q, want cost_budget", failedReason)
	}
}

// TestRun_PerRunCostCap_UnderBudget — a run that stays under the cap completes
// normally (M166).
func TestRun_PerRunCostCap_UnderBudget(t *testing.T) {
	b, _ := newTestBus(t)
	prov := mock.New(mock.WithUsage(mock.FinalText("ok"),
		agent.Usage{InputTokens: 100, OutputTokens: 50, Model: "mock"}))

	ans, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "agent-1", CorrelationID: "corr-under",
		MaxRunCostMicrocents: 10_000, // 150 << 10000
		CostFn:               sumCost,
	}, "cheap task")
	if err != nil {
		t.Fatalf("under-budget run errored: %v", err)
	}
	if ans != "ok" {
		t.Errorf("answer = %q want ok", ans)
	}
}

// TestRun_PerRunCostCap_InertWithoutCostFn — the cap is inert when no CostFn is
// wired (or the cap is 0): the run completes regardless of usage (M166).
func TestRun_PerRunCostCap_InertWithoutCostFn(t *testing.T) {
	b, _ := newTestBus(t)
	mk := func() agent.Provider {
		return mock.New(mock.WithUsage(mock.FinalText("ok"),
			agent.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, Model: "mock"}))
	}

	// Cap set, but CostFn nil → inert.
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: mk(), Bus: b, Actor: "a", CorrelationID: "c1",
		MaxRunCostMicrocents: 1, CostFn: nil,
	}, "x"); err != nil {
		t.Fatalf("nil CostFn should disable the cap, got %v", err)
	}
	// CostFn set, but cap 0 → inert.
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: mk(), Bus: b, Actor: "a", CorrelationID: "c2",
		MaxRunCostMicrocents: 0, CostFn: sumCost,
	}, "x"); err != nil {
		t.Fatalf("zero cap should disable the cap, got %v", err)
	}
}

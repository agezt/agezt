// SPDX-License-Identifier: MIT

package planner_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/planner"
)

// fakeEstimator is a CostEstimator that returns input+output*2
// microcents, used to verify the math wiring without coupling to
// the real price table.
type fakeEstimator struct{}

func (fakeEstimator) CostMicrocents(model string, in, out int) int64 {
	return int64(in) + int64(out)*2
}

// TestEstimateCost_LoopAndGate verifies loop nodes contribute the
// per-node token assumption while gates are free.
func TestEstimateCost_LoopAndGate(t *testing.T) {
	plan := planner.Plan{
		Nodes: []planner.Node{
			{ID: "a", Kind: "loop"},
			{ID: "b", Kind: "gate"},
			{ID: "c", Kind: "loop"},
		},
	}
	est, err := planner.EstimateCost(plan, "x", fakeEstimator{})
	if err != nil {
		t.Fatalf("EstimateCost: %v", err)
	}
	expectedLoop := int64(planner.PerNodeInputTokens) + int64(planner.PerNodeOutputTokens)*2
	if got := est.Nodes[0].Microcents; got != expectedLoop {
		t.Errorf("loop a microcents = %d, want %d", got, expectedLoop)
	}
	if got := est.Nodes[1].Microcents; got != 0 {
		t.Errorf("gate b microcents = %d, want 0", got)
	}
	if got, want := est.TotalMicrocents, 2*expectedLoop; got != want {
		t.Errorf("total = %d, want %d (2 loops only)", got, want)
	}
}

// TestEstimateCost_EmptyPlanErrors: zero nodes returns an error
// rather than a misleading $0.00.
func TestEstimateCost_EmptyPlanErrors(t *testing.T) {
	_, err := planner.EstimateCost(planner.Plan{}, "x", fakeEstimator{})
	if err == nil {
		t.Error("expected error on empty plan")
	}
}

// TestEstimateCost_NilEstimatorErrors: missing estimator → error
// rather than panic.
func TestEstimateCost_NilEstimatorErrors(t *testing.T) {
	plan := planner.Plan{Nodes: []planner.Node{{ID: "a", Kind: "loop"}}}
	_, err := planner.EstimateCost(plan, "x", nil)
	if err == nil {
		t.Error("expected error on nil estimator")
	}
}

// TestFormatUSD covers the rendering edges.
func TestFormatUSD(t *testing.T) {
	cases := []struct {
		mc   int64
		want string
	}{
		{0, "$0.0000"},
		{1_000_000_000, "$1.0000"}, // $1 = 1e9 microcents
		{500_000_000, "$0.5000"},   // 50¢
		{1_234_500_000, "$1.2345"}, // $1.2345
		{100_000, "$0.0001"},       // 0.01¢ rounded to display
	}
	for _, c := range cases {
		if got := planner.FormatUSD(c.mc); got != c.want {
			t.Errorf("FormatUSD(%d) = %q, want %q", c.mc, got, c.want)
		}
	}
}

// TestEstimateCost_UnknownKindIsFree: an unknown node kind (forward
// compat — e.g. when the planner gains a new kind not yet in the
// estimator's switch) is charged at zero rather than crashing.
func TestEstimateCost_UnknownKindIsFree(t *testing.T) {
	plan := planner.Plan{Nodes: []planner.Node{{ID: "a", Kind: "future-kind"}}}
	est, err := planner.EstimateCost(plan, "x", fakeEstimator{})
	if err != nil {
		t.Fatalf("EstimateCost: %v", err)
	}
	if est.TotalMicrocents != 0 {
		t.Errorf("unknown kind contributed %d microcents, want 0", est.TotalMicrocents)
	}
	if !strings.EqualFold(est.Nodes[0].Kind, "future-kind") {
		t.Errorf("kind not preserved in breakdown: %q", est.Nodes[0].Kind)
	}
}

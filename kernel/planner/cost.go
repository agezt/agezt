// SPDX-License-Identifier: MIT

package planner

import (
	"fmt"
	"strings"
)

// CostEstimate is the rough per-plan budget projection (M1.oo).
// Operators see this BEFORE executing a generated plan so they can
// say "no, that's $40 to run a search-and-summarize" before the
// daemon actually spends.
//
// **Honest naming.** "Estimate" not "calculation" — token counts
// are guesses based on per-node assumptions, not real
// measurements. The numbers are accurate to one significant figure
// at best, useful for "is this $0.02 or $40" sanity checking.
type CostEstimate struct {
	// TotalMicrocents is the sum of per-node estimates in
	// integer microcents (DECISIONS C1 — no float drift in the
	// kernel's money math).
	TotalMicrocents int64
	// Model is the model id assumed for every loop node.
	Model string
	// Nodes are per-node breakdowns in declaration order.
	Nodes []NodeCostEstimate
}

// NodeCostEstimate is one row of the breakdown.
type NodeCostEstimate struct {
	ID                  string
	Kind                string
	AssumedInputTokens  int
	AssumedOutputTokens int
	Microcents          int64
}

// PerNodeInputTokens is the rough average input size per loop
// node. Picked to roughly match a small task: system prompt (~500
// tokens) + intent (~50 tokens) + a handful of tool results
// echoing back (~2500 tokens) ≈ 3000.
const PerNodeInputTokens = 3000

// PerNodeOutputTokens is the rough average completion size per
// loop node. Picked at 1000 — enough headroom for "summarise X
// in 3 paragraphs" without assuming the model goes long.
const PerNodeOutputTokens = 1000

// CostEstimator is implemented by the Governor's pricing layer
// (or anything else that can map model+tokens → microcents).
// Planner imports its concrete cost function via this interface
// rather than reaching across to kernel/governor; keeps the layer
// boundary clean.
type CostEstimator interface {
	CostMicrocents(model string, inputTokens, outputTokens int) int64
}

// EstimateCost projects what running plan against the given model
// would roughly cost. Gate nodes are scored at zero (gates don't
// invoke the LLM — they're scheduler-only control flow). Loop
// nodes get the PerNodeInputTokens / PerNodeOutputTokens
// assumption and the model's per-token price.
//
// Returns ErrEmptyPlan when plan has no nodes.
func EstimateCost(plan Plan, model string, estimator CostEstimator) (CostEstimate, error) {
	if len(plan.Nodes) == 0 {
		return CostEstimate{}, fmt.Errorf("planner: estimate cost: empty plan")
	}
	if estimator == nil {
		return CostEstimate{}, fmt.Errorf("planner: estimate cost: nil estimator")
	}
	out := CostEstimate{Model: model, Nodes: make([]NodeCostEstimate, 0, len(plan.Nodes))}
	for _, n := range plan.Nodes {
		var in, outTok int
		switch strings.ToLower(n.Kind) {
		case "loop":
			in, outTok = PerNodeInputTokens, PerNodeOutputTokens
		case "gate":
			// Gates don't call the LLM — zero cost.
		default:
			// Unknown kind: don't guess.
		}
		mc := estimator.CostMicrocents(model, in, outTok)
		out.Nodes = append(out.Nodes, NodeCostEstimate{
			ID:                  n.ID,
			Kind:                n.Kind,
			AssumedInputTokens:  in,
			AssumedOutputTokens: outTok,
			Microcents:          mc,
		})
		out.TotalMicrocents += mc
	}
	return out, nil
}

// FormatUSD renders microcents as "$0.0123" with 4 decimal places
// — enough to show sub-cent estimates without exposing the integer
// microcents internals.
func FormatUSD(microcents int64) string {
	// 1 USD = 100 cents = 100 * 10_000_000 microcents = 1e9 microcents.
	// Take the sign once, up front: a sub-dollar negative (|amount| < $1) has
	// whole == 0, so the sign lives only in the fractional part — abs-ing that
	// without recording the sign would drop the leading '-' and print a negative
	// amount as positive.
	sign := ""
	if microcents < 0 {
		sign = "-"
		microcents = -microcents
	}
	whole := microcents / 1_000_000_000
	// 4-decimal display: divide the sub-dollar remainder by 100_000 to get
	// ten-thousandths of a dollar.
	dec := (microcents % 1_000_000_000) / 100_000
	return fmt.Sprintf("$%s%d.%04d", sign, whole, dec)
}

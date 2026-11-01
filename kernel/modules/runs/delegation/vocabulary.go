// SPDX-License-Identifier: MIT

package delegation

// Provenance: SPDX-License-Identifier: MIT kernel/delegation shared sub-agent
//             vocabulary: the depth context key, the sub-agent system prompt
//             and the payload helpers. The delegate / delegate_await TOOLS
//             live in this module now (tool.go); the unwired duplicates of
//             them that sat in kernel/delegation (SubAgentTool,
//             SubAgentAwaitTool, Prep, SpawnHandle) were removed in 2026-10
//             (architecture W0.5).
//
// The vocabulary moved from kernel/delegation to the runs module in W3.76f:
// the package was imported by exactly the delegation family it belonged to,
// and a vocabulary package outside the module tree was a split the programme
// had to undo anyway.

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
)

// DepthKey is a context key for tracking sub-agent nesting depth.
type DepthKey struct{}

// SystemPrompt is the system message used for all sub-agent runs.
const SystemPrompt = "You are a focused sub-agent spawned to complete ONE delegated task. " +
	"Work autonomously with the tools available, then report a concise, self-contained " +
	"result the lead agent can use directly. Do not ask clarifying questions; make a " +
	"reasonable assumption and state it."

// DepthFromCtx returns the sub-agent nesting depth carried by ctx (0 at the
// top-level lead).
func DepthFromCtx(ctx context.Context) int {
	if v, ok := ctx.Value(DepthKey{}).(int); ok {
		return v
	}
	return 0
}

// WithDepth returns a child context carrying the given nesting depth.
func WithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, DepthKey{}, depth)
}

// SpawnLink reads the child/parent correlation pair out of a
// subagent.spawned payload.
func SpawnLink(payload json.RawMessage) (child, parent string) {
	var ev struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if json.Unmarshal(payload, &ev) == nil {
		return ev.Child, ev.Parent
	}
	return "", ""
}

// BudgetCostMicrocents reads the cost (in microcents) out of a
// budget.consumed payload.
func BudgetCostMicrocents(payload json.RawMessage) int64 {
	var ev struct {
		CostMicrocents int64 `json:"cost_microcents"`
	}
	if json.Unmarshal(payload, &ev) == nil {
		return ev.CostMicrocents
	}
	return 0
}

// KeyedModelChain restricts a model chain to the models the daemon can
// actually route (M838): an unkeyed model would fail mid-delegation, so it is
// dropped; when nothing keyed remains, the daemon default takes its place.
func KeyedModelChain(subModel string, modelChain []string, avail func(string) bool, def string) (string, []string) {
	chain := []string{}
	if subModel != "" {
		chain = append(chain, subModel)
	}
	for _, m := range modelChain {
		if m != "" && !slices.Contains(chain, m) {
			chain = append(chain, m)
		}
	}
	kept := make([]string, 0, len(chain))
	for _, m := range chain {
		if avail(m) {
			kept = append(kept, m)
		}
	}
	if len(kept) == 0 {
		if d := strings.TrimSpace(def); d != "" {
			kept = append(kept, d)
		}
	}
	if len(kept) == 0 {
		return subModel, modelChain // nothing to do; keep originals
	}
	if len(kept) == 1 {
		return kept[0], nil
	}
	return kept[0], kept
}

// AppendUniqueStrings appends the values to in, skipping the ones already
// present.
func AppendUniqueStrings(in []string, values ...string) []string {
	for _, v := range values {
		in = AppendUniqueString(in, v)
	}
	return in
}

// AppendUniqueString appends the value to in unless it is already there.
func AppendUniqueString(in []string, value string) []string {
	if slices.Contains(in, value) {
		return in
	}
	return append(in, value)
}

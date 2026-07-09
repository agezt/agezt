// SPDX-License-Identifier: MIT

// Package workflowexec provides workflow graph execution: the RunWorkflow
// adapter and per-node dispatch. It is extracted from kernel/runtime to
// narrow the composition root and make workflow execution independently
// testable.
package workflowexec

import "encoding/json"

// StepCap is a defense-in-depth bound on executed steps per run —
// validation already rejects cycles, so this can only fire on an engine bug.
const StepCap = 256

// SnippetMax bounds the per-node data snippet journaled with each
// workflow.node event — enough to inspect, never enough to bloat the chain.
const SnippetMax = 2000

// MaxSubflowDepth is the maximum nesting depth for subworkflow nodes.
const MaxSubflowDepth = 3

// DepthKey is a context key for subworkflow nesting depth.
type DepthKey struct{}

// Result carries one run's outcome: per-node outputs (by node id)
// and the ordered list of executed node ids.
type Result struct {
	Outputs  map[string]any
	Executed []string
}

// Snippet renders a value for the journal: strings verbatim,
// everything else compact JSON, truncated at SnippetMax runes.
// Returns (text, truncated) where truncated is true when clipping occurred.
func Snippet(v any) (string, bool) {
	var s string
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		s = t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", false
		}
		s = string(b)
	}
	if r := []rune(s); len(r) > SnippetMax {
		return string(r[:SnippetMax]) + "…", true
	}
	return s, false
}

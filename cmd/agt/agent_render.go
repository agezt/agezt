// SPDX-License-Identifier: MIT
//
// cmd/agt agent renderers + helpers (impact summary printing,
// format shims str/intNumber). Split from agent.go during Day 211
// god-file refactor (#41, #53). Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/cmd/agt/format"
)

func printAgentImpactSummary(w io.Writer, summary map[string]any) bool {
	sections := []struct {
		key   string
		label string
	}{
		{"standing_orders", "standing orders"},
		{"schedules", "schedules"},
		{"memories", "private memory"},
		{"authored_shared_memories", "authored shared memory"},
		{"skills", "private skills"},
		{"configs", "agent config"},
		{"workspaces", "workspace"},
		{"subagents", "dependent sub-agents"},
		{"subagent_standing_orders", "sub-agent standing orders"},
		{"subagent_schedules", "sub-agent schedules"},
		{"subagent_memories", "sub-agent private memory"},
		{"subagent_authored_shared_memories", "sub-agent authored shared memory"},
		{"subagent_skills", "sub-agent skills"},
		{"subagent_configs", "sub-agent config"},
		{"subagent_workspaces", "sub-agent workspace"},
	}
	printed := false
	for _, sec := range sections {
		items := stringsAny(summary[sec.key])
		if len(items) == 0 {
			continue
		}
		if !printed {
			fmt.Fprintln(w, "impact:")
			printed = true
		}
		printImpactItems(w, sec.label, items)
	}
	return printed
}
func printImpactItems(w io.Writer, label string, items []any) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s (%d):\n", label, len(items))
	for _, item := range items {
		fmt.Fprintf(w, "    - %s\n", str(item))
	}
}
func stringsAny(v any) []any {
	switch xs := v.(type) {
	case []any:
		return xs
	case []string:
		out := make([]any, 0, len(xs))
		for _, x := range xs {
			out = append(out, x)
		}
		return out
	default:
		return nil
	}
}
func str(v any) string       { return format.Str(v) }
func intNumber(v any) int    { return format.Number(v) }

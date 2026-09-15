// SPDX-License-Identifier: MIT
//
// cmd/agt agent printing helpers (printAgentPolicy, emptyJSONValue,
// joinAnyStrings, padValue, printAgentLifecycle, printAgentTaskSummary).
// Extracted from agent_render.go during Day 211 god-file refactor (#53).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strings"
)

func printAgentPolicy(w io.Writer, label string, raw any) {
	pol, ok := raw.(map[string]any)
	if !ok || len(pol) == 0 {
		return
	}
	var parts []string
	for _, key := range []string{
		"max_attempts", "backoff", "base_delay_sec", "max_delay_sec", "retry_on",
		"doctor_agent", "stale_after_sec", "failure_window", "failure_threshold",
		"enabled", "escalate_to",
	} {
		v, ok := pol[key]
		if !ok || emptyJSONValue(v) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, v))
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "%-13s %s\n", label+":", strings.Join(parts, " "))
	}
}
func emptyJSONValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case float64:
		return x == 0
	case int:
		return x == 0
	case bool:
		return false
	case []any:
		return len(x) == 0
	default:
		return false
	}
}
func joinAnyStrings(values []any, sep string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		if s := str(v); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, sep)
}
func padValue(s string) string {
	if s == "" {
		return ""
	}
	return " " + s
}
func printAgentLifecycle(w io.Writer, raw any) {
	life, ok := raw.(map[string]any)
	if !ok || len(life) == 0 {
		return
	}
	var parts []string
	for _, key := range []string{"mode", "retire_on_complete", "max_cycles", "completed_cycles"} {
		if v, ok := life[key]; ok && !emptyJSONValue(v) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, v))
		}
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "lifecycle:    %s\n", strings.Join(parts, " "))
	}
}
func printAgentTaskSummary(w io.Writer, raw any) {
	tasks, ok := raw.([]any)
	if !ok || len(tasks) == 0 {
		return
	}
	cycle, total := 0, 0
	for _, rawTask := range tasks {
		task, _ := rawTask.(map[string]any)
		if str(task["scope"]) == "cycle" {
			cycle++
		} else {
			total++
		}
	}
	fmt.Fprintf(w, "tasklist:     %d cycle, %d total\n", cycle, total)
}

// SPDX-License-Identifier: MIT
//
// cmd/agt agent renderers + helpers (impact summary printing, policy
// flag application, profile/policy printing, lifecycle/tasklist rendering,
// and the small format.* shims str/intNumber). Split from agent.go during
// Day 211 god-file refactor (#41). Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/agezt/agezt/cmd/agt/format"
	"github.com/agezt/agezt/internal/brand"
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

// str / intNumber are shims → format.Str / format.Number (Day 8 extraction).
func str(v any) string       { return format.Str(v) }
func intNumber(v any) int    { return format.Number(v) }

func parseNonNegativeFlag(args []string, i *int, flag string, stderr io.Writer, cmd string) (int, bool) {
	if *i+1 >= len(args) {
		fmt.Fprintf(stderr, "%s agent %s: %s needs a value\n", brand.CLI, cmd, flag)
		return 0, false
	}
	*i = *i + 1
	n, err := strconv.Atoi(args[*i])
	if err != nil || n < 0 {
		fmt.Fprintf(stderr, "%s agent %s: invalid %s %q (want a non-negative integer)\n", brand.CLI, cmd, flag, args[*i])
		return 0, false
	}
	return n, true
}

func applyAgentPolicyFlags(profile map[string]any, f agentFlags) {
	if f.set["retry_attempts"] || f.set["retry_backoff"] || f.set["retry_base_sec"] || f.set["retry_max_sec"] || f.set["retry_on"] {
		pol := objectMap(profile["retry_policy"])
		if f.set["retry_attempts"] {
			pol["max_attempts"] = f.retryAttempts
		}
		if f.set["retry_backoff"] {
			pol["backoff"] = f.retryBackoff
		}
		if f.set["retry_base_sec"] {
			pol["base_delay_sec"] = f.retryBaseSec
		}
		if f.set["retry_max_sec"] {
			pol["max_delay_sec"] = f.retryMaxSec
		}
		if f.set["retry_on"] {
			pol["retry_on"] = stringList(f.retryOn)
		}
		profile["retry_policy"] = pol
	}
	if f.set["doctor_agent"] || f.set["health_stale_sec"] || f.set["health_window"] || f.set["health_threshold"] {
		pol := objectMap(profile["health_policy"])
		if f.set["doctor_agent"] {
			pol["doctor_agent"] = f.doctorAgent
		}
		if f.set["health_stale_sec"] {
			pol["stale_after_sec"] = f.healthStaleSec
		}
		if f.set["health_window"] {
			pol["failure_window"] = f.healthWindow
		}
		if f.set["health_threshold"] {
			pol["failure_threshold"] = f.healthThreshold
		}
		profile["health_policy"] = pol
	}
	if f.set["self_repair"] || f.set["self_repair_attempts"] || f.set["self_repair_escalate"] {
		pol := objectMap(profile["self_repair"])
		if f.set["self_repair"] {
			pol["enabled"] = f.selfRepairEnabled
		}
		if f.set["self_repair_attempts"] {
			pol["max_attempts"] = f.selfRepairAttempts
		}
		if f.set["self_repair_escalate"] {
			pol["escalate_to"] = f.selfRepairEscalate
		}
		profile["self_repair"] = pol
	}
	if f.set["silent_on_success"] || f.set["disable_memory_writes"] || f.set["notify_min_severity"] || f.set["notify_cooldown_sec"] {
		pol := objectMap(profile["noise_policy"])
		if f.set["silent_on_success"] {
			pol["silent_on_success"] = f.silentOnSuccess
		}
		if f.set["disable_memory_writes"] {
			pol["disable_memory_writes"] = f.disableMemoryWrites
		}
		if f.set["notify_min_severity"] {
			pol["min_notify_severity"] = strings.TrimSpace(f.notifyMinSeverity)
		}
		if f.set["notify_cooldown_sec"] {
			pol["min_notify_interval_sec"] = f.notifyCooldownSec
		}
		profile["noise_policy"] = pol
	}
}

func applyAgentAdvancedFlags(profile map[string]any, f agentFlags) error {
	if f.set["instructions"] {
		profile["instructions"] = stringList(f.instructions)
	}
	if f.set["tool_allow"] {
		profile["tool_allow"] = stringList(f.toolAllow)
	}
	if f.set["tool_deny"] {
		profile["tool_deny"] = stringList(f.toolDeny)
	}
	if f.set["trust_ceiling"] {
		profile["trust_ceiling"] = strings.TrimSpace(f.trustCeiling)
	}
	if f.set["execution_profile"] {
		profile["execution_profile"] = strings.TrimSpace(f.executionProfile)
	}
	if f.set["config_overrides"] {
		cfg, err := parseConfigOverrides(f.configOverrides)
		if err != nil {
			return err
		}
		profile["config_overrides"] = cfg
	}
	if f.set["lifecycle"] || f.set["max_cycles"] {
		life := objectMap(profile["lifecycle"])
		if f.set["lifecycle"] {
			mode := strings.TrimSpace(f.lifecycleMode)
			life["mode"] = mode
			life["retire_on_complete"] = mode == "retire_on_complete"
		}
		if f.set["max_cycles"] {
			life["max_cycles"] = f.lifecycleMaxCycles
		}
		profile["lifecycle"] = life
	}
	if f.set["cycle_tasks"] || f.set["total_tasks"] {
		var tasks []any
		if existing, ok := profile["tasklist"].([]any); ok {
			for _, rawTask := range existing {
				scope := taskScope(rawTask)
				if scope == "cycle" && f.set["cycle_tasks"] {
					continue
				}
				if scope == "total" && f.set["total_tasks"] {
					continue
				}
				tasks = append(tasks, rawTask)
			}
		}
		if f.set["cycle_tasks"] {
			for _, title := range f.cycleTasks {
				tasks = append(tasks, map[string]any{"title": title, "scope": "cycle", "status": "todo"})
			}
		}
		if f.set["total_tasks"] {
			for _, title := range f.totalTasks {
				tasks = append(tasks, map[string]any{"title": title, "scope": "total", "status": "todo"})
			}
		}
		profile["tasklist"] = tasks
	}
	return nil
}

func taskScope(raw any) string {
	task, _ := raw.(map[string]any)
	if strings.TrimSpace(str(task["scope"])) == "cycle" {
		return "cycle"
	}
	return "total"
}

func parseConfigOverrides(s string) (map[string]any, error) {
	out := map[string]any{}
	for _, part := range splitList(s) {
		eq := strings.Index(part, "=")
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --config entry %q (want KEY=VALUE)", part)
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid --config entry %q (empty key)", part)
		}
		out[key] = val
	}
	return out, nil
}

func objectMap(v any) map[string]any {
	out := map[string]any{}
	if m, ok := v.(map[string]any); ok {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func stringList(csv string) []any {
	var out []any
	for _, s := range splitList(csv) {
		out = append(out, s)
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, item := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

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

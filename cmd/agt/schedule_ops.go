// SPDX-License-Identifier: MIT
//
// cmd/agt schedule ops: enable + list (with render helpers
// scheduleTargetStatusText + scheduleExecutorText + scheduleActionText) +
// fires + stats + remove + run + scheduleByID. Split from schedule.go during
// Day 211 god-file refactor (#37). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdScheduleEnable(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "resume"
	if !enabled {
		verb = "pause"
	}
	asJSON := false
	var id string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule %s <id> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule %s: an id is required\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": enabled})
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	if updated, _ := res["updated"].(bool); !updated {
		fmt.Fprintf(stderr, "%s schedule %s: not found (%s)\n", brand.CLI, verb, id)
		return 3
	}
	fmt.Fprintf(stdout, "%s %sd\n", id, verb)
	return 0
}

func cmdScheduleList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s schedule list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	list, _ := res["schedules"].([]any)
	if len(list) == 0 {
		fmt.Fprintf(stdout, "no schedules. Add one with `%s schedule add \"<task>\" --every 1h`.\n", brand.CLI)
		return 0
	}
	for _, item := range list {
		m, _ := item.(map[string]any)
		id, _ := m["id"].(string)
		intent, _ := m["intent"].(string)
		cadence, _ := m["cadence"].(string)
		source, _ := m["source"].(string)
		agent, _ := m["agent"].(string)
		target, _ := m["target"].(string)
		workflowRef, _ := m["workflow"].(string)
		systemTask, _ := m["system_task"].(string)
		toolRef, _ := m["tool"].(string)
		enabled, _ := m["enabled"].(bool)
		next, _ := m["next_run_unix"].(float64)
		state := "enabled"
		if !enabled {
			state = "paused"
		}
		nextStr := "—"
		if next > 0 {
			nextStr = time.Unix(int64(next), 0).Format("2006-01-02 15:04")
		}
		action := scheduleActionText(map[string]string{
			"id":          id,
			"intent":      intent,
			"target":      target,
			"agent":       agent,
			"workflow":    workflowRef,
			"system_task": systemTask,
			"tool":        toolRef,
		})
		if contract, _ := m["execution_contract"].(string); contract != "" {
			action = contract
		}
		fmt.Fprintf(stdout, "  %-22s %-16s [%s,%s] next %s  %s",
			id, cadence, source, state, nextStr, action)
		executor, _ := m["executor"].(string)
		usesLLM, hasUsesLLM := m["uses_llm"].(bool)
		if exec := scheduleExecutorText(executor, usesLLM, hasUsesLLM); exec != "" {
			fmt.Fprintf(stdout, "  executor:%s", exec)
		}
		if targetStatus := scheduleTargetStatusText(m); targetStatus != "" {
			fmt.Fprintf(stdout, "  target:%s", targetStatus)
		}
		if intent != "" && action != intent && !strings.Contains(action, intent) {
			fmt.Fprintf(stdout, "  label:%q", intent)
		}
		if agent != "" {
			fmt.Fprintf(stdout, "  agent:%s", agent)
		}
		if workflowRef != "" {
			fmt.Fprintf(stdout, "  workflow:%s", workflowRef)
		}
		if systemTask != "" {
			fmt.Fprintf(stdout, "  system_task:%s", systemTask)
		}
		if toolRef != "" {
			fmt.Fprintf(stdout, "  tool:%s", toolRef)
		}
		// Last-firing outcome (M56) — how the schedule last went, when known.
		lastStatus, _ := m["last_status"].(string)
		if lastStatus != "" {
			lastReason, _ := m["last_reason"].(string)
			if lastStatus == "failed" && lastReason != "" {
				lastStatus = "failed (" + lastReason + ")"
			}
			lastWhen := ""
			if lf, ok := m["last_fired_unix_ms"].(float64); ok && lf > 0 {
				lastWhen = " " + time.UnixMilli(int64(lf)).Format("01-02 15:04")
			}
			fmt.Fprintf(stdout, "  last: %s%s", lastStatus, lastWhen)
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

func scheduleTargetStatusText(m map[string]any) string {
	status, _ := m["target_status"].(string)
	errText, _ := m["target_error"].(string)
	status = strings.TrimSpace(status)
	errText = strings.TrimSpace(errText)
	if strings.EqualFold(status, "blocked") || errText != "" {
		if errText == "" {
			return "blocked"
		}
		return "blocked (" + errText + ")"
	}
	if strings.EqualFold(status, "ready") {
		return "ready"
	}
	return ""
}

func scheduleExecutorText(executor string, usesLLM bool, known bool) string {
	if executor == "" {
		return ""
	}
	if !known {
		return executor
	}
	if usesLLM {
		return executor + "/llm"
	}
	return executor + "/no-llm"
}

func scheduleActionText(m map[string]string) string {
	switch m["target"] {
	case cadence.TargetWorkflow:
		if m["workflow"] != "" {
			return "run workflow " + m["workflow"]
		}
	case cadence.TargetSystemTask:
		if m["system_task"] != "" {
			return "run system task " + m["system_task"]
		}
	case cadence.TargetTool:
		if m["tool"] != "" {
			return "run tool " + m["tool"]
		}
	}
	if m["agent"] != "" && m["intent"] != "" {
		return "wake " + m["agent"] + ": " + m["intent"]
	}
	if m["intent"] != "" {
		return m["intent"]
	}
	return m["id"]
}

// cmdScheduleFires implements `agt schedule fires [N] [--json]` — the autonomy
// analogue of `agt runs list`. `schedule list` shows what's scheduled; this
// shows what actually FIRED and how it turned out (status, duration, spend),
// joined server-side from the schedule.fired events and the run outcomes (M54).
// Drill into any firing with `agt runs show <correlation>`.
func cmdScheduleFires(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args) // M129: a tenant's own schedule firings
	asJSON := false
	limit := 0
	id := ""
	status := ""
	intent := ""
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--intent":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --intent needs a substring\n", brand.CLI)
				return 2
			}
			i++
			intent = args[i]
		case strings.HasPrefix(a, "--intent="):
			intent = strings.TrimPrefix(a, "--intent=")
		case a == "--id":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --id needs a schedule id\n", brand.CLI)
				return 2
			}
			i++
			id = args[i]
		case strings.HasPrefix(a, "--id="):
			id = strings.TrimPrefix(a, "--id=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule fires: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule fires: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "--failed":
			status = "failed"
		case a == "--status":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --status needs a value\n", brand.CLI)
				return 2
			}
			i++
			status = args[i]
		case strings.HasPrefix(a, "--status="):
			status = strings.TrimPrefix(a, "--status=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s schedule fires [N] [--id <sched>] [--status <s>|--failed] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show recent scheduled-run firings and their outcomes (status, duration, spend)\n")
			fmt.Fprintf(stdout, "  --id <sched>   only this schedule's firings\n")
			fmt.Fprintf(stdout, "  --status <s>   only firings with this status (completed|failed|running|abandoned)\n")
			fmt.Fprintf(stdout, "  --failed       shorthand for --status failed\n")
			fmt.Fprintf(stdout, "  --intent <substr> only firings whose task/label contains <substr>\n")
			fmt.Fprintf(stdout, "drill into a firing with `%s runs show <correlation>`\n", brand.CLI)
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s schedule fires: unexpected arg %q (expected N, --id <sched>, --status <s>, or --json)\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	if id != "" {
		callArgs["id"] = id // M55: filter to one schedule's firings
	}
	if status != "" {
		callArgs["status"] = status // M61: filter by firing status
	}
	if intent != "" {
		callArgs["intent"] = intent // M80: intent substring filter
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS // M65: time window
	}
	res, err := c.Call(ctx, controlplane.CmdScheduleFires, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule fires: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	fires, _ := res["fires"].([]any)
	if len(fires) == 0 {
		fmt.Fprintf(stdout, "no scheduled firings yet (schedules fire on their cadence; see `%s schedule list`).\n", brand.CLI)
		return 0
	}
	for _, item := range fires {
		m, _ := item.(map[string]any)
		corr, _ := m["correlation_id"].(string)
		intent, _ := m["intent"].(string)
		action, _ := m["action"].(string)
		status, _ := m["status"].(string)
		reason, _ := m["reason"].(string)
		fired := intOfStatus(m["fired_unix_ms"])
		dur := intOfStatus(m["duration_ms"])
		spent := mcFromAny(m["spent_mc"])

		statusDisp := status
		if status == "failed" && reason != "" {
			statusDisp = "failed (" + reason + ")"
		}
		firedStr := "—"
		if fired > 0 {
			firedStr = time.UnixMilli(fired).Format("2006-01-02 15:04:05")
		}
		// Duration + spend only for terminal firings (running ones have neither).
		meta := ""
		if status == "completed" || status == "failed" {
			meta = " (" + fmtDuration(dur)
			if spent > 0 {
				meta += ", " + fmtUSD(spent)
			}
			meta += ")"
		}
		if action == "" {
			action = intent
		}
		fmt.Fprintf(stdout, "  %s  %-18s%s  %s  %s", firedStr, statusDisp, meta, corr, action)
		if intent != "" && action != intent && !strings.Contains(action, intent) {
			fmt.Fprintf(stdout, "  label:%q", intent)
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

// cmdScheduleStats implements `agt schedule stats [--id <sched>] [--since <dur>]
// [--json]` — the autonomy analogue of `agt runs stats`, aggregating scheduled
// firings: counts, success rate, and total spend (M57).
func cmdScheduleStats(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args) // M129: a tenant's own schedule stats
	asJSON := false
	id := ""
	sinceMS := int64(0)
	sinceLabel := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--id":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule stats: --id needs a schedule id\n", brand.CLI)
				return 2
			}
			i++
			id = args[i]
		case strings.HasPrefix(a, "--id="):
			id = strings.TrimPrefix(a, "--id=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s schedule stats [--id <sched>] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate scheduled-firing health: counts, success rate, total spend\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s schedule stats: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if id != "" {
		callArgs["id"] = id
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdScheduleStats, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	total := intOfStatus(res["total"])
	windowSuffix := ""
	if sinceLabel != "" {
		windowSuffix = " in the last " + sinceLabel
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no scheduled firings%s.\n", windowSuffix)
		return 0
	}
	completed := intOfStatus(res["completed"])
	failed := intOfStatus(res["failed"])
	running := intOfStatus(res["running"])
	abandoned := intOfStatus(res["abandoned"])
	terminal := intOfStatus(res["terminal"])
	schedules := intOfStatus(res["schedules"])

	fmt.Fprintf(stdout, "schedule firings (over %d firing(s)%s):\n\n", total, windowSuffix)
	fmt.Fprintf(stdout, "  schedules : %d distinct fired\n", schedules)
	fmt.Fprintf(stdout, "  completed : %d\n", completed)
	failedLine := fmt.Sprintf("%d", failed)
	if br := failedByReasonStr(res["failed_by_reason"]); br != "" {
		failedLine += " (" + br + ")"
	}
	fmt.Fprintf(stdout, "  failed    : %s\n", failedLine)
	fmt.Fprintf(stdout, "  running   : %d\n", running)
	fmt.Fprintf(stdout, "  abandoned : %d\n", abandoned)
	if terminal > 0 {
		rate, _ := res["success_rate"].(float64)
		fmt.Fprintf(stdout, "  success   : %.1f%% (%d/%d terminal)\n", rate*100, completed, terminal)
	} else {
		fmt.Fprintf(stdout, "  success   : n/a (no firing has finished yet)\n")
	}
	if spent := mcFromAny(res["spent_microcents"]); spent > 0 {
		fmt.Fprintf(stdout, "  spend     : %s\n", fmtUSD(spent))
	}
	return 0
}

func cmdScheduleRemove(args []string, stdout, stderr io.Writer) int {
	return scheduleByID(args, stdout, stderr, "rm", controlplane.CmdScheduleRemove, "removed", "removed", "not found")
}

func cmdScheduleRun(args []string, stdout, stderr io.Writer) int {
	return scheduleByID(args, stdout, stderr, "run", controlplane.CmdScheduleRun, "triggered", "triggered (fires on the next tick)", "not found")
}

// scheduleByID factors the rm/run commands: both take a single id, call a
// control-plane command, and report a boolean result key.
func scheduleByID(args []string, stdout, stderr io.Writer, verb, cmd, resultKey, okMsg, missMsg string) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule %s <id> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule %s: an id is required\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	ok, _ := res[resultKey].(bool)
	if !ok {
		fmt.Fprintf(stderr, "%s schedule %s: %s (%s)\n", brand.CLI, verb, missMsg, id)
		return 3
	}
	fmt.Fprintf(stdout, "%s %s\n", id, okMsg)
	return 0
}

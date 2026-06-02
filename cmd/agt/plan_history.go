// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdPlanHistory implements `agt plan history [N] [--status <s>] [--json]` — the
// plan analogue of `agt runs list` (M83). A plan run journals plan.started +
// plan.completed/failed under a "plan-…" correlation that isn't a task run, so
// it never shows in `agt runs list`; this lists those executions newest-first.
// Drill into one with `agt runs show <correlation>` (M82).
func cmdPlanHistory(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args) // M129: a tenant's own plan executions
	asJSON := false
	limit := 0
	status := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--failed":
			status = "failed"
		case a == "--status":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s plan history: --status needs a value\n", brand.CLI)
				return 2
			}
			i++
			status = args[i]
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s plan history [N] [--status <s>|--failed] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list recent plan executions (name, status, nodes, duration), newest first\n")
			fmt.Fprintf(stdout, "  --status <s>  only plans with this status (completed|failed|running)\n")
			fmt.Fprintf(stdout, "  --failed      shorthand for --status failed\n")
			fmt.Fprintf(stdout, "drill into one with `%s runs show <correlation>`\n", brand.CLI)
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s plan history: unexpected arg %q (expected N, --status <s>, or --json)\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	if status != "" {
		callArgs["status"] = status
	}
	res, err := c.Call(ctx, controlplane.CmdPlanHistory, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s plan history: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	plans, _ := res["plans"].([]any)
	if len(plans) == 0 {
		fmt.Fprintln(stdout, "no plan executions journaled yet.")
		return 0
	}
	fmt.Fprintf(stdout, "last %d plan execution(s):\n\n", len(plans))
	for _, raw := range plans {
		m, _ := raw.(map[string]any)
		corr, _ := m["correlation_id"].(string)
		name, _ := m["plan_name"].(string)
		status, _ := m["status"].(string)
		nodes := intOfStatus(m["node_count"])
		duration := intOfStatus(m["duration_ms"])
		started := int64(0)
		if f, ok := m["started_unix_ms"].(float64); ok {
			started = int64(f)
		}
		if name == "" {
			name = "(unnamed)"
		}
		whenStr := "—"
		if started > 0 {
			whenStr = time.UnixMilli(started).Format("2006-01-02 15:04:05")
		}
		statusStr := status
		if (status == "completed" || status == "failed") && duration > 0 {
			statusStr += " in " + fmtDuration(duration)
		}
		fmt.Fprintf(stdout, "  %s\n", corr)
		fmt.Fprintf(stdout, "    started : %s   status: %s   nodes: %d\n", whenStr, statusStr, nodes)
		fmt.Fprintf(stdout, "    plan    : %s\n", name)
	}
	return 0
}

// cmdPlanStats implements `agt plan stats [--json]` — the plan analogue of
// `agt runs stats` (M84): aggregate plan-execution health (counts, success
// rate, duration distribution).
func cmdPlanStats(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args) // M129: a tenant's own plan stats
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s plan stats [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate plan-execution health: total / completed / failed / running,\n")
			fmt.Fprintf(stdout, "success rate, and plan-duration avg/min/p50/p95/max\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s plan stats: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdPlanStats, withTenant(tenant, nil))
	if err != nil {
		fmt.Fprintf(stderr, "%s plan stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	total := intOfStatus(res["total"])
	if total == 0 {
		fmt.Fprintln(stdout, "no plan executions journaled yet.")
		return 0
	}
	completed := intOfStatus(res["completed"])
	failed := intOfStatus(res["failed"])
	running := intOfStatus(res["running"])
	terminal := intOfStatus(res["terminal"])
	rate, _ := res["success_rate"].(float64)
	fmt.Fprintf(stdout, "plan stats (over %d execution(s)):\n\n", total)
	fmt.Fprintf(stdout, "  completed : %d\n", completed)
	fmt.Fprintf(stdout, "  failed    : %d\n", failed)
	fmt.Fprintf(stdout, "  running   : %d\n", running)
	if terminal > 0 {
		fmt.Fprintf(stdout, "  success   : %.1f%% (%d/%d terminal)\n", rate*100, completed, terminal)
	} else {
		fmt.Fprintf(stdout, "  success   : n/a (no plan has finished yet)\n")
	}
	if dur, _ := res["duration_ms"].(map[string]any); dur != nil {
		if dcount := intOfStatus(dur["count"]); dcount > 0 {
			fmt.Fprintf(stdout, "\n  duration (over %d terminal plan(s)):\n", dcount)
			fmt.Fprintf(stdout, "    avg : %s\n", fmtDuration(intOfStatus(dur["avg"])))
			fmt.Fprintf(stdout, "    min : %s\n", fmtDuration(intOfStatus(dur["min"])))
			fmt.Fprintf(stdout, "    p50 : %s\n", fmtDuration(intOfStatus(dur["p50"])))
			fmt.Fprintf(stdout, "    p95 : %s\n", fmtDuration(intOfStatus(dur["p95"])))
			fmt.Fprintf(stdout, "    max : %s\n", fmtDuration(intOfStatus(dur["max"])))
		}
	}
	return 0
}

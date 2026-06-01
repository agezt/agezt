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
			fmt.Fprintf(stdout, "usage: %s plan history [N] [--status <s>|--failed] [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdPlanHistory, callArgs)
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

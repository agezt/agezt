// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdRuns dispatches `agt runs <subcommand>`. The only subcommand
// today is `list`; left as a dispatcher so future additions
// (`agt runs show <corr>`, `agt runs failed`) slot in without
// renaming.
func cmdRuns(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s runs: subcommand required (list)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list":
		return cmdRunsList(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s runs <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [N] [--json]   show the last N agent runs (default 20)\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s runs: unknown subcommand %q (list)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdRunsList implements `agt runs list [N] [--json]`.
// Walks the journal server-side, pairs task.received/task.completed
// by correlation_id, and shows the result sorted newest-first.
//
// Different from `agt journal tail` which is event-level
// (every kind, no aggregation); runs list is task-level
// (one row per agent loop invocation).
func cmdRunsList(args []string, stdout, stderr io.Writer) int {
	limit := 20
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs list [N] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the last N agent runs (default 20, max 1000)\n")
			return 0
		default:
			n, err := strconv.Atoi(a)
			if err != nil {
				fmt.Fprintf(stderr, "%s runs list: unexpected arg %q (expected N or --json)\n", brand.CLI, a)
				return 2
			}
			if n < 1 {
				fmt.Fprintf(stderr, "%s runs list: N must be >= 1 (got %d)\n", brand.CLI, n)
				return 2
			}
			limit = n
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdRunsList, map[string]any{"limit": limit})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	rows, _ := res["runs"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no runs yet (journal has no task.received events)")
		return 0
	}
	fmt.Fprintf(stdout, "last %d run(s):\n\n", len(rows))
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		corr, _ := r["correlation_id"].(string)
		intent, _ := r["intent"].(string)
		status, _ := r["status"].(string)
		started := intOfStatus(r["started_unix_ms"])
		duration := intOfStatus(r["duration_ms"])
		iters := intOfStatus(r["iters"])

		startedStr := "—"
		if started > 0 {
			startedStr = time.UnixMilli(started).Format("2006-01-02 15:04:05")
		}
		durationStr := "—"
		if status == "completed" {
			durationStr = fmtDuration(duration)
		}
		intentDisplay := intent
		if intentDisplay == "" {
			intentDisplay = "(no intent recorded)"
		}
		if len(intentDisplay) > 70 {
			intentDisplay = intentDisplay[:69] + "…"
		}

		fmt.Fprintf(stdout, "  %s\n", corr)
		fmt.Fprintf(stdout, "    started : %s   status: %-9s  duration: %s   iters: %d\n",
			startedStr, status, durationStr, iters)
		fmt.Fprintf(stdout, "    intent  : %s\n\n", intentDisplay)
	}
	return 0
}

// fmtDuration renders milliseconds as a human-readable duration.
// 0 → "—"; <1s → "Nms"; <60s → "N.Ns"; otherwise "MmNs".
// Distinct from fmtUptime (which uses seconds + always emits at
// least seconds-level granularity) — runs typically last under
// a minute so sub-second precision matters.
func fmtDuration(ms int64) string {
	switch {
	case ms <= 0:
		return "—"
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		m := ms / 60_000
		s := (ms % 60_000) / 1000
		return fmt.Sprintf("%dm%ds", m, s)
	}
}

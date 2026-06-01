// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdWarden dispatches `agt warden <subcommand>`. Today the only subcommand is
// `log` — the OS-sandbox execution audit (M96). Left as a dispatcher so future
// `agt warden profile` / `agt warden stats` slot in without renaming.
func cmdWarden(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s warden: subcommand required (log)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "log":
		return cmdWardenLog(args[1:], stdout, stderr)
	case "stats":
		return cmdWardenStats(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s warden: unknown subcommand %q (log, stats)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdWardenLog implements `agt warden log [N] [--issues] [--since <dur>]
// [--json]` (M96) — the sandboxed-execution audit: what the OS warden ran, under
// which profile, and whether it had to downgrade isolation or hit a limit.
func cmdWardenLog(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	issuesOnly := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--issues":
			issuesOnly = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s warden log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s warden log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s warden log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s warden log [N] [--issues] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show sandboxed executions (profile, exit, duration) + downgrades/limit breaches\n")
			fmt.Fprintf(stdout, "  --issues      only profile downgrades and limit breaches\n")
			fmt.Fprintf(stdout, "  --since <dur> only events in the last <dur>\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s warden log: unexpected arg %q (expected N, --issues, --since, or --json)\n", brand.CLI, a)
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
	if issuesOnly {
		callArgs["issues"] = true
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdWardenLog, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s warden log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["executions"].([]any)
	if len(rows) == 0 {
		if issuesOnly {
			fmt.Fprintln(stdout, "no warden downgrades or limit breaches.")
		} else {
			fmt.Fprintln(stdout, "no sandboxed executions journaled yet.")
		}
		return 0
	}
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		kind, _ := m["kind"].(string)
		profile, _ := m["profile"].(string)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		whenStr := "—"
		if ts > 0 {
			whenStr = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		switch kind {
		case "exec":
			argv0, _ := m["argv0"].(string)
			exit := intOfStatus(m["exit_code"])
			dur := intOfStatus(m["duration_ms"])
			line := fmt.Sprintf("  %s  exec       %-16s profile=%-10s exit=%d  %s", whenStr, argv0, profile, exit, fmtDuration(dur))
			if dg, _ := m["downgraded"].(bool); dg {
				line += "  [downgraded]"
			}
			if to, _ := m["timed_out"].(bool); to {
				line += "  [TIMED OUT]"
			}
			fmt.Fprintln(stdout, line)
		case "downgrade":
			req, _ := m["requested"].(string)
			reason, _ := m["reason"].(string)
			fmt.Fprintf(stdout, "  %s  DOWNGRADE  %s → %s  (%s)\n", whenStr, req, profile, reason)
		case "limit":
			argv0, _ := m["argv0"].(string)
			reason, _ := m["reason"].(string)
			fmt.Fprintf(stdout, "  %s  LIMIT      %-16s %s exceeded\n", whenStr, argv0, reason)
		}
	}
	return 0
}

// cmdWardenStats implements `agt warden stats [--since <dur>] [--json]` (M97) —
// sandbox posture: how many executions, how often the warden had to downgrade
// isolation, time-outs, limit breaches, and a by-effective-profile breakdown.
func cmdWardenStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	sinceMS := int64(0)
	sinceLabel := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s warden stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s warden stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s warden stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s warden stats [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate sandbox posture: executions, downgrade rate, timeouts, limit breaches, by profile\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s warden stats: unexpected arg %q\n", brand.CLI, a)
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
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdWardenStats, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s warden stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	total := intOfStatus(res["executions"])
	windowSuffix := ""
	if sinceLabel != "" {
		windowSuffix = " in the last " + sinceLabel
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no sandboxed executions%s.\n", windowSuffix)
		return 0
	}
	downgraded := intOfStatus(res["downgraded"])
	rate, _ := res["downgrade_rate"].(float64)
	timedOut := intOfStatus(res["timed_out"])
	limits := intOfStatus(res["limit_breaches"])
	fmt.Fprintf(stdout, "sandbox executions (over %d%s):\n\n", total, windowSuffix)
	fmt.Fprintf(stdout, "  downgraded    : %d\n", downgraded)
	fmt.Fprintf(stdout, "  downgrade     : %.1f%%\n", rate*100)
	fmt.Fprintf(stdout, "  timed out     : %d\n", timedOut)
	fmt.Fprintf(stdout, "  limit breaches: %d\n", limits)
	if byP, _ := res["by_profile"].(map[string]any); len(byP) > 0 {
		fmt.Fprintf(stdout, "\n  by effective profile:\n")
		names := make([]string, 0, len(byP))
		for n := range byP {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(stdout, "    %-12s %d\n", n, intOfStatus(byP[n]))
		}
	}
	return 0
}

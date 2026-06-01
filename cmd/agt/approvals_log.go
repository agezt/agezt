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

// cmdApprovalsLog implements `agt approvals log [N] [--denied] [--since <dur>]
// [--json]` (M87) — the HITL approval audit. `agt approvals` shows PENDING
// requests; this shows the HISTORY: what was asked, how it resolved, and who
// decided. The human analogue of `agt edict log`.
func cmdApprovalsLog(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	deniedOnly := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--denied":
			deniedOnly = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s approvals log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s approvals log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s approvals log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s approvals log [N] [--denied] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the HITL approval audit (requested → granted/denied/timeout, who decided)\n")
			fmt.Fprintf(stdout, "  --denied      only denials and timeouts\n")
			fmt.Fprintf(stdout, "  --since <dur> only approvals in the last <dur>\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s approvals log: unexpected arg %q (expected N, --denied, --since, or --json)\n", brand.CLI, a)
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
	if deniedOnly {
		callArgs["denied"] = true
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdApprovalsLog, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s approvals log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["approvals"].([]any)
	if len(rows) == 0 {
		if deniedOnly {
			fmt.Fprintln(stdout, "no denied approvals.")
		} else {
			fmt.Fprintln(stdout, "no approvals journaled yet.")
		}
		return 0
	}
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		status, _ := m["status"].(string)
		capability, _ := m["capability"].(string)
		tool, _ := m["tool"].(string)
		by, _ := m["resolved_by"].(string)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		whenStr := "—"
		if ts > 0 {
			whenStr = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		line := fmt.Sprintf("  %s  %-8s %-12s %s", whenStr, status, capability, tool)
		if by != "" {
			line += "  (by " + by + ")"
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// cmdApprovalsStats implements `agt approvals stats [--since <dur>] [--json]`
// (M88) — the HITL approval aggregate (grant rate, denied-by-capability), the
// human analogue of `agt edict stats`.
func cmdApprovalsStats(args []string, stdout, stderr io.Writer) int {
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
				fmt.Fprintf(stderr, "%s approvals stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s approvals stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s approvals stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s approvals stats [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate HITL approvals: granted/denied/timeout, grant rate, denied-by-capability\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s approvals stats: unexpected arg %q\n", brand.CLI, a)
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
	res, err := c.Call(ctx, controlplane.CmdApprovalsStats, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s approvals stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	total := intOfStatus(res["total"])
	windowSuffix := ""
	if sinceLabel != "" {
		windowSuffix = " in the last " + sinceLabel
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no approvals%s.\n", windowSuffix)
		return 0
	}
	granted := intOfStatus(res["granted"])
	denied := intOfStatus(res["denied"])
	timeout := intOfStatus(res["timeout"])
	pending := intOfStatus(res["pending"])
	rate, _ := res["grant_rate"].(float64)
	fmt.Fprintf(stdout, "approvals (over %d%s):\n\n", total, windowSuffix)
	fmt.Fprintf(stdout, "  granted   : %d\n", granted)
	fmt.Fprintf(stdout, "  denied    : %d\n", denied)
	fmt.Fprintf(stdout, "  timeout   : %d\n", timeout)
	fmt.Fprintf(stdout, "  pending   : %d\n", pending)
	fmt.Fprintf(stdout, "  grant     : %.1f%%\n", rate*100)
	if byCap, _ := res["denied_by_capability"].(map[string]any); len(byCap) > 0 {
		fmt.Fprintf(stdout, "\n  denied/timed-out by capability:\n")
		caps := make([]string, 0, len(byCap))
		for capName := range byCap {
			caps = append(caps, capName)
		}
		sort.Strings(caps)
		for _, capName := range caps {
			fmt.Fprintf(stdout, "    %-14s %d\n", capName, intOfStatus(byCap[capName]))
		}
	}
	return 0
}

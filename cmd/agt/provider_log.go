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

// cmdProviderLog implements `agt provider log [N] [--fallbacks] [--since <dur>]
// [--json]` (M89) — the provider-activity timeline. `provider check` probes
// whether a provider works; this shows what the governor actually did at request
// time: which provider handled calls (routing.decision) and when the primary had
// to fall back (provider.fallback).
func cmdProviderLog(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	fallbacksOnly := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--fallbacks":
			fallbacksOnly = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s provider log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s provider log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s provider log [N] [--fallbacks] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show provider-routing activity (which provider handled calls, when it fell back)\n")
			fmt.Fprintf(stdout, "  --fallbacks   only provider fallbacks (primary errored)\n")
			fmt.Fprintf(stdout, "  --since <dur> only activity in the last <dur>\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s provider log: unexpected arg %q (expected N, --fallbacks, --since, or --json)\n", brand.CLI, a)
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
	if fallbacksOnly {
		callArgs["fallbacks"] = true
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdProviderLog, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s provider log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	events, _ := res["events"].([]any)
	if len(events) == 0 {
		if fallbacksOnly {
			fmt.Fprintln(stdout, "no provider fallbacks (primary handled every call).")
		} else {
			fmt.Fprintln(stdout, "no provider routing journaled yet.")
		}
		return 0
	}
	for _, raw := range events {
		m, _ := raw.(map[string]any)
		kind, _ := m["kind"].(string)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		whenStr := "—"
		if ts > 0 {
			whenStr = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		if kind == "fallback" {
			failed, _ := m["failed"].(string)
			next, _ := m["next"].(string)
			reason, _ := m["reason"].(string)
			line := fmt.Sprintf("  %s  FALLBACK  %s → %s", whenStr, failed, next)
			if reason != "" {
				line += "  (" + reason + ")"
			}
			fmt.Fprintln(stdout, line)
		} else {
			primary, _ := m["primary"].(string)
			chain, _ := m["chain"].(string)
			taskType, _ := m["task_type"].(string)
			line := fmt.Sprintf("  %s  route     → %s", whenStr, primary)
			if chain != "" && chain != primary {
				line += "  (chain: " + chain + ")"
			}
			if taskType != "" {
				line += "  [" + taskType + "]"
			}
			fmt.Fprintln(stdout, line)
		}
	}
	return 0
}

// cmdProviderStats implements `agt provider stats [--since <dur>] [--json]`
// (M90) — provider-routing reliability: routed calls, fallback rate,
// calls-by-primary, fallbacks-by-failed-provider.
func cmdProviderStats(args []string, stdout, stderr io.Writer) int {
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
				fmt.Fprintf(stderr, "%s provider stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s provider stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s provider stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s provider stats [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate provider routing: routed calls, fallback rate, calls-by-primary\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s provider stats: unexpected arg %q\n", brand.CLI, a)
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
	res, err := c.Call(ctx, controlplane.CmdProviderStats, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s provider stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	routed := intOfStatus(res["routed"])
	windowSuffix := ""
	if sinceLabel != "" {
		windowSuffix = " in the last " + sinceLabel
	}
	if routed == 0 {
		fmt.Fprintf(stdout, "no provider routing%s.\n", windowSuffix)
		return 0
	}
	fallbacks := intOfStatus(res["fallbacks"])
	rate, _ := res["fallback_rate"].(float64)
	fmt.Fprintf(stdout, "provider routing (over %d routed call(s)%s):\n\n", routed, windowSuffix)
	fmt.Fprintf(stdout, "  fallbacks : %d\n", fallbacks)
	fmt.Fprintf(stdout, "  fallback  : %.1f%%\n", rate*100)
	if byP, _ := res["by_primary"].(map[string]any); len(byP) > 0 {
		fmt.Fprintf(stdout, "\n  calls by primary:\n")
		names := make([]string, 0, len(byP))
		for n := range byP {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(stdout, "    %-18s %d\n", n, intOfStatus(byP[n]))
		}
	}
	if fb, _ := res["fallbacks_by_primary"].(map[string]any); len(fb) > 0 {
		fmt.Fprintf(stdout, "\n  fallbacks by failed provider:\n")
		names := make([]string, 0, len(fb))
		for n := range fb {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(stdout, "    %-18s %d\n", n, intOfStatus(fb[n]))
		}
	}
	return 0
}

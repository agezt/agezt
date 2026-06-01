// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdRateLimit dispatches `agt ratelimit <subcommand>` (M106) — visibility into
// governor call-rate throttling (rate.limited events), per primary or tenant.
func cmdRateLimit(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s ratelimit: subcommand required (log|stats)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "log":
		return cmdRateLimitLog(args[1:], stdout, stderr)
	case "stats":
		return cmdRateLimitStats(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s ratelimit: unknown subcommand %q (log|stats)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdRateLimitLog(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s ratelimit log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s ratelimit log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s ratelimit log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s ratelimit log [N] [--tenant <id>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show recent call-rate throttle events (governor refused a call over the per-minute cap)\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s ratelimit log: unexpected arg %q\n", brand.CLI, a)
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
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdRateLimitLog, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s ratelimit log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["throttles"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no call-rate throttles journaled yet.")
		return 0
	}
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		when := "—"
		if ts > 0 {
			when = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(stdout, "  %s  throttled  used=%d  limit=%d/min\n",
			when, intOfStatus(m["used"]), intOfStatus(m["limit_per_min"]))
	}
	return 0
}

func cmdRateLimitStats(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
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
				fmt.Fprintf(stderr, "%s ratelimit stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s ratelimit stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s ratelimit stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s ratelimit stats [--tenant <id>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate call-rate throttling: total throttled, configured limit, worst overshoot\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s ratelimit stats: unexpected arg %q\n", brand.CLI, a)
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
	res, err := c.Call(ctx, controlplane.CmdRateLimitStats, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s ratelimit stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	total := intOfStatus(res["throttled"])
	suffix := ""
	if sinceLabel != "" {
		suffix = " in the last " + sinceLabel
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no call-rate throttles%s.\n", suffix)
		return 0
	}
	fmt.Fprintf(stdout, "call-rate throttling%s:\n\n", suffix)
	fmt.Fprintf(stdout, "  throttled : %d call(s) refused\n", total)
	fmt.Fprintf(stdout, "  limit     : %d/min\n", intOfStatus(res["limit_per_min"]))
	fmt.Fprintf(stdout, "  worst     : %d call(s) in a window (overshoot)\n", intOfStatus(res["worst_used"]))
	return 0
}

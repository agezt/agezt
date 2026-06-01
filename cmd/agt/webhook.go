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

// cmdWebhook dispatches `agt webhook <subcommand>` (M112) — visibility into
// outbound webhook delivery (webhook.delivered / webhook.failed).
func cmdWebhook(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s webhook: subcommand required (log|stats)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "log":
		return cmdWebhookLog(args[1:], stdout, stderr)
	case "stats":
		return cmdWebhookStats(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s webhook: unknown subcommand %q (log|stats)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdWebhookLog(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	failedOnly := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--failed":
			failedOnly = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s webhook log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s webhook log [N] [--failed] [--tenant <id>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show recent outbound webhook deliveries (2xx) and failures (exhausted retries)\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s webhook log: unexpected arg %q\n", brand.CLI, a)
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
	if failedOnly {
		callArgs["failed"] = true
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdWebhookLog, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s webhook log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["deliveries"].([]any)
	if len(rows) == 0 {
		if failedOnly {
			fmt.Fprintln(stdout, "no webhook failures journaled.")
		} else {
			fmt.Fprintln(stdout, "no webhook deliveries journaled yet.")
		}
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
		url, _ := m["url"].(string)
		kind, _ := m["event_kind"].(string)
		attempts := intOfStatus(m["attempts"])
		if ok, _ := m["ok"].(bool); ok {
			fmt.Fprintf(stdout, "  %s  OK      %-22s → %s  [%d, %d attempt(s)]\n",
				when, kind, url, intOfStatus(m["status"]), attempts)
		} else {
			errMsg, _ := m["error"].(string)
			fmt.Fprintf(stdout, "  %s  FAILED  %-22s → %s  after %d attempt(s): %s\n",
				when, kind, url, attempts, errMsg)
		}
	}
	return 0
}

func cmdWebhookStats(args []string, stdout, stderr io.Writer) int {
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
				fmt.Fprintf(stderr, "%s webhook stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s webhook stats [--tenant <id>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate webhook delivery: total, delivered, failed, failure rate, by URL\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s webhook stats: unexpected arg %q\n", brand.CLI, a)
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
	res, err := c.Call(ctx, controlplane.CmdWebhookStats, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s webhook stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	total := intOfStatus(res["total"])
	suffix := ""
	if sinceLabel != "" {
		suffix = " in the last " + sinceLabel
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no webhook deliveries%s.\n", suffix)
		return 0
	}
	rate, _ := res["failure_rate"].(float64)
	fmt.Fprintf(stdout, "webhook deliveries (over %d%s):\n\n", total, suffix)
	fmt.Fprintf(stdout, "  delivered    : %d\n", intOfStatus(res["delivered"]))
	fmt.Fprintf(stdout, "  failed       : %d\n", intOfStatus(res["failed"]))
	fmt.Fprintf(stdout, "  failure rate : %.1f%%\n", rate*100)
	if byURL, _ := res["by_url"].(map[string]any); len(byURL) > 0 {
		fmt.Fprintf(stdout, "\n  by URL:\n")
		urls := make([]string, 0, len(byURL))
		for u := range byURL {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		for _, u := range urls {
			c, _ := byURL[u].(map[string]any)
			fmt.Fprintf(stdout, "    %-40s delivered=%d failed=%d\n", u, intOfStatus(c["delivered"]), intOfStatus(c["failed"]))
		}
	}
	return 0
}

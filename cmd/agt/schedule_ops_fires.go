// SPDX-License-Identifier: MIT
//
// cmd/agt schedule fires sub-command handler (cmdScheduleFires).
// Extracted from schedule_ops.go during Day 211 god-file refactor (#58).
// Public API unchanged.
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
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

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

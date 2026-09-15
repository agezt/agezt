// SPDX-License-Identifier: MIT
//
// cmd/agt schedule stats/remove/run handlers + scheduleByID helper
// (cmdScheduleStats, cmdScheduleRemove, cmdScheduleRun, scheduleByID).
// Extracted from schedule_ops.go during Day 211 god-file refactor (#58).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

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

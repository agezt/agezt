// SPDX-License-Identifier: MIT
//
// cmd/agt tool stats command: cmdToolStats (the per-tool invocation
// aggregate — error rate + per-tool calls/errors breakdown, the
// execution-dashboard analogue of `agt edict stats`).
// Extracted from tool_log.go during the Day-210 god-file split.
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdToolStats implements `agt tool stats [--tool <name>] [--since <dur>]
// [--tenant <id>] [--json]` — a tool-invocation aggregate (error rate + a
// per-tool calls/errors breakdown), the execution-dashboard analogue of
// `agt edict stats` (M67). Completes the tool list/log/stats triad.
func cmdToolStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	toolFilter := ""
	tenant := ""
	sinceMS := int64(0)
	sinceLabel := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tool":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s tool stats: --tool needs a name\n", brand.CLI)
				return 2
			}
			i++
			toolFilter = args[i]
		case strings.HasPrefix(a, "--tool="):
			toolFilter = strings.TrimPrefix(a, "--tool=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s tool stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s tool stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s tool stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s tool stats: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s tool stats [--tool <name>] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate tool invocations: total, errored (rate), calls/errors by tool\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s tool stats: unexpected arg %q\n", brand.CLI, a)
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
	if toolFilter != "" {
		callArgs["tool"] = toolFilter
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	if tenant != "" {
		callArgs["tenant"] = tenant
	}
	res, err := c.Call(ctx, controlplane.CmdToolStats, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s tool stats: %v\n", brand.CLI, err)
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
		fmt.Fprintf(stdout, "no tool invocations%s.\n", windowSuffix)
		return 0
	}
	errored := intOfStatus(res["errored"])
	rate, _ := res["error_rate"].(float64)
	fmt.Fprintf(stdout, "tool invocations (over %d%s):\n\n", total, windowSuffix)
	fmt.Fprintf(stdout, "  errored   : %d\n", errored)
	fmt.Fprintf(stdout, "  error     : %.1f%%\n", rate*100)
	if byTool, _ := res["by_tool"].(map[string]any); len(byTool) > 0 {
		fmt.Fprintf(stdout, "\n  by tool:\n")
		names := make([]string, 0, len(byTool))
		for name := range byTool {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			m, _ := byTool[name].(map[string]any)
			line := fmt.Sprintf("    %-16s %d call(s), %d error(s)",
				name, intOfStatus(m["calls"]), intOfStatus(m["errors"]))
			// Per-tool mean latency (M75) — present only for tools with a
			// joinable invoked→result span, so unmeasured tools stay clean.
			if _, ok := m["avg_ms"]; ok {
				line += fmt.Sprintf(", avg %s", fmtDuration(intOfStatus(m["avg_ms"])))
			}
			fmt.Fprintln(stdout, line)
		}
	}
	// Failure-mode breakdown (M79) — what the errors actually were, most-frequent
	// first, so an operator sees denied / not-available / timeout at a glance.
	if byErr, _ := res["errors_by_message"].(map[string]any); len(byErr) > 0 {
		type em struct {
			msg string
			n   int64
		}
		ems := make([]em, 0, len(byErr))
		for msg, c := range byErr {
			ems = append(ems, em{msg, intOfStatus(c)})
		}
		sort.Slice(ems, func(i, j int) bool {
			if ems[i].n != ems[j].n {
				return ems[i].n > ems[j].n // most frequent first
			}
			return ems[i].msg < ems[j].msg // stable tiebreak
		})
		fmt.Fprintf(stdout, "\n  errors by message:\n")
		for _, e := range ems {
			fmt.Fprintf(stdout, "    %2d  %s\n", e.n, e.msg)
		}
	}
	// Latency distribution (M71) — same nearest-rank block as `runs stats`,
	// over tool calls whose invoked→result span was joinable.
	if dur, _ := res["duration_ms"].(map[string]any); dur != nil {
		if dcount := intOfStatus(dur["count"]); dcount > 0 {
			fmt.Fprintf(stdout, "\n  latency (over %d call(s)):\n", dcount)
			fmt.Fprintf(stdout, "    avg : %s\n", fmtDuration(intOfStatus(dur["avg"])))
			fmt.Fprintf(stdout, "    min : %s\n", fmtDuration(intOfStatus(dur["min"])))
			fmt.Fprintf(stdout, "    p50 : %s\n", fmtDuration(intOfStatus(dur["p50"])))
			fmt.Fprintf(stdout, "    p95 : %s\n", fmtDuration(intOfStatus(dur["p95"])))
			fmt.Fprintf(stdout, "    max : %s\n", fmtDuration(intOfStatus(dur["max"])))
		}
	}
	return 0
}

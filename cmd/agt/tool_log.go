// SPDX-License-Identifier: MIT
//
// cmd/agt tool log command: cmdToolLog (the per-tool invocation log
// viewer).
// The cmdToolStats aggregate lives in tool_stats.go.
// Extracted from tool_log.go during the Day-210 god-file split.
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
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdToolLog(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	errorsOnly := false
	limit := 0
	toolFilter := ""
	tenant := ""
	sinceMS := int64(0)
	slowMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--errors":
			errorsOnly = true
		case a == "--slow":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s tool log: --slow needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s tool log: bad --slow %q\n", brand.CLI, args[i])
				return 2
			}
			slowMS = d.Milliseconds()
		case strings.HasPrefix(a, "--slow="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--slow="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s tool log: bad --slow\n", brand.CLI)
				return 2
			}
			slowMS = d.Milliseconds()
		case a == "--tool":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s tool log: --tool needs a name\n", brand.CLI)
				return 2
			}
			i++
			toolFilter = args[i]
		case strings.HasPrefix(a, "--tool="):
			toolFilter = strings.TrimPrefix(a, "--tool=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s tool log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s tool log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s tool log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s tool log: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s tool log [N] [--errors] [--slow <dur>] [--tool <name>] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show recent tool invocations (what the agent ran: tool, input, output, ok/ERROR, latency)\n")
			fmt.Fprintf(stdout, "  --errors      only show failed calls\n")
			fmt.Fprintf(stdout, "  --slow <dur>  only calls at/above this latency (e.g. 500ms, 2s)\n")
			fmt.Fprintf(stdout, "  --tool <name> only calls to this tool\n")
			fmt.Fprintf(stdout, "  --since <dur> only calls in the last <dur>\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s tool log: unexpected arg %q (expected N, --errors, --slow, --tool, --since, --tenant, or --json)\n", brand.CLI, a)
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
	if errorsOnly {
		callArgs["errors"] = true
	}
	if toolFilter != "" {
		callArgs["tool"] = toolFilter
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	if slowMS > 0 {
		callArgs["slow_ms"] = slowMS
	}
	if tenant != "" {
		callArgs["tenant"] = tenant
	}
	res, err := c.Call(ctx, controlplane.CmdToolLog, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s tool log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	invs, _ := res["invocations"].([]any)
	if len(invs) == 0 {
		switch {
		case errorsOnly:
			fmt.Fprintf(stdout, "no failed tool calls.\n")
		case slowMS > 0:
			fmt.Fprintf(stdout, "no tool calls at/above that latency.\n")
		default:
			fmt.Fprintf(stdout, "no tool invocations journaled yet.\n")
		}
		return 0
	}
	for _, item := range invs {
		m, _ := item.(map[string]any)
		tool, _ := m["tool"].(string)
		output, _ := m["output"].(string)
		isErr, _ := m["error"].(bool)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		dur := int64(0)
		if f, ok := m["duration_ms"].(float64); ok {
			dur = int64(f)
		}
		verdict := "ok"
		if isErr {
			verdict = "ERROR"
		}
		whenStr := "—"
		if ts > 0 {
			whenStr = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		durStr := "    —"
		if dur > 0 {
			durStr = fmt.Sprintf("%5dms", dur)
		}
		line := fmt.Sprintf("  %s  %-5s %s  %-16s", whenStr, verdict, durStr, tool)
		if output != "" {
			line += "  " + output
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

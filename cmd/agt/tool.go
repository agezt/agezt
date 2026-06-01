// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"sort"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdTool dispatches `agt tool <subcommand>`. Currently the only
// subcommand is `list`; left as a dispatcher (vs flattening into
// `agt tool-list`) so future `agt tool invoke <name>` /
// `agt tool describe <name>` slot in without renaming.
func cmdTool(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s tool: subcommand required (list, log, stats)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list":
		return cmdToolList(args[1:], stdout, stderr)
	case "log":
		return cmdToolLog(args[1:], stdout, stderr)
	case "stats":
		return cmdToolStats(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s tool: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}

// cmdToolList implements `agt tool list` and `agt tool list --json`.
// Shows the in-process tools the daemon will advertise to the model.
// First place to look when a model isn't calling a tool the operator
// expected — confirms whether the tool is even registered before
// chasing prompt/schema bugs.
func cmdToolList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s tool list [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list the in-process tools the daemon advertises to the model\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s tool list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s tool list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	rows, _ := res["tools"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no tools registered")
		return 0
	}
	fmt.Fprintf(stdout, "%d tool(s):\n", len(rows))
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		name, _ := r["name"].(string)
		desc, _ := r["description"].(string)
		if desc == "" {
			fmt.Fprintf(stdout, "  %s\n", name)
		} else {
			fmt.Fprintf(stdout, "  %-20s %s\n", name, desc)
		}
	}
	return 0
}

// cmdToolLog implements `agt tool log [N] [--errors] [--tool <name>]
// [--since <dur>] [--tenant <id>] [--json]` — a read-only audit of recent tool
// invocations (M66). `tool list` shows the tools advertised; this shows the
// calls the agent actually made and how each turned out. The execution
// analogue of `agt edict log`.
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
		return encodeJSON(stdout, res)
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

	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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

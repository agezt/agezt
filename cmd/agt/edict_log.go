// SPDX-License-Identifier: MIT

package main

// agt edict LOG subcommand: cmdEdictLog (the journaled-policy-events log
// reader). Carved out of edict.go during the Day 181 god-file split so
// the main file can stay focused on the dispatcher + Mode + Level.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)
func cmdEdictLog(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	deniedOnly := false
	limit := 0
	tenant := ""
	sinceMS := int64(0)
	toolFilter := ""
	capFilter := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--denied":
			deniedOnly = true
		case a == "--tool":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict log: --tool needs a name\n", brand.CLI)
				return 2
			}
			i++
			toolFilter = args[i]
		case strings.HasPrefix(a, "--tool="):
			toolFilter = strings.TrimPrefix(a, "--tool=")
		case a == "--capability" || a == "--cap":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict log: --capability needs a name\n", brand.CLI)
				return 2
			}
			i++
			capFilter = args[i]
		case strings.HasPrefix(a, "--capability="):
			capFilter = strings.TrimPrefix(a, "--capability=")
		case strings.HasPrefix(a, "--cap="):
			capFilter = strings.TrimPrefix(a, "--cap=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s edict log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s edict log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict log: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s edict log [N] [--denied] [--tool <name>] [--capability <cap>] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show recent policy decisions (every tool-call gating: tool, capability, allow/deny, reason)\n")
			fmt.Fprintf(stdout, "  --denied           only show denials\n")
			fmt.Fprintf(stdout, "  --tool <name>      only decisions for this tool\n")
			fmt.Fprintf(stdout, "  --capability <cap> only decisions for this capability (alias --cap)\n")
			fmt.Fprintf(stdout, "  --since <dur>      only decisions in the last <dur>\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s edict log: unexpected arg %q (expected N, --denied, --tool, --capability, --since, --tenant, or --json)\n", brand.CLI, a)
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
	if deniedOnly {
		callArgs["denied"] = true
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS // M65: time window
	}
	if toolFilter != "" {
		callArgs["tool"] = toolFilter // M74
	}
	if capFilter != "" {
		callArgs["capability"] = capFilter // M74
	}
	if tenant != "" {
		callArgs["tenant"] = tenant
	}
	res, err := c.Call(ctx, controlplane.CmdEdictLog, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s edict log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	decisions, _ := res["decisions"].([]any)
	if len(decisions) == 0 {
		if deniedOnly {
			fmt.Fprintf(stdout, "no denied policy decisions.\n")
		} else {
			fmt.Fprintf(stdout, "no policy decisions journaled yet.\n")
		}
		return 0
	}
	for _, item := range decisions {
		m, _ := item.(map[string]any)
		tool, _ := m["tool"].(string)
		capability, _ := m["capability"].(string)
		reason, _ := m["reason"].(string)
		allow, _ := m["allow"].(bool)
		hard, _ := m["hard_denied"].(bool)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		verdict := "allow"
		if !allow {
			verdict = "DENY"
			if hard {
				verdict = "DENY(hard)"
			}
		}
		whenStr := "—"
		if ts > 0 {
			whenStr = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		line := fmt.Sprintf("  %s  %-10s %-12s %s", whenStr, verdict, capability, tool)
		if reason != "" {
			line += "  (" + reason + ")"
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// cmdEdictStats implements `agt edict stats [--since <dur>] [--tenant <id>]
// [--json]` — a policy-decision aggregate (denial rate + denied-by-capability),
// the security-dashboard analogue of `agt runs stats` (M64).

// SPDX-License-Identifier: MIT

package main

// agt edict STATS subcommand: cmdEdictStats (the policy-stats aggregator).
// Carved out of edict.go during the Day 181 god-file split so the main
// file can stay focused on the dispatcher + Mode + Level.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)
func cmdEdictStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	tenant := ""
	sinceMS := int64(0)
	sinceLabel := ""
	toolFilter := ""
	capFilter := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tool":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict stats: --tool needs a name\n", brand.CLI)
				return 2
			}
			i++
			toolFilter = args[i]
		case strings.HasPrefix(a, "--tool="):
			toolFilter = strings.TrimPrefix(a, "--tool=")
		case a == "--capability" || a == "--cap":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict stats: --capability needs a name\n", brand.CLI)
				return 2
			}
			i++
			capFilter = args[i]
		case strings.HasPrefix(a, "--capability="):
			capFilter = strings.TrimPrefix(a, "--capability=")
		case strings.HasPrefix(a, "--cap="):
			capFilter = strings.TrimPrefix(a, "--cap=")
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict stats: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s edict stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s edict stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s edict stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s edict stats [--tool <name>] [--capability <cap>] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate policy decisions: total, allowed, denied (rate), denied-by-capability\n")
			fmt.Fprintf(stdout, "  --tool <name>      scope to one tool\n")
			fmt.Fprintf(stdout, "  --capability <cap> scope to one capability (alias --cap)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s edict stats: unexpected arg %q\n", brand.CLI, a)
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
	if tenant != "" {
		callArgs["tenant"] = tenant
	}
	if toolFilter != "" {
		callArgs["tool"] = toolFilter // M76
	}
	if capFilter != "" {
		callArgs["capability"] = capFilter // M76
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdEdictStats, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s edict stats: %v\n", brand.CLI, err)
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
		fmt.Fprintf(stdout, "no policy decisions%s.\n", windowSuffix)
		return 0
	}
	allowed := intOfStatus(res["allowed"])
	denied := intOfStatus(res["denied"])
	hard := intOfStatus(res["hard_denied"])
	rate, _ := res["denial_rate"].(float64)
	fmt.Fprintf(stdout, "policy decisions (over %d%s):\n\n", total, windowSuffix)
	fmt.Fprintf(stdout, "  allowed   : %d\n", allowed)
	fmt.Fprintf(stdout, "  denied    : %d (hard %d)\n", denied, hard)
	fmt.Fprintf(stdout, "  denial    : %.1f%%\n", rate*100)
	if byCap, _ := res["denied_by_capability"].(map[string]any); len(byCap) > 0 {
		fmt.Fprintf(stdout, "\n  denied by capability:\n")
		// Deterministic order by capability name.
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

// cmdEdictMode implements `agt edict mode <allow|deny|prompt> [--json]`.
// Changes the engine-wide approval mode on the running daemon (M21): how
// Ask-class levels (L1..L3) are folded — allow (fold to allow + journal
// note), deny (strict; only L4 runs), or prompt (block for live HITL).
// The hard-deny floor is unaffected. Journaled as a policy.changed event.

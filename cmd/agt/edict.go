// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

// cmdEdict dispatches `agt edict <subcommand>`. The only
// subcommand today is `show`; left as a dispatcher so future
// additions (`agt edict explain <capability>`, `agt edict
// test <input>`) slot in without renaming.
func cmdEdict(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s edict: subcommand required (show|test|deny)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "show":
		return cmdEdictShow(args[1:], stdout, stderr)
	case "overlay":
		return cmdEdictOverlay(args[1:], stdout, stderr)
	case "compact":
		return cmdEdictCompact(args[1:], stdout, stderr)
	case "test":
		return cmdEdictTest(args[1:], stdout, stderr)
	case "deny":
		return cmdEdictDeny(args[1:], stdout, stderr)
	case "level":
		return cmdEdictLevel(args[1:], stdout, stderr)
	case "mode":
		return cmdEdictMode(args[1:], stdout, stderr)
	case "log":
		return cmdEdictLog(args[1:], stdout, stderr)
	case "stats":
		return cmdEdictStats(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s edict <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  show [--json]                          display loaded policies\n")
		fmt.Fprintf(stdout, "  overlay [--json]                       net runtime policy overlay (level/mode/deny changes in effect)\n")
		fmt.Fprintf(stdout, "  compact [--json]                       snapshot the overlay so boot replays it + later changes only\n")
		fmt.Fprintf(stdout, "  log [N] [--denied] [--json]            recent policy decisions (allow/deny audit)\n")
		fmt.Fprintf(stdout, "  stats [--since <dur>] [--json]         policy-decision aggregate (denial rate, by capability)\n")
		fmt.Fprintf(stdout, "  test <capability> [<input>] [--json]   dry-run a decision; no side effects\n")
		fmt.Fprintf(stdout, "  deny list|add|rm ...                   manage hard-deny rules at runtime\n")
		fmt.Fprintf(stdout, "  level <capability> <level> [--json]    set a capability's trust level at runtime\n")
		fmt.Fprintf(stdout, "  mode <allow|deny|prompt> [--json]      set the approval mode at runtime\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s edict: unknown subcommand %q (show|test|deny|level|mode|log)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdEdictLog implements `agt edict log [N] [--denied] [--tenant <id>] [--json]`
// — a read-only audit of recent policy.decision events (M63). `edict show` lists
// the rules; this lists the decisions they produced.
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
func cmdEdictMode(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	var mode string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict mode <allow|deny|prompt> [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "allow = fold Ask to allow; deny = strict (only L4); prompt = live HITL\n")
			return 0
		default:
			if mode == "" {
				mode = a
				continue
			}
			fmt.Fprintf(stderr, "%s edict mode: unexpected arg %q (mode already set)\n", brand.CLI, a)
			return 2
		}
	}
	if mode == "" {
		fmt.Fprintf(stderr, "%s edict mode: mode required (allow|deny|prompt)\n", brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictSetMode, withTenant(tenant, map[string]any{"mode": mode}))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict mode: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	from, _ := res["from"].(string)
	to, _ := res["to"].(string)
	fmt.Fprintf(stdout, "approval mode: %s → %s\n", from, to)
	return 0
}

// cmdEdictLevel implements `agt edict level <capability> <level> [--json]`.
// Changes a capability's trust level on the running daemon (M19). The
// hard-deny floor still fires regardless of level, so loosening a level
// can't unlock a catastrophic command. The change is journaled as a
// policy.changed event. Use `edict show` to read the current ladder.
func cmdEdictLevel(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	var capability, level string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict level <capability> <level> [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "level is L0..L4 or deny/ask/askfirst/askscoped/allow\n")
			fmt.Fprintf(stdout, "the hard-deny floor still applies regardless of level\n")
			return 0
		default:
			if capability == "" {
				capability = a
				continue
			}
			if level == "" {
				level = a
				continue
			}
			fmt.Fprintf(stderr, "%s edict level: unexpected arg %q (capability and level already set)\n", brand.CLI, a)
			return 2
		}
	}
	if capability == "" || level == "" {
		fmt.Fprintf(stderr, "%s edict level: capability and level required (e.g. `%s edict level shell L4`)\n", brand.CLI, brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictSetLevel, withTenant(tenant, map[string]any{
		"capability": capability,
		"level":      level,
	}))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict level: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	cap, _ := res["capability"].(string)
	from, _ := res["from"].(string)
	to, _ := res["to"].(string)
	fmt.Fprintf(stdout, "%s: %s → %s\n", cap, from, to)
	return 0
}

// cmdEdictDeny dispatches `agt edict deny <list|add|rm>`. Runtime
// management of the hard-deny floor (M18): operators can tighten the
// floor without a restart, but `rm` only touches runtime-added rules —
// the built-in and AGEZT_EDICT_DENY rules stay put. Every add/rm is
// journaled as a policy.changed event.

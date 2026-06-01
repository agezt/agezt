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
		return encodeJSON(stdout, res)
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

	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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

	c := dial(stderr)
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

	c := dial(stderr)
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
func cmdEdictDeny(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s edict deny: subcommand required (list|add|rm)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list":
		return cmdEdictDenyList(args[1:], stdout, stderr)
	case "add":
		return cmdEdictDenyAdd(args[1:], stdout, stderr)
	case "rm", "remove":
		return cmdEdictDenyRemove(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s edict deny <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [--json]            list hard-deny rules (with removable flag)\n")
		fmt.Fprintf(stdout, "  add <rule> [--json]      add a rule: \"substring\" or \"<cap>:substring\"\n")
		fmt.Fprintf(stdout, "  rm <name> [--json]       remove a runtime-added rule by name\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s edict deny: unknown subcommand %q (list|add|rm)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdEdictDenyList implements `agt edict deny list [--json]`.
func cmdEdictDenyList(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict deny list [--tenant <id>] [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s edict deny list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictDenyList, withTenant(tenant, nil))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict deny list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	rules, _ := res["rules"].([]any)
	if len(rules) == 0 {
		fmt.Fprintln(stdout, "hard-deny: (no rules)")
		return 0
	}
	fmt.Fprintf(stdout, "hard-deny rules (%d):\n", len(rules))
	for _, raw := range rules {
		r, _ := raw.(map[string]any)
		name, _ := r["name"].(string)
		sub, _ := r["substring"].(string)
		removable, _ := r["removable"].(bool)
		appliesAny, _ := r["applies_to"].([]any)
		scope := "all capabilities"
		if len(appliesAny) > 0 {
			caps := make([]string, 0, len(appliesAny))
			for _, a := range appliesAny {
				if s, ok := a.(string); ok {
					caps = append(caps, s)
				}
			}
			scope = "caps: " + joinCaps(caps)
		}
		tag := "floor"
		if removable {
			tag = "runtime"
		}
		fmt.Fprintf(stdout, "  %-22s  [%-7s]  match=%q  (%s)\n", name, tag, sub, scope)
	}
	return 0
}

// cmdEdictDenyAdd implements `agt edict deny add <rule> [--json]`.
func cmdEdictDenyAdd(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	var rule string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict deny add <rule> [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "rule is \"substring\" (all caps) or \"<capability>:substring\" (scoped)\n")
			return 0
		default:
			if rule == "" {
				rule = a
				continue
			}
			fmt.Fprintf(stderr, "%s edict deny add: unexpected arg %q (rule already set; quote multi-word rules)\n", brand.CLI, a)
			return 2
		}
	}
	if rule == "" {
		fmt.Fprintf(stderr, "%s edict deny add: rule required (e.g. \"git push\" or \"shell:/etc/shadow\")\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictDenyAdd, withTenant(tenant, map[string]any{"rule": rule}))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict deny add: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	name, _ := res["name"].(string)
	sub, _ := res["substring"].(string)
	count, _ := res["count"].(float64)
	fmt.Fprintf(stdout, "added %s  match=%q  (%d hard-deny rules now)\n", name, sub, int(count))
	return 0
}

// cmdEdictDenyRemove implements `agt edict deny rm <name> [--json]`.
func cmdEdictDenyRemove(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	var name string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict deny rm <name> [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "only runtime-added rules (runtime[N]) can be removed\n")
			return 0
		default:
			if name == "" {
				name = a
				continue
			}
			fmt.Fprintf(stderr, "%s edict deny rm: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if name == "" {
		fmt.Fprintf(stderr, "%s edict deny rm: rule name required (see `%s edict deny list`)\n", brand.CLI, brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictDenyRemove, withTenant(tenant, map[string]any{"name": name}))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict deny rm: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	removed, _ := res["removed"].(bool)
	if !removed {
		fmt.Fprintf(stdout, "no rule named %q\n", name)
		return 3
	}
	count, _ := res["count"].(float64)
	fmt.Fprintf(stdout, "removed %s  (%d hard-deny rules now)\n", name, int(count))
	return 0
}

// cmdEdictShow implements `agt edict show [--json]`. Operators
// debugging "why was my shell call denied?" / "is the daemon
// actually in prompt mode?" — this is where the answer lives.
func cmdEdictShow(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict show [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "display the policy snapshot the daemon's edict engine loaded\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s edict show: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictShow, withTenant(tenant, nil))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict show: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	policy, _ := res["ask_policy"].(string)
	fmt.Fprintf(stdout, "ask_policy: %s\n", policy)

	// Levels block — sorted by capability name (server already
	// sorts the keys but maps lose order on the JSON round-trip).
	levels, _ := res["levels"].(map[string]any)
	if len(levels) > 0 {
		caps := make([]string, 0, len(levels))
		for k := range levels {
			caps = append(caps, k)
		}
		sort.Strings(caps)
		fmt.Fprintln(stdout, "\ncapability levels:")
		for _, c := range caps {
			lvl, _ := levels[c].(string)
			fmt.Fprintf(stdout, "  %-18s %s\n", c, lvl)
		}
	}

	// Hard-deny block.
	rules, _ := res["hard_deny"].([]any)
	if len(rules) == 0 {
		fmt.Fprintln(stdout, "\nhard-deny: (no rules)")
		return 0
	}
	fmt.Fprintf(stdout, "\nhard-deny rules (%d):\n", len(rules))
	for _, raw := range rules {
		r, _ := raw.(map[string]any)
		name, _ := r["name"].(string)
		sub, _ := r["substring"].(string)
		appliesAny, _ := r["applies_to"].([]any)
		scope := "all capabilities"
		if len(appliesAny) > 0 {
			caps := make([]string, 0, len(appliesAny))
			for _, a := range appliesAny {
				if s, ok := a.(string); ok {
					caps = append(caps, s)
				}
			}
			scope = "caps: " + joinCaps(caps)
		}
		fmt.Fprintf(stdout, "  %-22s  match=%q  (%s)\n", name, sub, scope)
	}
	return 0
}

// cmdEdictTest implements `agt edict test <capability> [<input>] [--json]`.
// Dry-runs a policy decision without journaling or consuming
// approval slots. Exit codes:
//
//	0 — decision = allow (including AskAllow folded)
//	3 — decision = deny  (or RequiresApproval, since the call
//	    wouldn't proceed in that state either)
//	1 — daemon/network error
//	2 — usage error
//
// The non-zero "deny" exit (3, not 1) lets CI scripts distinguish
// "policy said no" from "couldn't reach the daemon".
func cmdEdictTest(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	var capability, input string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict test <capability> [<input>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "dry-run a policy decision; never journals, never consumes approval slots\n")
			fmt.Fprintf(stdout, "exit 0 = allow, 3 = deny, 1 = error, 2 = usage\n")
			return 0
		default:
			if capability == "" {
				capability = a
				continue
			}
			if input == "" {
				input = a
				continue
			}
			fmt.Fprintf(stderr, "%s edict test: unexpected arg %q (capability and input already set)\n", brand.CLI, a)
			return 2
		}
	}
	if capability == "" {
		fmt.Fprintf(stderr, "%s edict test: capability required (e.g. shell, file_write, http_post)\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictTest, withTenant(tenant, map[string]any{
		"capability": capability,
		"input":      input,
	}))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict test: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else {
		decision, _ := res["decision"].(string)
		level, _ := res["level"].(string)
		reason, _ := res["reason"].(string)
		hardDenied, _ := res["hard_denied"].(bool)
		hardRule, _ := res["hard_deny_rule"].(string)
		wouldAsk, _ := res["would_ask"].(bool)
		needsApproval, _ := res["requires_approval"].(bool)

		fmt.Fprintf(stdout, "decision : %s (level=%s)\n", decision, level)
		if reason != "" {
			fmt.Fprintf(stdout, "reason   : %s\n", reason)
		}
		if hardDenied {
			fmt.Fprintf(stdout, "hard-deny: %s\n", hardRule)
		}
		if wouldAsk {
			fmt.Fprintf(stdout, "note     : Ask-class — current AskPolicy folded it\n")
		}
		if needsApproval {
			fmt.Fprintf(stdout, "note     : RequiresApproval — runtime would pause for HITL grant\n")
		}
	}

	// Map decision → exit code. RequiresApproval is treated as deny
	// for CI purposes: the call wouldn't proceed without a grant,
	// so a script wanting "is this safe to run?" should fail closed.
	decision, _ := res["decision"].(string)
	needsApproval, _ := res["requires_approval"].(bool)
	if decision == "deny" || needsApproval {
		return 3
	}
	return 0
}

// extractTenantFlag pulls an optional "--tenant <id>" (or "--tenant=<id>")
// out of args, returning the id ("" if absent) and the remaining args. Lets
// every `agt edict` subcommand target a tenant's isolated policy engine
// (M22) without each reimplementing the flag. Place it after the subcommand,
// e.g. `agt edict deny add --tenant acme "shell:kubectl delete"`.
func extractTenantFlag(args []string) (tenant string, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--tenant":
			if i+1 < len(args) {
				tenant = args[i+1]
				i++ // consume the value
			}
		case strings.HasPrefix(a, "--tenant="):
			tenant = a[len("--tenant="):]
		default:
			rest = append(rest, a)
		}
	}
	return tenant, rest
}

// withTenant adds the tenant id to a control-plane args map when non-empty
// (empty routes to the primary kernel server-side). Tolerates a nil map.
func withTenant(tenant string, m map[string]any) map[string]any {
	if tenant == "" {
		return m
	}
	if m == nil {
		m = map[string]any{}
	}
	m["tenant"] = tenant
	return m
}

// joinCaps formats a capability list for the human renderer. Kept
// as a tiny helper so the test suite doesn't have to pull strings
// just for this.
func joinCaps(caps []string) string {
	if len(caps) == 0 {
		return ""
	}
	out := caps[0]
	for _, c := range caps[1:] {
		out += ", " + c
	}
	return out
}

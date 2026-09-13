// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
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

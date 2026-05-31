// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
	case "test":
		return cmdEdictTest(args[1:], stdout, stderr)
	case "deny":
		return cmdEdictDeny(args[1:], stdout, stderr)
	case "level":
		return cmdEdictLevel(args[1:], stdout, stderr)
	case "mode":
		return cmdEdictMode(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s edict <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  show [--json]                          display loaded policies\n")
		fmt.Fprintf(stdout, "  test <capability> [<input>] [--json]   dry-run a decision; no side effects\n")
		fmt.Fprintf(stdout, "  deny list|add|rm ...                   manage hard-deny rules at runtime\n")
		fmt.Fprintf(stdout, "  level <capability> <level> [--json]    set a capability's trust level at runtime\n")
		fmt.Fprintf(stdout, "  mode <allow|deny|prompt> [--json]      set the approval mode at runtime\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s edict: unknown subcommand %q (show|test|deny|level|mode)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdEdictMode implements `agt edict mode <allow|deny|prompt> [--json]`.
// Changes the engine-wide approval mode on the running daemon (M21): how
// Ask-class levels (L1..L3) are folded — allow (fold to allow + journal
// note), deny (strict; only L4 runs), or prompt (block for live HITL).
// The hard-deny floor is unaffected. Journaled as a policy.changed event.
func cmdEdictMode(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var mode string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict mode <allow|deny|prompt> [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdEdictSetMode, map[string]any{"mode": mode})
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
	asJSON := false
	var capability, level string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict level <capability> <level> [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdEdictSetLevel, map[string]any{
		"capability": capability,
		"level":      level,
	})
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
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict deny list [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdEdictDenyList, nil)
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
	asJSON := false
	var rule string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict deny add <rule> [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdEdictDenyAdd, map[string]any{"rule": rule})
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
	asJSON := false
	var name string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict deny rm <name> [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdEdictDenyRemove, map[string]any{"name": name})
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
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict show [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdEdictShow, nil)
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
	asJSON := false
	var capability, input string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict test <capability> [<input>] [--json]\n", brand.CLI)
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
	res, err := c.Call(ctx, controlplane.CmdEdictTest, map[string]any{
		"capability": capability,
		"input":      input,
	})
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

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
		fmt.Fprintf(stderr, "%s edict: subcommand required (show)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "show":
		return cmdEdictShow(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s edict <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  show [--json]   display loaded policies (ask_policy, levels, hard-deny rules)\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s edict: unknown subcommand %q (show)\n", brand.CLI, args[0])
		return 2
	}
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

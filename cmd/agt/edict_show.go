// SPDX-License-Identifier: MIT

package main

// agt edict SHOW + TEST subcommands: cmdEdictShow + cmdEdictTest.
// Carved out of edict_deny.go during the Day 188 god-file split so the
// deny file can stay focused on the deny-list CRUD ops (List/Add/Remove)
// and the helpers file can stay focused on the shared tenant/caps helpers.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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

	c := dialpkg.New(stderr)
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

	c := dialpkg.New(stderr)
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

// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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

	c := dialpkg.New(stderr)
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

	c := dialpkg.New(stderr)
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

	c := dialpkg.New(stderr)
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


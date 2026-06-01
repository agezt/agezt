// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdEdictOverlay implements `agt edict overlay [--tenant <id>] [--json]` (M94)
// — the net durable policy overlay: every runtime level/mode/deny change folded
// into what's actually in effect now. `edict show` = base config; this = runtime
// overrides on top.
func cmdEdictOverlay(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s edict overlay [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the net runtime policy overlay (level/mode/deny changes folded from history)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s edict overlay: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdEdictOverlay, withTenant(tenant, map[string]any{}))
	if err != nil {
		fmt.Fprintf(stderr, "%s edict overlay: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	if empty, _ := res["empty"].(bool); empty {
		fmt.Fprintln(stdout, "no runtime policy overrides (engine runs on its loaded config).")
		return 0
	}
	folded := intOfStatus(res["changes_folded"])
	fmt.Fprintf(stdout, "runtime policy overlay (folded from %d change(s)):\n\n", folded)
	if mode, _ := res["mode"].(string); mode != "" {
		fmt.Fprintf(stdout, "  mode      : %s\n", mode)
	}
	if levels, _ := res["levels"].(map[string]any); len(levels) > 0 {
		fmt.Fprintf(stdout, "  levels:\n")
		names := make([]string, 0, len(levels))
		for n := range levels {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			lvl, _ := levels[n].(string)
			fmt.Fprintf(stdout, "    %-14s %s\n", n, lvl)
		}
	}
	if denies, _ := res["deny_rules"].([]any); len(denies) > 0 {
		fmt.Fprintf(stdout, "  runtime deny rules:\n")
		for _, raw := range denies {
			m, _ := raw.(map[string]any)
			name, _ := m["name"].(string)
			sub, _ := m["substring"].(string)
			fmt.Fprintf(stdout, "    %-14s %q\n", name, sub)
		}
	}
	return 0
}

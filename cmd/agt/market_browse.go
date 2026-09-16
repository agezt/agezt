// SPDX-License-Identifier: MIT
//
// cmd/agt `market` read-only subcommands (cmdMarketList, cmdMarketShow).
// Extracted from market.go during Day 211 god-file refactor (#76).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdMarketList(args []string, query string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s market list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s market list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketList, map[string]any{"query": query})
	if err != nil {
		fmt.Fprintf(stderr, "%s market list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	packs, _ := res["packs"].([]any)
	if len(packs) == 0 {
		fmt.Fprintln(stdout, "no packs")
		return 0
	}
	fmt.Fprintf(stdout, "%d pack(s):\n", len(packs))
	for _, raw := range packs {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		mark := " "
		if b, _ := p["installed"].(bool); b {
			mark = "✓"
		}
		name, _ := p["name"].(string)
		cat, _ := p["category"].(string)
		desc, _ := p["description"].(string)
		fmt.Fprintf(stdout, "  %s %-22s [%s] %s\n", mark, name, cat, desc)
	}
	return 0
}
func cmdMarketShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var name, marketplace string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market show <pack> [--marketplace m] [--json]\n", brand.CLI)
			return 0
		case a == "--marketplace" && i+1 < len(args):
			i++
			marketplace = args[i]
		case name == "":
			name = a
		default:
			fmt.Fprintf(stderr, "%s market show: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if name == "" {
		fmt.Fprintf(stderr, "%s market show: a pack name is required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketShow, map[string]any{"name": name, "marketplace": marketplace})
	if err != nil {
		fmt.Fprintf(stderr, "%s market show: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	pack, _ := res["pack"].(map[string]any)
	sk, _ := res["skill_count"].(float64)
	mc, _ := res["mcp_count"].(float64)
	tl, _ := res["tool_count"].(float64)
	installed, _ := res["installed"].(bool)
	fmt.Fprintf(stdout, "%s @ %v [%v]\n", name, pack["version"], pack["category"])
	if d, _ := pack["description"].(string); d != "" {
		fmt.Fprintf(stdout, "  %s\n", d)
	}
	fmt.Fprintf(stdout, "  contents: %d skill(s) · %d MCP server(s) · %d tool requirement(s)\n", int(sk), int(mc), int(tl))
	fmt.Fprintf(stdout, "  installed: %v\n", installed)
	// Pre-install security review (informational, default-allow).
	if vet, ok := res["vet"].(map[string]any); ok {
		verdict, _ := vet["verdict"].(string)
		findings, _ := vet["findings"].([]any)
		fmt.Fprintf(stdout, "  security review: %s\n", verdict)
		for _, f := range findings {
			row, _ := f.(map[string]any)
			if row == nil {
				continue
			}
			fmt.Fprintf(stdout, "    [%v] %v: %v\n", row["severity"], row["where"], row["detail"])
		}
	}
	return 0
}

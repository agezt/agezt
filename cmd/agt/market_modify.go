// SPDX-License-Identifier: MIT
//
// cmd/agt `market` write subcommands (cmdMarketInstall, cmdMarketUninstall).
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

func cmdMarketInstall(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var name, marketplace, version string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market install <pack> [--marketplace m] [--version v] [--json]\n", brand.CLI)
			return 0
		case a == "--marketplace" && i+1 < len(args):
			i++
			marketplace = args[i]
		case a == "--version" && i+1 < len(args):
			i++
			version = args[i]
		case name == "":
			name = a
		default:
			fmt.Fprintf(stderr, "%s market install: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if name == "" {
		fmt.Fprintf(stderr, "%s market install: a pack name is required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketInstall, map[string]any{
		"name": name, "marketplace": marketplace, "version": version,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s market install: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	skills, _ := res["skill_ids"].([]any)
	mcps, _ := res["mcp_servers"].([]any)
	tools, _ := res["tool_reqs"].([]any)
	fmt.Fprintf(stdout, "installed %s: %d skill(s), %d MCP server(s), %d tool requirement(s)\n", name, len(skills), len(mcps), len(tools))
	if unsigned, _ := res["unsigned"].(bool); unsigned {
		fmt.Fprintln(stdout, "  ⚠ pack was unsigned (installed anyway)")
	}
	if len(tools) > 0 {
		fmt.Fprintf(stdout, "  tools to install in the Toolbox: ")
		for i, t := range tools {
			if i > 0 {
				fmt.Fprint(stdout, ", ")
			}
			fmt.Fprintf(stdout, "%v", t)
		}
		fmt.Fprintln(stdout)
	}
	return 0
}
func cmdMarketUninstall(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var name string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market uninstall <pack> [--json]\n", brand.CLI)
			return 0
		case name == "":
			name = a
		default:
			fmt.Fprintf(stderr, "%s market uninstall: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if name == "" {
		fmt.Fprintf(stderr, "%s market uninstall: a pack name is required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketUninstall, map[string]any{"name": name})
	if err != nil {
		fmt.Fprintf(stderr, "%s market uninstall: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	fmt.Fprintf(stdout, "uninstalled %s\n", name)
	return 0
}

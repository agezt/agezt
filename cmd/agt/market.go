// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

// cmdMarket dispatches `agt market <subcommand>` — the capability marketplace:
// browse + install packs that bundle skills, MCP servers, and CLI-tool needs.
func cmdMarket(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s market: subcommand required (list|search|show|install|uninstall)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list", "ls":
		return cmdMarketList(args[1:], "", stdout, stderr)
	case "search":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "%s market search: a query is required\n", brand.CLI)
			return 2
		}
		return cmdMarketList(args[2:], args[1], stdout, stderr)
	case "show", "get":
		return cmdMarketShow(args[1:], stdout, stderr)
	case "install":
		return cmdMarketInstall(args[1:], stdout, stderr)
	case "uninstall":
		return cmdMarketUninstall(args[1:], stdout, stderr)
	case "sources":
		return cmdMarketSources(args[1:], stdout, stderr)
	case "add":
		return cmdMarketAddSource(args[1:], stdout, stderr)
	case "remove", "rm":
		return cmdMarketRemoveSource(args[1:], stdout, stderr)
	case "sync":
		return cmdMarketSync(args[1:], stdout, stderr)
	case "validate":
		return cmdMarketValidate(args[1:], stdout, stderr)
	case "publish":
		return cmdMarketPublish(args[1:], stdout, stderr)
	case "keygen":
		return cmdMarketKeygen(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s market <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [--json]                  browse the catalogue + install state\n")
		fmt.Fprintf(stdout, "  search <query> [--json]        filter packs by name/description/tag\n")
		fmt.Fprintf(stdout, "  show <pack> [--json]           a pack's contents (skills, MCP, tools)\n")
		fmt.Fprintf(stdout, "  install <pack> [--marketplace m] [--version v] [--json]   materialize a pack\n")
		fmt.Fprintf(stdout, "  uninstall <pack> [--json]      reverse a pack's footprint\n")
		fmt.Fprintf(stdout, "  sources [--json]               list configured remote marketplaces\n")
		fmt.Fprintf(stdout, "  add <url> [--name n] [--pubkey hex] [--json]   register a remote source\n")
		fmt.Fprintf(stdout, "  remove <source> [--json]       drop a source + its cached catalogue\n")
		fmt.Fprintf(stdout, "  sync [source] [--json]         fetch a source (or all) into the local cache\n")
		fmt.Fprintf(stdout, "  validate <dir>                 compile + validate a pack authoring dir (pack.json)\n")
		fmt.Fprintf(stdout, "  publish <dir> --out <dir> [--key <file>] [--name N]   build (+sign) a pack into a hostable marketplace\n")
		fmt.Fprintf(stdout, "  keygen [--out <prefix>]        generate an Ed25519 signing keypair\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s market: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}

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


// SPDX-License-Identifier: MIT
//
// cmd/agt `market` top-level dispatch (cmdMarket).
// Extracted from market.go during Day 211 god-file refactor (#76).
// Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

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

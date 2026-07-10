// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/market"
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
	}
	fmt.Fprintf(stdout, "uninstalled %s\n", name)
	return 0
}

func cmdMarketSources(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s market sources [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s market sources: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketSources, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s market sources: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	srcs, _ := res["sources"].([]any)
	if len(srcs) == 0 {
		fmt.Fprintln(stdout, "no remote sources — add one with `agt market add <url>`")
		return 0
	}
	fmt.Fprintf(stdout, "%d source(s):\n", len(srcs))
	for _, raw := range srcs {
		s, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := s["name"].(string)
		url, _ := s["url"].(string)
		key := ""
		if pk, _ := s["pubkey"].(string); pk != "" {
			key = " (signed-key pinned)"
		}
		fmt.Fprintf(stdout, "  %-20s %s%s\n", name, url, key)
	}
	return 0
}

func cmdMarketAddSource(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var rawURL, name, pubkey string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market add <url> [--name n] [--pubkey hex] [--json]\n", brand.CLI)
			return 0
		case a == "--name" && i+1 < len(args):
			i++
			name = args[i]
		case a == "--pubkey" && i+1 < len(args):
			i++
			pubkey = args[i]
		case rawURL == "":
			rawURL = a
		default:
			fmt.Fprintf(stderr, "%s market add: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if rawURL == "" {
		fmt.Fprintf(stderr, "%s market add: a marketplace URL is required\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketAddSource, map[string]any{"url": rawURL, "name": name, "pubkey": pubkey})
	if err != nil {
		fmt.Fprintf(stderr, "%s market add: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	sn, _ := res["name"].(string)
	fmt.Fprintf(stdout, "added source %q — now run `%s market sync %s`\n", sn, brand.CLI, sn)
	return 0
}

func cmdMarketRemoveSource(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var name string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market remove <source> [--json]\n", brand.CLI)
			return 0
		case name == "":
			name = a
		default:
			fmt.Fprintf(stderr, "%s market remove: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if name == "" {
		fmt.Fprintf(stderr, "%s market remove: a source name is required\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketRemoveSource, map[string]any{"name": name})
	if err != nil {
		fmt.Fprintf(stderr, "%s market remove: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	if found, _ := res["removed"].(bool); !found {
		fmt.Fprintf(stdout, "no source named %q\n", name)
		return 0
	}
	fmt.Fprintf(stdout, "removed source %q\n", name)
	return 0
}

func cmdMarketSync(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var name string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market sync [source] [--json]\n", brand.CLI)
			return 0
		case name == "":
			name = a
		default:
			fmt.Fprintf(stderr, "%s market sync: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	// A sync hits the network for every configured source; give it room.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMarketSync, map[string]any{"name": name})
	if err != nil {
		fmt.Fprintf(stderr, "%s market sync: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	results, _ := res["results"].([]any)
	for _, raw := range results {
		r, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		src, _ := r["source"].(string)
		packs, _ := r["packs"].(float64)
		fmt.Fprintf(stdout, "synced %s: %d pack(s)\n", src, int(packs))
	}
	if pe, _ := res["partial_error"].(string); pe != "" {
		fmt.Fprintf(stderr, "warning: some sources failed: %s\n", pe)
	}
	if len(results) == 0 {
		fmt.Fprintln(stdout, "no sources synced")
	}
	return 0
}

// cmdMarketValidate compiles a pack authoring dir and reports its contents
// without publishing. Local-only (no daemon).
func cmdMarketValidate(args []string, stdout, stderr io.Writer) int {
	var dir string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market validate <dir>\n", brand.CLI)
			return 0
		case dir == "":
			dir = a
		default:
			fmt.Fprintf(stderr, "%s market validate: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if dir == "" {
		fmt.Fprintf(stderr, "%s market validate: a pack directory is required\n", brand.CLI)
		return 2
	}
	p, err := market.BuildPackFromDir(dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s market validate: %v\n", brand.CLI, err)
		return 1
	}
	skills, mcps, tools := p.Counts()
	hash, _ := p.ContentHash()
	fmt.Fprintf(stdout, "ok: %s@%s — %d skill(s), %d MCP, %d tool req\n", p.Name, p.Version, skills, mcps, tools)
	fmt.Fprintf(stdout, "  sha256 %s\n", hash)
	return 0
}

// cmdMarketPublish builds a pack from <dir>, optionally signs it, and writes it
// into a statically-hostable marketplace (out/packs/<name>.json + out/marketplace.json).
func cmdMarketPublish(args []string, stdout, stderr io.Writer) int {
	var dir, out, keyFile, name string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market publish <dir> --out <dir> [--key <file>] [--name N]\n", brand.CLI)
			return 0
		case a == "--out" && i+1 < len(args):
			i++
			out = args[i]
		case a == "--key" && i+1 < len(args):
			i++
			keyFile = args[i]
		case a == "--name" && i+1 < len(args):
			i++
			name = args[i]
		case dir == "":
			dir = a
		default:
			fmt.Fprintf(stderr, "%s market publish: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if dir == "" || out == "" {
		fmt.Fprintf(stderr, "%s market publish: <dir> and --out <dir> are required\n", brand.CLI)
		return 2
	}
	p, err := market.BuildPackFromDir(dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s market publish: %v\n", brand.CLI, err)
		return 1
	}
	if name == "" {
		name = "local"
	}
	var priv []byte
	if keyFile != "" {
		kb, rerr := os.ReadFile(keyFile)
		if rerr != nil {
			fmt.Fprintf(stderr, "%s market publish: read key: %v\n", brand.CLI, rerr)
			return 1
		}
		pk, perr := market.PrivateKeyFromHex(string(kb))
		if perr != nil {
			fmt.Fprintf(stderr, "%s market publish: %v\n", brand.CLI, perr)
			return 1
		}
		priv = pk
	}
	if err := market.Publish(p, out, name, priv, time.Now().UnixMilli()); err != nil {
		fmt.Fprintf(stderr, "%s market publish: %v\n", brand.CLI, err)
		return 1
	}
	signed := "unsigned"
	if priv != nil {
		signed = "signed"
	}
	fmt.Fprintf(stdout, "published %s@%s (%s) → %s\n", p.Name, p.Version, signed, filepath.Join(out, "marketplace.json"))
	return 0
}

// cmdMarketKeygen writes a fresh Ed25519 keypair: <prefix>.key (private seed
// hex, 0600) and <prefix>.pub (public hex). Pin the .pub on a Source.
func cmdMarketKeygen(args []string, stdout, stderr io.Writer) int {
	prefix := "agezt-market"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market keygen [--out <prefix>]\n", brand.CLI)
			return 0
		case a == "--out" && i+1 < len(args):
			i++
			prefix = args[i]
		default:
			fmt.Fprintf(stderr, "%s market keygen: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	pub, priv, err := market.GenerateKeypair()
	if err != nil {
		fmt.Fprintf(stderr, "%s market keygen: %v\n", brand.CLI, err)
		return 1
	}
	if err := os.WriteFile(prefix+".key", []byte(priv+"\n"), 0o600); err != nil {
		fmt.Fprintf(stderr, "%s market keygen: write key: %v\n", brand.CLI, err)
		return 1
	}
	if err := os.WriteFile(prefix+".pub", []byte(pub+"\n"), 0o644); err != nil {
		fmt.Fprintf(stderr, "%s market keygen: write pub: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s.key (keep secret) and %s.pub\n", prefix, prefix)
	fmt.Fprintf(stdout, "  pubkey: %s\n", pub)
	fmt.Fprintf(stdout, "publish with: %s market publish <dir> --out <dir> --key %s.key\n", brand.CLI, prefix)
	fmt.Fprintf(stdout, "consumers pin it: %s market add <url> --pubkey %s\n", brand.CLI, pub)
	return 0
}

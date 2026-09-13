// SPDX-License-Identifier: MIT

package main

// agt market sources + sync + validate + publish + keygen subcommands:
// cmdMarketSources + cmdMarketAddSource + cmdMarketRemoveSource +
// cmdMarketSync + cmdMarketValidate + cmdMarketPublish + cmdMarketKeygen.
// Carved out of market.go during the Day 175 god-file split so the main
// file can stay focused on the dispatcher + List + Show + Install +
// Uninstall.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/market"
)
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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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

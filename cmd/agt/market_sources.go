// SPDX-License-Identifier: MIT

// Package main: `agt market sources add|remove|sync` — the source management
// ops (add a catalog source / remove one / sync all configured sources).
// Extracted from market_ops.go during the Day-211 god-file split. Public API
// unchanged.
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

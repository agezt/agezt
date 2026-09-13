// SPDX-License-Identifier: MIT

// Package main: `agt market sources` (list) + `agt market validate` — the
// read/inspect ops. Source management (AddSource + RemoveSource + Sync) moved
// to market_sources.go; publish flow (Publish + Keygen) moved to
// market_publish.go. Day-211 god-file split. Public API unchanged.
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

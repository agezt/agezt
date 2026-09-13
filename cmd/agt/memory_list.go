// SPDX-License-Identifier: MIT

package main

// agt memory READ-side subcommands: cmdMemoryList + cmdMemorySearch
// + cmdMemoryGet + cmdMemoryForget. Carved out of memory.go
// during the Day 199 god-file split so the main file can stay
// focused on the dispatcher and the add file can stay focused on
// the write-side ADD subcommand.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/agezt/agezt/internal/brand"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdMemoryList implements `agt memory list [--json]`.
func cmdMemoryList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s memory list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s memory list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	recs, _ := res["records"].([]any)
	if len(recs) == 0 {
		fmt.Fprintln(stdout, "no memory records")
		return 0
	}
	fmt.Fprintf(stdout, "%d record(s):\n", len(recs))
	for _, raw := range recs {
		if r, ok := raw.(map[string]any); ok {
			fmt.Fprintln(stdout, renderRecordLine(r))
		}
	}
	return 0
}

// cmdMemorySearch implements `agt memory search <query> [N] [--json]`.
func cmdMemorySearch(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var query string
	limit := 0
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory search <query> [N] [--json]\n", brand.CLI)
			return 0
		case query == "":
			query = a
		default:
			if n, err := strconv.Atoi(a); err == nil {
				limit = n
				continue
			}
			// Allow multi-word queries: fold extra words into the query.
			query += " " + a
		}
	}
	if query == "" {
		fmt.Fprintf(stderr, "%s memory search: query required\n", brand.CLI)
		return 2
	}
	callArgs := map[string]any{"query": query}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemorySearch, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory search: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	results, _ := res["results"].([]any)
	if len(results) == 0 {
		fmt.Fprintf(stdout, "no records match %q\n", query)
		return 0
	}
	fmt.Fprintf(stdout, "%d match(es) for %q:\n", len(results), query)
	for _, raw := range results {
		m, _ := raw.(map[string]any)
		rec, _ := m["record"].(map[string]any)
		score, _ := m["score"].(float64)
		fmt.Fprintf(stdout, "  (%.2f) %s\n", score, renderRecordLine(rec))
	}
	return 0
}

// cmdMemoryGet implements `agt memory get <id> [--json]`. Exit 3 = absent.
func cmdMemoryGet(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory get <id> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "exit 0 = found, 3 = absent, 1 = error\n")
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s memory get: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s memory get: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s memory get: %v\n", brand.CLI, err)
		return 1
	}
	found, _ := res["found"].(bool)
	if asJSON {
		_ = jsonout.Write(stdout, res)
		if !found {
			return 3
		}
		return 0
	}
	if !found {
		fmt.Fprintf(stderr, "%s memory get: %s not found\n", brand.CLI, id)
		return 3
	}
	rec, _ := res["record"].(map[string]any)
	return jsonout.Write(stdout, rec)
}

// cmdMemoryForget implements `agt memory forget <id> [--json]`.


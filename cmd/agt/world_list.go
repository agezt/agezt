// SPDX-License-Identifier: MIT
//
// cmd/agt `world list` + `world show` subcommands
// (cmdWorldList, cmdWorldShow).
// Extracted from world.go during Day 211 god-file refactor (#80).
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

func cmdWorldList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s world list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s world list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	res := worldCall(controlplane.CmdWorldList, nil, "world list", stdout, stderr, asJSON)
	if res == nil {
		return 1
	}
	if asJSON {
		return 0
	}
	ents, _ := res["entities"].([]any)
	relCount, _ := res["relation_count"].(float64)
	if len(ents) == 0 {
		fmt.Fprintln(stdout, "no entities")
		return 0
	}
	fmt.Fprintf(stdout, "%d %s, %d relation(s):\n", len(ents), plural(len(ents), "entity", "entities"), int(relCount))
	for _, raw := range ents {
		if e, ok := raw.(map[string]any); ok {
			fmt.Fprintln(stdout, renderEntityLine(e))
		}
	}
	return 0
}
func cmdWorldShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s world show <id> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "exit 0 = found, 3 = absent, 1 = error\n")
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s world show: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s world show: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorldGet, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s world show: %v\n", brand.CLI, err)
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
		fmt.Fprintf(stderr, "%s world show: %s not found\n", brand.CLI, id)
		return 3
	}
	ent, _ := res["entity"].(map[string]any)
	return jsonout.Write(stdout, ent)
}

// SPDX-License-Identifier: MIT

package main

// agt world WRITE subcommands: cmdWorldForget, cmdWorldAdd, cmdWorldRelate.
// Carved out of world.go during the Day 153 god-file split so the main
// file can focus on the dispatcher + read-only operations.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)
func cmdWorldForget(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s world forget <id> [--json]\n", brand.CLI)
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s world forget: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s world forget: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorldForget, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s world forget: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	if ok, _ := res["forgotten"].(bool); ok {
		fmt.Fprintf(stdout, "forgot %s\n", id)
	} else {
		fmt.Fprintf(stdout, "no such entity %s\n", id)
	}
	return 0
}

// cmdWorldAdd implements `agt world add <name> [--kind K] [--alias A ...]`.
func cmdWorldAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	kind := ""
	var aliases []string
	var positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s world add <name> [--kind K] [--alias A ...] [--json]\n", brand.CLI)
			return 0
		case a == "--kind":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s world add: --kind needs a value\n", brand.CLI)
				return 2
			}
			i++
			kind = strings.ToLower(args[i])
		case a == "--alias":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s world add: --alias needs a value\n", brand.CLI)
				return 2
			}
			i++
			aliases = append(aliases, args[i])
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		fmt.Fprintf(stderr, "%s world add: expected exactly one <name> (quote multi-word names)\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{"name": positional[0]}
	if kind != "" {
		callArgs["kind"] = kind
	}
	if len(aliases) > 0 {
		as := make([]any, len(aliases))
		for i, a := range aliases {
			as[i] = a
		}
		callArgs["aliases"] = as
	}

	res := worldCall(controlplane.CmdWorldAdd, callArgs, "world add", stdout, stderr, asJSON)
	if res == nil {
		return 1
	}
	if asJSON {
		return 0
	}
	id, _ := res["id"].(string)
	created, _ := res["created"].(bool)
	verb := "reinforced"
	if created {
		verb = "added"
	}
	fmt.Fprintf(stdout, "%s %s\n", verb, id)
	return 0
}

// cmdWorldRelate implements `agt world relate <from> <verb> <to>`.
func cmdWorldRelate(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var positional []string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s world relate <from> <verb> <to> [--json]\n", brand.CLI)
			return 0
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 3 {
		fmt.Fprintf(stderr, "%s world relate: expected <from> <verb> <to>\n", brand.CLI)
		return 2
	}
	callArgs := map[string]any{"from": positional[0], "verb": positional[1], "to": positional[2]}
	res := worldCall(controlplane.CmdWorldRelate, callArgs, "world relate", stdout, stderr, asJSON)
	if res == nil {
		return 1
	}
	if asJSON {
		return 0
	}
	fmt.Fprintf(stdout, "%s %s %s\n", positional[0], positional[1], positional[2])
	return 0
}

// cmdWorldResolve implements `agt world resolve <phrase> [N] [--json]`.

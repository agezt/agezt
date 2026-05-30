// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdWorld dispatches `agt world <subcommand>`. The world model is the
// journaled entity/relation graph the agent resolves references against
// ("the portfolio" → those repos); this is the operator's read/write path.
func cmdWorld(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s world: subcommand required (add|relate|resolve|neighbors|list|show)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "add":
		return cmdWorldAdd(args[1:], stdout, stderr)
	case "relate":
		return cmdWorldRelate(args[1:], stdout, stderr)
	case "resolve":
		return cmdWorldResolve(args[1:], stdout, stderr)
	case "neighbors", "neighbours":
		return cmdWorldNeighbors(args[1:], stdout, stderr)
	case "list", "ls":
		return cmdWorldList(args[1:], stdout, stderr)
	case "show", "get":
		return cmdWorldShow(args[1:], stdout, stderr)
	case "forget":
		return cmdWorldForget(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s world <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  add <name> [--kind K] [--alias A ...] [--json]\n")
		fmt.Fprintf(stdout, "  relate <from> <verb> <to> [--json]\n")
		fmt.Fprintf(stdout, "  resolve <phrase> [N] [--json]\n")
		fmt.Fprintf(stdout, "  neighbors <name> [--json]\n")
		fmt.Fprintf(stdout, "  list [--json]\n")
		fmt.Fprintf(stdout, "  show <id> [--json]     (exit 3 = absent)\n")
		fmt.Fprintf(stdout, "  forget <id> [--json]   tombstone an entity (reversible, journaled)\n")
		fmt.Fprintf(stdout, "kinds: project|repo|person|org|account|device|channel|topic|task\n")
		fmt.Fprintf(stdout, "verbs: owns|depends_on|member_of|prefers|relates_to|assigned_to|derived_from\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s world: unknown subcommand %q (add|relate|resolve|neighbors|list|show|forget)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdWorldForget implements `agt world forget <id> [--json]` — tombstones an
// entity (soft delete; reversible, journaled). Mirrors `memory forget`.
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
func cmdWorldResolve(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var phrase string
	limit := 0
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s world resolve <phrase> [N] [--json]\n", brand.CLI)
			return 0
		case phrase == "":
			phrase = a
		default:
			if n, err := strconv.Atoi(a); err == nil {
				limit = n
				continue
			}
			phrase += " " + a
		}
	}
	if phrase == "" {
		fmt.Fprintf(stderr, "%s world resolve: phrase required\n", brand.CLI)
		return 2
	}
	callArgs := map[string]any{"query": phrase}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	res := worldCall(controlplane.CmdWorldResolve, callArgs, "world resolve", stdout, stderr, asJSON)
	if res == nil {
		return 1
	}
	if asJSON {
		return 0
	}
	results, _ := res["results"].([]any)
	if len(results) == 0 {
		fmt.Fprintf(stdout, "%q resolves to nothing known\n", phrase)
		return 0
	}
	fmt.Fprintf(stdout, "%q resolves to:\n", phrase)
	for _, raw := range results {
		m, _ := raw.(map[string]any)
		ent, _ := m["entity"].(map[string]any)
		score, _ := m["score"].(float64)
		fmt.Fprintf(stdout, "  (%.2f) %s\n", score, renderEntityLine(ent))
	}
	return 0
}

// cmdWorldNeighbors implements `agt world neighbors <name> [--json]`.
func cmdWorldNeighbors(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var name string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s world neighbors <name> [--json]\n", brand.CLI)
			return 0
		case name == "":
			name = a
		default:
			name += " " + a
		}
	}
	if name == "" {
		fmt.Fprintf(stderr, "%s world neighbors: name required\n", brand.CLI)
		return 2
	}
	res := worldCall(controlplane.CmdWorldNeighbors, map[string]any{"query": name}, "world neighbors", stdout, stderr, asJSON)
	if res == nil {
		return 1
	}
	if asJSON {
		return 0
	}
	if found, _ := res["found"].(bool); !found {
		fmt.Fprintf(stdout, "no entity matches %q\n", name)
		return 0
	}
	ent, _ := res["entity"].(map[string]any)
	ns, _ := res["neighbors"].([]any)
	centerName, _ := ent["name"].(string)
	if len(ns) == 0 {
		fmt.Fprintf(stdout, "%s has no known relations\n", centerName)
		return 0
	}
	fmt.Fprintf(stdout, "%s connects to:\n", centerName)
	for _, raw := range ns {
		m, _ := raw.(map[string]any)
		verb, _ := m["verb"].(string)
		outgoing, _ := m["outgoing"].(bool)
		other, _ := m["other"].(map[string]any)
		otherName, _ := other["name"].(string)
		if otherName == "" {
			otherName = "(forgotten)"
		}
		if outgoing {
			fmt.Fprintf(stdout, "  %s %s\n", verb, otherName)
		} else {
			fmt.Fprintf(stdout, "  %s %s (incoming)\n", verb, otherName)
		}
	}
	return 0
}

// cmdWorldList implements `agt world list [--json]`.
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

// cmdWorldShow implements `agt world show <id> [--json]`. Exit 3 = absent.
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
	c := dial(stderr)
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
		_ = encodeJSON(stdout, res)
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
	return encodeJSON(stdout, ent)
}

// worldCall dials the daemon, makes one control-plane call, and on --json
// prints the raw result. Returns the result map, or nil on error (after
// writing the error to stderr). On --json success it has already printed.
func worldCall(cmd string, callArgs map[string]any, label string, stdout, stderr io.Writer, asJSON bool) map[string]any {
	c := dial(stderr)
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, label, err)
		return nil
	}
	if asJSON {
		_ = encodeJSON(stdout, res)
	}
	return res
}

// renderEntityLine formats an entity map (as returned over the wire) into a
// single human-readable line: "<id12> [kind] name (aka ...)".
func renderEntityLine(e map[string]any) string {
	id, _ := e["id"].(string)
	if len(id) > 12 {
		id = id[:12]
	}
	kind, _ := e["kind"].(string)
	name, _ := e["name"].(string)
	line := fmt.Sprintf("  %s [%s] %s", id, kind, name)
	if aliases, ok := e["aliases"].([]any); ok && len(aliases) > 0 {
		parts := make([]string, 0, len(aliases))
		for _, a := range aliases {
			if sv, ok := a.(string); ok {
				parts = append(parts, sv)
			}
		}
		line += " (aka " + strings.Join(parts, ", ") + ")"
	}
	return line
}

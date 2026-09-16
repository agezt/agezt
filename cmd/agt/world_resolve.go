// SPDX-License-Identifier: MIT
//
// cmd/agt `world resolve` + `world neighbors` subcommands
// (cmdWorldResolve, cmdWorldNeighbors).
// Extracted from world.go during Day 211 god-file refactor (#80).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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

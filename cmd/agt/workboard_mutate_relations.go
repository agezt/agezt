// SPDX-License-Identifier: MIT
//
// cmd/agt `workboard` seat/actor/link subcommands (seat, actor, link).
// Extracted from workboard_mutate.go during Day 211 god-file refactor (#96).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdWorkboardSeat(args []string, stdout, stderr io.Writer) int {
	id, seatID, asJSON := "", "", false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard seat: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
			} else if seatID == "" {
				seatID = a
			}
		}
	}
	if id == "" || seatID == "" {
		fmt.Fprintf(stderr, "usage: %s workboard seat <id> <seat>   (seat: default|reader|builder|isolated; \"default\" clears)\n", brand.CLI)
		return 2
	}
	if seatID == "default" || seatID == "none" || seatID == "clear" {
		seatID = ""
	}
	res, code := callWorkboard(controlplane.CmdWorkboardSeat, map[string]any{"id": id, "seat": seatID}, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}
func cmdWorkboardActor(args []string, stdout, stderr io.Writer, name, cmd string) int {
	id, asJSON, callArgs, ok := parseWorkboardIDActorArgs(args, name, "actor", stderr)
	if !ok {
		return 2
	}
	callArgs["id"] = id
	res, code := callWorkboard(cmd, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}
func cmdWorkboardLink(args []string, stdout, stderr io.Writer) int {
	id, asJSON, callArgs, ok := parseWorkboardIDActorArgs(args, "link", "actor", stderr)
	if !ok {
		return 2
	}
	callArgs["id"] = id
	if str(callArgs["type"]) == "" || str(callArgs["target"]) == "" {
		fmt.Fprintf(stderr, "%s workboard link: --type and --target required\n", brand.CLI)
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardLink, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

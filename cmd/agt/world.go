// SPDX-License-Identifier: MIT
//
// cmd/agt `world` top-level dispatch (cmdWorld).
// Extracted from world.go during Day 211 god-file refactor (#80).
// Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

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
	case "log":
		return cmdWorldLog(args[1:], stdout, stderr)
	case "show", "get":
		return cmdWorldShow(args[1:], stdout, stderr)
	case "forget":
		return cmdWorldForget(args[1:], stdout, stderr)
	case "audit":
		return cmdWorldAudit(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s world <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  add <name> [--kind K] [--alias A ...] [--json]\n")
		fmt.Fprintf(stdout, "  relate <from> <verb> <to> [--json]\n")
		fmt.Fprintf(stdout, "  resolve <phrase> [N] [--json]\n")
		fmt.Fprintf(stdout, "  neighbors <name> [--json]\n")
		fmt.Fprintf(stdout, "  list [--json]\n")
		fmt.Fprintf(stdout, "  show <id> [--json]     (exit 3 = absent)\n")
		fmt.Fprintf(stdout, "  forget <id> [--json]   tombstone an entity (reversible, journaled)\n")
		fmt.Fprintf(stdout, "  audit [--json]         world-model health: entity/relation counts, decayed, untyped\n")
		fmt.Fprintf(stdout, "kinds: project|repo|person|org|account|device|channel|topic|task\n")
		fmt.Fprintf(stdout, "verbs: owns|depends_on|member_of|prefers|relates_to|assigned_to|derived_from\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s world: unknown subcommand %q (add|relate|resolve|neighbors|list|show|forget)\n", brand.CLI, args[0])
		return 2
	}
}

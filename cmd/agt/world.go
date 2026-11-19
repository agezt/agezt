package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt `world` top-level dispatch
//             (cmdWorld). Extracted from world.go during Day 211 god-file refactor
//             (#80). Public API unchanged.

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
	if sub, ok := cliWorld.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s world: unknown subcommand %q (add|relate|resolve|neighbors|list|show|forget)\n", brand.CLI, args[0])
	return 2
}

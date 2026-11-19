package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt `journal` subcommand
//             (cmdJournal). Extracted from main_run_modes.go during Day 211 god-file
//             refactor (#95). Public API unchanged.

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

func cmdJournal(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s journal: subcommand required (verify|tail)\n", brand.CLI)
		return 2
	}
	if sub, ok := cliJournal.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s journal: unknown subcommand %q (verify|tail|grep|head|export|import|stats)\n", brand.CLI, args[0])
	return 2
}

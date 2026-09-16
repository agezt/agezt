// SPDX-License-Identifier: MIT
//
// cmd/agt `journal` subcommand (cmdJournal).
// Extracted from main_run_modes.go during Day 211 god-file refactor (#95).
// Public API unchanged.
package main

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
	switch args[0] {
	case "verify":
		return cmdJournalVerify(args[1:], stdout, stderr)
	case "tail":
		return cmdJournalTail(args[1:], stdout, stderr)
	case "grep":
		return cmdJournalGrep(args[1:], stdout, stderr)
	case "head":
		return cmdJournalHead(args[1:], stdout, stderr)
	case "export":
		return cmdJournalExport(args[1:], stdout, stderr)
	case "import":
		return cmdJournalImport(args[1:], stdout, stderr)
	case "stats":
		return cmdJournalStats(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s journal: unknown subcommand %q (verify|tail|grep|head|export|import|stats)\n", brand.CLI, args[0])
		return 2
	}
}

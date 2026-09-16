// SPDX-License-Identifier: MIT
//
// cmd/agt `toolforge` top-level dispatcher (cmdToolforge) + usage helper (toolforgeUsage).
// Extracted from toolforge.go during Day 211 god-file refactor (#97).
// Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

func cmdToolforge(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return toolforgeUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdToolforgeList(args[1:], stdout, stderr)
	case "show":
		return cmdToolforgeShow(args[1:], stdout, stderr)
	case "draft", "add", "create":
		return cmdToolforgeDraft(args[1:], stdout, stderr)
	case "edit", "set":
		return cmdToolforgeEdit(args[1:], stdout, stderr)
	case "test":
		return cmdToolforgeTest(args[1:], stdout, stderr)
	case "promote":
		return cmdToolforgePromote(args[1:], stdout, stderr)
	case "quarantine":
		return cmdToolforgeQuarantine(args[1:], stdout, stderr)
	case "remove", "rm":
		return cmdToolforgeRemove(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return toolforgeUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s toolforge: unknown subcommand %q\n", brand.CLI, args[0])
		return toolforgeUsage(stderr)
	}
}
func toolforgeUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s toolforge <list|show|draft|edit|test|promote|quarantine|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                   show all script tools\n")
	fmt.Fprintf(w, "  show <name|id> [--json]                         one tool's full record (incl. code)\n")
	fmt.Fprintf(w, "  draft <name> --lang L --desc TEXT (--file PATH | --code SRC) [--schema-file PATH]\n")
	fmt.Fprintf(w, "  edit <name|id> [--desc TEXT] [--lang L] [--file PATH | --code SRC] [--schema-file PATH]\n")
	fmt.Fprintf(w, "       (a code change demotes the tool to draft and clears its test record)\n")
	fmt.Fprintf(w, "  test <name|id> [--input JSON]                   run the code once in the sandbox; promotion requires a pass\n")
	fmt.Fprintf(w, "  promote <name|id>                               make a TESTED tool live (callable as forge_<name>)\n")
	fmt.Fprintf(w, "  quarantine <name|id> [--reason TEXT]            pull a live tool from production (kill switch)\n")
	fmt.Fprintf(w, "  remove <name|id>                                delete a tool\n")
	fmt.Fprintf(w, "script contract: the call's JSON input is in ./stdin.txt; print the result to stdout; exit non-zero on failure\n")
	return 0
}

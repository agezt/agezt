package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt `toolforge` top-level dispatcher
//             (cmdToolforge) + usage helper (toolforgeUsage). Extracted from
//             toolforge.go during Day 211 god-file refactor (#97). Public API
//             unchanged.

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

func cmdToolforge(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return toolforgeUsage(stderr)
	}
	if sub, ok := cliToolforge.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s toolforge: unknown subcommand %q\n", brand.CLI, args[0])
	return toolforgeUsage(stderr)
}
func toolforgeUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s toolforge %s\n", brand.CLI, cliToolforge.synopsis())
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

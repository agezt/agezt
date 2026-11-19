package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt `market` top-level dispatch
//             (cmdMarket). Extracted from market.go during Day 211 god-file refactor
//             (#76). Public API unchanged.

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

func cmdMarket(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s market: subcommand required (list|search|show|install|uninstall)\n", brand.CLI)
		return 2
	}
	if sub, ok := cliMarket.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s market: unknown subcommand %q\n", brand.CLI, args[0])
	return 2
}

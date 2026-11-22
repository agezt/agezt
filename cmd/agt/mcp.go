package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt `mcp` top-level dispatch
//             (cmdMCP). Extracted from mcp.go during Day 211 god-file refactor
//             (#77). Public API unchanged.

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

func cmdMCP(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return mcpUsage(stderr)
	}
	if sub, ok := cliMCP.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s mcp: unknown subcommand %q\n", brand.CLI, args[0])
	return mcpUsage(stderr)
}

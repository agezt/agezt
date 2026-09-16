// SPDX-License-Identifier: MIT
//
// cmd/agt `mcp` top-level dispatch (cmdMCP).
// Extracted from mcp.go during Day 211 god-file refactor (#77).
// Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

func cmdMCP(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return mcpUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdMCPList(args[1:], stdout, stderr)
	case "add", "register":
		return cmdMCPAdd(args[1:], stdout, stderr)
	case "attach":
		return cmdMCPRefAction(args[1:], stdout, stderr, "attach")
	case "detach":
		return cmdMCPRefAction(args[1:], stdout, stderr, "detach")
	case "enable":
		return cmdMCPSetEnabled(args[1:], stdout, stderr, true)
	case "disable":
		return cmdMCPSetEnabled(args[1:], stdout, stderr, false)
	case "remove", "rm":
		return cmdMCPRefAction(args[1:], stdout, stderr, "remove")
	case "-h", "--help", "help":
		return mcpUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s mcp: unknown subcommand %q\n", brand.CLI, args[0])
		return mcpUsage(stderr)
	}
}

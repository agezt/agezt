// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

// cmdMemory dispatches `agt memory <subcommand>`. Memory-lite is the
// content-addressed, journaled knowledge store the agent reads as injected
// context; this is the operator's read/write path into it.
func cmdMemory(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s memory: subcommand required (add|list|log|search|get|forget|audit|clean|consolidate|profile|prune|bulk-forget|find-related)\n", brand.CLI)
		return 2
	}
	if sub, ok := cliMemory.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s memory: unknown subcommand %q (add|list|log|search|get|forget|promote|audit|clean|consolidate|profile|prune|bulk-forget|find-related)\n", brand.CLI, args[0])
	return 2
}

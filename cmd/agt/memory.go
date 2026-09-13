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
	switch args[0] {
	case "add":
		return cmdMemoryAdd(args[1:], stdout, stderr)
	case "list", "ls":
		return cmdMemoryList(args[1:], stdout, stderr)
	case "log":
		return cmdMemoryLog(args[1:], stdout, stderr)
	case "search":
		return cmdMemorySearch(args[1:], stdout, stderr)
	case "get":
		return cmdMemoryGet(args[1:], stdout, stderr)
	case "forget", "rm":
		return cmdMemoryForget(args[1:], stdout, stderr)
	case "promote":
		return cmdMemoryPromote(args[1:], stdout, stderr)
	case "audit":
		return cmdMemoryAudit(args[1:], stdout, stderr)
	case "clean":
		return cmdMemoryClean(args[1:], stdout, stderr)
	case "consolidate":
		return cmdMemoryConsolidate(args[1:], stdout, stderr)
	case "profile":
		return cmdMemoryProfile(args[1:], stdout, stderr)
	case "prune":
		return cmdMemoryPrune(args[1:], stdout, stderr)
	case "bulk-forget":
		return cmdMemoryBulkForget(args[1:], stdout, stderr)
	case "find-related":
		return cmdMemoryFindRelated(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s memory <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  add <subject> <content> [--type T] [--tag k=v] [--conf F] [--json]\n")
		fmt.Fprintf(stdout, "  list [--json]\n")
		fmt.Fprintf(stdout, "  log [N] [--op written|forgotten|superseded|promoted] [--since <dur>] [--json]\n")
		fmt.Fprintf(stdout, "  search <query> [N] [--json]\n")
		fmt.Fprintf(stdout, "  get <id> [--json]      (exit 3 = absent)\n")
		fmt.Fprintf(stdout, "  forget <id> [--json]\n")
		fmt.Fprintf(stdout, "  promote <id> [--json]  share a private (agent-scoped) record with every agent\n")
		fmt.Fprintf(stdout, "  audit [--json]         report expired, suspended, and competing memories\n")
		fmt.Fprintf(stdout, "  clean [--execute]      hard-delete low-value log/transient memory records (dry-run by default)\n")
		fmt.Fprintf(stdout, "  consolidate [--json]   one brain-distillation pass: merge related records, supersede originals\n")
		fmt.Fprintf(stdout, "  profile [--json]       rebuild the operator profile from accumulated memory (M1000)\n")
		fmt.Fprintf(stdout, "  prune [--days N] [--execute]   hard-delete soft-deleted records (dry-run by default)\n")
		fmt.Fprintf(stdout, "  bulk-forget <id>...    soft-delete multiple records in one call\n")
		fmt.Fprintf(stdout, "  find-related --id <id> [--limit N] [--json]\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s memory: unknown subcommand %q (add|list|log|search|get|forget|promote|audit|clean|consolidate|profile|prune|bulk-forget|find-related)\n", brand.CLI, args[0])
		return 2
	}
}


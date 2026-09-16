// SPDX-License-Identifier: MIT

// agent_lifecycle_helpers.go owns the three pure helpers
// used by `agt agent remove`:
//   - agentRemoveResultSummary  (renders the daemon result map
//     as a single human-readable line)
//   - buildAgentRemovePayload    (parses argv into the ref +
//     flags the daemon expects + reports usage on failure)
//   - printAgentRemoveUsage      (the `--with-*` flag help text)
// The five top-level subcommand funcs (Tombstone /
// Graveyard / Retire / Revive / Remove) live in
// agent_lifecycle.go.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func agentRemoveResultSummary(res map[string]any) string {
	parts := []string{}
	add := func(key, label string) {
		n := intNumber(res[key])
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add("standing_removed", "standing")
	add("schedules_removed", "schedule")
	add("memories_forgotten", "private memory")
	add("authored_memories_forgotten", "authored shared memory")
	add("skills_archived", "skill archived")
	add("configs_deleted", "config deleted")
	add("workspaces_deleted", "workspace deleted")
	add("subagents_retired", "subagent retired")
	add("mailbox_messages_retained", "mailbox/audit retained")
	return strings.Join(parts, ", ")
}

func buildAgentRemovePayload(args []string, stderr io.Writer) (string, map[string]any, bool, bool) {
	ref := ""
	cascade := map[string]any{}
	for _, a := range args {
		switch a {
		case "--with-all":
			cascade["standing"] = true
			cascade["schedules"] = true
			cascade["memory"] = true
			cascade["authored_memory"] = true
			cascade["skills"] = true
			cascade["config"] = true
			cascade["workspace"] = true
			cascade["subagents"] = true
		case "--with-standing":
			cascade["standing"] = true
		case "--with-schedules":
			cascade["schedules"] = true
		case "--with-memory":
			cascade["memory"] = true
		case "--with-authored-memory", "--with-authored-shared-memory":
			cascade["authored_memory"] = true
		case "--with-skills":
			cascade["skills"] = true
		case "--with-config":
			cascade["config"] = true
		case "--with-workspace", "--with-workdir":
			cascade["workspace"] = true
		case "--with-subagents":
			cascade["subagents"] = true
		case "-h", "--help":
			return "", nil, true, false
		default:
			if strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "%s agent remove: unknown flag %s\n", brand.CLI, a)
				return "", nil, false, false
			}
			if ref != "" {
				printAgentRemoveUsage(stderr)
				return "", nil, false, false
			}
			ref = a
		}
	}
	if ref == "" {
		printAgentRemoveUsage(stderr)
		return "", nil, false, false
	}
	return ref, cascade, false, true
}

func printAgentRemoveUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s agent remove <slug|id> [--with-all|--with-schedules|--with-memory|--with-authored-memory|--with-skills|--with-config|--with-workspace|--with-standing|--with-subagents]\n", brand.CLI)
	fmt.Fprintf(w, "  --with-memory cleans private memory; --with-authored-memory cleans shared memory records written by the agent\n")
}

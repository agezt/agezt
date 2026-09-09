// SPDX-License-Identifier: MIT

package main

func AllCommands() []*Command {
	seen := make(map[string]bool)
	var cmds []*Command
	for _, cmd := range CommandRegistry {
		if !seen[cmd.Name] {
			cmds = append(cmds, cmd)
			seen[cmd.Name] = true
		}
	}
	return cmds
}

// lookup is the test-only map-backed lookup kept for
// TestLookupAndRegister (coverage_help_test.go). The dispatcher's
// hot path uses registry.Execute (see commands.go) so production
// code does not go through this function. Lives in a _test.go
// file because it has no binary caller (the deadcodecheck gate
// surfaces symbols with no binary reachability, and the rule
// for same-package test-only helpers is to keep them in _test.go
// per tools/deadcodecheck/main.go:117-119).
func lookup(name string) *Command {
	return CommandRegistry[name]
}

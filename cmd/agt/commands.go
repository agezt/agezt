// SPDX-License-Identifier: MIT

package main

// Package documentation lives in doc.go.

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/cmd/agt/router"
	"github.com/agezt/agezt/internal/brand"
)

// Command is the public command type. Aliased to router.Command so
// any field on router.Command (Name, Aliases, Description, HelpLong,
// HelpHandler, Run) is reachable from the same struct literal in
// cmd_register.go's Register(&Command{...}) calls.
type Command = router.Command

// CommandRegistry is the parallel index used by test helpers
// (AllCommands, help_test.go's coverage sync check) to iterate
// the registered command set. The dispatcher (registry below)
// is the actual lookup source; Register() populates both.
var CommandRegistry = map[string]*Command{}

// registry is the actual dispatcher. Populated by Register() in
// parallel with CommandRegistry; consulted by ExecuteCommand. The
// router.Registry type's `Execute` method handles the unknown-
// command "did you mean" branch via `Suggest` (Levenshtein, ≤2).
var registry = router.NewRegistry()

// Register adds a command to both the parallel index (for test
// helpers) and the live dispatcher (for ExecuteCommand). Aliases
// are registered to point to the same command in both. Matches
// the pre-router behaviour: re-registration of a name wins.
func Register(cmd *Command) {
	if cmd == nil || cmd.Name == "" {
		return
	}
	CommandRegistry[cmd.Name] = cmd
	registry.Register(cmd)
	for _, alias := range cmd.Aliases {
		CommandRegistry[alias] = cmd
	}
}

// ExecuteCommand looks up and runs a command by name. Delegates
// to router.Registry.Execute (Day 31 wiring); the inline
// implementation that lived here before the router extraction
// was the duplicate this layer replaced.
func ExecuteCommand(name string, args []string, stdout, stderr io.Writer) int {
	code := registry.Execute(name, args, stdout, stderr, fmt.Sprintf("%s: unknown command %q", brand.CLI, name))
	if code == 2 {
		// The inline version's unknown-branch ended with
		// "run `agt help` for the command list" hint. The
		// router's Execute just appends "did you mean" —
		// preserve the post-line help pointer so the user
		// keeps the existing escape hatch.
		fmt.Fprintf(stderr, "\nrun `%s help` for the command list\n", brand.CLI)
	}
	return code
}

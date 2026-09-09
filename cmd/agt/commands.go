// SPDX-License-Identifier: MIT

// Package main provides the command registration system for the Agezt CLI.
// The dispatcher delegates to cmd/agt/router (Day 31 wiring: the inline
// Command + CommandRegistry + ExecuteCommand that lived here since the
// pre-sprint days now thin-wrap the router.Registry type so the package is
// actually exercised by the binary; the previous Day 1-6 extraction left
// it allowlisted-as-dead in tools/deadcodecheck until this slice).
//
// The parallel `CommandRegistry` map exists for test helpers and
// coverage probes that iterate the command list (`commands_test_helpers_test.go`,
// `help_test.go:31` sync-check). Tests keep their existing shape;
// the registry is the actual dispatcher.
package main

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


// SPDX-License-Identifier: MIT

// Package main provides the command registration system for the Agezt CLI.
// This replaces the 50+ case switch statement with a declarative registry pattern,
// making it easier to add, remove, and test commands.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

// Command represents a CLI command with its handler.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Run         func(args []string, stdout, stderr io.Writer) int
	HelpHandler func(args []string, stdout, stderr io.Writer) int
}

// CommandRegistry is the global registry of all agt commands.
// Commands are registered at init() time via Register().
var CommandRegistry = map[string]*Command{}

// Register adds a command to the global registry.
// Aliases are also registered to point to the same command.
func Register(cmd *Command) {
	CommandRegistry[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		CommandRegistry[alias] = cmd
	}
}

// lookup finds a command by name or alias.
func lookup(name string) *Command {
	return CommandRegistry[name]
}

// ExecuteCommand looks up and runs a command by name.
func ExecuteCommand(name string, args []string, stdout, stderr io.Writer) int {
	cmd := lookup(name)
	if cmd == nil {
		// Short and actionable error message (M936)
		fmt.Fprintf(stderr, "%s: unknown command %q", brand.CLI, name)
		if sug := suggestCommands(name); len(sug) > 0 {
			fmt.Fprintf(stderr, " — did you mean %s?", strings.Join(sug, ", "))
		}
		fmt.Fprintf(stderr, "\nrun `%s help` for the command list\n", brand.CLI)
		return 2
	}
	return cmd.Run(args, stdout, stderr)
}

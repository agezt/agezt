// SPDX-License-Identifier: MIT

package main

import "io"

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

func RunHelp(name string, args []string, stdout, stderr io.Writer) int {
	cmd := lookup(name)
	if cmd == nil || cmd.HelpHandler == nil {
		return 2
	}
	return cmd.HelpHandler(args, stdout, stderr)
}

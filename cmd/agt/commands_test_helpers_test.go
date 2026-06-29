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

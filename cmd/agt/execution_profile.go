// SPDX-License-Identifier: MIT
//
// cmd/agt `exec-profile` top-level dispatch (cmdExecProfile).
// Extracted from execution_profile.go during Day 211 god-file refactor (#94).
// Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

func cmdExecProfile(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return cmdExecProfileList(nil, stdout, stderr)
	}
	switch args[0] {
	case "list", "ls":
		return cmdExecProfileList(args[1:], stdout, stderr)
	case "show":
		return cmdExecProfileShow(args[1:], stdout, stderr)
	case "check", "doctor":
		return cmdExecProfileCheck(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s exec-profile: unknown subcommand %q (list|show|check)\n", brand.CLI, args[0])
		return 2
	}
}

// SPDX-License-Identifier: MIT

package main

// agt skill TRANSITION subcommand: cmdSkillTransition (promote/
// quarantine/revert). Carved out of skill.go during the Day 197
// god-file split so the main file can stay focused on the
// dispatcher + List/Show/History and the share file can stay
// focused on the Reassign (share/unshare) subcommand.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
)

// cmdSkillTransition handles the promote/quarantine/revert commands, which all
// take a single <id> and an optional --reason (quarantine).
func cmdSkillTransition(args []string, cmd, label string, stdout, stderr io.Writer) int {
	asJSON := false
	reason := ""
	var id string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			if label == "quarantine" || label == "archive" {
				fmt.Fprintf(stdout, "usage: %s skill %s <id> [--reason R] [--json]\n", brand.CLI, label)
			} else {
				fmt.Fprintf(stdout, "usage: %s skill %s <id> [--json]\n", brand.CLI, label)
			}
			return 0
		case a == "--reason":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill %s: --reason needs a value\n", brand.CLI, label)
				return 2
			}
			i++
			reason = args[i]
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill %s: unexpected arg %q\n", brand.CLI, label, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill %s: id required\n", brand.CLI, label)
		return 2
	}
	callArgs := map[string]any{"id": id}
	if reason != "" {
		callArgs["reason"] = reason
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill %s: %v\n", brand.CLI, label, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	switch label {
	case "promote":
		fmt.Fprintf(stdout, "%s -> %v\n", id, res["status"])
	case "quarantine":
		fmt.Fprintf(stdout, "quarantined %s\n", id)
	case "archive":
		fmt.Fprintf(stdout, "archived %s\n", id)
	case "revert":
		if restored, _ := res["restored"].(string); restored != "" {
			fmt.Fprintf(stdout, "reverted %s (restored %s)\n", id, restored)
		} else {
			fmt.Fprintf(stdout, "reverted %s (archived; no parent to restore)\n", id)
		}
	}
	return 0
}


// SPDX-License-Identifier: MIT

package main

// agt skill SHARE/UNSHARE subcommand: cmdSkillReassign. Carved out
// of skill.go during the Day 197 god-file split so the main file
// can stay focused on the dispatcher + List/Show/History and the
// transition file can stay focused on the Transition
// (promote/quarantine/revert) subcommand.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdSkillReassign handles `skill share <id>` (promote a private skill to the
// shared pool) and `skill reassign <id> --agent <slug>` (change the owning
// agent; --agent "" or omitted shares it). share is the one-arg ownership
// valve that mirrors `memory promote`; reassign is its general form.
func cmdSkillReassign(args []string, share bool, stdout, stderr io.Writer) int {
	label := "reassign"
	if share {
		label = "share"
	}
	asJSON := false
	agent := ""
	var id string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			if share {
				fmt.Fprintf(stdout, "usage: %s skill share <id> [--json]\n", brand.CLI)
			} else {
				fmt.Fprintf(stdout, "usage: %s skill reassign <id> [--agent <slug>] [--json]   (omit --agent to share)\n", brand.CLI)
			}
			return 0
		case a == "--agent" && !share:
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill reassign: --agent needs a value\n", brand.CLI)
				return 2
			}
			i++
			agent = args[i]
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
	cmd := controlplane.CmdSkillReassign
	callArgs := map[string]any{"id": id, "agent": agent}
	if share {
		cmd = controlplane.CmdSkillShare
		callArgs = map[string]any{"id": id}
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
	if agent == "" {
		fmt.Fprintf(stdout, "shared %s with every agent\n", id)
	} else {
		fmt.Fprintf(stdout, "reassigned %s to %s\n", id, agent)
	}
	return 0
}

// renderSkillLine formats a skill map into a single line:
// "<id12> [status] name — description".


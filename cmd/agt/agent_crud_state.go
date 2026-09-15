// SPDX-License-Identifier: MIT
//
// cmd/agt agent enable/disable helper (cmdAgentSetEnabled).
// Extracted from agent_crud.go during Day 211 god-file refactor (#64).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdAgentSetEnabled(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "pause"
	if enabled {
		verb = "resume"
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent %s <slug|id>\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": args[0], "enabled": enabled})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s %sd\n", str(p["slug"]), verb)
	return 0
}

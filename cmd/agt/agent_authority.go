// SPDX-License-Identifier: MIT
//
// cmd/agt `agent authority` + `agent impact` subcommands
// (cmdAgentAuthority, cmdAgentImpact).
// Extracted from agent_authority.go during Day 211 god-file refactor (#85).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdAgentAuthority(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	ref := ""
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--explain":
			// Accepted for documentation parity with docs/COMPARISON.md; the
			// text render IS the explain form, so this is a no-op alias.
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s agent authority <slug|id> [--json] [--explain]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the effective runtime authority for an agent (merged profile + policy)\n")
			return 0
		default:
			if !strings.HasPrefix(a, "--") && ref == "" {
				ref = a
			}
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent authority <slug|id> [--json] [--explain]\n", brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch the agent profile.
	listRes, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent authority: %v\n", brand.CLI, err)
		return 1
	}
	var profile map[string]any
	if profiles, _ := listRes["profiles"].([]any); profiles != nil {
		for _, raw := range profiles {
			p, _ := raw.(map[string]any)
			if p != nil && (str(p["slug"]) == ref || str(p["id"]) == ref) {
				profile = p
				break
			}
		}
	}
	if profile == nil {
		fmt.Fprintf(stderr, "%s agent authority: unknown agent %q\n", brand.CLI, ref)
		return 1
	}

	// Fetch the live Edict policy snapshot (levels + hard-deny + approval mode).
	edictRes, err := c.Call(ctx, controlplane.CmdEdictShow, nil)
	if err != nil {
		// The daemon might be an older version without CmdEdictShow; degrade
		// gracefully and note the policy overlay is unavailable.
		edictRes = map[string]any{}
	}

	// Build the merged authority view.
	authority := buildAgentAuthority(profile, edictRes)

	if asJSON {
		return jsonout.Write(stdout, authority)
	}
	renderAgentAuthority(stdout, authority)
	return 0
}
func cmdAgentImpact(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent impact <slug|id>\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent impact: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s lifecycle impact\n", str(res["slug"]))
	if !printAgentImpactSummary(stdout, res) {
		fmt.Fprintln(stdout, "impact: none")
	}
	return 0
}

// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdReflect dispatches `agt reflect <subcommand>`. Reflection is the
// meta-cognition loop: it reviews the system's own behaviour from the journal,
// decays unused world-model entities, and surfaces advisory proposals.
func cmdReflect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s reflect: subcommand required (run|show)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "run":
		return cmdReflectRun(args[1:], stdout, stderr)
	case "show":
		return cmdReflectShow(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s reflect <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  run [--json]      run a reflection pass now (decays stale world-model entities)\n")
		fmt.Fprintf(stdout, "  show [--json]     print the latest reflection report\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s reflect: unknown subcommand %q (run|show)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdReflectRun(args []string, stdout, stderr io.Writer) int {
	asJSON, code := reflectFlags(args, "run", stdout, stderr)
	if code >= 0 {
		return code
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdReflectRun, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s reflect run: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	renderReport(stdout, res)
	return 0
}

func cmdReflectShow(args []string, stdout, stderr io.Writer) int {
	asJSON, code := reflectFlags(args, "show", stdout, stderr)
	if code >= 0 {
		return code
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdReflectShow, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s reflect show: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	if found, _ := res["found"].(bool); !found {
		fmt.Fprintln(stdout, "no reflection yet — run `agt reflect run`")
		return 0
	}
	report, _ := res["report"].(map[string]any)
	renderReport(stdout, report)
	return 0
}

// reflectFlags parses the shared --json/--help flags. Returns (asJSON, code):
// code >= 0 means "return this exit code now"; code == -1 means "continue".
func reflectFlags(args []string, label string, stdout, stderr io.Writer) (bool, int) {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s reflect %s [--json]\n", brand.CLI, label)
			return false, 0
		default:
			fmt.Fprintf(stderr, "%s reflect %s: unexpected arg %q\n", brand.CLI, label, a)
			return false, 2
		}
	}
	return asJSON, -1
}

// renderReport prints a reflection report map (as returned over the wire).
func renderReport(w io.Writer, rep map[string]any) {
	obs, _ := rep["observations"].(map[string]any)
	num := func(m map[string]any, k string) int {
		if v, ok := m[k].(float64); ok {
			return int(v)
		}
		return 0
	}
	fmt.Fprintln(w, "reflection report:")
	if obs != nil {
		fmt.Fprintf(w, "  tasks    : %d started, %d completed, %d failed\n",
			num(obs, "tasks_started"), num(obs, "tasks_completed"), num(obs, "tasks_failed"))
		fmt.Fprintf(w, "  pulse    : %d briefs sent\n", num(obs, "briefs_sent"))
		fmt.Fprintf(w, "  skills   : %d activations\n", num(obs, "skills_activated"))
		fmt.Fprintf(w, "  approvals: %d granted, %d denied\n",
			num(obs, "approvals_granted"), num(obs, "approvals_denied"))
		fmt.Fprintf(w, "  world    : %d entities\n", num(obs, "entities_total"))
	}
	fmt.Fprintf(w, "  decayed  : %d stale entities\n", num(rep, "entities_decayed"))
	proposals, _ := rep["proposals"].([]any)
	if len(proposals) == 0 {
		fmt.Fprintln(w, "  proposals: none")
		return
	}
	fmt.Fprintf(w, "  proposals: %d\n", len(proposals))
	for _, raw := range proposals {
		p, _ := raw.(map[string]any)
		area, _ := p["area"].(string)
		obsv, _ := p["observation"].(string)
		sug, _ := p["suggestion"].(string)
		fmt.Fprintf(w, "    [%s] %s → %s\n", area, obsv, sug)
	}
}

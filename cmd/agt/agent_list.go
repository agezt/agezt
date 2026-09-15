// SPDX-License-Identifier: MIT
//
// cmd/agt agent list sub-command handler (cmdAgentList).
// Extracted from agent.go during Day 211 god-file refactor (#60).
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
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdAgentList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	profiles, _ := res["profiles"].([]any)
	if len(profiles) == 0 {
		fmt.Fprintf(stdout, "no agents yet — create one with `%s agent add <slug> --soul \"...\"`\n", brand.CLI)
		return 0
	}
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if p == nil {
			continue
		}
		state := agentListStateLabel(p)
		model, _ := p["model"].(string)
		if model == "" {
			model = "(default)"
		}
		kind := str(p["kind"])
		if kind == "" {
			kind = "custom"
		}
		fmt.Fprintf(stdout, "%-20s %-9s %-8s model=%s", str(p["slug"]), state, kind, model)
		if tt, _ := p["task_type"].(string); tt != "" {
			fmt.Fprintf(stdout, " task=%s", tt)
		}
		if parent := str(p["parent_agent"]); parent != "" {
			fmt.Fprintf(stdout, " parent=%s", parent)
		} else if owner := str(p["owner_agent"]); owner != "" {
			fmt.Fprintf(stdout, " owner=%s", owner)
		}
		if mc, ok := p["max_cost_mc"].(float64); ok && mc > 0 {
			fmt.Fprintf(stdout, " max-cost=%s", fmtUSD(int64(mc)))
		}
		if suffix := agentListStatusSuffix(p); suffix != "" {
			fmt.Fprintf(stdout, " %s", suffix)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v agent(s)\n", res["count"])
	return 0
}

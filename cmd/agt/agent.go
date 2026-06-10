// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdAgent dispatches `agt agent <subcommand>` — the management surface for the
// durable agent roster (M783): named agent identities with their own soul
// (system prompt), model, per-run cost ceiling, memory scope, and workdir.
// `agt run --agent <slug>` runs AS one. Every mutation is journaled (roster.*).
func cmdAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return agentUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdAgentList(args[1:], stdout, stderr)
	case "add", "create":
		return cmdAgentAdd(args[1:], stdout, stderr)
	case "show":
		return cmdAgentShow(args[1:], stdout, stderr)
	case "set", "edit":
		return cmdAgentSet(args[1:], stdout, stderr)
	case "pause":
		return cmdAgentSetEnabled(args[1:], stdout, stderr, false)
	case "resume":
		return cmdAgentSetEnabled(args[1:], stdout, stderr, true)
	case "remove", "rm":
		return cmdAgentRemove(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return agentUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s agent: unknown subcommand %q\n", brand.CLI, args[0])
		return agentUsage(stderr)
	}
}

func agentUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s agent <list|add|show|set|pause|resume|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                  show the agent roster\n")
	fmt.Fprintf(w, "  add <slug> [--name N] [--soul PROMPT] [--model M] [--fallbacks m1,m2]\n")
	fmt.Fprintf(w, "      [--task TYPE] [--max-cost USD] [--max-daily USD] [--memory-scope S] [--workdir DIR] [--desc TEXT]\n")
	fmt.Fprintf(w, "  show <slug|id> [--json]                        one agent's full profile\n")
	fmt.Fprintf(w, "  set <slug|id> [same flags as add]              edit an agent (slug is immutable)\n")
	fmt.Fprintf(w, "  pause <slug|id>                                disable an agent (runs refused)\n")
	fmt.Fprintf(w, "  resume <slug|id>                               re-enable an agent\n")
	fmt.Fprintf(w, "  remove <slug|id>                               delete an agent\n")
	fmt.Fprintf(w, "run as an agent:  %s run --agent <slug> \"intent\"\n", brand.CLI)
	return 0
}

// agentFlags parses the shared add/set flag set. Returns the profile fields
// that were explicitly provided (set tracks which, for partial edits).
type agentFlags struct {
	name, soul, model, task, memScope, workdir, desc string
	fallbacks                                        []string
	maxCostMc, maxDailyMc                            int64
	set                                              map[string]bool
}

func parseAgentFlags(args []string, stderr io.Writer, cmd string) (agentFlags, []string, bool) {
	f := agentFlags{set: map[string]bool{}}
	var rest []string
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent %s: %s needs a value\n", brand.CLI, cmd, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--name":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.name, f.set["name"] = args[i], true
		case "--soul":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.soul, f.set["soul"] = args[i], true
		case "--model":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.model, f.set["model"] = args[i], true
		case "--fallbacks":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			for m := range strings.SplitSeq(args[i], ",") {
				if m = strings.TrimSpace(m); m != "" {
					f.fallbacks = append(f.fallbacks, m)
				}
			}
			f.set["fallbacks"] = true
		case "--task":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.task, f.set["task"] = args[i], true
		case "--max-cost":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			mc, err := parseUSDToMicrocents(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --max-cost %q (want a dollar amount like 0.50)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.maxCostMc, f.set["max_cost"] = mc, true
		case "--max-daily":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			mc, err := parseUSDToMicrocents(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --max-daily %q (want a dollar amount like 5.00)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.maxDailyMc, f.set["max_daily"] = mc, true
		case "--memory-scope":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.memScope, f.set["memory_scope"] = args[i], true
		case "--workdir":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.workdir, f.set["workdir"] = args[i], true
		case "--desc":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.desc, f.set["desc"] = args[i], true
		default:
			rest = append(rest, a)
		}
	}
	return f, rest, true
}

func cmdAgentList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
		state := "enabled"
		if en, _ := p["enabled"].(bool); !en {
			state = "PAUSED"
		}
		model, _ := p["model"].(string)
		if model == "" {
			model = "(default)"
		}
		fmt.Fprintf(stdout, "%-20s %-9s model=%s", str(p["slug"]), state, model)
		if tt, _ := p["task_type"].(string); tt != "" {
			fmt.Fprintf(stdout, " task=%s", tt)
		}
		if mc, ok := p["max_cost_mc"].(float64); ok && mc > 0 {
			fmt.Fprintf(stdout, " max-cost=%s", fmtUSD(int64(mc)))
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v agent(s)\n", res["count"])
	return 0
}

func cmdAgentShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	ref := ""
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if !strings.HasPrefix(a, "--") && ref == "" {
			ref = a
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent show <slug|id> [--json]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent show: %v\n", brand.CLI, err)
		return 1
	}
	profiles, _ := res["profiles"].([]any)
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if p == nil || (str(p["slug"]) != ref && str(p["id"]) != ref) {
			continue
		}
		if asJSON {
			return encodeJSON(stdout, p)
		}
		fmt.Fprintf(stdout, "slug:         %s\n", str(p["slug"]))
		fmt.Fprintf(stdout, "id:           %s\n", str(p["id"]))
		fmt.Fprintf(stdout, "name:         %s\n", str(p["name"]))
		state := "enabled"
		if en, _ := p["enabled"].(bool); !en {
			state = "PAUSED"
		}
		fmt.Fprintf(stdout, "state:        %s\n", state)
		if v := str(p["model"]); v != "" {
			fmt.Fprintf(stdout, "model:        %s\n", v)
		}
		if fb, _ := p["fallbacks"].([]any); len(fb) > 0 {
			parts := make([]string, 0, len(fb))
			for _, m := range fb {
				parts = append(parts, str(m))
			}
			fmt.Fprintf(stdout, "fallbacks:    %s\n", strings.Join(parts, " → "))
		}
		if v := str(p["task_type"]); v != "" {
			fmt.Fprintf(stdout, "task type:    %s\n", v)
		}
		if mc, ok := p["max_cost_mc"].(float64); ok && mc > 0 {
			fmt.Fprintf(stdout, "max cost/run: %s\n", fmtUSD(int64(mc)))
		}
		if mc, ok := p["max_daily_mc"].(float64); ok && mc > 0 {
			fmt.Fprintf(stdout, "max cost/day: %s\n", fmtUSD(int64(mc)))
		}
		if v := str(p["memory_scope"]); v != "" {
			fmt.Fprintf(stdout, "memory scope: %s\n", v)
		}
		if v := str(p["workdir"]); v != "" {
			fmt.Fprintf(stdout, "workdir:      %s\n", v)
		}
		if v := str(p["description"]); v != "" {
			fmt.Fprintf(stdout, "description:  %s\n", v)
		}
		if v := str(p["soul"]); v != "" {
			fmt.Fprintf(stdout, "soul:\n  %s\n", strings.ReplaceAll(v, "\n", "\n  "))
		}
		return 0
	}
	fmt.Fprintf(stderr, "%s agent show: unknown agent %q\n", brand.CLI, ref)
	return 1
}

func cmdAgentAdd(args []string, stdout, stderr io.Writer) int {
	f, rest, ok := parseAgentFlags(args, stderr, "add")
	if !ok {
		return 2
	}
	if len(rest) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent add <slug> [--soul PROMPT] [--model M] ...\n", brand.CLI)
		return 2
	}
	profile := map[string]any{
		"slug": rest[0], "name": f.name, "soul": f.soul, "model": f.model,
		"task_type": f.task, "max_cost_mc": f.maxCostMc, "max_daily_mc": f.maxDailyMc,
		"memory_scope": f.memScope, "workdir": f.workdir, "description": f.desc,
	}
	if len(f.fallbacks) > 0 {
		fb := make([]any, len(f.fallbacks))
		for i, m := range f.fallbacks {
			fb[i] = m
		}
		profile["fallbacks"] = fb
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{"profile": profile})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent add: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s created (run it: %s run --agent %s \"...\")\n", str(p["slug"]), brand.CLI, str(p["slug"]))
	return 0
}

func cmdAgentSet(args []string, stdout, stderr io.Writer) int {
	f, rest, ok := parseAgentFlags(args, stderr, "set")
	if !ok {
		return 2
	}
	if len(rest) != 1 || len(f.set) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent set <slug|id> --soul PROMPT|--model M|... (at least one flag)\n", brand.CLI)
		return 2
	}
	ref := rest[0]
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// agent_edit applies the mutable fields WHOLESALE, so a partial edit must
	// start from the current profile and overlay only the provided flags.
	cur, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent set: %v\n", brand.CLI, err)
		return 1
	}
	var base map[string]any
	if profiles, _ := cur["profiles"].([]any); profiles != nil {
		for _, raw := range profiles {
			p, _ := raw.(map[string]any)
			if p != nil && (str(p["slug"]) == ref || str(p["id"]) == ref) {
				base = p
				break
			}
		}
	}
	if base == nil {
		fmt.Fprintf(stderr, "%s agent set: unknown agent %q\n", brand.CLI, ref)
		return 1
	}
	overlay := func(key, flag, val string) {
		if f.set[flag] {
			base[key] = val
		}
	}
	overlay("name", "name", f.name)
	overlay("soul", "soul", f.soul)
	overlay("model", "model", f.model)
	overlay("task_type", "task", f.task)
	overlay("memory_scope", "memory_scope", f.memScope)
	overlay("workdir", "workdir", f.workdir)
	overlay("description", "desc", f.desc)
	if f.set["max_cost"] {
		base["max_cost_mc"] = f.maxCostMc
	}
	if f.set["max_daily"] {
		base["max_daily_mc"] = f.maxDailyMc
	}
	if f.set["fallbacks"] {
		fb := make([]any, len(f.fallbacks))
		for i, m := range f.fallbacks {
			fb[i] = m
		}
		base["fallbacks"] = fb
	}
	res, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{"ref": ref, "profile": base})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent set: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s updated\n", str(p["slug"]))
	return 0
}

func cmdAgentSetEnabled(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "pause"
	if enabled {
		verb = "resume"
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent %s <slug|id>\n", brand.CLI, verb)
		return 2
	}
	c := dial(stderr)
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

func cmdAgentRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent remove <slug|id>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent remove: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); !removed {
		fmt.Fprintf(stderr, "%s agent remove: unknown agent %q\n", brand.CLI, args[0])
		return 1
	}
	fmt.Fprintf(stdout, "agent %s removed\n", args[0])
	return 0
}

// str renders any JSON value as its string form ("" for nil/non-strings).
func str(v any) string {
	s, _ := v.(string)
	return s
}

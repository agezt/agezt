// SPDX-License-Identifier: MIT
//
// cmd/agt agent add/set CRUD handlers (cmdAgentAdd, cmdAgentSet).
// Extracted from agent_crud.go during Day 211 god-file refactor (#41, #64).
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

func cmdAgentAdd(args []string, stdout, stderr io.Writer) int {
	f, rest, ok := parseAgentFlags(args, stderr, "add")
	if !ok {
		return 2
	}
	if len(rest) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent add <slug> [--soul TEXT] [--model M] ...\n", brand.CLI)
		return 2
	}
	profile := map[string]any{
		"slug": rest[0], "name": f.name, "soul": f.soul, "model": f.model,
		"task_type": f.task, "max_cost_mc": f.maxCostMc, "max_daily_mc": f.maxDailyMc,
		"memory_scope": f.memScope, "workdir": f.workdir, "description": f.desc,
	}
	if f.set["owner_agent"] {
		profile["owner_agent"] = f.ownerAgent
	}
	if f.set["parent_agent"] {
		profile["parent_agent"] = f.parentAgent
	}
	if f.set["direct_callable"] {
		profile["direct_callable"] = f.directCallable
	}
	if err := applyAgentAdvancedFlags(profile, f); err != nil {
		fmt.Fprintf(stderr, "%s agent add: %v\n", brand.CLI, err)
		return 2
	}
	applyAgentPolicyFlags(profile, f)
	if len(f.fallbacks) > 0 {
		fb := make([]any, len(f.fallbacks))
		for i, m := range f.fallbacks {
			fb[i] = m
		}
		profile["fallbacks"] = fb
	}
	c := dialpkg.New(stderr)
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
		fmt.Fprintf(stderr, "usage: %s agent set <slug|id> --soul TEXT|--model M|... (at least one flag)\n", brand.CLI)
		return 2
	}
	ref := rest[0]
	c := dialpkg.New(stderr)
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
	overlay("owner_agent", "owner_agent", f.ownerAgent)
	overlay("parent_agent", "parent_agent", f.parentAgent)
	overlay("trust_ceiling", "trust_ceiling", f.trustCeiling)
	overlay("execution_profile", "execution_profile", f.executionProfile)
	if f.set["max_cost"] {
		base["max_cost_mc"] = f.maxCostMc
	}
	if f.set["max_daily"] {
		base["max_daily_mc"] = f.maxDailyMc
	}
	if f.set["direct_callable"] {
		base["direct_callable"] = f.directCallable
	}
	if err := applyAgentAdvancedFlags(base, f); err != nil {
		fmt.Fprintf(stderr, "%s agent set: %v\n", brand.CLI, err)
		return 2
	}
	applyAgentPolicyFlags(base, f)
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

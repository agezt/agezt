// SPDX-License-Identifier: MIT
//
// cmd/agt agent CRUD sub-commands: add + set + task + wake + repair +
// repair-status + set-enabled + the callAgentAsyncAction helper + payload
// builders + applyAgentActionFlags. Split from agent.go during Day 211
// god-file refactor (#41). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
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

func cmdAgentTask(args []string, stdout, stderr io.Writer) int {
	payload, label, ok := buildAgentTaskPayload(args, stderr)
	if !ok {
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent task: %v\n", brand.CLI, err)
		return 1
	}
	task, _ := res["task"].(map[string]any)
	if label == "" {
		label = str(task["status"])
	}
	fmt.Fprintf(stdout, "agent %s task %s", str(payload["ref"]), label)
	if id := str(task["id"]); id != "" {
		fmt.Fprintf(stdout, " %s", id)
	}
	if title := str(task["title"]); title != "" {
		fmt.Fprintf(stdout, " (%s)", title)
	}
	fmt.Fprintln(stdout)
	return 0
}

func buildAgentTaskPayload(args []string, stderr io.Writer) (map[string]any, string, bool) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printAgentTaskUsage(stderr)
		return nil, "", false
	}
	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch cmd {
	case "add":
		if len(args) < 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		payload := map[string]any{"op": "add", "ref": args[1], "title": args[2], "scope": "total", "status": "todo"}
		if !applyTaskFlags(payload, args[3:], stderr) {
			return nil, "", false
		}
		return payload, "added", true
	case "set", "update", "edit":
		if len(args) < 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		payload := map[string]any{"op": "update", "ref": args[1], "id": args[2]}
		if !applyTaskFlags(payload, args[3:], stderr) {
			return nil, "", false
		}
		if len(payload) <= 3 {
			fmt.Fprintf(stderr, "%s agent task set: at least one field flag is required\n", brand.CLI)
			return nil, "", false
		}
		return payload, "updated", true
	case "remove", "delete", "rm":
		if len(args) != 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		return map[string]any{"op": "remove", "ref": args[1], "id": args[2]}, "removed", true
	case "todo", "doing", "done", "blocked", "retired":
		if len(args) != 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		return map[string]any{"op": "update", "ref": args[1], "id": args[2], "status": cmd}, cmd, true
	default:
		fmt.Fprintf(stderr, "%s agent task: unknown task command %q\n", brand.CLI, args[0])
		printAgentTaskUsage(stderr)
		return nil, "", false
	}
}

func applyTaskFlags(payload map[string]any, args []string, stderr io.Writer) bool {
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent task: %s needs a value\n", brand.CLI, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["title"] = args[i]
		case "--desc", "--description":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["description"] = args[i]
		case "--scope":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["scope"] = args[i]
		case "--status":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["status"] = args[i]
		case "--id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["id"] = args[i]
		default:
			fmt.Fprintf(stderr, "%s agent task: unknown flag %s\n", brand.CLI, args[i])
			return false
		}
	}
	return true
}

func printAgentTaskUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s agent task add <slug|id> <title> [--id ID] [--scope cycle|total] [--status todo|doing|done|blocked|retired] [--desc TEXT]\n", brand.CLI)
	fmt.Fprintf(w, "       %s agent task set <slug|id> <task-id> [--title TEXT] [--scope cycle|total] [--status ...] [--desc TEXT]\n", brand.CLI)
	fmt.Fprintf(w, "       %s agent task <done|doing|todo|blocked|retired|remove> <slug|id> <task-id>\n", brand.CLI)
}

func cmdAgentWake(args []string, stdout, stderr io.Writer) int {
	payload, ok := buildAgentWakePayload(args, stderr)
	if !ok {
		return 2
	}
	return callAgentAsyncAction(controlplane.CmdAgentWake, "wake", payload, stdout, stderr)
}

func cmdAgentRepair(args []string, stdout, stderr io.Writer) int {
	payload, ok := buildAgentRepairPayload(args, stderr)
	if !ok {
		return 2
	}
	return callAgentAsyncAction(controlplane.CmdAgentRepair, "repair", payload, stdout, stderr)
}

func cmdAgentRepairStatus(args []string, stdout, stderr io.Writer) int {
	ref := ""
	limit := 20
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s agent repair-status: --limit needs a value\n", brand.CLI)
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s agent repair-status: invalid --limit %q\n", brand.CLI, args[i])
				return 2
			}
			limit = n
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
			return 0
		default:
			if strings.HasPrefix(args[i], "--") || ref != "" {
				fmt.Fprintf(stderr, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
				return 2
			}
			ref = args[i]
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{"ref": ref, "limit": limit})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent repair-status: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s repair: %v history, %v inflight", str(res["slug"]), res["count"], res["inflight_count"])
	if next := intNumber(res["next_eligible_ms"]); next > 0 {
		fmt.Fprintf(stdout, " next_eligible_ms=%d", next)
	}
	fmt.Fprintln(stdout)
	if latest, _ := res["latest"].(map[string]any); latest != nil {
		fmt.Fprintf(stdout, "latest: %s %s\n", str(latest["phase"]), str(latest["reason"]))
	}
	return 0
}

func callAgentAsyncAction(cmd string, label string, payload map[string]any, stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent %s: %v\n", brand.CLI, label, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s %s accepted", str(res["agent"]), label)
	if corr := str(res["correlation_id"]); corr != "" {
		fmt.Fprintf(stdout, " corr=%s", corr)
	}
	fmt.Fprintln(stdout)
	return 0
}

func buildAgentWakePayload(args []string, stderr io.Writer) (map[string]any, bool) {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent wake <slug|id> [intent text] [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n", brand.CLI)
		return nil, false
	}
	payload := map[string]any{"ref": args[0]}
	var intent []string
	if !applyAgentActionFlags(payload, args[1:], &intent, stderr, "wake") {
		return nil, false
	}
	if len(intent) > 0 {
		payload["intent"] = strings.Join(intent, " ")
	}
	return payload, true
}

func buildAgentRepairPayload(args []string, stderr io.Writer) (map[string]any, bool) {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent repair <slug|id> [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n", brand.CLI)
		return nil, false
	}
	payload := map[string]any{"ref": args[0]}
	var extras []string
	if !applyAgentActionFlags(payload, args[1:], &extras, stderr, "repair") {
		return nil, false
	}
	if len(extras) > 0 && str(payload["reason"]) == "" {
		payload["reason"] = strings.Join(extras, " ")
	}
	return payload, true
}

func applyAgentActionFlags(payload map[string]any, args []string, positional *[]string, stderr io.Writer, cmd string) bool {
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent %s: %s needs a value\n", brand.CLI, cmd, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reason":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["reason"] = args[i]
		case "--incident", "--incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["incident_id"] = args[i]
		case "--root", "--root-incident", "--root-incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["root_incident_id"] = args[i]
		case "--parent", "--parent-incident", "--parent-incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["parent_incident_id"] = args[i]
		default:
			if strings.HasPrefix(args[i], "--") {
				fmt.Fprintf(stderr, "%s agent %s: unknown flag %s\n", brand.CLI, cmd, args[i])
				return false
			}
			*positional = append(*positional, args[i])
		}
	}
	return true
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

// cmdAgentAuthority implements `agt agent authority <slug|id> [--json]`.
//
// It is the effective-policy proof surface (see docs/COMPARISON.md): it merges
// the agent's own authority fields (tool allow/deny, trust ceiling, memory
// scope, workdir, config) with the live Edict policy snapshot (capability
// levels, hard-deny rules, approval mode) into a single view of what this
// agent can actually do at runtime. Read-only; no side effects.
//
// This is deliberately client-side: it fetches CmdAgentList + CmdEdictShow and
// joins them, so no new control-plane command is needed. If the daemon's
// authority model grows (config-center access per agent, schedule/channel wake
// permissions), this command should extend to surface those too.

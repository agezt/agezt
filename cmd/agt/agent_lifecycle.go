// SPDX-License-Identifier: MIT
//
// cmd/agt agent lifecycle sub-commands: tombstone + graveyard + retire +
// revive + remove. Split from agent.go during Day 211 god-file refactor
// (#41). Public API unchanged.
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
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdAgentTombstone(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var ref string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if !strings.HasPrefix(a, "-") && ref == "" {
			ref = a
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent tombstone <slug|id> [--json]\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentTombstone, map[string]any{"ref": ref})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent tombstone: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	t, _ := res["tombstone"].(map[string]any)
	if t == nil {
		fmt.Fprintln(stderr, "tombstone: empty response")
		return 1
	}
	fmt.Fprintf(stdout, "⚰ tombstone — %s", str(t["slug"]))
	if name := str(t["name"]); name != "" && name != str(t["slug"]) {
		fmt.Fprintf(stdout, " (%s)", name)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "kind:         %s\n", str(t["kind"]))
	if mgr := str(t["manager"]); mgr != "" {
		fmt.Fprintf(stdout, "manager:      %s\n", mgr)
	}
	if retired, _ := t["retired"].(bool); retired {
		line := "status:       retired (graveyard)"
		if ms := intNumber(t["retired_ms"]); ms > 0 {
			line += " since " + time.UnixMilli(int64(ms)).Format(time.RFC3339)
		}
		fmt.Fprintln(stdout, line)
		if reason := str(t["retired_reason"]); reason != "" {
			fmt.Fprintf(stdout, "reason:       %s\n", reason)
		}
	} else {
		fmt.Fprintln(stdout, "status:       active (snapshot)")
	}
	mode := str(t["lifecycle_mode"])
	if mode == "" {
		mode = "persistent"
	}
	fmt.Fprintf(stdout, "lifecycle:    %s", mode)
	if mx := intNumber(t["max_cycles"]); mx > 0 {
		fmt.Fprintf(stdout, " %d/%d cycles", intNumber(t["completed_cycles"]), mx)
	} else if done := intNumber(t["completed_cycles"]); done > 0 {
		fmt.Fprintf(stdout, " %d cycles", done)
	}
	fmt.Fprintln(stdout)
	if fp, ok := t["footprint"].(map[string]any); ok {
		var parts []string
		for _, f := range []struct{ key, label string }{
			{"standing_orders", "standing"}, {"schedules", "schedules"},
			{"memories", "memories"}, {"skills", "skills"}, {"configs", "configs"},
			{"workspaces", "workspaces"}, {"workflow_refs", "workflow refs"},
			{"mailbox_messages", "mailbox msgs"}, {"subagents", "sub-agents"},
		} {
			if n := intNumber(fp[f.key]); n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, f.label))
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(stdout, "footprint:    %s\n", strings.Join(parts, ", "))
		} else {
			fmt.Fprintln(stdout, "footprint:    none")
		}
	}
	return 0
}

func cmdAgentGraveyard(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	olderThan := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--older-than":
			if i+1 < len(args) {
				olderThan = args[i+1]
				i++
			}
		default:
			if strings.HasPrefix(args[i], "--older-than=") {
				olderThan = strings.TrimPrefix(args[i], "--older-than=")
			}
		}
	}
	reqArgs := map[string]any{}
	if olderThan != "" {
		if d, err := strconv.ParseFloat(olderThan, 64); err == nil {
			reqArgs["older_than_days"] = d
		} else {
			fmt.Fprintf(stderr, "%s agent graveyard: --older-than expects a number of days\n", brand.CLI)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentGraveyard, reqArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent graveyard: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	rows, _ := res["graveyard"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "graveyard is empty (no retired agents match)")
		return 0
	}
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		if r == nil {
			continue
		}
		fmt.Fprintf(stdout, "%-20s retired %dd ago", str(r["slug"]), intNumber(r["age_days"]))
		if kind := str(r["kind"]); kind != "" {
			fmt.Fprintf(stdout, " [%s]", kind)
		}
		if reason := str(r["retired_reason"]); reason != "" {
			fmt.Fprintf(stdout, " — %s", reason)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v retired agent(s)\n", res["count"])
	return 0
}

// cmdAgentRetire moves a dead agent to the graveyard (M846): it shows what
// depends on the agent first, then retires. The agent is paused and any
// delegation to it is refused until revived. The roster keeps it (recoverable),
// distinct from `remove` which deletes.
func cmdAgentRetire(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintf(stderr, "usage: %s agent retire <slug|id> [reason]\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := map[string]any{"ref": args[0]}
	if len(args) > 1 {
		req["reason"] = strings.Join(args[1:], " ")
	}
	res, err := c.Call(ctx, controlplane.CmdAgentRetire, req)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent retire: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s retired to the graveyard (revive it: %s agent revive %s)\n", str(p["slug"]), brand.CLI, str(p["slug"]))
	if summary, _ := res["impact_summary"].(map[string]any); len(summary) > 0 {
		printAgentImpactSummary(stdout, summary)
	} else if impact, _ := res["impact"].([]any); len(impact) > 0 {
		printImpactItems(stdout, "standing orders", impact)
	}
	return 0
}

// cmdAgentRevive brings a retired agent back from the graveyard (M846). It is
// restored paused so the operator decides when to resume it.
func cmdAgentRevive(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent revive <slug|id>\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentRevive, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent revive: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s revived (paused — resume it: %s agent resume %s)\n", str(p["slug"]), brand.CLI, str(p["slug"]))
	return 0
}

func cmdAgentRemove(args []string, stdout, stderr io.Writer) int {
	ref, cascade, help, ok := buildAgentRemovePayload(args, stderr)
	if help {
		printAgentRemoveUsage(stdout)
		return 0
	}
	if !ok {
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload := map[string]any{"ref": ref}
	if len(cascade) > 0 {
		payload["cascade"] = cascade
	}
	if impact, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": ref}); err == nil {
		printAgentImpactSummary(stdout, impact)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentRemove, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent remove: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); !removed {
		fmt.Fprintf(stderr, "%s agent remove: unknown agent %q\n", brand.CLI, ref)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s removed", ref)
	if summary := agentRemoveResultSummary(res); summary != "" {
		fmt.Fprintf(stdout, " (%s)", summary)
	}
	fmt.Fprintln(stdout)
	return 0
}

func agentRemoveResultSummary(res map[string]any) string {
	parts := []string{}
	add := func(key, label string) {
		n := intNumber(res[key])
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add("standing_removed", "standing")
	add("schedules_removed", "schedule")
	add("memories_forgotten", "private memory")
	add("authored_memories_forgotten", "authored shared memory")
	add("skills_archived", "skill archived")
	add("configs_deleted", "config deleted")
	add("workspaces_deleted", "workspace deleted")
	add("subagents_retired", "subagent retired")
	add("mailbox_messages_retained", "mailbox/audit retained")
	return strings.Join(parts, ", ")
}

func buildAgentRemovePayload(args []string, stderr io.Writer) (string, map[string]any, bool, bool) {
	ref := ""
	cascade := map[string]any{}
	for _, a := range args {
		switch a {
		case "--with-all":
			cascade["standing"] = true
			cascade["schedules"] = true
			cascade["memory"] = true
			cascade["authored_memory"] = true
			cascade["skills"] = true
			cascade["config"] = true
			cascade["workspace"] = true
			cascade["subagents"] = true
		case "--with-standing":
			cascade["standing"] = true
		case "--with-schedules":
			cascade["schedules"] = true
		case "--with-memory":
			cascade["memory"] = true
		case "--with-authored-memory", "--with-authored-shared-memory":
			cascade["authored_memory"] = true
		case "--with-skills":
			cascade["skills"] = true
		case "--with-config":
			cascade["config"] = true
		case "--with-workspace", "--with-workdir":
			cascade["workspace"] = true
		case "--with-subagents":
			cascade["subagents"] = true
		case "-h", "--help":
			return "", nil, true, false
		default:
			if strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "%s agent remove: unknown flag %s\n", brand.CLI, a)
				return "", nil, false, false
			}
			if ref != "" {
				printAgentRemoveUsage(stderr)
				return "", nil, false, false
			}
			ref = a
		}
	}
	if ref == "" {
		printAgentRemoveUsage(stderr)
		return "", nil, false, false
	}
	return ref, cascade, false, true
}

func printAgentRemoveUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s agent remove <slug|id> [--with-all|--with-schedules|--with-memory|--with-authored-memory|--with-skills|--with-config|--with-workspace|--with-standing|--with-subagents]\n", brand.CLI)
	fmt.Fprintf(w, "  --with-memory cleans private memory; --with-authored-memory cleans shared memory records written by the agent\n")
}

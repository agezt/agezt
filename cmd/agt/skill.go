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

// cmdSkill dispatches `agt skill <subcommand>`. Forge is the journaled
// skill-lifecycle: the agent proposes drafts, the operator governs them through
// draft→shadow→active and can revert non-destructively.
func cmdSkill(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s skill: subcommand required (list|show|history|promote|quarantine|revert|share|reassign|diff|export|import|registry)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list", "ls":
		return cmdSkillList(args[1:], stdout, stderr)
	case "show", "get":
		return cmdSkillShow(args[1:], stdout, stderr)
	case "history", "log":
		return cmdSkillHistory(args[1:], stdout, stderr)
	case "promote":
		return cmdSkillTransition(args[1:], controlplane.CmdSkillPromote, "promote", stdout, stderr)
	case "quarantine":
		return cmdSkillTransition(args[1:], controlplane.CmdSkillQuarantine, "quarantine", stdout, stderr)
	case "revert":
		return cmdSkillTransition(args[1:], controlplane.CmdSkillRevert, "revert", stdout, stderr)
	case "share":
		return cmdSkillReassign(args[1:], true, stdout, stderr)
	case "reassign":
		return cmdSkillReassign(args[1:], false, stdout, stderr)
	case "diff":
		return cmdSkillDiff(args[1:], stdout, stderr)
	case "export":
		return cmdSkillExport(args[1:], stdout, stderr)
	case "import":
		return cmdSkillImport(args[1:], stdout, stderr)
	case "files":
		return cmdSkillFiles(args[1:], stdout, stderr)
	case "cat":
		return cmdSkillCat(args[1:], stdout, stderr)
	case "registry":
		return cmdSkillRegistry(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s skill <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [--json]                 list all skills + lifecycle state\n")
		fmt.Fprintf(stdout, "  show <id> [--json]            read one skill (exit 3 = absent)\n")
		fmt.Fprintf(stdout, "  history <id> [--json]         the skill's lifecycle event chain\n")
		fmt.Fprintf(stdout, "  promote <id> [--json]         advance draft->shadow->active\n")
		fmt.Fprintf(stdout, "  quarantine <id> [--reason R] [--json]   pull from production\n")
		fmt.Fprintf(stdout, "  revert <id> [--json]          archive + restore lineage parent\n")
		fmt.Fprintf(stdout, "  share <id> [--json]           promote a private (per-agent) skill to the shared pool\n")
		fmt.Fprintf(stdout, "  reassign <id> [--agent S]     change a skill's owning agent (omit --agent to share)\n")
		fmt.Fprintf(stdout, "  diff <id> [<id2>]             diff a skill's body vs its parent (or vs id2)\n")
		fmt.Fprintf(stdout, "  export <id> [--out <file>]    write a portable, verifiable skill bundle\n")
		fmt.Fprintf(stdout, "  export --all [--dir <dir>]    export every skill into a directory registry\n")
		fmt.Fprintf(stdout, "  import <bundle|dir> [--json]  install a bundle/SKILL.md/skill directory as a draft\n")
		fmt.Fprintf(stdout, "  files <id> [--json]           list a skill's bundle resources + their directory\n")
		fmt.Fprintf(stdout, "  cat <id> <path>               print one bundle resource (reference file or script)\n")
		fmt.Fprintf(stdout, "  registry <dir> [--install <name>]   list/install verifiable bundles in a directory\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s skill: unknown subcommand %q (list|show|history|promote|quarantine|revert|share|reassign|diff|export|import|registry)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdSkillList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s skill list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s skill list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	sks, _ := res["skills"].([]any)
	if len(sks) == 0 {
		fmt.Fprintln(stdout, "no skills")
		return 0
	}
	active, _ := res["active_count"].(float64)
	fmt.Fprintf(stdout, "%d skill(s), %d active:\n", len(sks), int(active))
	for _, raw := range sks {
		if sk, ok := raw.(map[string]any); ok {
			fmt.Fprintln(stdout, renderSkillLine(sk))
		}
	}
	return 0
}

func cmdSkillShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill show <id> [--json]\n", brand.CLI)
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill show: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill show: id required\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillGet, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill show: %v\n", brand.CLI, err)
		return 1
	}
	found, _ := res["found"].(bool)
	if asJSON {
		_ = encodeJSON(stdout, res)
		if !found {
			return 3
		}
		return 0
	}
	if !found {
		fmt.Fprintf(stderr, "%s skill show: %s not found\n", brand.CLI, id)
		return 3
	}
	sk, _ := res["skill"].(map[string]any)
	return encodeJSON(stdout, sk)
}

func cmdSkillHistory(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill history <id> [--json]\n", brand.CLI)
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill history: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill history: id required\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillHistory, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill history: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	events, _ := res["events"].([]any)
	if len(events) == 0 {
		fmt.Fprintf(stdout, "no history for %s\n", id)
		return 0
	}
	fmt.Fprintf(stdout, "%d lifecycle event(s):\n", len(events))
	for _, raw := range events {
		e, _ := raw.(map[string]any)
		kind, _ := e["kind"].(string)
		seq, _ := e["seq"].(float64)
		p, _ := e["payload"].(map[string]any)
		fmt.Fprintf(stdout, "  seq=%-5d %-18s %s\n", int(seq), kind, renderSkillEventDetail(kind, p))
	}
	return 0
}

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
			fmt.Fprintf(stdout, "usage: %s skill %s <id> [--json]\n", brand.CLI, label)
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
	}
	switch label {
	case "promote":
		fmt.Fprintf(stdout, "%s -> %v\n", id, res["status"])
	case "quarantine":
		fmt.Fprintf(stdout, "quarantined %s\n", id)
	case "revert":
		if restored, _ := res["restored"].(string); restored != "" {
			fmt.Fprintf(stdout, "reverted %s (restored %s)\n", id, restored)
		} else {
			fmt.Fprintf(stdout, "reverted %s (archived; no parent to restore)\n", id)
		}
	}
	return 0
}

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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
func renderSkillLine(sk map[string]any) string {
	id, _ := sk["id"].(string)
	if len(id) > 12 {
		id = id[:12]
	}
	status, _ := sk["status"].(string)
	name, _ := sk["name"].(string)
	desc, _ := sk["description"].(string)
	line := fmt.Sprintf("  %s [%s] %s", id, status, name)
	if desc != "" {
		line += " — " + desc
	}
	// For a shadow skill, surface its evaluation progress toward promotion
	// (SPEC-05 §5.2 / M402): "· shadow <wins>/<evals>".
	if status == "shadow" {
		if ev, wins, ok := shadowProgress(sk); ok {
			line += fmt.Sprintf("  · shadow %d/%d", wins, ev)
		}
	}
	return line
}

// shadowProgress pulls the shadow-evaluation counters from a skill map's metrics,
// reporting (evals, wins, present). Returns ok=false when no evaluations yet.
func shadowProgress(sk map[string]any) (evals, wins int, ok bool) {
	m, _ := sk["metrics"].(map[string]any)
	if m == nil {
		return 0, 0, false
	}
	ev, _ := m["shadow_evals"].(float64)
	w, _ := m["shadow_wins"].(float64)
	if ev == 0 {
		return 0, 0, false
	}
	return int(ev), int(w), true
}

// renderSkillEventDetail summarizes a lifecycle event's payload for `history`.
func renderSkillEventDetail(kind string, p map[string]any) string {
	switch kind {
	case "skill.promoted":
		return fmt.Sprintf("%v -> %v", p["from"], p["to"])
	case "skill.quarantined":
		if r, _ := p["reason"].(string); r != "" {
			return "reason: " + r
		}
		return "(no reason)"
	case "skill.reverted":
		if r, _ := p["restored"].(string); r != "" {
			return "restored " + shortID(r)
		}
		return "archived"
	case "skill.created":
		return fmt.Sprintf("%v", p["name"])
	case "skill.shared":
		if fa, _ := p["from_agent"].(string); fa != "" {
			return "shared (was private to " + fa + ")"
		}
		return "shared"
	case "skill.reassigned":
		return fmt.Sprintf("%v -> %v", p["from_agent"], p["to_agent"])
	case "skill.activated":
		return fmt.Sprintf("matched %v", p["matched"])
	default:
		return ""
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

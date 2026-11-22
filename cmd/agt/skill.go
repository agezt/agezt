// SPDX-License-Identifier: MIT

package main

// Provenance: agt skill command: dispatcher +
//             List/Show/History/Transition/Reassign/Hygiene subcommands. Code
//             extracted from skill.go during the Day-97 god-file split. Public API
//             unchanged.

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// cmdSkill dispatches `agt skill <subcommand>`. Forge is the journaled
// skill-lifecycle: the agent proposes drafts, the operator governs them through
// draft→shadow→active and can revert non-destructively.
func cmdSkill(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s skill: subcommand required (list|show|history|promote|quarantine|archive|revert|share|reassign|diff|export|import|registry|workshop)\n", brand.CLI)
		return 2
	}
	if sub, ok := cliSkill.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s skill: unknown subcommand %q (list|show|history|promote|quarantine|archive|revert|share|reassign|diff|export|import|registry|workshop)\n", brand.CLI, args[0])
	return 2
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
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, opapi.SkillList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
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
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, opapi.SkillGet, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill show: %v\n", brand.CLI, err)
		return 1
	}
	found, _ := res["found"].(bool)
	if asJSON {
		_ = jsonout.Write(stdout, res)
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
	return jsonout.Write(stdout, sk)
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
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, opapi.SkillHistory, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill history: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
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

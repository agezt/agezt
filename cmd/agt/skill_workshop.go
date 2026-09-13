// SPDX-License-Identifier: MIT
//
// cmd/agt skill_workshop command: the entry dispatcher + the read-only
// sub-commands (List + Inspect).
// The scan + curate sub-commands live in skill_workshop_curate.go.
// Extracted from skill_workshop.go during the Day-205 god-file split.
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdSkillWorkshop is the Forge Workshop operator surface: proposals are still
// ordinary Forge skills, but the commands speak in review verbs instead of raw
// lifecycle verbs.
func cmdSkillWorkshop(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return skillWorkshopUsage(stderr)
	}
	switch args[0] {
	case "list", "ls":
		return cmdSkillWorkshopList(args[1:], stdout, stderr)
	case "inspect", "show", "get":
		return cmdSkillWorkshopInspect(args[1:], stdout, stderr)
	case "scan":
		return cmdSkillWorkshopScan(args[1:], stdout, stderr)
	case "diff":
		return cmdSkillDiff(args[1:], stdout, stderr)
	case "curate", "curator":
		return cmdSkillWorkshopCurate(args[1:], stdout, stderr)
	case "apply", "promote":
		return cmdSkillWorkshopApply(args[1:], stdout, stderr)
	case "reject":
		return cmdSkillWorkshopReject(args[1:], stdout, stderr)
	case "quarantine":
		return cmdSkillWorkshopQuarantine(args[1:], stdout, stderr)
	case "propose", "import":
		return cmdSkillImport(args[1:], stdout, stderr)
	case "propose-create", "create":
		return cmdSkillWorkshopProposeCreate(args[1:], stdout, stderr)
	case "propose-update", "update":
		return cmdSkillWorkshopProposeUpdate(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return skillWorkshopUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s skill workshop: unknown subcommand %q\n", brand.CLI, args[0])
		return skillWorkshopUsage(stderr)
	}
}

func skillWorkshopUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s skill workshop <list|inspect|scan|diff|curate|apply|reject|quarantine|propose|propose-create|propose-update>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                      pending draft/shadow proposals\n")
	fmt.Fprintf(w, "  inspect <id> [--json]                              proposal, lineage, resources, and lifecycle history\n")
	fmt.Fprintf(w, "  scan <id> [--json]                                 deterministic risk scan for a proposal\n")
	fmt.Fprintf(w, "  diff <id> [<id2>]                                  diff a proposal against its parent, or old->new\n")
	fmt.Fprintf(w, "  curate [--idle-days N] [--execute] [--json]        deterministic stale-skill cleanup (dry-run by default)\n")
	fmt.Fprintf(w, "  apply <id> [--json]                                advance one gate: draft->shadow or shadow->active\n")
	fmt.Fprintf(w, "  reject <id> [--reason R] [--json]                  archive a proposal with a journaled reason\n")
	fmt.Fprintf(w, "  quarantine <id> [--reason R] [--json]              pull a live/shadow skill from production\n")
	fmt.Fprintf(w, "  propose <bundle|dir|SKILL.md> [--json]             import a portable skill as a draft proposal\n")
	fmt.Fprintf(w, "  propose-create --name N (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n")
	fmt.Fprintf(w, "  propose-update <id> (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n")
	return 0
}

func cmdSkillWorkshopList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s skill workshop list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop list: %v\n", brand.CLI, err)
		return 1
	}
	proposals := workshopProposals(res["skills"])
	if asJSON {
		return jsonout.Write(stdout, map[string]any{"proposals": proposals, "count": len(proposals)})
	}
	if len(proposals) == 0 {
		fmt.Fprintln(stdout, "no pending workshop proposals")
		return 0
	}
	fmt.Fprintf(stdout, "%d workshop proposal(s):\n", len(proposals))
	for _, sk := range proposals {
		fmt.Fprintln(stdout, renderSkillLine(sk))
	}
	return 0
}

func cmdSkillWorkshopInspect(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	id := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop inspect <id> [--json]\n", brand.CLI)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop inspect: unexpected flag %q\n", brand.CLI, a)
			return 2
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop inspect: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill workshop inspect: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sk, found, err := workshopFetchSkill(ctx, c, id)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop inspect: %v\n", brand.CLI, err)
		return 1
	}
	if !found {
		if asJSON {
			_ = jsonout.Write(stdout, map[string]any{"found": false, "id": id})
		} else {
			fmt.Fprintf(stderr, "%s skill workshop inspect: %s not found\n", brand.CLI, id)
		}
		return 3
	}
	history, err := c.Call(ctx, controlplane.CmdSkillHistory, map[string]any{"id": str(sk["id"])})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop inspect: history: %v\n", brand.CLI, err)
		return 1
	}
	events, _ := history["events"].([]any)
	if asJSON {
		scan := workshopScanSkill(sk)
		return jsonout.Write(stdout, map[string]any{
			"found": true, "skill": sk, "history": events, "history_count": len(events), "scan": scan,
		})
	}
	renderWorkshopInspect(stdout, sk, events)
	return 0
}

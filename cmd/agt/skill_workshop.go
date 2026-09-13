// SPDX-License-Identifier: MIT

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

func cmdSkillWorkshopScan(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	id := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop scan <id> [--json]\n", brand.CLI)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop scan: unexpected flag %q\n", brand.CLI, a)
			return 2
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop scan: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill workshop scan: id required\n", brand.CLI)
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
		fmt.Fprintf(stderr, "%s skill workshop scan: %v\n", brand.CLI, err)
		return 1
	}
	if !found {
		if asJSON {
			_ = jsonout.Write(stdout, map[string]any{"found": false, "id": id})
		} else {
			fmt.Fprintf(stderr, "%s skill workshop scan: %s not found\n", brand.CLI, id)
		}
		return 3
	}
	report := workshopScanSkill(sk)
	if asJSON {
		return jsonout.Write(stdout, map[string]any{"found": true, "id": str(sk["id"]), "scan": report})
	}
	renderWorkshopScan(stdout, report)
	return 0
}

func cmdSkillWorkshopCurate(args []string, stdout, stderr io.Writer) int {
	if workshopHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s skill workshop curate [--idle-days N] [--execute] [--json]\n", brand.CLI)
		return 0
	}
	idleDays, execute, asJSON, ok := parseWorkshopCurateArgs(args, stdout, stderr)
	if !ok {
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if idleDays > 0 {
		callArgs["idle_days"] = idleDays
	}
	res, err := c.Call(ctx, controlplane.CmdSkillHygiene, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop curate: %v\n", brand.CLI, err)
		return 1
	}
	idle, _ := res["idle"].([]any)
	quarantined := make([]any, 0, len(idle))
	if execute {
		for _, raw := range idle {
			sk, _ := raw.(map[string]any)
			if sk == nil {
				continue
			}
			id := str(sk["id"])
			if id == "" {
				continue
			}
			reason := fmt.Sprintf("workshop curator: idle for %d+ days", intNumber(res["idle_days"]))
			if _, cerr := saveSkillStatusRollbackCheckpoint(ctx, c, "curate.quarantine", id, reason); cerr != nil {
				fmt.Fprintf(stderr, "%s skill workshop curate: checkpoint %s: %v\n", brand.CLI, id, cerr)
				return 1
			}
			qres, qerr := c.Call(ctx, controlplane.CmdSkillQuarantine, map[string]any{"id": id, "reason": reason})
			if qerr != nil {
				fmt.Fprintf(stderr, "%s skill workshop curate: quarantine %s: %v\n", brand.CLI, id, qerr)
				return 1
			}
			quarantined = append(quarantined, qres)
		}
	}
	out := map[string]any{
		"idle_days":   intNumber(res["idle_days"]),
		"candidates":  idle,
		"count":       len(idle),
		"execute":     execute,
		"quarantined": quarantined,
	}
	if asJSON {
		return jsonout.Write(stdout, out)
	}
	renderWorkshopCurate(stdout, out)
	return 0
}

func parseWorkshopCurateArgs(args []string, stdout, stderr io.Writer) (idleDays int, execute bool, asJSON bool, ok bool) {
	ok = true
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--execute":
			execute = true
		case a == "--idle-days":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill workshop curate: --idle-days needs a value\n", brand.CLI)
				return 0, false, false, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s skill workshop curate: bad --idle-days %q\n", brand.CLI, args[i])
				return 0, false, false, false
			}
			idleDays = n
		case strings.HasPrefix(a, "--idle-days="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--idle-days="))
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s skill workshop curate: bad --idle-days\n", brand.CLI)
				return 0, false, false, false
			}
			idleDays = n
		default:
			fmt.Fprintf(stderr, "%s skill workshop curate: unexpected arg %q\n", brand.CLI, a)
			return 0, false, false, false
		}
	}
	return idleDays, execute, asJSON, true
}

func renderWorkshopCurate(w io.Writer, out map[string]any) {
	idle, _ := out["candidates"].([]any)
	days := intNumber(out["idle_days"])
	execute, _ := out["execute"].(bool)
	if len(idle) == 0 {
		fmt.Fprintf(w, "curator: no active skills idle for %d+ days\n", days)
		return
	}
	action := "would quarantine"
	if execute {
		action = "quarantined"
	}
	fmt.Fprintf(w, "curator: %s %d active skill(s) idle for %d+ days\n", action, len(idle), days)
	for _, raw := range idle {
		sk, _ := raw.(map[string]any)
		if sk == nil {
			continue
		}
		uses := intNumber(sk["uses"])
		detail := "never used"
		if last := intNumber(sk["last_used_ms"]); last > 0 {
			detail = "last used " + time.UnixMilli(int64(last)).Format(time.RFC3339)
		}
		fmt.Fprintf(w, "  %s  %d use(s), %s\n", renderSkillLine(sk), uses, detail)
	}
	if !execute {
		fmt.Fprintf(w, "run with --execute to quarantine these candidates with a journaled curator reason\n")
	}
}

type workshopScanReport struct {
	Findings    []workshopScanFinding `json:"findings"`
	Count       int                   `json:"count"`
	MaxSeverity string                `json:"max_severity"`
}

type workshopScanFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}


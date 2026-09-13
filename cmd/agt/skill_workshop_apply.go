// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdSkillWorkshopApply(args []string, stdout, stderr io.Writer) int {
	if workshopHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s skill workshop apply <id> [--json]\n", brand.CLI)
		return 0
	}
	id, asJSON, ok := parseWorkshopIDJSON(args, "apply", stdout, stderr)
	if !ok {
		return 2
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill workshop apply: id required\n", brand.CLI)
		return 2
	}
	return callSkillWorkshopTransition(controlplane.CmdSkillPromote, "apply", id, "", asJSON, stdout, stderr)
}

func cmdSkillWorkshopReject(args []string, stdout, stderr io.Writer) int {
	if workshopHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s skill workshop reject <id> [--reason R] [--json]\n", brand.CLI)
		return 0
	}
	id, reason, asJSON, ok := parseWorkshopReasonArgs(args, "reject", "workshop reject", stdout, stderr)
	if !ok {
		return 2
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill workshop reject: id required\n", brand.CLI)
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
		fmt.Fprintf(stderr, "%s skill workshop reject: %v\n", brand.CLI, err)
		return 1
	}
	if !found {
		fmt.Fprintf(stderr, "%s skill workshop reject: %s not found\n", brand.CLI, id)
		return 3
	}
	if !workshopCanReject(str(sk["status"])) {
		fmt.Fprintf(stderr, "%s skill workshop reject: %s is %s, not a pending proposal; use quarantine or revert for live skills\n", brand.CLI, id, str(sk["status"]))
		return 1
	}
	return callSkillWorkshopTransitionWithClient(ctx, c, controlplane.CmdSkillArchive, "reject", id, reason, asJSON, stdout, stderr)
}

func cmdSkillWorkshopQuarantine(args []string, stdout, stderr io.Writer) int {
	if workshopHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s skill workshop quarantine <id> [--reason R] [--json]\n", brand.CLI)
		return 0
	}
	id, reason, asJSON, ok := parseWorkshopReasonArgs(args, "quarantine", "workshop quarantine", stdout, stderr)
	if !ok {
		return 2
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill workshop quarantine: id required\n", brand.CLI)
		return 2
	}
	return callSkillWorkshopTransition(controlplane.CmdSkillQuarantine, "quarantine", id, reason, asJSON, stdout, stderr)
}

func cmdSkillWorkshopProposeCreate(args []string, stdout, stderr io.Writer) int {
	if workshopHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s skill workshop propose-create --name N (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n", brand.CLI)
		return 0
	}
	f, rest, ok := parseWorkshopProposalFlags(args, true, "propose-create", stdout, stderr)
	if !ok {
		return 2
	}
	if len(rest) != 0 {
		fmt.Fprintf(stderr, "%s skill workshop propose-create: unexpected arg %q\n", brand.CLI, rest[0])
		return 2
	}
	if strings.TrimSpace(f.name) == "" {
		fmt.Fprintf(stderr, "%s skill workshop propose-create: --name required\n", brand.CLI)
		return 2
	}
	if strings.TrimSpace(f.body) == "" {
		fmt.Fprintf(stderr, "%s skill workshop propose-create: --body or --body-file required\n", brand.CLI)
		return 2
	}
	return callSkillWorkshopImport(workshopImportArgs(f), f.asJSON, stdout, stderr)
}

func cmdSkillWorkshopProposeUpdate(args []string, stdout, stderr io.Writer) int {
	if workshopHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s skill workshop propose-update <id> (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n", brand.CLI)
		return 0
	}
	f, rest, ok := parseWorkshopProposalFlags(args, false, "propose-update", stdout, stderr)
	if !ok {
		return 2
	}
	if len(rest) != 1 {
		fmt.Fprintf(stderr, "usage: %s skill workshop propose-update <id> (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n", brand.CLI)
		return 2
	}
	if strings.TrimSpace(f.body) == "" {
		fmt.Fprintf(stderr, "%s skill workshop propose-update: --body or --body-file required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	base, found, err := workshopFetchSkill(ctx, c, rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop propose-update: %v\n", brand.CLI, err)
		return 1
	}
	if !found {
		fmt.Fprintf(stderr, "%s skill workshop propose-update: %s not found\n", brand.CLI, rest[0])
		return 3
	}
	if f.name == "" {
		f.name = str(base["name"])
	}
	if !f.set["desc"] {
		f.desc = str(base["description"])
	}
	if !f.set["triggers"] {
		f.triggers = strings.Join(workshopStringSlice(base["triggers"]), ",")
	}
	if !f.set["tools"] {
		f.tools = strings.Join(workshopStringSlice(base["tools_required"]), ",")
	}
	if !f.set["agent"] {
		f.agent = str(base["agent"])
	}
	return callSkillWorkshopImportWithClient(ctx, c, workshopImportArgs(f), f.asJSON, stdout, stderr)
}

func workshopHelpRequested(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func callSkillWorkshopTransition(cmd, label, id, reason string, asJSON bool, stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return callSkillWorkshopTransitionWithClient(ctx, c, cmd, label, id, reason, asJSON, stdout, stderr)
}

func callSkillWorkshopTransitionWithClient(ctx context.Context, c *controlplane.Client, cmd, label, id, reason string, asJSON bool, stdout, stderr io.Writer) int {
	callArgs := map[string]any{"id": id}
	if reason != "" {
		callArgs["reason"] = reason
	}
	if _, cerr := saveSkillStatusRollbackCheckpoint(ctx, c, label, id, reason); cerr != nil {
		fmt.Fprintf(stderr, "%s skill workshop %s: checkpoint: %v\n", brand.CLI, label, cerr)
		return 1
	}
	res, err := c.Call(ctx, cmd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop %s: %v\n", brand.CLI, label, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	switch label {
	case "apply":
		fmt.Fprintf(stdout, "applied %s -> %v\n", id, res["status"])
	case "reject":
		fmt.Fprintf(stdout, "rejected %s (archived)\n", id)
	case "quarantine":
		fmt.Fprintf(stdout, "quarantined %s\n", id)
	}
	return 0
}


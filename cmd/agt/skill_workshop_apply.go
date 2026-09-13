// SPDX-License-Identifier: MIT

package main

// agt skill workshop WRITE subcommands + their helpers: Apply, Reject,
// Quarantine, ProposeCreate, ProposeUpdate, callSkillWorkshopTransition,
// callSkillWorkshopImport, parseWorkshopProposalFlags, readWorkshopBodyFile,
// workshopImportArgs, parseWorkshopIDJSON, parseWorkshopReasonArgs,
// workshopCanReject. Carved out of skill_workshop.go during the Day 155
// god-file split so the main file can stay focused on read + scan + curate.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"os"
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

func callSkillWorkshopImport(callArgs map[string]any, asJSON bool, stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return callSkillWorkshopImportWithClient(ctx, c, callArgs, asJSON, stdout, stderr)
}

func callSkillWorkshopImportWithClient(ctx context.Context, c *controlplane.Client, callArgs map[string]any, asJSON bool, stdout, stderr io.Writer) int {
	res, err := c.Call(ctx, controlplane.CmdSkillImport, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop propose: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	id := str(res["id"])
	created, _ := res["created"].(bool)
	verb := "refreshed existing proposal"
	if created {
		verb = "created proposal"
	}
	fmt.Fprintf(stdout, "%s %q\n", verb, str(res["name"]))
	fmt.Fprintf(stdout, "  id: %s  status: %s\n", shortHash(id), str(res["status"]))
	fmt.Fprintf(stdout, "  inspect: %s skill workshop inspect %s\n", brand.CLI, id)
	return 0
}

type workshopProposalFlags struct {
	name, desc, body, triggers, tools, agent string
	asJSON                                   bool
	set                                      map[string]bool
}

func parseWorkshopProposalFlags(args []string, allowName bool, cmd string, stdout, stderr io.Writer) (workshopProposalFlags, []string, bool) {
	f := workshopProposalFlags{set: map[string]bool{}}
	var rest []string
	readValue := func(i int, flag string) (string, bool) {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s skill workshop %s: %s needs a value\n", brand.CLI, cmd, flag)
			return "", false
		}
		return args[i+1], true
	}
	setBody := func(v string) bool {
		if f.set["body"] {
			fmt.Fprintf(stderr, "%s skill workshop %s: specify only one of --body or --body-file\n", brand.CLI, cmd)
			return false
		}
		f.body, f.set["body"] = v, true
		return true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			f.asJSON = true
		case a == "-h" || a == "--help":
			if cmd == "propose-create" {
				fmt.Fprintf(stdout, "usage: %s skill workshop propose-create --name N (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n", brand.CLI)
			} else {
				fmt.Fprintf(stdout, "usage: %s skill workshop propose-update <id> (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n", brand.CLI)
			}
			return f, nil, false
		case a == "--name" && allowName:
			v, ok := readValue(i, a)
			if !ok {
				return f, nil, false
			}
			i++
			f.name, f.set["name"] = v, true
		case strings.HasPrefix(a, "--name=") && allowName:
			f.name, f.set["name"] = strings.TrimPrefix(a, "--name="), true
		case a == "--desc" || a == "--description":
			v, ok := readValue(i, a)
			if !ok {
				return f, nil, false
			}
			i++
			f.desc, f.set["desc"] = v, true
		case strings.HasPrefix(a, "--desc="):
			f.desc, f.set["desc"] = strings.TrimPrefix(a, "--desc="), true
		case strings.HasPrefix(a, "--description="):
			f.desc, f.set["desc"] = strings.TrimPrefix(a, "--description="), true
		case a == "--body":
			v, ok := readValue(i, a)
			if !ok || !setBody(v) {
				return f, nil, false
			}
			i++
		case strings.HasPrefix(a, "--body="):
			if !setBody(strings.TrimPrefix(a, "--body=")) {
				return f, nil, false
			}
		case a == "--body-file":
			v, ok := readValue(i, a)
			if !ok {
				return f, nil, false
			}
			i++
			body, ok := readWorkshopBodyFile(v, cmd, stderr)
			if !ok || !setBody(body) {
				return f, nil, false
			}
		case strings.HasPrefix(a, "--body-file="):
			body, ok := readWorkshopBodyFile(strings.TrimPrefix(a, "--body-file="), cmd, stderr)
			if !ok || !setBody(body) {
				return f, nil, false
			}
		case a == "--triggers":
			v, ok := readValue(i, a)
			if !ok {
				return f, nil, false
			}
			i++
			f.triggers, f.set["triggers"] = v, true
		case strings.HasPrefix(a, "--triggers="):
			f.triggers, f.set["triggers"] = strings.TrimPrefix(a, "--triggers="), true
		case a == "--tools" || a == "--tools-required":
			v, ok := readValue(i, a)
			if !ok {
				return f, nil, false
			}
			i++
			f.tools, f.set["tools"] = v, true
		case strings.HasPrefix(a, "--tools="):
			f.tools, f.set["tools"] = strings.TrimPrefix(a, "--tools="), true
		case strings.HasPrefix(a, "--tools-required="):
			f.tools, f.set["tools"] = strings.TrimPrefix(a, "--tools-required="), true
		case a == "--agent":
			v, ok := readValue(i, a)
			if !ok {
				return f, nil, false
			}
			i++
			f.agent, f.set["agent"] = v, true
		case strings.HasPrefix(a, "--agent="):
			f.agent, f.set["agent"] = strings.TrimPrefix(a, "--agent="), true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected flag %q\n", brand.CLI, cmd, a)
			return f, nil, false
		default:
			rest = append(rest, a)
		}
	}
	return f, rest, true
}

func readWorkshopBodyFile(path, cmd string, stderr io.Writer) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop %s: read --body-file %s: %v\n", brand.CLI, cmd, path, err)
		return "", false
	}
	return string(data), true
}

func workshopImportArgs(f workshopProposalFlags) map[string]any {
	out := map[string]any{
		"name":        f.name,
		"description": f.desc,
		"body":        f.body,
	}
	if xs := splitList(f.triggers); len(xs) > 0 {
		out["triggers"] = stringsToAny(xs)
	}
	if xs := splitList(f.tools); len(xs) > 0 {
		out["tools_required"] = stringsToAny(xs)
	}
	if strings.TrimSpace(f.agent) != "" {
		out["agent"] = strings.TrimSpace(f.agent)
	}
	return out
}

func parseWorkshopIDJSON(args []string, cmd string, stdout, stderr io.Writer) (string, bool, bool) {
	asJSON := false
	id := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop %s <id> [--json]\n", brand.CLI, cmd)
			return "", false, false
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected flag %q\n", brand.CLI, cmd, a)
			return "", false, false
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected arg %q\n", brand.CLI, cmd, a)
			return "", false, false
		}
	}
	return id, asJSON, true
}

func parseWorkshopReasonArgs(args []string, cmd, defaultReason string, stdout, stderr io.Writer) (string, string, bool, bool) {
	asJSON := false
	reason := defaultReason
	id := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop %s <id> [--reason R] [--json]\n", brand.CLI, cmd)
			return "", "", false, false
		case a == "--reason":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill workshop %s: --reason needs a value\n", brand.CLI, cmd)
				return "", "", false, false
			}
			i++
			reason = args[i]
		case strings.HasPrefix(a, "--reason="):
			reason = strings.TrimPrefix(a, "--reason=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected flag %q\n", brand.CLI, cmd, a)
			return "", "", false, false
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected arg %q\n", brand.CLI, cmd, a)
			return "", "", false, false
		}
	}
	return id, reason, asJSON, true
}

func workshopFetchSkill(ctx context.Context, c *controlplane.Client, id string) (map[string]any, bool, error) {
	res, err := c.Call(ctx, controlplane.CmdSkillGet, map[string]any{"id": id})
	if err != nil {
		return nil, false, err
	}
	found, _ := res["found"].(bool)
	if !found {
		return nil, false, nil
	}
	sk, _ := res["skill"].(map[string]any)
	return sk, sk != nil, nil
}

func workshopProposals(raw any) []map[string]any {
	var out []map[string]any
	items, _ := raw.([]any)
	for _, item := range items {
		sk, _ := item.(map[string]any)
		if sk == nil {
			continue
		}
		switch str(sk["status"]) {
		case "draft", "shadow":
			out = append(out, sk)
		}
	}
	return out
}

func workshopCanReject(status string) bool {
	return status == "draft" || status == "shadow"
}


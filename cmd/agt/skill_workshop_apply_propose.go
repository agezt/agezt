// SPDX-License-Identifier: MIT

package main

// agt skill workshop propose workflow: callSkillWorkshopImport +
// callSkillWorkshopImportWithClient + parseWorkshopProposalFlags +
// readWorkshopBodyFile + workshopImportArgs. Carved out of
// skill_workshop_apply.go during the Day 184 god-file split so the
// apply/main file can stay focused on the 5 cmd entry points +
// help + transition dispatch.
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


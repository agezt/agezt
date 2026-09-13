// SPDX-License-Identifier: MIT
//
// cmd/agt okr command: the entry dispatcher + the read-only
// sub-commands (List + Show).
// The mutation sub-commands live in okr_mutate.go; the parse / render /
// dispatch helpers live in okr_helpers.go.
// Extracted from okr.go during the Day-209 god-file split.
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdOKR(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return okrUsage(stderr)
	}
	switch args[0] {
	case "list", "ls":
		return cmdOKRList(args[1:], stdout, stderr)
	case "show", "get":
		return cmdOKRShow(args[1:], stdout, stderr)
	case "create", "add":
		return cmdOKRCreate(args[1:], stdout, stderr)
	case "kr", "keyresult":
		return cmdOKRKeyResult(args[1:], stdout, stderr)
	case "link":
		return cmdOKRLink(args[1:], stdout, stderr, controlplane.CmdOKRLink, "link")
	case "unlink":
		return cmdOKRLink(args[1:], stdout, stderr, controlplane.CmdOKRUnlink, "unlink")
	case "archive":
		return cmdOKRArchive(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return okrUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s okr: unknown subcommand %q\n", brand.CLI, args[0])
		return okrUsage(stderr)
	}
}

func okrUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s okr <list|show|create|kr|link|unlink|archive>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--status S] [--tenant T] [--limit N] [--archived] [--json]\n")
	fmt.Fprintf(w, "  show <id> [--json]\n")
	fmt.Fprintf(w, "  create --title T [--desc D] [--owner O] [--tenant T] [--json]\n")
	fmt.Fprintf(w, "  kr <id> --title T [--target N] [--json]        (add a key result; target 0 = all linked tasks)\n")
	fmt.Fprintf(w, "  link <id> --kr KR --task TASK [--json]         (roll a workboard task up into a key result)\n")
	fmt.Fprintf(w, "  unlink <id> --kr KR --task TASK [--json]\n")
	fmt.Fprintf(w, "  archive <id> [--json]\n")
	return 0
}

func cmdOKRList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--archived":
			callArgs["include_archived"] = true
		case "--status", "--tenant", "--limit":
			val, ok := okrFlagValue(args, &i, args[i], stderr)
			if !ok {
				return 2
			}
			switch args[i-1] {
			case "--status":
				callArgs["status"] = val
			case "--tenant":
				callArgs["tenant"] = val
			case "--limit":
				n, err := strconv.Atoi(val)
				if err != nil {
					fmt.Fprintf(stderr, "%s okr list: --limit needs an integer\n", brand.CLI)
					return 2
				}
				callArgs["limit"] = n
			}
		default:
			fmt.Fprintf(stderr, "%s okr list: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
	}
	res, code := callOKR(controlplane.CmdOKRList, callArgs, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	objs, _ := res["objectives"].([]any)
	if len(objs) == 0 {
		fmt.Fprintln(stdout, "no objectives")
		return 0
	}
	for _, raw := range objs {
		renderOKRLine(stdout, mapAny(raw))
	}
	return 0
}

func cmdOKRShow(args []string, stdout, stderr io.Writer) int {
	id, asJSON, ok := okrIDArg(args, "show", stderr)
	if !ok {
		return 2
	}
	res, code := callOKR(controlplane.CmdOKRShow, map[string]any{"id": id}, stderr)
	if code != 0 {
		return code
	}
	obj := mapAny(res["objective"])
	if asJSON {
		return jsonout.Write(stdout, obj)
	}
	renderOKRObjective(stdout, obj)
	return 0
}

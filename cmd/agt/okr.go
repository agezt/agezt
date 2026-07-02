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
		return encodeJSON(stdout, res)
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
		return encodeJSON(stdout, obj)
	}
	renderOKRObjective(stdout, obj)
	return 0
}

func cmdOKRCreate(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--title", "--desc", "--description", "--owner", "--tenant":
			val, ok := okrFlagValue(args, &i, a, stderr)
			if !ok {
				return 2
			}
			switch a {
			case "--title":
				callArgs["title"] = val
			case "--desc", "--description":
				callArgs["description"] = val
			case "--owner":
				callArgs["owner"] = val
			case "--tenant":
				callArgs["tenant"] = val
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s okr create: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if _, exists := callArgs["title"]; !exists {
				callArgs["title"] = a
				continue
			}
			fmt.Fprintf(stderr, "%s okr create: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if str(callArgs["title"]) == "" {
		fmt.Fprintf(stderr, "usage: %s okr create --title T\n", brand.CLI)
		return 2
	}
	res, code := callOKR(controlplane.CmdOKRCreate, callArgs, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}

func cmdOKRKeyResult(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--title", "--target":
			val, ok := okrFlagValue(args, &i, a, stderr)
			if !ok {
				return 2
			}
			if a == "--title" {
				callArgs["title"] = val
			} else {
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 {
					fmt.Fprintf(stderr, "%s okr kr: --target needs a non-negative integer\n", brand.CLI)
					return 2
				}
				callArgs["target"] = n
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s okr kr: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if _, exists := callArgs["id"]; !exists {
				callArgs["id"] = a
				continue
			}
			fmt.Fprintf(stderr, "%s okr kr: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if str(callArgs["id"]) == "" || str(callArgs["title"]) == "" {
		fmt.Fprintf(stderr, "usage: %s okr kr <id> --title T [--target N]\n", brand.CLI)
		return 2
	}
	res, code := callOKR(controlplane.CmdOKRKeyResult, callArgs, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}

func cmdOKRLink(args []string, stdout, stderr io.Writer, cmd, name string) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--kr", "--key-result", "--task":
			val, ok := okrFlagValue(args, &i, a, stderr)
			if !ok {
				return 2
			}
			if a == "--task" {
				callArgs["task"] = val
			} else {
				callArgs["key_result"] = val
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s okr %s: unexpected flag %q\n", brand.CLI, name, a)
				return 2
			}
			if _, exists := callArgs["id"]; !exists {
				callArgs["id"] = a
				continue
			}
			fmt.Fprintf(stderr, "%s okr %s: unexpected arg %q\n", brand.CLI, name, a)
			return 2
		}
	}
	if str(callArgs["id"]) == "" || str(callArgs["key_result"]) == "" || str(callArgs["task"]) == "" {
		fmt.Fprintf(stderr, "usage: %s okr %s <id> --kr KR --task TASK\n", brand.CLI, name)
		return 2
	}
	res, code := callOKR(cmd, callArgs, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}

func cmdOKRArchive(args []string, stdout, stderr io.Writer) int {
	id, asJSON, ok := okrIDArg(args, "archive", stderr)
	if !ok {
		return 2
	}
	res, code := callOKR(controlplane.CmdOKRArchive, map[string]any{"id": id}, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}

// --- helpers ---

func okrFlagValue(args []string, i *int, flag string, stderr io.Writer) (string, bool) {
	if *i+1 >= len(args) {
		fmt.Fprintf(stderr, "%s okr: %s needs a value\n", brand.CLI, flag)
		return "", false
	}
	*i++
	return args[*i], true
}

func okrIDArg(args []string, name string, stderr io.Writer) (string, bool, bool) {
	id := ""
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s okr %s: unexpected flag %q\n", brand.CLI, name, a)
				return "", false, false
			}
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "usage: %s okr %s <id>\n", brand.CLI, name)
		return "", false, false
	}
	return id, asJSON, true
}

func callOKR(cmd string, args map[string]any, stderr io.Writer) (map[string]any, int) {
	c := dial(stderr)
	if c == nil {
		return nil, 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s okr: %v\n", brand.CLI, err)
		return nil, 1
	}
	return res, 0
}

func renderOKRMutation(res map[string]any, code int, asJSON bool, stdout io.Writer) int {
	if code != 0 {
		return code
	}
	obj := mapAny(res["objective"])
	if asJSON {
		return encodeJSON(stdout, obj)
	}
	renderOKRLine(stdout, obj)
	return 0
}

func renderOKRLine(w io.Writer, obj map[string]any) {
	fmt.Fprintf(w, "%-26s %-9s %3d%%  %s\n",
		shortID(str(obj["id"])), str(obj["status"]), intNumber(obj["percent"]), str(obj["title"]))
}

func renderOKRObjective(w io.Writer, obj map[string]any) {
	fmt.Fprintf(w, "id:       %s\n", str(obj["id"]))
	fmt.Fprintf(w, "title:    %s\n", str(obj["title"]))
	fmt.Fprintf(w, "status:   %s  (%d%%)\n", str(obj["status"]), intNumber(obj["percent"]))
	if d := str(obj["description"]); d != "" {
		fmt.Fprintf(w, "desc:     %s\n", d)
	}
	if owner := str(obj["owner"]); owner != "" {
		fmt.Fprintf(w, "owner:    %s\n", owner)
	}
	progress := mapAny(obj["progress"])
	krs, _ := progress["key_results"].([]any)
	if len(krs) == 0 {
		fmt.Fprintln(w, "key results: none")
		return
	}
	fmt.Fprintln(w, "key results:")
	for _, raw := range krs {
		kr := mapAny(raw)
		mark := " "
		if achieved, _ := kr["achieved"].(bool); achieved {
			mark = "✓"
		}
		fmt.Fprintf(w, "  %s %s  %d/%d done (target %d) — %d%%\n",
			mark, str(kr["title"]),
			intNumber(kr["done"]), intNumber(kr["total"]), intNumber(kr["target"]), intNumber(kr["percent"]))
	}
}

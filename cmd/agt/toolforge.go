// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdToolforge dispatches `agt toolforge <subcommand>` — the operator surface
// of the script-tool forge (M794): agent-authored (or operator-authored)
// scripts tested in the code_exec sandbox and promoted into callable
// forge_<name> tools. Promotion lives HERE, not in the agent's tool, so a
// human signs off before code goes live. Every mutation is journaled
// (scripttool.*).
func cmdToolforge(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return toolforgeUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdToolforgeList(args[1:], stdout, stderr)
	case "show":
		return cmdToolforgeShow(args[1:], stdout, stderr)
	case "draft", "add", "create":
		return cmdToolforgeDraft(args[1:], stdout, stderr)
	case "edit", "set":
		return cmdToolforgeEdit(args[1:], stdout, stderr)
	case "test":
		return cmdToolforgeTest(args[1:], stdout, stderr)
	case "promote":
		return cmdToolforgePromote(args[1:], stdout, stderr)
	case "quarantine":
		return cmdToolforgeQuarantine(args[1:], stdout, stderr)
	case "remove", "rm":
		return cmdToolforgeRemove(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return toolforgeUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s toolforge: unknown subcommand %q\n", brand.CLI, args[0])
		return toolforgeUsage(stderr)
	}
}

func toolforgeUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s toolforge <list|show|draft|edit|test|promote|quarantine|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                   show all script tools\n")
	fmt.Fprintf(w, "  show <name|id> [--json]                         one tool's full record (incl. code)\n")
	fmt.Fprintf(w, "  draft <name> --lang L --desc TEXT (--file PATH | --code SRC) [--schema-file PATH]\n")
	fmt.Fprintf(w, "  edit <name|id> [--desc TEXT] [--lang L] [--file PATH | --code SRC] [--schema-file PATH]\n")
	fmt.Fprintf(w, "       (a code change demotes the tool to draft and clears its test record)\n")
	fmt.Fprintf(w, "  test <name|id> [--input JSON]                   run the code once in the sandbox; promotion requires a pass\n")
	fmt.Fprintf(w, "  promote <name|id>                               make a TESTED tool live (callable as forge_<name>)\n")
	fmt.Fprintf(w, "  quarantine <name|id> [--reason TEXT]            pull a live tool from production (kill switch)\n")
	fmt.Fprintf(w, "  remove <name|id>                                delete a tool\n")
	fmt.Fprintf(w, "script contract: the call's JSON input is in ./stdin.txt; print the result to stdout; exit non-zero on failure\n")
	return 0
}

func cmdToolforgeList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgeList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	tools, _ := res["tools"].([]any)
	if len(tools) == 0 {
		fmt.Fprintf(stdout, "no script tools yet — draft one with `%s toolforge draft <name> --lang python --desc \"...\" --file tool.py`\n", brand.CLI)
		return 0
	}
	for _, raw := range tools {
		st, _ := raw.(map[string]any)
		if st == nil {
			continue
		}
		status, _ := st["status"].(string)
		tested := "untested"
		if ok, _ := st["tested_ok"].(bool); ok {
			tested = "tested"
		}
		fmt.Fprintf(stdout, "%-24s %-12s %-8s lang=%s", str(st["name"]), strings.ToUpper(status), tested, str(st["language"]))
		if ca := str(st["callable_as"]); ca != "" {
			fmt.Fprintf(stdout, " → %s", ca)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v tool(s), %v live\n", res["count"], res["active_count"])
	return 0
}

func cmdToolforgeShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	ref := ""
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if !strings.HasPrefix(a, "--") && ref == "" {
			ref = a
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s toolforge show <name|id> [--json]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgeShow, map[string]any{"ref": ref})
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge show: %v\n", brand.CLI, err)
		return 1
	}
	st, _ := res["tool"].(map[string]any)
	if st == nil {
		fmt.Fprintf(stderr, "%s toolforge show: unknown tool %q\n", brand.CLI, ref)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, st)
	}
	fmt.Fprintf(stdout, "name:        %s\n", str(st["name"]))
	fmt.Fprintf(stdout, "id:          %s\n", str(st["id"]))
	fmt.Fprintf(stdout, "status:      %s\n", str(st["status"]))
	fmt.Fprintf(stdout, "language:    %s\n", str(st["language"]))
	tested := "no"
	if ok, _ := st["tested_ok"].(bool); ok {
		tested = "yes"
	}
	fmt.Fprintf(stdout, "tested:      %s\n", tested)
	if ca := str(st["callable_as"]); ca != "" {
		fmt.Fprintf(stdout, "callable as: %s\n", ca)
	}
	if v := str(st["description"]); v != "" {
		fmt.Fprintf(stdout, "description: %s\n", v)
	}
	if v := str(st["input_schema"]); v != "" {
		fmt.Fprintf(stdout, "schema:      %s\n", v)
	}
	if v := str(st["code"]); v != "" {
		fmt.Fprintf(stdout, "code:\n  %s\n", strings.ReplaceAll(v, "\n", "\n  "))
	}
	return 0
}

// toolforgeFlags parses the shared draft/edit flag set. Code can ride from a
// file (--file, the natural authoring path) or inline (--code).
type toolforgeFlags struct {
	desc, lang, code, schema string
	set                      map[string]bool
}

func parseToolforgeFlags(args []string, stderr io.Writer, cmd string) (toolforgeFlags, []string, bool) {
	f := toolforgeFlags{set: map[string]bool{}}
	var rest []string
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s toolforge %s: %s needs a value\n", brand.CLI, cmd, flag)
			return false
		}
		return true
	}
	readFile := func(path, what string) (string, bool) {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "%s toolforge %s: read %s: %v\n", brand.CLI, cmd, what, err)
			return "", false
		}
		return string(b), true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--desc", "--description":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.desc, f.set["desc"] = args[i], true
		case "--lang", "--language":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.lang, f.set["lang"] = args[i], true
		case "--code":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.code, f.set["code"] = args[i], true
		case "--file":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			src, ok := readFile(args[i], "--file")
			if !ok {
				return f, nil, false
			}
			f.code, f.set["code"] = src, true
		case "--schema-file":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			sch, ok := readFile(args[i], "--schema-file")
			if !ok {
				return f, nil, false
			}
			f.schema, f.set["schema"] = sch, true
		default:
			rest = append(rest, a)
		}
	}
	return f, rest, true
}

func cmdToolforgeDraft(args []string, stdout, stderr io.Writer) int {
	f, rest, ok := parseToolforgeFlags(args, stderr, "draft")
	if !ok {
		return 2
	}
	if len(rest) != 1 || !f.set["code"] {
		fmt.Fprintf(stderr, "usage: %s toolforge draft <name> --lang L --desc TEXT (--file PATH | --code SRC) [--schema-file PATH]\n", brand.CLI)
		return 2
	}
	tool := map[string]any{
		"name": rest[0], "description": f.desc, "language": f.lang,
		"code": f.code, "input_schema": f.schema,
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgeDraft, map[string]any{"tool": tool})
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge draft: %v\n", brand.CLI, err)
		return 1
	}
	st, _ := res["tool"].(map[string]any)
	fmt.Fprintf(stdout, "drafted %s (%s) — test it with `%s toolforge test %s`, then promote\n",
		str(st["name"]), str(st["language"]), brand.CLI, str(st["name"]))
	return 0
}

func cmdToolforgeEdit(args []string, stdout, stderr io.Writer) int {
	f, rest, ok := parseToolforgeFlags(args, stderr, "edit")
	if !ok {
		return 2
	}
	if len(rest) != 1 || len(f.set) == 0 {
		fmt.Fprintf(stderr, "usage: %s toolforge edit <name|id> [--desc TEXT] [--lang L] [--file PATH | --code SRC] [--schema-file PATH]\n", brand.CLI)
		return 2
	}
	tool := map[string]any{
		"description": f.desc, "language": f.lang, "code": f.code, "input_schema": f.schema,
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgeEdit, map[string]any{"ref": rest[0], "tool": tool})
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge edit: %v\n", brand.CLI, err)
		return 1
	}
	st, _ := res["tool"].(map[string]any)
	note := ""
	if ok, _ := st["tested_ok"].(bool); !ok {
		note = " (draft again — re-test before promoting)"
	}
	fmt.Fprintf(stdout, "updated %s%s\n", str(st["name"]), note)
	return 0
}

func cmdToolforgeTest(args []string, stdout, stderr io.Writer) int {
	ref, input := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--input":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s toolforge test: --input needs a value\n", brand.CLI)
				return 2
			}
			i++
			input = args[i]
		default:
			if !strings.HasPrefix(args[i], "--") && ref == "" {
				ref = args[i]
			}
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s toolforge test <name|id> [--input JSON]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	// A sandbox run can legitimately take minutes (pip installs, network) —
	// give it the sandbox's own ceiling rather than the 5s management budget.
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgeTest, map[string]any{"ref": ref, "input": input})
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge test: %v\n", brand.CLI, err)
		return 1
	}
	out, _ := res["output"].(string)
	if passed, _ := res["ok"].(bool); passed {
		fmt.Fprintf(stdout, "PASS — promote with `%s toolforge promote %s`\n%s\n", brand.CLI, ref, out)
		return 0
	}
	fmt.Fprintf(stdout, "FAIL — fix the code and re-test\n%s\n", out)
	return 1
}

func cmdToolforgePromote(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s toolforge promote <name|id>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgePromote, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge promote: %v\n", brand.CLI, err)
		return 1
	}
	st, _ := res["tool"].(map[string]any)
	fmt.Fprintf(stdout, "promoted %s — live for every run as %s\n", str(st["name"]), str(st["callable_as"]))
	return 0
}

func cmdToolforgeQuarantine(args []string, stdout, stderr io.Writer) int {
	ref, reason := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reason":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s toolforge quarantine: --reason needs a value\n", brand.CLI)
				return 2
			}
			i++
			reason = args[i]
		default:
			if !strings.HasPrefix(args[i], "--") && ref == "" {
				ref = args[i]
			}
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s toolforge quarantine <name|id> [--reason TEXT]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgeQuarantine, map[string]any{"ref": ref, "reason": reason})
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge quarantine: %v\n", brand.CLI, err)
		return 1
	}
	st, _ := res["tool"].(map[string]any)
	fmt.Fprintf(stdout, "quarantined %s — no longer offered to runs\n", str(st["name"]))
	return 0
}

func cmdToolforgeRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s toolforge remove <name|id>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolforgeRemove, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s toolforge remove: %v\n", brand.CLI, err)
		return 1
	}
	if ok, _ := res["removed"].(bool); !ok {
		fmt.Fprintf(stderr, "%s toolforge remove: unknown tool %q\n", brand.CLI, args[0])
		return 1
	}
	fmt.Fprintf(stdout, "removed %s\n", args[0])
	return 0
}

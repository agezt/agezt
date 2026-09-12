// SPDX-License-Identifier: MIT

// agt toolforge command: dispatcher + Usage + List + Show + toolforgeFlags + parseToolforgeFlags.
// Code extracted from toolforge.go during the Day-126 god-file split.
// Public API unchanged.
package main


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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, st)
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


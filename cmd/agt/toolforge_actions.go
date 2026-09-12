// SPDX-License-Identifier: MIT

// agt toolforge command: mutation verbs (Draft + Edit + Test + Promote + Quarantine + Remove).
// Code extracted from toolforge.go during the Day-126 god-file split.
// Public API unchanged.
package main


import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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

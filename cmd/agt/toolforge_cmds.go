// SPDX-License-Identifier: MIT
//
// cmd/agt `toolforge list` + `toolforge show` subcommands (cmdToolforgeList, cmdToolforgeShow).
// Extracted from toolforge.go during Day 211 god-file refactor (#97).
// Public API unchanged.
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

// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdTaste(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return tasteUsage(stderr)
	}
	switch args[0] {
	case "list", "ls":
		return cmdTasteList(args[1:], stdout, stderr)
	case "add", "create":
		return cmdTasteAdd(args[1:], stdout, stderr)
	case "remove", "rm", "delete":
		return cmdTasteRemove(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return tasteUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s taste: unknown subcommand %q\n", brand.CLI, args[0])
		return tasteUsage(stderr)
	}
}

func tasteUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s taste <list|add|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--scope S] [--tag T] [--limit N] [--json]\n")
	fmt.Fprintf(w, "  add --title T --body TEXT [--scope AGENT] [--tag X] [--json]   (scope empty = every run)\n")
	fmt.Fprintf(w, "  remove <id> [--json]\n")
	return 0
}

func cmdTasteList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--scope", "--tag", "--limit":
			val, ok := tasteFlagValue(args, &i, args[i], stderr)
			if !ok {
				return 2
			}
			switch args[i-1] {
			case "--scope":
				callArgs["scope"] = val
			case "--tag":
				callArgs["tag"] = val
			case "--limit":
				callArgs["limit"] = val
			}
		default:
			fmt.Fprintf(stderr, "%s taste list: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
	}
	res, code := callTaste(controlplane.CmdTasteList, callArgs, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	exemplars, _ := res["exemplars"].([]any)
	if len(exemplars) == 0 {
		fmt.Fprintln(stdout, "no exemplars")
		return 0
	}
	for _, raw := range exemplars {
		e := mapAny(raw)
		scope := str(e["scope"])
		if scope == "" {
			scope = "all"
		}
		fmt.Fprintf(stdout, "%-26s [%-10s] %s\n", shortID(str(e["id"])), scope, str(e["title"]))
	}
	return 0
}

func cmdTasteAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	var tags []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--title", "--body", "--scope", "--tag":
			val, ok := tasteFlagValue(args, &i, a, stderr)
			if !ok {
				return 2
			}
			switch a {
			case "--title":
				callArgs["title"] = val
			case "--body":
				callArgs["body"] = val
			case "--scope":
				callArgs["scope"] = val
			case "--tag":
				tags = append(tags, val)
			}
		default:
			fmt.Fprintf(stderr, "%s taste add: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if len(tags) > 0 {
		callArgs["tags"] = tags
	}
	if str(callArgs["title"]) == "" || str(callArgs["body"]) == "" {
		fmt.Fprintf(stderr, "usage: %s taste add --title T --body TEXT\n", brand.CLI)
		return 2
	}
	res, code := callTaste(controlplane.CmdTasteCreate, callArgs, stderr)
	if code != 0 {
		return code
	}
	e := mapAny(res["exemplar"])
	if asJSON {
		return encodeJSON(stdout, e)
	}
	fmt.Fprintf(stdout, "added %s %s\n", shortID(str(e["id"])), str(e["title"]))
	return 0
}

func cmdTasteRemove(args []string, stdout, stderr io.Writer) int {
	id := ""
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s taste remove: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "usage: %s taste remove <id>\n", brand.CLI)
		return 2
	}
	res, code := callTaste(controlplane.CmdTasteDelete, map[string]any{"id": id}, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	fmt.Fprintf(stdout, "removed %s\n", str(res["deleted"]))
	return 0
}

func tasteFlagValue(args []string, i *int, flag string, stderr io.Writer) (string, bool) {
	if *i+1 >= len(args) {
		fmt.Fprintf(stderr, "%s taste: %s needs a value\n", brand.CLI, flag)
		return "", false
	}
	*i++
	return args[*i], true
}

func callTaste(cmd string, args map[string]any, stderr io.Writer) (map[string]any, int) {
	c := dial(stderr)
	if c == nil {
		return nil, 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s taste: %v\n", brand.CLI, err)
		return nil, 1
	}
	return res, 0
}

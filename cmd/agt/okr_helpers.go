// SPDX-License-Identifier: MIT
//
// cmd/agt okr helpers: okrFlagValue + okrIDArg (the arg parsers) +
// callOKR (the dispatcher caller) + renderOKRMutation + renderOKRLine +
// renderOKRObjective (the renderers).
// Extracted from okr.go during the Day-209 god-file split.
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, obj)
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

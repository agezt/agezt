// SPDX-License-Identifier: MIT
//
// cmd/agt `toolforge` shared helpers (toolforgeFlags type + parseToolforgeFlags).
// Extracted from toolforge.go during Day 211 god-file refactor (#97).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/agezt/agezt/internal/brand"
)

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

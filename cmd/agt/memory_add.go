// SPDX-License-Identifier: MIT

package main

// agt memory ADD subcommand: cmdMemoryAdd + validMemoryEvidence.
// Carved out of memory.go during the Day 199 god-file split so the
// main file can stay focused on the dispatcher and the list file
// can stay focused on the read-side subcommands.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdMemoryAdd implements `agt memory add <subject> <content> [flags]`.
func cmdMemoryAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	typ := ""
	conf := 0.0
	evidence := ""
	halfLifeMS := int64(0)
	tags := map[string]any{}
	var positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory add <subject> <content> [--type T] [--evidence E] [--half-life D] [--tag k=v] [--conf F] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "types: FACT (default) | SUMMARY | RELATION | PREFERENCE | OBSERVATION\n")
			fmt.Fprintf(stdout, "evidence: observed | inferred | curated | constraint\n")
			return 0
		case a == "--type":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --type needs a value\n", brand.CLI)
				return 2
			}
			i++
			typ = strings.ToUpper(args[i])
		case a == "--evidence":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --evidence needs a value\n", brand.CLI)
				return 2
			}
			i++
			evidence = strings.ToLower(args[i])
			if !validMemoryEvidence(evidence) {
				fmt.Fprintf(stderr, "%s memory add: bad --evidence %q\n", brand.CLI, args[i])
				return 2
			}
		case a == "--half-life":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --half-life needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s memory add: bad --half-life %q\n", brand.CLI, args[i])
				return 2
			}
			halfLifeMS = d.Milliseconds()
		case a == "--conf":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --conf needs a value\n", brand.CLI)
				return 2
			}
			i++
			f, err := strconv.ParseFloat(args[i], 64)
			if err != nil {
				fmt.Fprintf(stderr, "%s memory add: --conf must be a number: %v\n", brand.CLI, err)
				return 2
			}
			conf = f
		case a == "--tag":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --tag needs k=v\n", brand.CLI)
				return 2
			}
			i++
			k, v, ok := strings.Cut(args[i], "=")
			if !ok || k == "" {
				fmt.Fprintf(stderr, "%s memory add: --tag must be k=v, got %q\n", brand.CLI, args[i])
				return 2
			}
			tags[k] = v
		default:
			positional = append(positional, a)
		}
	}

	// Grammar: <subject> <content>. A single positional is treated as
	// content with an empty subject (subjects are optional in the store).
	var subject, content string
	switch len(positional) {
	case 1:
		content = positional[0]
	case 2:
		subject, content = positional[0], positional[1]
	default:
		fmt.Fprintf(stderr, "%s memory add: expected <subject> <content> (or just <content>)\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{"subject": subject, "content": content}
	if typ != "" {
		callArgs["type"] = typ
	}
	if conf > 0 {
		callArgs["confidence"] = conf
	}
	if evidence != "" {
		callArgs["evidence"] = evidence
	}
	if halfLifeMS > 0 {
		callArgs["half_life_ms"] = halfLifeMS
	}
	if len(tags) > 0 {
		callArgs["tags"] = tags
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryAdd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory add: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	id, _ := res["id"].(string)
	created, _ := res["created"].(bool)
	verb := "reinforced"
	if created {
		verb = "stored"
	}
	fmt.Fprintf(stdout, "%s %s\n", verb, id)
	return 0
}

func validMemoryEvidence(v string) bool {
	switch v {
	case "observed", "inferred", "curated", "constraint":
		return true
	default:
		return false
	}
}


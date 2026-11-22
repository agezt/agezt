package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt runs sub-command top-level
//             handlers (cmdRuns, cmdRunsSteerVerb, cmdRunsSteer). Extracted from
//             runs.go during Day 211 god-file refactor (#36, #59). Public API
//             unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func cmdRuns(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s runs: subcommand required (list|show)\n", brand.CLI)
		return 2
	}
	if sub, ok := cliRuns.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s runs: unknown subcommand %q (list|show|last|stats|cancel|pause|resume|step|steer|intervene)\n", brand.CLI, args[0])
	return 2
}
func cmdRunsSteerVerb(cmd, verb string, args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs %s <correlation> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if corr == "" {
				corr = a
				continue
			}
			fmt.Fprintf(stderr, "%s runs %s: unexpected arg %q\n", brand.CLI, verb, a)
			return 2
		}
	}
	if corr == "" {
		fmt.Fprintf(stderr, "%s runs %s: correlation id required\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"correlation": corr})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if ok, _ := res["ok"].(bool); ok {
		fmt.Fprintf(stdout, "%s: %s\n", corr, verb+"d")
		return 0
	}
	fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
	return 1
}
func cmdRunsSteer(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	var parts []string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s runs steer <correlation> <directive...> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "inject a directive into a running agent (folded into its next prompt)\n")
			return 0
		case corr == "":
			corr = a
		default:
			parts = append(parts, a)
		}
	}
	if corr == "" || len(parts) == 0 {
		fmt.Fprintf(stderr, "%s runs steer: correlation id and a directive are required\n", brand.CLI)
		return 2
	}
	directive := strings.Join(parts, " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, opapi.RunSteer, map[string]any{"correlation": corr, "directive": directive})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs steer: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if acc, _ := res["accepted"].(bool); acc {
		fmt.Fprintf(stdout, "%s: steering directive accepted\n", corr)
		return 0
	}
	fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
	return 1
}

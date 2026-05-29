// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ersinkoc/agezt/internal/brand"
	"github.com/ersinkoc/agezt/kernel/planner"
)

// cmdPlanValidate implements `agt plan validate <file.json>`.
// Pure client-side: runs planner.ValidateJSON over the file and
// prints either a one-line OK with node count, or the first
// validation error. Exit code 0 on valid, 1 on invalid.
//
// **Why no daemon RPC.** Plan validation is a pure function over
// the JSON — no providers, no credentials, no scheduler state.
// Operators want this in CI before the plan ever reaches a
// daemon, so client-side execution beats a round-trip.
func cmdPlanValidate(args []string, stdout, stderr io.Writer) int {
	path := ""
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s plan validate <file.json>\n", brand.CLI)
			fmt.Fprintf(stdout, "verify a hand-authored plan against the same validators the daemon applies before execution\n")
			return 0
		default:
			if path == "" {
				path = a
			} else {
				fmt.Fprintf(stderr, "%s plan validate: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}
	if path == "" {
		fmt.Fprintf(stderr, "%s plan validate: plan file path required\n", brand.CLI)
		return 2
	}
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s plan validate: read %s: %v\n", brand.CLI, path, err)
		return 1
	}
	plan, err := planner.ValidateJSON(body)
	if err != nil {
		fmt.Fprintf(stderr, "%s plan validate: %s: %v\n", brand.CLI, path, err)
		return 1
	}
	name := plan.Name
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Fprintf(stdout, "ok: %s — %s, %d node(s), max_parallel=%d\n",
		path, name, len(plan.Nodes), plan.MaxParallel)
	return 0
}

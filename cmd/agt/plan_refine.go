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

// cmdPlanRefine implements `agt plan refine <file.json> --feedback "..."`.
//
// Reads an existing plan from disk, ships it + the feedback string
// to the daemon's CmdPlanRefine endpoint, prints the revised plan
// JSON to stdout (operator can redirect to a file or pipe to `agt
// plan run-` for execution).
//
// **Why the operator runs this manually.** M1.uu's contract is that
// re-planning stays human-in-the-loop. Refine never fires
// automatically from a failed run; the operator chooses to re-plan
// and provides the change instruction. This bounds the LLM-spend
// per task to "as many times as the operator chooses to call it"
// rather than "as many times as the agent loop wants to."
func cmdPlanRefine(args []string, stdout, stderr io.Writer) int {
	path := ""
	feedback := ""
	model := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--feedback", "-f":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s plan refine: --feedback needs a value\n", brand.CLI)
				return 2
			}
			feedback = args[i]
		case "--model", "-m":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s plan refine: --model needs a value\n", brand.CLI)
				return 2
			}
			model = args[i]
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s plan refine <file.json> --feedback \"<change request>\"\n", brand.CLI)
			fmt.Fprintf(stdout, "produce a revised plan based on operator feedback (M1.uu)\n")
			return 0
		default:
			if path == "" {
				path = args[i]
			} else {
				fmt.Fprintf(stderr, "%s plan refine: unexpected arg %q\n", brand.CLI, args[i])
				return 2
			}
		}
	}
	if path == "" {
		fmt.Fprintf(stderr, "%s plan refine: original plan file path required\n", brand.CLI)
		return 2
	}
	if strings.TrimSpace(feedback) == "" {
		fmt.Fprintf(stderr, "%s plan refine: --feedback required (describe what to change)\n", brand.CLI)
		return 2
	}
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s plan refine: read %s: %v\n", brand.CLI, path, err)
		return 1
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	args2 := map[string]any{
		"plan_json": string(body),
		"feedback":  feedback,
	}
	if model != "" {
		args2["model"] = model
	}
	res, err := c.Call(ctx, controlplane.CmdPlanRefine, args2)
	if err != nil {
		fmt.Fprintf(stderr, "%s plan refine: %v\n", brand.CLI, err)
		return 1
	}
	planJSON, _ := res["plan_json"].(string)
	if planJSON == "" {
		fmt.Fprintf(stderr, "%s plan refine: daemon returned empty plan_json\n", brand.CLI)
		return 1
	}
	fmt.Fprintln(stdout, planJSON)
	return 0
}

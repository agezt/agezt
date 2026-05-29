// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/planner"
)

// cmdPlanCost implements `agt plan cost <file.json> --model <id>`.
// Reads a plan from disk, runs planner.EstimateCost against the
// governor's pricing table, prints a per-node breakdown and total
// in USD.
//
// **Why client-side, not a control-plane RPC.** Cost estimation
// is a pure function over (plan JSON, model id, price table). The
// price table is static (catalog-driven, but seeded at compile
// time too), so the CLI can compute the answer without bothering
// the daemon. Operators get instant feedback even if the daemon
// is down.
func cmdPlanCost(args []string, stdout, stderr io.Writer) int {
	path := ""
	model := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model", "-m":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s plan cost: --model needs a value\n", brand.CLI)
				return 2
			}
			model = args[i]
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s plan cost <file.json> --model <model-id>\n", brand.CLI)
			fmt.Fprintf(stdout, "estimate run cost of a plan using the governor's price table\n")
			return 0
		default:
			if path == "" {
				path = args[i]
			} else {
				fmt.Fprintf(stderr, "%s plan cost: unexpected arg %q\n", brand.CLI, args[i])
				return 2
			}
		}
	}
	if path == "" {
		fmt.Fprintf(stderr, "%s plan cost: plan file path required\n", brand.CLI)
		return 2
	}
	if model == "" {
		fmt.Fprintf(stderr, "%s plan cost: --model required (try the default you'd run with: e.g. claude-sonnet-4-6)\n", brand.CLI)
		return 2
	}
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s plan cost: read %s: %v\n", brand.CLI, path, err)
		return 1
	}
	var plan planner.Plan
	if err := json.Unmarshal(body, &plan); err != nil {
		fmt.Fprintf(stderr, "%s plan cost: parse %s: %v\n", brand.CLI, path, err)
		return 1
	}

	est, err := planner.EstimateCost(plan, model, governorEstimator{})
	if err != nil {
		fmt.Fprintf(stderr, "%s plan cost: %v\n", brand.CLI, err)
		return 1
	}

	fmt.Fprintf(stdout, "plan:   %s (%d nodes)\n", path, len(est.Nodes))
	fmt.Fprintf(stdout, "model:  %s\n", est.Model)
	fmt.Fprintf(stdout, "assumed: %d input + %d output tokens per loop node (rough)\n",
		planner.PerNodeInputTokens, planner.PerNodeOutputTokens)
	fmt.Fprintf(stdout, "\nper-node:\n")
	for _, n := range est.Nodes {
		fmt.Fprintf(stdout, "  %-20s  %-6s  %s\n",
			n.ID, n.Kind, planner.FormatUSD(n.Microcents))
	}
	fmt.Fprintf(stdout, "\nestimated total: %s\n", planner.FormatUSD(est.TotalMicrocents))
	fmt.Fprintf(stdout, "(estimate — actual usage depends on tool output sizes and model behaviour)\n")
	return 0
}

// governorEstimator adapts the governor's CostMicrocents function
// to the planner.CostEstimator interface. Tiny shim that exists
// only to satisfy the interface boundary.
type governorEstimator struct{}

func (governorEstimator) CostMicrocents(model string, inputTokens, outputTokens int) int64 {
	return governor.CostMicrocents(model, inputTokens, outputTokens)
}

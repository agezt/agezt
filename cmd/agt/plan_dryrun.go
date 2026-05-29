// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/planner"
)

// runDryRunPreview is the body of `agt plan run --dry-run`. It
// has already done the daemon call to generate the plan — this
// function does the offline-only steps an operator wants before
// committing to execution:
//
//  1. Pretty-print the plan JSON
//  2. Validate it (catches cycles, missing fields, etc.)
//  3. Render the Mermaid DAG
//  4. Estimate cost when --model was supplied
//
// Pure client-side after step 0; no daemon round-trip beyond the
// planner call the caller already made. Exit 0 on a clean
// preview, 1 if validation fails (the plan would have been
// rejected by the daemon at submit time anyway).
//
// `model` may be empty — cost estimation is skipped and the
// caller is told how to add it.
func runDryRunPreview(planJSON string, nodeCount int, model string, stdout, stderr io.Writer) int {
	// Validate first — rendering a plan that wouldn't execute is
	// misleading. Same defensive choice `plan visualize` makes.
	plan, err := planner.ValidateJSON([]byte(planJSON))
	if err != nil {
		fmt.Fprintf(stderr, "%s plan run --dry-run: validation failed: %v\n", brand.CLI, err)
		return 1
	}

	fmt.Fprintf(stdout, "--- dry run (no execution) ---\n")
	fmt.Fprintf(stdout, "generated %d-node plan", nodeCount)
	if plan.Name != "" {
		fmt.Fprintf(stdout, " %q", plan.Name)
	}
	if plan.MaxParallel > 0 {
		fmt.Fprintf(stdout, ", max_parallel=%d", plan.MaxParallel)
	}
	fmt.Fprintln(stdout)

	// Pretty-print the plan JSON for inclusion in a PR or
	// follow-up `agt plan run` from disk.
	pretty, err := jsonPretty([]byte(planJSON))
	if err == nil {
		fmt.Fprintf(stdout, "\nplan JSON:\n%s\n", pretty)
	} else {
		// Fallback: emit the raw JSON the daemon returned. Better
		// to surface uncondensed JSON than nothing.
		fmt.Fprintf(stdout, "\nplan JSON:\n%s\n", planJSON)
	}

	fmt.Fprintln(stdout, "\nMermaid:")
	fmt.Fprintln(stdout, "```mermaid")
	renderPlanMermaid(stdout, plan)
	fmt.Fprintln(stdout, "```")

	if model == "" {
		fmt.Fprintf(stdout, "\ncost: (pass --model <id> to estimate; e.g. --model claude-sonnet-4-6)\n")
		return 0
	}
	est, err := planner.EstimateCost(plan, model, governorEstimator{})
	if err != nil {
		// Plan validated but cost estimation failed (unknown model,
		// no pricing in catalog). Surface — operator might have
		// typo'd the model id. Don't exit non-zero: the preview
		// itself succeeded, cost was an optional bonus.
		fmt.Fprintf(stderr, "%s plan run --dry-run: cost estimate skipped: %v\n", brand.CLI, err)
		return 0
	}
	fmt.Fprintf(stdout, "\ncost estimate (model=%s):\n", est.Model)
	fmt.Fprintf(stdout, "  assumed %d input + %d output tokens per loop node\n",
		planner.PerNodeInputTokens, planner.PerNodeOutputTokens)
	for _, n := range est.Nodes {
		fmt.Fprintf(stdout, "  %-20s  %-6s  %s\n",
			n.ID, n.Kind, planner.FormatUSD(n.Microcents))
	}
	fmt.Fprintf(stdout, "  total: %s (estimate; actual usage depends on tool outputs)\n",
		planner.FormatUSD(est.TotalMicrocents))
	return 0
}

// jsonPretty re-marshals a raw JSON byte slice with indentation
// to make the plan readable in the dry-run output. Returns the
// original bytes if the input isn't valid JSON (so the caller's
// fallback path can surface them anyway).
func jsonPretty(raw []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw, err
	}
	return json.MarshalIndent(v, "", "  ")
}

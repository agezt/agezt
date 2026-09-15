// SPDX-License-Identifier: MIT
//
// cmd/agt plan sub-commands + helpers (cmdPlan, cmdPlanExecuteFile,
// cmdPlanGenerate, cmdPlanRun, runPlanJSON, formatTime shim).
// Extracted from main_approvals_plan.go during Day 211 god-file refactor (#67).
// Public API unchanged.
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
	"github.com/agezt/agezt/kernel/event"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/format"
)

func cmdPlan(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s plan: subcommand required (generate|run|history|<file.json>)\n", brand.CLI)
		fmt.Fprintf(stderr, "  generate \"<intent>\"  — ask the planner LLM for a plan; print JSON\n")
		fmt.Fprintf(stderr, "  run      \"<intent>\"  — generate, then execute the plan\n")
		fmt.Fprintf(stderr, "  history [N]          — list recent plan executions and outcomes\n")
		fmt.Fprintf(stderr, "  <file.json>          — execute a hand-authored plan\n")
		return 2
	}
	switch args[0] {
	case "generate", "gen":
		return cmdPlanGenerate(args[1:], stdout, stderr)
	case "history", "runs", "ls":
		return cmdPlanHistory(args[1:], stdout, stderr)
	case "stats":
		return cmdPlanStats(args[1:], stdout, stderr)
	case "run":
		return cmdPlanRun(args[1:], stdout, stderr)
	case "cost":
		return cmdPlanCost(args[1:], stdout, stderr)
	case "refine":
		return cmdPlanRefine(args[1:], stdout, stderr)
	case "validate":
		return cmdPlanValidate(args[1:], stdout, stderr)
	case "visualize", "viz":
		return cmdPlanVisualize(args[1:], stdout, stderr)
	default:
		// Backwards-compatible: `agt plan <file.json>` still executes
		// a hand-authored plan. Detected by checking if the arg is a
		// file path (existence test); falls through to the historical
		// CmdPlan handler.
		return cmdPlanExecuteFile(args, stdout, stderr)
	}
}
func cmdPlanExecuteFile(args []string, stdout, stderr io.Writer) int {
	body, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%s plan: read %s: %v\n", brand.CLI, args[0], err)
		return 1
	}
	return runPlanJSON(string(body), stdout, stderr)
}
func cmdPlanGenerate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		fmt.Fprintf(stderr, "%s plan generate: intent required (quote it as one argument)\n", brand.CLI)
		return 2
	}
	intent := strings.Join(args, " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	// Planner is one LLM call; even a slow model fits in 2 minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdPlanGenerate, map[string]any{"intent": intent})
	if err != nil {
		fmt.Fprintf(stderr, "%s plan generate: %v\n", brand.CLI, err)
		return 1
	}
	planJSON, _ := res["plan_json"].(string)
	if planJSON == "" {
		fmt.Fprintf(stderr, "%s plan generate: daemon returned empty plan_json\n", brand.CLI)
		return 1
	}
	fmt.Fprintln(stdout, planJSON)
	return 0
}
func cmdPlanRun(args []string, stdout, stderr io.Writer) int {
	dryRun := false
	model := ""
	var intentParts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--dry-run":
			dryRun = true
		case "--model", "-m":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s plan run: --model needs a value\n", brand.CLI)
				return 2
			}
			model = args[i]
		default:
			intentParts = append(intentParts, a)
		}
	}
	intent := strings.TrimSpace(strings.Join(intentParts, " "))
	if intent == "" {
		fmt.Fprintf(stderr, "%s plan run: intent required (quote it as one argument)\n", brand.CLI)
		return 2
	}
	if model != "" && !dryRun {
		// --model only makes sense in dry-run mode (execution uses
		// the governor's primary). Warn rather than fail — operator
		// intent is clear; ignoring silently would be worse.
		fmt.Fprintf(stderr, "%s plan run: --model is only used with --dry-run; ignored for execution\n", brand.CLI)
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdPlanGenerate, map[string]any{"intent": intent})
	if err != nil {
		fmt.Fprintf(stderr, "%s plan run (generate): %v\n", brand.CLI, err)
		return 1
	}
	planJSON, _ := res["plan_json"].(string)
	nodeCount, _ := res["node_count"].(float64)
	if planJSON == "" {
		fmt.Fprintf(stderr, "%s plan run: daemon returned empty plan_json\n", brand.CLI)
		return 1
	}

	if dryRun {
		return runDryRunPreview(planJSON, int(nodeCount), model, stdout, stderr)
	}

	fmt.Fprintf(stdout, "generated %d-node plan:\n%s\n\n--- executing ---\n", int(nodeCount), planJSON)
	return runPlanJSON(planJSON, stdout, stderr)
}
func runPlanJSON(planJSON string, stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	result, err := c.Stream(ctx, controlplane.CmdPlan,
		map[string]any{"plan_json": planJSON},
		func(ev *event.Event) {
			// Skip the high-rate ephemeral token/reasoning chunks — with concurrent
			// plan nodes they interleave into an unreadable "[evt seq=0]" flood. The
			// structural events (node transitions, tool calls) and the per-node
			// outputs at the end carry the signal.
			if ev.Kind == event.KindLLMToken || ev.Kind == event.KindLLMReasoning {
				return
			}
			fmt.Fprintf(stdout, "  [evt seq=%d kind=%s subject=%s]\n", ev.Seq, ev.Kind, ev.Subject)
		})
	if err != nil {
		fmt.Fprintf(stderr, "%s plan: %v\n", brand.CLI, err)
		return 1
	}
	planID, _ := result["plan_id"].(string)
	outputs, _ := result["node_outputs"].(map[string]any)
	fmt.Fprintf(stdout, "\n--- plan completed ---\nplan_id: %s\n", planID)
	for id, out := range outputs {
		s, _ := out.(string)
		fmt.Fprintf(stdout, "\n[%s]\n%s\n", id, s)
	}
	return 0
}
func formatTime(s string) string { return format.Time(s) }

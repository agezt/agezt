// SPDX-License-Identifier: MIT
//
// cmd/agt approvals + plan + catalog + decide sub-commands + the
// formatTime shim. Split from main.go during Day 211 god-file
// refactor (#42). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
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


func cmdApprovals(args []string, stdout, stderr io.Writer) int {
	// `agt approvals log` — the resolved/pending audit timeline (M87); plain
	// `agt approvals` stays the pending-only list.
	if len(args) > 0 && args[0] == "log" {
		return cmdApprovalsLog(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "stats" {
		return cmdApprovalsStats(args[1:], stdout, stderr)
	}
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s approvals [log] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list pending HITL approval requests (or `approvals log` for the resolved audit)\n")
			fmt.Fprintf(stdout, "  --json   emit the full pending array (CI/automation pipelines)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s approvals: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdApprovals, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s approvals: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		// Pass the full response shape through — exits 0 even when
		// pending is empty (the empty array is the correct, valid
		// machine answer; jq pipelines should not need to special-
		// case "no approvals" via stderr scraping).
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	pending, _ := res["pending"].([]any)
	if len(pending) == 0 {
		fmt.Fprintf(stdout, "no pending approvals\n")
		return 0
	}
	fmt.Fprintf(stdout, "%d pending approval(s):\n", len(pending))
	for _, raw := range pending {
		m, _ := raw.(map[string]any)
		fmt.Fprintf(stdout, "\n  id         : %v\n", m["id"])
		fmt.Fprintf(stdout, "  capability : %v\n", m["capability"])
		fmt.Fprintf(stdout, "  tool       : %v\n", m["tool_name"])
		fmt.Fprintf(stdout, "  reason     : %v\n", m["reason"])
		if v := strings.TrimSpace(fmt.Sprint(m["canonical_intent"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  intent     : %v\n", v)
		}
		if v := strings.TrimSpace(fmt.Sprint(m["harmful_interpretation"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  harmful    : %v\n", v)
		}
		if v := strings.TrimSpace(fmt.Sprint(m["confirmation_prompt"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  confirm    : %v\n", v)
		}
		if v := strings.TrimSpace(fmt.Sprint(m["effect_class"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  effect     : %v\n", v)
		}
		if resources := toStringSlice(m["affected_resources"]); len(resources) > 0 {
			fmt.Fprintf(stdout, "  resources  : %s\n", strings.Join(resources, ", "))
		}
		if axes, ok := m["regret_axes"].(map[string]any); ok && len(axes) > 0 {
			fmt.Fprintf(stdout, "  regret     : physical=%v informational=%v social=%v identity=%v\n",
				axes["physical"], axes["informational"], axes["social"], axes["identity"])
		}
		fmt.Fprintf(stdout, "  actor      : %v\n", m["actor"])
		fmt.Fprintf(stdout, "  input      : %v\n", m["input"])
		fmt.Fprintf(stdout, "  timeout    : unix %v\n", m["timeout_unix"])
	}
	fmt.Fprintf(stdout, "\nResolve with: %s approve <id> [reason]  |  %s deny <id> [reason]\n", brand.CLI, brand.CLI)
	return 0
}

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

// cmdPlanGenerate runs `agt plan generate "<intent>"` — calls the
// daemon's CmdPlanGenerate and prints the JSON to stdout. Operator
// can pipe to a file (`agt plan generate "X" > plan.json`) or to
// jq for inspection (`agt plan generate "X" | jq`).
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

// cmdPlanRun is `agt plan run "<intent>"` — generate then execute
// in one operator-facing command. The Generate call returns the
// JSON; we then forward it to CmdPlan via the same machinery
// `agt plan <file>` uses. Keeps server endpoints single-purpose.
//
// Flags:
//
//	--dry-run            preview only: generate + validate + visualize
//	                     (+ cost if --model set), then exit without
//	                     executing. CI / human-review workflow.
//	--model <id>         cost-estimation model id (only meaningful
//	                     with --dry-run)
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

// runPlanJSON forwards a plan JSON to the daemon's CmdPlan and
// renders streamed events + the final result. Shared by the
// `agt plan <file>` (hand-authored) and `agt plan run` (generated)
// paths so both render identically.
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

func cmdCatalog(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s catalog: subcommand required: sync|list|discover\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "sync":
		return cmdCatalogSync(args[1:], stdout, stderr)
	case "discover":
		callArgs := map[string]any{}
		if len(args) > 1 {
			callArgs["endpoint"] = args[1]
		}
		return cmdSimple(controlplane.CmdCatalogDiscover, callArgs, stdout, stderr)
	case "list":
		return cmdCatalogList(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s catalog: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}

func cmdCatalogList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s catalog list [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list synced providers + models + pricing\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s catalog list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdCatalogList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s catalog list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	providers, _ := res["providers"].([]any)
	syncedAt, _ := res["api_synced_at"].(string)
	source, _ := res["api_source_url"].(string)

	fmt.Fprintf(stdout, "%d providers (synced %s from %s)\n",
		len(providers), formatTime(syncedAt), source)

	for _, raw := range providers {
		p, _ := raw.(map[string]any)
		credentialed := p["credentialed"] == true
		credBadge := "[no creds]"
		if credentialed {
			credBadge = "[creds OK]"
		}
		fmt.Fprintf(stdout, "\n  %s  (%s, family=%s)  %s\n",
			p["id"], p["name"], p["family"], credBadge)
		if api, _ := p["api"].(string); api != "" {
			fmt.Fprintf(stdout, "    api  : %s\n", api)
		}
		if env, ok := p["env"].([]any); ok && len(env) > 0 {
			fmt.Fprintf(stdout, "    env  : ")
			for i, e := range env {
				if i > 0 {
					fmt.Fprint(stdout, ", ")
				}
				fmt.Fprint(stdout, e)
			}
			fmt.Fprintln(stdout)
		}
		models, _ := p["models"].([]any)
		if len(models) == 0 {
			fmt.Fprintf(stdout, "    (no models)\n")
			continue
		}
		fmt.Fprintf(stdout, "    %d model(s):\n", len(models))
		for _, mraw := range models {
			m, _ := mraw.(map[string]any)
			cost := "free"
			if in, ok := m["cost_input_usd_per_mtok"].(float64); ok {
				out, _ := m["cost_output_usd_per_mtok"].(float64)
				cost = fmt.Sprintf("$%.2f / $%.2f per MTok", in, out)
			}
			fmt.Fprintf(stdout, "      %-40s  %s\n", m["id"], cost)
		}
	}
	return 0
}

// formatTime is a shim → format.Time (Day 8 extraction).
func formatTime(s string) string { return format.Time(s) }

func cmdDecide(decision string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s %s: id required\n", brand.CLI, decision)
		return 2
	}
	id := args[0]
	reason := strings.Join(args[1:], " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdDecide, map[string]any{
		"id": id, "decision": decision, "reason": reason,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, decision, err)
		return 1
	}
	enc, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintf(stdout, "%s\n", enc)
	return 0
}

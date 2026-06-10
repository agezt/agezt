// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdWorkflow dispatches `agt workflow <subcommand>` — the operator surface
// of the workflow engine (M798): durable, named graphs of typed nodes
// (trigger/tool/llm/condition/transform/delay) saved as JSON, run on demand
// (cron/event triggers arrive with M799), every run arc journaled
// (workflow.*). The console canvas edits the same graphs.
func cmdWorkflow(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return workflowUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdWorkflowList(args[1:], stdout, stderr)
	case "show":
		return cmdWorkflowShow(args[1:], stdout, stderr)
	case "save", "add", "import":
		return cmdWorkflowSave(args[1:], stdout, stderr)
	case "run":
		return cmdWorkflowRun(args[1:], stdout, stderr)
	case "draft":
		return cmdWorkflowDraft(args[1:], stdout, stderr)
	case "refine":
		return cmdWorkflowRefine(args[1:], stdout, stderr)
	case "runs":
		return cmdWorkflowRuns(args[1:], stdout, stderr)
	case "enable":
		return cmdWorkflowSetEnabled(args[1:], stdout, stderr, true)
	case "disable":
		return cmdWorkflowSetEnabled(args[1:], stdout, stderr, false)
	case "remove", "rm":
		return cmdWorkflowRemove(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return workflowUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s workflow: unknown subcommand %q\n", brand.CLI, args[0])
		return workflowUsage(stderr)
	}
}

func workflowUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s workflow <list|show|save|draft|run|enable|disable|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                      all workflows (name, nodes, enabled)\n")
	fmt.Fprintf(w, "  show <name|id> [--json]            one workflow's full graph\n")
	fmt.Fprintf(w, "  save --file GRAPH.json             create or update (upsert by name) a workflow\n")
	fmt.Fprintf(w, "  draft \"DESCRIPTION\" [--name N] [--save]  copilot designs a graph from plain language\n")
	fmt.Fprintf(w, "  refine <name|id> \"CHANGE\" [--save]   copilot revises a stored graph per a change request\n")
	fmt.Fprintf(w, "  runs <name|id> [N] [--json]          run history folded from the journal (newest first)\n")
	fmt.Fprintf(w, "  run <name|id> [--payload JSON]     execute now; payload lands in {{trigger.payload}}\n")
	fmt.Fprintf(w, "  enable|disable <name|id>           arm/disarm its triggers (M799)\n")
	fmt.Fprintf(w, "  remove <name|id>                   delete a workflow\n")
	fmt.Fprintf(w, "node types: trigger, tool {tool,args}, llm {prompt,system,model}, condition {left,op,right},\n")
	fmt.Fprintf(w, "            transform {template}, delay {seconds} — string fields take {{node_id.output}} templates\n")
	return 0
}

func cmdWorkflowList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	items, _ := res["workflows"].([]any)
	if len(items) == 0 {
		fmt.Fprintf(stdout, "no workflows yet — save one with `%s workflow save --file graph.json` or build it on the console canvas\n", brand.CLI)
		return 0
	}
	for _, raw := range items {
		w, _ := raw.(map[string]any)
		if w == nil {
			continue
		}
		state := "enabled"
		if en, _ := w["enabled"].(bool); !en {
			state = "disabled"
		}
		nodes, _ := w["node_count"].(float64)
		trig := str(w["trigger_kind"])
		if d := str(w["trigger_detail"]); d != "" {
			trig += " (" + d + ")"
		}
		fmt.Fprintf(stdout, "%-24s %-9s %d node(s)  %-24s", str(w["name"]), state, int(nodes), trig)
		if d := str(w["description"]); d != "" {
			fmt.Fprintf(stdout, "  %s", d)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v workflow(s)\n", res["count"])
	return 0
}

func cmdWorkflowShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	ref := ""
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if !strings.HasPrefix(a, "--") && ref == "" {
			ref = a
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s workflow show <name|id> [--json]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowShow, map[string]any{"ref": ref})
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow show: %v\n", brand.CLI, err)
		return 1
	}
	w, _ := res["workflow"].(map[string]any)
	if w == nil {
		fmt.Fprintf(stderr, "%s workflow show: unknown workflow %q\n", brand.CLI, ref)
		return 1
	}
	// The graph IS the artifact — print it as JSON either way; --json just
	// skips the human header.
	if !asJSON {
		state := "enabled"
		if en, _ := w["enabled"].(bool); !en {
			state = "disabled"
		}
		fmt.Fprintf(stdout, "%s (%s) — %v node(s), %v edge(s)\n", str(w["name"]), state, w["node_count"], w["edge_count"])
	}
	return encodeJSON(stdout, w)
}

func cmdWorkflowSave(args []string, stdout, stderr io.Writer) int {
	file := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--file" {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s workflow save: --file needs a path\n", brand.CLI)
				return 2
			}
			i++
			file = args[i]
		}
	}
	if file == "" {
		fmt.Fprintf(stderr, "usage: %s workflow save --file GRAPH.json\n", brand.CLI)
		return 2
	}
	b, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow save: read %s: %v\n", brand.CLI, file, err)
		return 1
	}
	var graph map[string]any
	if err := json.Unmarshal(b, &graph); err != nil {
		fmt.Fprintf(stderr, "%s workflow save: %s is not valid JSON: %v\n", brand.CLI, file, err)
		return 1
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": graph})
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow save: %v\n", brand.CLI, err)
		return 1
	}
	w, _ := res["workflow"].(map[string]any)
	verb := "updated"
	if created, _ := res["created"].(bool); created {
		verb = "created"
	}
	fmt.Fprintf(stdout, "%s %s (%v node(s)) — run it with `%s workflow run %s`\n", verb, str(w["name"]), w["node_count"], brand.CLI, str(w["name"]))
	return 0
}

// cmdWorkflowDraft (M802): the copilot designs a workflow from plain
// language. The draft prints as JSON for review; --save persists it in one
// step (it arrives disabled either way).
func cmdWorkflowDraft(args []string, stdout, stderr io.Writer) int {
	desc, name := "", ""
	save := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s workflow draft: --name needs a value\n", brand.CLI)
				return 2
			}
			i++
			name = args[i]
		case "--save":
			save = true
		default:
			if !strings.HasPrefix(args[i], "--") && desc == "" {
				desc = args[i]
			}
		}
	}
	if strings.TrimSpace(desc) == "" {
		fmt.Fprintf(stderr, "usage: %s workflow draft \"DESCRIPTION\" [--name N] [--save]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	// Up to two provider round-trips (draft + one repair).
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	callArgs := map[string]any{"description": desc}
	if name != "" {
		callArgs["name"] = name
	}
	res, err := c.Call(ctx, controlplane.CmdWorkflowDraft, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow draft: %v\n", brand.CLI, err)
		return 1
	}
	w, _ := res["workflow"].(map[string]any)
	if w == nil {
		fmt.Fprintf(stderr, "%s workflow draft: empty draft\n", brand.CLI)
		return 1
	}
	fmt.Fprintf(stdout, "drafted %s — %v node(s), %v edge(s)\n", str(w["name"]), w["node_count"], w["edge_count"])
	if rc := encodeJSON(stdout, w); rc != 0 {
		return rc
	}
	if !save {
		fmt.Fprintf(stdout, "review it, then persist with `%s workflow draft ... --save` or `%s workflow save --file graph.json`\n", brand.CLI, brand.CLI)
		return 0
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	saveRes, err := c.Call(saveCtx, controlplane.CmdWorkflowSave, map[string]any{"workflow": w})
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow draft: save: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "saved (%s) — run it with `%s workflow run %s`\n",
		savedState(saveRes), brand.CLI, str(w["name"]))
	return 0
}

// savedState reads the persisted enabled flag out of a workflow_save result —
// a fresh save arrives enabled (store semantics), an update keeps its state.
func savedState(res map[string]any) string {
	if w, _ := res["workflow"].(map[string]any); w != nil {
		if en, _ := w["enabled"].(bool); !en {
			return "disabled"
		}
	}
	return "enabled — triggers armed"
}

// cmdWorkflowRefine (M805): the copilot revises a STORED workflow per a
// plain-language change request; prints the revision for review, --save
// persists it (upsert by name).
func cmdWorkflowRefine(args []string, stdout, stderr io.Writer) int {
	ref, instruction := "", ""
	save := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--save":
			save = true
		default:
			if strings.HasPrefix(args[i], "--") {
				continue
			}
			if ref == "" {
				ref = args[i]
			} else if instruction == "" {
				instruction = args[i]
			}
		}
	}
	if ref == "" || strings.TrimSpace(instruction) == "" {
		fmt.Fprintf(stderr, "usage: %s workflow refine <name|id> \"CHANGE REQUEST\" [--save]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowRefine, map[string]any{"ref": ref, "instruction": instruction})
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow refine: %v\n", brand.CLI, err)
		return 1
	}
	w, _ := res["workflow"].(map[string]any)
	if w == nil {
		fmt.Fprintf(stderr, "%s workflow refine: empty revision\n", brand.CLI)
		return 1
	}
	fmt.Fprintf(stdout, "refined %s — %v node(s), %v edge(s)\n", str(w["name"]), w["node_count"], w["edge_count"])
	if rc := encodeJSON(stdout, w); rc != 0 {
		return rc
	}
	if !save {
		fmt.Fprintf(stdout, "review it, then persist with --save\n")
		return 0
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	saveRes, err := c.Call(saveCtx, controlplane.CmdWorkflowSave, map[string]any{"workflow": w})
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow refine: save: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "saved (%s) — run it with `%s workflow run %s`\n",
		savedState(saveRes), brand.CLI, str(w["name"]))
	return 0
}

func cmdWorkflowRun(args []string, stdout, stderr io.Writer) int {
	ref, payloadRaw := "", ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--payload":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s workflow run: --payload needs a value\n", brand.CLI)
				return 2
			}
			i++
			payloadRaw = args[i]
		case "--json":
			asJSON = true
		default:
			if !strings.HasPrefix(args[i], "--") && ref == "" {
				ref = args[i]
			}
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s workflow run <name|id> [--payload JSON] [--json]\n", brand.CLI)
		return 2
	}
	callArgs := map[string]any{"ref": ref}
	if payloadRaw != "" {
		// A JSON payload rides structured; anything else rides as a string.
		var v any
		if err := json.Unmarshal([]byte(payloadRaw), &v); err == nil {
			callArgs["payload"] = v
		} else {
			callArgs["payload"] = payloadRaw
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	// Runs can legitimately take minutes (delay nodes, slow tools, HITL asks).
	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowRun, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow run: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	executed, _ := res["executed"].([]any)
	fmt.Fprintf(stdout, "completed — %d node(s) executed (correlation %s)\n", len(executed), str(res["correlation_id"]))
	outputs, _ := res["outputs"].(map[string]any)
	for _, raw := range executed {
		id := str(raw)
		out := outputs[id]
		rendered := ""
		switch v := out.(type) {
		case string:
			rendered = v
		case nil:
			rendered = ""
		default:
			b, _ := json.Marshal(v)
			rendered = string(b)
		}
		if len(rendered) > 120 {
			rendered = rendered[:120] + "…"
		}
		fmt.Fprintf(stdout, "  %-20s %s\n", id, rendered)
	}
	return 0
}

// cmdWorkflowRuns (M806): the workflow's run history, folded from the
// journal — every started→node…→completed|failed arc, newest first.
func cmdWorkflowRuns(args []string, stdout, stderr io.Writer) int {
	ref := ""
	limit := 0
	asJSON := false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case strings.HasPrefix(a, "--"):
		case ref == "":
			ref = a
		default:
			if n, err := strconv.Atoi(a); err == nil {
				limit = n
			}
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s workflow runs <name|id> [N] [--json]\n", brand.CLI)
		return 2
	}
	callArgs := map[string]any{"ref": ref}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowRuns, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow runs: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	runs, _ := res["runs"].([]any)
	if len(runs) == 0 {
		fmt.Fprintf(stdout, "no runs recorded for %s yet\n", str(res["workflow"]))
		return 0
	}
	for _, raw := range runs {
		r, _ := raw.(map[string]any)
		if r == nil {
			continue
		}
		started, _ := r["started_ms"].(float64)
		when := time.UnixMilli(int64(started)).Format("2006-01-02 15:04:05")
		dur := ""
		if fin, ok := r["finished_ms"].(float64); ok && started > 0 {
			dur = " " + (time.Duration(int64(fin-started)) * time.Millisecond).Truncate(time.Millisecond).String()
		}
		nodes, _ := r["node_events"].([]any)
		fmt.Fprintf(stdout, "%s  %-9s %2d node(s)%s  %s", when, str(r["status"]), len(nodes), dur, str(r["correlation_id"]))
		if e := str(r["error"]); e != "" {
			if len(e) > 60 {
				e = e[:60] + "…"
			}
			fmt.Fprintf(stdout, "  %s", e)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v run(s)\n", res["count"])
	return 0
}

func cmdWorkflowSetEnabled(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "enable"
	if !enabled {
		verb = "disable"
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s workflow %s <name|id>\n", brand.CLI, verb)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSetEnabled, map[string]any{"ref": args[0], "enabled": enabled}); err != nil {
		fmt.Fprintf(stderr, "%s workflow %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s %sd\n", args[0], verb)
	return 0
}

func cmdWorkflowRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s workflow remove <name|id>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowRemove, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow remove: %v\n", brand.CLI, err)
		return 1
	}
	if ok, _ := res["removed"].(bool); !ok {
		fmt.Fprintf(stderr, "%s workflow remove: unknown workflow %q\n", brand.CLI, args[0])
		return 1
	}
	fmt.Fprintf(stdout, "removed %s\n", args[0])
	return 0
}

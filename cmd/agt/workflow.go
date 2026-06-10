// SPDX-License-Identifier: MIT

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
	fmt.Fprintf(w, "usage: %s workflow <list|show|save|run|enable|disable|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                      all workflows (name, nodes, enabled)\n")
	fmt.Fprintf(w, "  show <name|id> [--json]            one workflow's full graph\n")
	fmt.Fprintf(w, "  save --file GRAPH.json             create or update (upsert by name) a workflow\n")
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
		fmt.Fprintf(stdout, "%-24s %-9s %d node(s)", str(w["name"]), state, int(nodes))
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

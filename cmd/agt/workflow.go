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
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
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
	case "templates":
		return cmdWorkflowTemplates(args[1:], stdout, stderr)
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
	fmt.Fprintf(w, "  templates [--json] [--use T --name N]  built-in gallery; --use saves template T as workflow N\n")
	fmt.Fprintf(w, "  run <name|id> [--payload JSON] [--async]  execute now; --async returns immediately (journal carries the arc)\n")
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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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
		contract := workflowContractText(state != "disabled", trig)
		fmt.Fprintf(stdout, "%-24s %-9s %d node(s)  %-24s  %s", str(w["name"]), state, int(nodes), trig, contract)
		if d := str(w["description"]); d != "" {
			fmt.Fprintf(stdout, "  %s", d)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v workflow(s)\n", res["count"])
	return 0
}

func workflowContractText(enabled bool, trigger string) string {
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	if strings.TrimSpace(trigger) == "" {
		trigger = "manual/API"
	}
	return state + " reusable chain · trigger " + strings.TrimSpace(trigger) + " · runnable by user, agent, schedule, or webhook"
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
	c := dialpkg.New(stderr)
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
	return jsonout.Write(stdout, w)
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
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if name := str(graph["name"]); name != "" {
		if _, _, cerr := saveWorkflowSnapshotRollbackCheckpointIfFound(ctx, c, "workflow.save", name, ""); cerr != nil {
			fmt.Fprintf(stderr, "%s workflow save: checkpoint: %v\n", brand.CLI, cerr)
			return 1
		}
	}
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

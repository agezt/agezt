// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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
	c := dialpkg.New(stderr)
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
	if rc := jsonout.Write(stdout, w); rc != 0 {
		return rc
	}
	if !save {
		fmt.Fprintf(stdout, "review it, then persist with `%s workflow draft ... --save` or `%s workflow save --file graph.json`\n", brand.CLI, brand.CLI)
		return 0
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	if name := str(w["name"]); name != "" {
		if _, _, cerr := saveWorkflowSnapshotRollbackCheckpointIfFound(saveCtx, c, "workflow.draft.save", name, ""); cerr != nil {
			fmt.Fprintf(stderr, "%s workflow draft: checkpoint: %v\n", brand.CLI, cerr)
			return 1
		}
	}
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
	c := dialpkg.New(stderr)
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
	if rc := jsonout.Write(stdout, w); rc != 0 {
		return rc
	}
	if !save {
		fmt.Fprintf(stdout, "review it, then persist with --save\n")
		return 0
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	if _, _, cerr := saveWorkflowSnapshotRollbackCheckpointIfFound(saveCtx, c, "workflow.refine.save", ref, ""); cerr != nil {
		fmt.Fprintf(stderr, "%s workflow refine: checkpoint: %v\n", brand.CLI, cerr)
		return 1
	}
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
	asJSON, async := false, false
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
		case "--async":
			async = true
		default:
			if !strings.HasPrefix(args[i], "--") && ref == "" {
				ref = args[i]
			}
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s workflow run <name|id> [--payload JSON] [--async] [--json]\n", brand.CLI)
		return 2
	}
	callArgs := map[string]any{"ref": ref}
	if async {
		callArgs["async"] = true
	}
	if payloadRaw != "" {
		// A JSON payload rides structured; anything else rides as a string.
		var v any
		if err := json.Unmarshal([]byte(payloadRaw), &v); err == nil {
			callArgs["payload"] = v
		} else {
			callArgs["payload"] = payloadRaw
		}
	}
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
	}
	if accepted, _ := res["accepted"].(bool); accepted {
		fmt.Fprintf(stdout, "started — follow it with `%s workflow runs %s` or `%s why <event>` (correlation %s)\n",
			brand.CLI, ref, brand.CLI, str(res["correlation_id"]))
		return 0
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

// cmdWorkflowTemplates (M807): list the built-in gallery; --use T --name N
// instantiates template T as a new workflow named N (a plain save — the
// gallery itself never writes).

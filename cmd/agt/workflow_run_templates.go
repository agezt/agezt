// SPDX-License-Identifier: MIT

package main

// agt workflow templates READ subcommand: cmdWorkflowTemplates.
// Carved out of workflow_run.go during the Day 185 god-file split so
// the main file can stay focused on WRITE/AI (Draft/Refine/Run) and
// the list file can stay focused on LIST/MANAGE (Runs/SetEnabled/Remove).
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdWorkflowTemplates(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	use, name := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--use":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s workflow templates: --use needs a template name\n", brand.CLI)
				return 2
			}
			i++
			use = args[i]
		case "--name":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s workflow templates: --name needs a value\n", brand.CLI)
				return 2
			}
			i++
			name = args[i]
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorkflowTemplates, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s workflow templates: %v\n", brand.CLI, err)
		return 1
	}
	items, _ := res["templates"].([]any)

	if use != "" {
		if name == "" {
			fmt.Fprintf(stderr, "%s workflow templates: --use also needs --name <new workflow name>\n", brand.CLI)
			return 2
		}
		for _, raw := range items {
			t, _ := raw.(map[string]any)
			if t == nil || str(t["name"]) != use {
				continue
			}
			w, _ := t["workflow"].(map[string]any)
			if w == nil {
				fmt.Fprintf(stderr, "%s workflow templates: template %q carries no graph\n", brand.CLI, use)
				return 1
			}
			if _, _, cerr := saveWorkflowSnapshotRollbackCheckpointIfFound(ctx, c, "workflow.template.save", name, ""); cerr != nil {
				fmt.Fprintf(stderr, "%s workflow templates: checkpoint: %v\n", brand.CLI, cerr)
				return 1
			}
			w["name"] = name
			delete(w, "id")
			saveRes, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": w})
			if err != nil {
				fmt.Fprintf(stderr, "%s workflow templates: save: %v\n", brand.CLI, err)
				return 1
			}
			fmt.Fprintf(stdout, "created %s from template %s (%s) — run it with `%s workflow run %s`\n",
				name, use, savedState(saveRes), brand.CLI, name)
			return 0
		}
		fmt.Fprintf(stderr, "%s workflow templates: unknown template %q\n", brand.CLI, use)
		return 1
	}

	if asJSON {
		return jsonout.Write(stdout, res)
	}
	for _, raw := range items {
		t, _ := raw.(map[string]any)
		if t == nil {
			continue
		}
		nodes, _ := t["node_count"].(float64)
		fmt.Fprintf(stdout, "%-20s %-8s %d node(s)  %s\n", str(t["name"]), str(t["category"]), int(nodes), str(t["description"]))
	}
	fmt.Fprintf(stdout, "%v template(s) — instantiate with `%s workflow templates --use T --name N`\n", res["count"], brand.CLI)
	return 0
}

// cmdWorkflowRuns (M806): the workflow's run history, folded from the
// journal — every started→node…→completed|failed arc, newest first.

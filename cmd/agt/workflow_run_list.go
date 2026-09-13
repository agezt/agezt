// SPDX-License-Identifier: MIT

package main

// agt workflow LIST/MANAGE subcommands: cmdWorkflowRuns +
// cmdWorkflowSetEnabled + cmdWorkflowRemove. Carved out of
// workflow_run.go during the Day 185 god-file split so the main file
// can stay focused on WRITE/AI (Draft/Refine/Run) and the templates
// file can stay focused on READ (Templates).
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, cerr := saveWorkflowSnapshotRollbackCheckpointIfFound(ctx, c, "workflow."+verb, args[0], ""); cerr != nil {
		fmt.Fprintf(stderr, "%s workflow %s: checkpoint: %v\n", brand.CLI, verb, cerr)
		return 1
	}
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
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, cerr := saveWorkflowSnapshotRollbackCheckpointIfFound(ctx, c, "workflow.remove", args[0], ""); cerr != nil {
		fmt.Fprintf(stderr, "%s workflow remove: checkpoint: %v\n", brand.CLI, cerr)
		return 1
	}
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


// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// budgetDim is one spend ceiling (global or a task-type cap) and what's spent
// against it. A ceiling <= 0 means "no cap" (unlimited on that dimension).
type budgetDim struct {
	name           string
	spent, ceiling int64
}

// effectiveHeadroom returns the binding remaining spend across all dimensions
// (the minimum over capped ones) and whether EVERY dimension is uncapped. A run
// can proceed only if every capped dimension still has headroom, so the binding
// constraint is the smallest remaining. Returns (headroom, allUnlimited).
func effectiveHeadroom(dims []budgetDim) (int64, bool) {
	headroom := int64(0)
	haveCap := false
	for _, d := range dims {
		if d.ceiling <= 0 {
			continue // uncapped dimension never binds
		}
		r := d.ceiling - d.spent
		if !haveCap || r < headroom {
			headroom = r
			haveCap = true
		}
	}
	return headroom, !haveCap
}

// cmdBudgetCheck implements `agt budget check [--task-type <t>] [--json]` (M107)
// — a pre-flight: how much daily-spend headroom remains before a run, so an
// operator (or a CI gate) can decide whether to submit work. Exit 0 = headroom
// remains (or uncapped), 3 = exhausted on some dimension, 2 = usage.
func cmdBudgetCheck(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	taskType := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--task-type":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s budget check: --task-type needs a value\n", brand.CLI)
				return 2
			}
			i++
			taskType = args[i]
		case strings.HasPrefix(a, "--task-type="):
			taskType = strings.TrimPrefix(a, "--task-type=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s budget check [--task-type <t>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "report remaining daily-spend headroom before submitting a run\n")
			fmt.Fprintf(stdout, "exit 0 = headroom remains, 3 = budget exhausted\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s budget check: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdBudget, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s budget check: %v\n", brand.CLI, err)
		return 1
	}

	globalSpent := mcFromAny(res["spent_mc"])
	globalCeiling := mcFromAny(res["ceiling_mc"])
	dims := []budgetDim{{name: "global", spent: globalSpent, ceiling: globalCeiling}}

	var taskFound bool
	var taskSpent, taskCeiling int64
	if taskType != "" {
		rows, _ := res["per_task"].([]any)
		for _, row := range rows {
			r, _ := row.(map[string]any)
			if tt, _ := r["task_type"].(string); tt == taskType {
				taskFound = true
				taskSpent = mcFromAny(r["spent_mc"])
				taskCeiling = mcFromAny(r["ceiling_mc"])
				dims = append(dims, budgetDim{name: "task:" + taskType, spent: taskSpent, ceiling: taskCeiling})
			}
		}
	}

	headroom, unlimited := effectiveHeadroom(dims)
	exhausted := !unlimited && headroom <= 0

	if asJSON {
		out := map[string]any{
			"global_spent_mc":   globalSpent,
			"global_ceiling_mc": globalCeiling,
			"headroom_mc":       headroom,
			"unlimited":         unlimited,
			"exhausted":         exhausted,
		}
		if taskType != "" {
			out["task_type"] = taskType
			out["task_found"] = taskFound
			out["task_spent_mc"] = taskSpent
			out["task_ceiling_mc"] = taskCeiling
		}
		code := encodeJSON(stdout, out)
		if exhausted {
			return 3
		}
		return code
	}

	if globalCeiling > 0 {
		fmt.Fprintf(stdout, "global   : %s remaining of %s ceiling\n", fmtUSD(globalCeiling-globalSpent), fmtUSD(globalCeiling))
	} else {
		fmt.Fprintf(stdout, "global   : no ceiling (uncapped)\n")
	}
	if taskType != "" {
		switch {
		case !taskFound:
			fmt.Fprintf(stdout, "task %-4q: no per-task cap configured (only the global ceiling binds)\n", taskType)
		case taskCeiling > 0:
			fmt.Fprintf(stdout, "task %-4q: %s remaining of %s cap\n", taskType, fmtUSD(taskCeiling-taskSpent), fmtUSD(taskCeiling))
		default:
			fmt.Fprintf(stdout, "task %-4q: no cap (only the global ceiling binds)\n", taskType)
		}
	}
	if unlimited {
		fmt.Fprintf(stdout, "→ uncapped: a run can proceed.\n")
		return 0
	}
	if exhausted {
		fmt.Fprintf(stdout, "→ EXHAUSTED: %s headroom — a run would be refused.\n", fmtUSD(headroom))
		return 3
	}
	fmt.Fprintf(stdout, "→ headroom: %s before the binding cap.\n", fmtUSD(headroom))
	return 0
}

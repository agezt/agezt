// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdBudget implements `agt budget` and `agt budget --json`.
// Calls CmdBudget and renders the governor's current spend
// snapshot — global ceiling + per-task-type caps from M1.zz.
//
// Default output is a small table; --json passes the raw shape
// straight through for jq / CI pipelines.
func cmdBudget(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s budget [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show current-day spend vs daily ceiling + per-task-type caps\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s budget: unexpected arg %q\n", brand.CLI, a)
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
		fmt.Fprintf(stderr, "%s budget: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	utcDate, _ := res["utc_date"].(string)
	spent := mcFromAny(res["spent_mc"])
	ceiling := mcFromAny(res["ceiling_mc"])

	fmt.Fprintf(stdout, "as of %s UTC\n\n", utcDate)
	fmt.Fprintf(stdout, "  global   %s spent", fmtUSD(spent))
	if ceiling > 0 {
		fmt.Fprintf(stdout, " / %s ceiling (%s%%)",
			fmtUSD(ceiling), pct(spent, ceiling))
	} else {
		fmt.Fprintf(stdout, " (no ceiling)")
	}
	fmt.Fprintln(stdout)

	rows, _ := res["per_task"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "\n  no per-task-type caps configured (AGEZT_TASK_BUDGETS)")
		return 0
	}
	fmt.Fprintln(stdout, "\n  per task type:")
	for _, row := range rows {
		r, _ := row.(map[string]any)
		tt, _ := r["task_type"].(string)
		s := mcFromAny(r["spent_mc"])
		cap := mcFromAny(r["ceiling_mc"])
		fmt.Fprintf(stdout, "    %-16s %s / %s (%s%%)\n",
			tt, fmtUSD(s), fmtUSD(cap), pct(s, cap))
	}
	return 0
}

// mcFromAny extracts an int64-microcents value from a JSON-decoded
// `any`. JSON numbers decode as float64, so a direct cast loses
// precision past ~2^53 microcents (~$90k); we accept that — the
// daily ceiling never reaches that magnitude. Returns 0 for
// missing/wrong-type values rather than erroring, since the caller
// is rendering a display and partial data is better than blank.
func mcFromAny(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// fmtUSD formats a microcents value as $X.XXXX. Matches the
// planner-cost formatter so operators see the same precision
// across budget and plan-cost output.
func fmtUSD(microcents int64) string {
	dollars := float64(microcents) / 1e9
	return fmt.Sprintf("$%.4f", dollars)
}

// pct returns an integer-percent string ("47") for spent / cap.
// "—" when cap is zero so the caller doesn't need a separate
// branch in its format string.
func pct(spent, cap int64) string {
	if cap <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d", spent*100/cap)
}

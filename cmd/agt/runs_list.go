// SPDX-License-Identifier: MIT
//
// cmd/agt runs list sub-command + render helpers (renderRunRow,
// renderRunsTree). Split from runs.go during Day 211 god-file refactor (#36).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

// cmdRunsList implements `agt runs list [N] [--json]`.
// Walks the journal server-side, pairs task.received/task.completed
// by correlation_id, and shows the result sorted newest-first.
//
// Different from `agt journal tail` which is event-level
// (every kind, no aggregation); runs list is task-level
// (one row per agent loop invocation).
func cmdRunsList(args []string, stdout, stderr io.Writer) int {
	limit := 20
	asJSON := false
	tree := false
	tenant := ""
	status := ""
	intent := ""
	model := ""
	minCostMC := int64(0)
	maxCostMC := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tree":
			tree = true
		case a == "--failed":
			status = "failed"
		case a == "--min-cost" || strings.HasPrefix(a, "--min-cost="):
			v := strings.TrimPrefix(a, "--min-cost=")
			if a == "--min-cost" {
				if i+1 >= len(args) {
					fmt.Fprintf(stderr, "%s runs list: --min-cost needs a dollar amount\n", brand.CLI)
					return 2
				}
				i++
				v = args[i]
			}
			mc, err := usdToMicrocents(v)
			if err != nil {
				fmt.Fprintf(stderr, "%s runs list: --min-cost: %v\n", brand.CLI, err)
				return 2
			}
			minCostMC = mc
		case a == "--max-cost" || strings.HasPrefix(a, "--max-cost="):
			v := strings.TrimPrefix(a, "--max-cost=")
			if a == "--max-cost" {
				if i+1 >= len(args) {
					fmt.Fprintf(stderr, "%s runs list: --max-cost needs a dollar amount\n", brand.CLI)
					return 2
				}
				i++
				v = args[i]
			}
			mc, err := usdToMicrocents(v)
			if err != nil {
				fmt.Fprintf(stderr, "%s runs list: --max-cost: %v\n", brand.CLI, err)
				return 2
			}
			maxCostMC = mc
		case a == "--model":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs list: --model needs a substring\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		case strings.HasPrefix(a, "--model="):
			model = strings.TrimPrefix(a, "--model=")
		case a == "--intent":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs list: --intent needs a substring\n", brand.CLI)
				return 2
			}
			i++
			intent = args[i]
		case strings.HasPrefix(a, "--intent="):
			intent = strings.TrimPrefix(a, "--intent=")
		case a == "--status":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs list: --status needs a value\n", brand.CLI)
				return 2
			}
			i++
			status = args[i]
		case strings.HasPrefix(a, "--status="):
			status = strings.TrimPrefix(a, "--status=")
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs list: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s runs list [N] [--tree] [--status <s>|--failed] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the last N agent runs (default 20, max 1000); --tenant reads a tenant's own runs\n")
			fmt.Fprintf(stdout, "  --tree            group sub-agent runs under the lead that delegated them\n")
			fmt.Fprintf(stdout, "  --status <s>      only runs with this status (completed|failed|running|abandoned)\n")
			fmt.Fprintf(stdout, "  --failed          shorthand for --status failed\n")
			fmt.Fprintf(stdout, "  --intent <substr> only runs whose intent contains <substr> (case-insensitive)\n")
			fmt.Fprintf(stdout, "  --model <substr>  only runs whose model contains <substr> (case-insensitive)\n")
			fmt.Fprintf(stdout, "  --min-cost <usd>  only runs that cost at least <usd> (e.g. 0.01)\n")
			fmt.Fprintf(stdout, "  --max-cost <usd>  only runs that cost at most <usd>\n")
			return 0
		default:
			n, err := strconv.Atoi(a)
			if err != nil {
				fmt.Fprintf(stderr, "%s runs list: unexpected arg %q (expected N, --status <s>, --tenant <id>, or --json)\n", brand.CLI, a)
				return 2
			}
			if n < 1 {
				fmt.Fprintf(stderr, "%s runs list: N must be >= 1 (got %d)\n", brand.CLI, n)
				return 2
			}
			limit = n
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	callArgs := map[string]any{"limit": limit}
	if tenant != "" {
		callArgs["tenant"] = tenant
	}
	if status != "" {
		callArgs["status"] = status // M61: filter by run status
	}
	if intent != "" {
		callArgs["intent"] = intent // M77: filter by intent substring
	}
	if model != "" {
		callArgs["model"] = model // M123: filter by model substring
	}
	if minCostMC > 0 {
		callArgs["min_cost_mc"] = minCostMC // M125: filter by cost floor
	}
	if maxCostMC > 0 {
		callArgs["max_cost_mc"] = maxCostMC // M125: filter by cost ceiling
	}
	res, err := c.Call(ctx, controlplane.CmdRunsList, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s runs list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	rows, _ := res["runs"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no runs yet (journal has no task.received events)")
		return 0
	}
	fmt.Fprintf(stdout, "last %d run(s):\n\n", len(rows))
	if tree {
		renderRunsTree(stdout, rows)
		return 0
	}
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		renderRunRow(stdout, r, "  ", true)
	}
	return 0
}

// renderRunRow prints one run's three-line summary at the given base indent.
// showParentTag appends a "↳ sub-agent of <lead>" marker (flat list, M41); the
// tree view (M43) suppresses it since the hierarchy already conveys parentage.
func renderRunRow(w io.Writer, r map[string]any, base string, showParentTag bool) {
	corr, _ := r["correlation_id"].(string)
	intent, _ := r["intent"].(string)
	status, _ := r["status"].(string)
	reason, _ := r["reason"].(string)
	parent, _ := r["parent_correlation"].(string)
	started := intOfStatus(r["started_unix_ms"])
	duration := intOfStatus(r["duration_ms"])
	iters := intOfStatus(r["iters"])
	spent := mcFromAny(r["spent_mc"]) // M50: this run's spend (microcents)
	model, _ := r["model"].(string)   // M123: the run's primary model ("" if unpriced/mock)

	startedStr := "—"
	if started > 0 {
		startedStr = time.UnixMilli(started).Format("2006-01-02 15:04:05")
	}
	// Both completed and failed runs have a real terminal timestamp, so both
	// carry a meaningful duration; running/abandoned don't.
	durationStr := "—"
	if status == "completed" || status == "failed" {
		durationStr = fmtDuration(duration)
	}
	// Annotate a failure with its classified reason (M30).
	statusDisplay := status
	if status == "failed" && reason != "" {
		statusDisplay = "failed (" + reason + ")"
	}
	intentDisplay := intent
	if intentDisplay == "" {
		intentDisplay = "(no intent recorded)"
	}
	intentDisplay = truncate(intentDisplay, 69)

	corrDisplay := corr
	if showParentTag && parent != "" {
		corrDisplay = corr + "  ↳ sub-agent of " + parent
	}
	fmt.Fprintf(w, "%s%s\n", base, corrDisplay)
	fmt.Fprintf(w, "%s  started : %s   status: %-18s  duration: %s   iters: %d",
		base, startedStr, statusDisplay, durationStr, iters)
	// Append the run's model when journaled (M123) — answers "which model served
	// this run" inline, the natural pair to the --model filter. Quiet for an
	// unpriced/mock run that journaled no model.
	if model != "" {
		fmt.Fprintf(w, "   model: %s", model)
	}
	// Append spend only when this run cost something (a free/local model or
	// the offline mock spends $0) — keeps the row clean in the common case (M50).
	if spent > 0 {
		fmt.Fprintf(w, "   spend: %s", fmtUSD(spent))
	}
	fmt.Fprintf(w, "\n%s  intent  : %s\n", base, intentDisplay)
	// One-line preview of what the run answered (M59), when journaled (M51/M52) —
	// so `agt runs list` shows results, not just intents. Quiet when absent.
	if ap, _ := r["answer_preview"].(string); ap != "" {
		fmt.Fprintf(w, "%s  answer  : %q\n", base, ap)
	}
	fmt.Fprintln(w)
}

// renderRunsTree groups sub-agent runs under the lead that delegated them
// (M43), using each row's parent_correlation (M41). A run whose parent isn't
// in the fetched set (e.g. trimmed by the limit) is treated as a root so it
// still shows. Child order follows the server's newest-first ordering; depth
// adds two spaces of indent per level.
func renderRunsTree(w io.Writer, rows []any) {
	byCorr := map[string]map[string]any{}
	children := map[string][]map[string]any{}
	order := make([]map[string]any, 0, len(rows))
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		corr, _ := r["correlation_id"].(string)
		if corr == "" {
			continue
		}
		byCorr[corr] = r
		order = append(order, r)
	}
	var roots []map[string]any
	for _, r := range order {
		parent, _ := r["parent_correlation"].(string)
		if parent != "" && byCorr[parent] != nil {
			children[parent] = append(children[parent], r)
		} else {
			roots = append(roots, r)
		}
	}
	var walk func(r map[string]any, depth int)
	walk = func(r map[string]any, depth int) {
		renderRunRow(w, r, strings.Repeat("  ", depth+1), false)
		corr, _ := r["correlation_id"].(string)
		for _, ch := range children[corr] {
			walk(ch, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
}

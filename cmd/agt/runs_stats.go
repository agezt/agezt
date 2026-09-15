// SPDX-License-Identifier: MIT
//
// cmd/agt runs stats handler (cmdRunsStats).
// Extracted from runs.go during Day 211 god-file refactor (#59).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdRunsStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var sinceMS int64
	var sinceLabel string
	var tenant string
	var intent string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--intent":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs stats: --intent needs a substring\n", brand.CLI)
				return 2
			}
			i++
			intent = args[i]
		case strings.HasPrefix(a, "--intent="):
			intent = strings.TrimPrefix(a, "--intent=")
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs stats: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "--since":
			// `--since 1h` — value is the next arg.
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs stats: --since needs a duration (e.g. 1h, 30m)\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s runs stats: --since: want a positive Go duration (e.g. 90s, 1h), got %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			// `--since=1h` form.
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s runs stats: --since: want a positive Go duration (e.g. 90s, 1h), got %q\n", brand.CLI, strings.TrimPrefix(a, "--since="))
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s runs stats [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate run health over the journal:\n")
			fmt.Fprintf(stdout, "  total / completed / failed / running / abandoned counts,\n")
			fmt.Fprintf(stdout, "  success rate, completed-run duration avg/min/max/p50/p95,\n")
			fmt.Fprintf(stdout, "  delegation fan-out (sub-agent runs spawned), and spend\n")
			fmt.Fprintf(stdout, "  --since <dur>     restrict to runs started in the last <dur> (e.g. 1h, 30m)\n")
			fmt.Fprintf(stdout, "  --intent <substr> aggregate only runs whose intent contains <substr>\n")
			fmt.Fprintf(stdout, "  --tenant <id>    read a tenant's own runs (needs that tenant's token)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s runs stats: unexpected arg %q (expected --since <dur>, --tenant <id>, or --json)\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	if intent != "" {
		callArgs["intent"] = intent // M78: scope aggregate to matching intents
	}
	if tenant != "" {
		callArgs["tenant"] = tenant
	}
	res, err := c.Call(ctx, controlplane.CmdRunsStats, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s runs stats: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	windowSuffix := ""
	if sinceLabel != "" {
		windowSuffix = ", last " + sinceLabel
	}
	total := intOfStatus(res["total"])
	if total == 0 {
		if sinceLabel != "" {
			fmt.Fprintf(stdout, "no runs in the last %s\n", sinceLabel)
		} else {
			fmt.Fprintln(stdout, "no runs yet (journal has no task.received events)")
		}
		return 0
	}
	completed := intOfStatus(res["completed"])
	failed := intOfStatus(res["failed"])
	running := intOfStatus(res["running"])
	abandoned := intOfStatus(res["abandoned"])
	terminal := intOfStatus(res["terminal"])

	fmt.Fprintf(stdout, "run stats (over %d run(s)%s):\n\n", total, windowSuffix)
	fmt.Fprintf(stdout, "  completed : %d\n", completed)
	// Annotate the failed count with its per-reason breakdown (M36) so the
	// operator sees WHY runs fail, e.g. "failed : 3 (timeout=2, error=1)".
	failedLine := fmt.Sprintf("%d", failed)
	if br := failedByReasonStr(res["failed_by_reason"]); br != "" {
		failedLine += " (" + br + ")"
	}
	fmt.Fprintf(stdout, "  failed    : %s\n", failedLine)
	fmt.Fprintf(stdout, "  running   : %d\n", running)
	fmt.Fprintf(stdout, "  abandoned : %d\n", abandoned)

	// success rate is undefined until at least one run reaches a
	// terminal state — show n/a rather than a misleading 0%.
	if terminal > 0 {
		rate, _ := res["success_rate"].(float64)
		fmt.Fprintf(stdout, "  success   : %.1f%% (%d/%d terminal)\n", rate*100, completed, terminal)
	} else {
		fmt.Fprintf(stdout, "  success   : n/a (no run has finished yet)\n")
	}
	if avgIters, _ := res["avg_iters"].(float64); completed > 0 {
		fmt.Fprintf(stdout, "  avg iters : %.1f\n", avgIters)
	}

	// Duration block — only meaningful when at least one run
	// completed (running/abandoned runs have no end time).
	if dur, _ := res["duration_ms"].(map[string]any); dur != nil {
		dcount := intOfStatus(dur["count"])
		if dcount > 0 {
			fmt.Fprintf(stdout, "\n  duration (over %d completed run(s)):\n", dcount)
			fmt.Fprintf(stdout, "    avg : %s\n", fmtDuration(intOfStatus(dur["avg"])))
			fmt.Fprintf(stdout, "    min : %s\n", fmtDuration(intOfStatus(dur["min"])))
			fmt.Fprintf(stdout, "    p50 : %s\n", fmtDuration(intOfStatus(dur["p50"])))
			fmt.Fprintf(stdout, "    p95 : %s\n", fmtDuration(intOfStatus(dur["p95"])))
			fmt.Fprintf(stdout, "    max : %s\n", fmtDuration(intOfStatus(dur["max"])))
		}
	}

	// Spend distribution block (M60) — how per-run cost is spread (a few
	// expensive runs vs many cheap ones), over priced runs only. Omitted when
	// nothing was priced (free/local model or offline mock).
	if sp, _ := res["spend_microcents"].(map[string]any); sp != nil {
		if scount := intOfStatus(sp["count"]); scount > 0 {
			fmt.Fprintf(stdout, "\n  spend dist (over %d priced run(s)):\n", scount)
			fmt.Fprintf(stdout, "    avg : %s\n", fmtUSD(mcFromAny(sp["avg"])))
			fmt.Fprintf(stdout, "    min : %s\n", fmtUSD(mcFromAny(sp["min"])))
			fmt.Fprintf(stdout, "    p50 : %s\n", fmtUSD(mcFromAny(sp["p50"])))
			fmt.Fprintf(stdout, "    p95 : %s\n", fmtUSD(mcFromAny(sp["p95"])))
			fmt.Fprintf(stdout, "    max : %s\n", fmtUSD(mcFromAny(sp["max"])))
		}
	}

	// Delegation block (M45) — surfaces the SCALE of multi-agent fan-out the
	// other lines can't show: a sub-agent run is just another row in the
	// totals above, indistinguishable from a top-level one. Printed only when
	// delegation actually occurred in the window, so single-agent operators
	// never see noise. "delegations" counts sub-agent runs; "from N run(s)"
	// is the number of leads that delegated; "max fan-out" is the widest.
	if delegations := intOfStatus(res["delegations"]); delegations > 0 {
		leads := intOfStatus(res["delegating_runs"])
		maxFanout := intOfStatus(res["max_fanout"])
		fmt.Fprintf(stdout, "\n  delegations: %d (from %d run(s), max fan-out %d)\n", delegations, leads, maxFanout)
	}

	// Spend block (M47) — what the window's runs cost, with the share
	// attributable to sub-agent runs. Printed only when priced usage was
	// journaled (a free/local model or the offline mock spends $0), so it
	// never shows a misleading $0.0000 line. Reuses the `agt budget`
	// formatter so spend reads identically across surfaces.
	if spent := mcFromAny(res["spent_microcents"]); spent > 0 {
		line := fmt.Sprintf("  spend      : %s", fmtUSD(spent))
		if deleg := mcFromAny(res["delegated_spent_microcents"]); deleg > 0 {
			line += fmt.Sprintf(" (delegated: %s)", fmtUSD(deleg))
		}
		fmt.Fprintln(stdout, line)
	}

	// Per-model breakdown (M124) — where the spend goes across a multi-provider
	// mix: run count + spend per model, sorted by spend desc (ties by name).
	// Printed only when a model was attributed; an all-free/mock window has none.
	if bm, _ := res["by_model"].(map[string]any); len(bm) > 0 {
		type mrow struct {
			model       string
			runs, spent int64
		}
		rows := make([]mrow, 0, len(bm))
		for m, raw := range bm {
			e, _ := raw.(map[string]any)
			rows = append(rows, mrow{model: m, runs: int64(intOfStatus(e["runs"])), spent: mcFromAny(e["spent_microcents"])})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].spent != rows[j].spent {
				return rows[i].spent > rows[j].spent
			}
			return rows[i].model < rows[j].model
		})
		fmt.Fprintf(stdout, "\n  by model:\n")
		for _, r := range rows {
			if r.spent > 0 {
				fmt.Fprintf(stdout, "    %-28s %d run(s), %s\n", r.model, r.runs, fmtUSD(r.spent))
			} else {
				fmt.Fprintf(stdout, "    %-28s %d run(s)\n", r.model, r.runs)
			}
		}
	}
	return 0
}

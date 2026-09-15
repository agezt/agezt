// SPDX-License-Identifier: MIT
//
// cmd/agt runs control sub-commands: cmdRuns + cmdRunsSteerVerb +
// cmdRunsSteer + cmdRunsIntervene + cmdRunsCancel + cmdRunsStats.
// Split from runs.go during Day 211 god-file refactor (#36).
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

func cmdRuns(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s runs: subcommand required (list|show)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list":
		return cmdRunsList(args[1:], stdout, stderr)
	case "show":
		return cmdRunsShow(args[1:], stdout, stderr)
	case "last":
		return cmdRunsLast(args[1:], stdout, stderr)
	case "stats":
		return cmdRunsStats(args[1:], stdout, stderr)
	case "cancel":
		return cmdRunsCancel(args[1:], stdout, stderr)
	case "pause":
		return cmdRunsSteerVerb(controlplane.CmdRunPause, "pause", args[1:], stdout, stderr)
	case "resume":
		return cmdRunsSteerVerb(controlplane.CmdRunResume, "resume", args[1:], stdout, stderr)
	case "step":
		return cmdRunsSteerVerb(controlplane.CmdRunStep, "step", args[1:], stdout, stderr)
	case "steer":
		return cmdRunsSteer(args[1:], stdout, stderr)
	case "intervene":
		return cmdRunsIntervene(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s runs <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [N] [--json]            show the last N agent runs (default 20)\n")
		fmt.Fprintf(stdout, "  show <correlation> [--json]  render one run as a task arc\n")
		fmt.Fprintf(stdout, "  last [--json]                shorthand for show <newest correlation>\n")
		fmt.Fprintf(stdout, "  stats [--json]               aggregate run health (counts, success rate, durations)\n")
		fmt.Fprintf(stdout, "  cancel <correlation>         cancel one in-flight run (→ failed/canceled)\n")
		fmt.Fprintf(stdout, "  pause <correlation>          park a live run at its next iteration boundary\n")
		fmt.Fprintf(stdout, "  resume <correlation>         let a paused run continue\n")
		fmt.Fprintf(stdout, "  step <correlation>           advance a run one iteration, then re-pause\n")
		fmt.Fprintf(stdout, "  steer <correlation> <text>   inject an operator directive into a live run\n")
		fmt.Fprintf(stdout, "  intervene <primitive> <correlation> [text] [--lease <dur>] [--key <id>]\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s runs: unknown subcommand %q (list|show|last|stats|cancel|pause|resume|step|steer|intervene)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdRunsSteerVerb implements the no-argument steering verbs (pause/resume/step):
// each takes just a correlation id and reports whether a live run matched.
func cmdRunsSteerVerb(cmd, verb string, args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs %s <correlation> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if corr == "" {
				corr = a
				continue
			}
			fmt.Fprintf(stderr, "%s runs %s: unexpected arg %q\n", brand.CLI, verb, a)
			return 2
		}
	}
	if corr == "" {
		fmt.Fprintf(stderr, "%s runs %s: correlation id required\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"correlation": corr})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if ok, _ := res["ok"].(bool); ok {
		fmt.Fprintf(stdout, "%s: %s\n", corr, verb+"d")
		return 0
	}
	fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
	return 1
}

// cmdRunsSteer injects an operator directive into a live run; the agent folds it
// into its next prompt at the iteration boundary.
func cmdRunsSteer(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	var parts []string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s runs steer <correlation> <directive...> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "inject a directive into a running agent (folded into its next prompt)\n")
			return 0
		case corr == "":
			corr = a
		default:
			parts = append(parts, a)
		}
	}
	if corr == "" || len(parts) == 0 {
		fmt.Fprintf(stderr, "%s runs steer: correlation id and a directive are required\n", brand.CLI)
		return 2
	}
	directive := strings.Join(parts, " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdRunSteer, map[string]any{"correlation": corr, "directive": directive})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs steer: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if acc, _ := res["accepted"].(bool); acc {
		fmt.Fprintf(stdout, "%s: steering directive accepted\n", corr)
		return 0
	}
	fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
	return 1
}

func cmdRunsIntervene(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var primitive, corr, key string
	var lease time.Duration
	var parts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--lease":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs intervene: --lease needs a duration\n", brand.CLI)
				return 2
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s runs intervene: bad --lease %q\n", brand.CLI, args[i+1])
				return 2
			}
			lease = d
			i++
		case a == "--key":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs intervene: --key needs an idempotency key\n", brand.CLI)
				return 2
			}
			key = args[i+1]
			i++
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s runs intervene <halt|abort|redirect|adjust|query> <correlation> [directive...] [--lease <dur>] [--key <id>] [--json]\n", brand.CLI)
			return 0
		case primitive == "":
			primitive = a
		case corr == "":
			corr = a
		default:
			parts = append(parts, a)
		}
	}
	if primitive == "" || corr == "" {
		fmt.Fprintf(stderr, "%s runs intervene: primitive and correlation are required\n", brand.CLI)
		return 2
	}
	directive := strings.Join(parts, " ")
	callArgs := map[string]any{"primitive": primitive, "correlation": corr}
	if directive != "" {
		callArgs["directive"] = directive
	}
	if lease > 0 {
		callArgs["lease_ms"] = float64(lease / time.Millisecond)
	}
	if key != "" {
		callArgs["idempotency_key"] = key
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdRunIntervene, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s runs intervene: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if acc, _ := res["accepted"].(bool); acc {
		fmt.Fprintf(stdout, "%s: %s %v\n", corr, primitive, res["state"])
		return 0
	}
	fmt.Fprintf(stderr, "intervention not accepted for correlation %q: %v\n", corr, res["reason"])
	return 1
}

// cmdRunsCancel implements `agt runs cancel <correlation> [--json]`.
// Cancels a single in-flight run by correlation id without halting the
// whole daemon (the targeted alternative to `agt halt`). The cancelled
// run terminates as `failed (canceled)` in `agt runs`. Exit code 0 when a
// live run was cancelled, 1 when no matching active run was found (already
// finished, never existed, or wrong id) so scripts can branch on it.
func cmdRunsCancel(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs cancel <correlation> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "cancel one in-flight run by correlation id (leaves the daemon running)\n")
			return 0
		default:
			if corr == "" {
				corr = a
				continue
			}
			fmt.Fprintf(stderr, "%s runs cancel: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if corr == "" {
		fmt.Fprintf(stderr, "%s runs cancel: correlation id required\n", brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdCancelRun, map[string]any{"correlation": corr})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs cancel: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	}
	cancelled, _ := res["cancelled"].(bool)
	if !cancelled {
		if !asJSON {
			fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
		}
		return 1
	}
	if !asJSON {
		fmt.Fprintf(stdout, "cancelled run %s (it will terminate as failed/canceled)\n", corr)
	}
	return 0
}

// cmdRunsStats implements `agt runs stats [--json]`. Asks the
// daemon to fold the whole journal into a single health summary
// (counts, success rate, duration percentiles) and renders it.
// Different from `runs list` (one row per run) — this is the
// fleet-level view: "how are my runs doing overall?". Purely
// additive, read-only observability.
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

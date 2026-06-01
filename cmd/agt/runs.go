// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdRuns dispatches `agt runs <subcommand>`. The only subcommand
// today is `list`; left as a dispatcher so future additions
// (`agt runs show <corr>`, `agt runs failed`) slot in without
// renaming.
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
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s runs <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [N] [--json]            show the last N agent runs (default 20)\n")
		fmt.Fprintf(stdout, "  show <correlation> [--json]  render one run as a task arc\n")
		fmt.Fprintf(stdout, "  last [--json]                shorthand for show <newest correlation>\n")
		fmt.Fprintf(stdout, "  stats [--json]               aggregate run health (counts, success rate, durations)\n")
		fmt.Fprintf(stdout, "  cancel <correlation>         cancel one in-flight run (→ failed/canceled)\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s runs: unknown subcommand %q (list|show|last|stats|cancel)\n", brand.CLI, args[0])
		return 2
	}
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

	c := dial(stderr)
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

	c := dial(stderr)
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
	return 0
}

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
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tree":
			tree = true
		case a == "--failed":
			status = "failed"
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

	c := dial(stderr)
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
	if len(intentDisplay) > 70 {
		intentDisplay = intentDisplay[:69] + "…"
	}

	corrDisplay := corr
	if showParentTag && parent != "" {
		corrDisplay = corr + "  ↳ sub-agent of " + parent
	}
	fmt.Fprintf(w, "%s%s\n", base, corrDisplay)
	fmt.Fprintf(w, "%s  started : %s   status: %-18s  duration: %s   iters: %d",
		base, startedStr, statusDisplay, durationStr, iters)
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

// cmdRunsShow implements `agt runs show <correlation> [--json]`.
//
// Walks the correlation chain (via CmdWhy on the daemon) and
// renders the events as a task arc: intent → per-round
// (llm.request → tool calls → llm.response) → final answer.
// Different from `agt why <event_id> --payload`, which dumps
// every event verbatim; runs show is opinionated, with
// per-round grouping and an answer banner at the end. `agt why`
// remains the right tool for "I need every event"; runs show
// is the right tool for "what did the agent actually do?".
//
// Resolves the chain by correlation_id directly — operators
// already have it from `agt runs list` or the `(correlation_id:
// ...)` footer `agt run` prints. To make that work without
// requiring an event_id we leverage the property that
// correlation_id is also the *id* of the task.received event in
// the M0.5 schema NOT — actually `Why` takes any event ID in
// the chain, so we use the first event the chain produces. The
// server-side Why handler walks the journal for the chain, so
// we send a probe with the correlation as event_id; if that
// fails, we surface a clear "no events for that correlation".
func cmdRunsShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs show <correlation> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "render a single run as a task arc (intent → rounds → answer)\n")
			return 0
		default:
			if corr == "" {
				corr = a
				continue
			}
			fmt.Fprintf(stderr, "%s runs show: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if corr == "" {
		fmt.Fprintf(stderr, "%s runs show: correlation id required\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First-pass: ask the daemon to enumerate runs and find the
	// chain whose correlation matches. We pull the first event ID
	// in that chain and use it as the seed for CmdWhy (which
	// requires any event ID in the chain, not the correlation
	// itself). This indirection means `runs show` works with the
	// existing Why contract without a new server endpoint.
	listRes, err := c.Call(ctx, controlplane.CmdRunsList, map[string]any{"limit": 1000})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs show: %v\n", brand.CLI, err)
		return 1
	}
	runs, _ := listRes["runs"].([]any)
	var matchedRow map[string]any
	// Index every run's summary by correlation so we can show a delegated
	// sub-agent's outcome inline on the lead's arc (M44).
	summaries := map[string]map[string]any{}
	for _, raw := range runs {
		r, _ := raw.(map[string]any)
		s, _ := r["correlation_id"].(string)
		if s != "" {
			summaries[s] = r
		}
		if s == corr {
			matchedRow = r
		}
	}
	// A plan-execution correlation ("plan-…") isn't a task run, so it won't be
	// in collectRuns/runs list (M82). Don't error yet — walk the chain first and,
	// if it carries plan.started, synthesise a summary from it below.

	// Now walk the chain. We need an event ID — fetch via a small
	// pulse-style trick: use the correlation_id as event_id and
	// see if the daemon resolves it. The journal's Why looks up
	// the target event then enumerates same-correlation; if the
	// correlation_id is also an event ULID (current convention),
	// it works. Otherwise we'd need a CmdWhyByCorrelation endpoint —
	// but in the M0.5+ schema correlation_id is set to "run-<ULID>"
	// or "plan-<ULID>", neither of which match an event ID. So we
	// instead pull events via the journal_tail endpoint and filter
	// client-side.
	tailRes, err := c.Call(ctx, controlplane.CmdJournalTail, map[string]any{"n": 10_000})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs show: %v\n", brand.CLI, err)
		return 1
	}
	allEvents, _ := tailRes["events"].([]any)
	chain := make([]map[string]any, 0)
	for _, raw := range allEvents {
		e, _ := raw.(map[string]any)
		if s, _ := e["correlation_id"].(string); s == corr {
			chain = append(chain, e)
		}
	}
	if len(chain) == 0 {
		fmt.Fprintf(stderr, "%s runs show: no run with correlation %q (try `%s runs list`)\n",
			brand.CLI, corr, brand.CLI)
		return 1
	}
	// Plan-execution run (M82): no task-run summary, but the chain has plan
	// events — synthesise a header from them so the arc renders.
	if matchedRow == nil {
		matchedRow = synthesizePlanSummary(chain)
	}

	if asJSON {
		// Echo the matched row metadata plus the full event chain
		// so jq pipelines have everything in one document.
		out := map[string]any{
			"correlation_id": corr,
			"summary":        matchedRow,
			"events":         chain,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	// Build per-correlation outcomes so the arc can show a delegated
	// sub-agent's terminal result inline (M44).
	outcomes := map[string]childOutcome{}
	for cid, s := range summaries {
		st, _ := s["status"].(string)
		rs, _ := s["reason"].(string)
		ap, _ := s["answer_preview"].(string)
		outcomes[cid] = childOutcome{
			status:        st,
			reason:        rs,
			iters:         int64(intOfStatus(s["iters"])),
			durationMS:    int64(intOfStatus(s["duration_ms"])),
			spentMC:       mcFromAny(s["spent_mc"]), // M50
			answerPreview: ap,                       // M52
		}
	}

	renderTaskArc(stdout, corr, matchedRow, chain, outcomes)
	return 0
}

// childOutcome is a delegated sub-agent's terminal result, shown inline under
// the lead's `delegated → …` line (M44). The run answer text isn't journaled
// (the schema records text_chars/usage, not the message body), so the outcome
// is the status/iters/duration — enough to answer "did the delegation
// succeed?"; the sub-agent's events are a `agt runs show <child>` away.
type childOutcome struct {
	status        string
	reason        string
	iters         int64
	durationMS    int64
	spentMC       int64  // this sub-agent's spend in microcents (M50; 0 = none/unpriced)
	answerPreview string // one-line excerpt of the sub-agent's answer (M52; "" if none)
}

// cmdRunsLast implements `agt runs last [--json]` — a convenience
// shortcut for the most-common operator pattern: render the
// task arc for the run that just finished (or is still running).
// Replaces the awkward two-step
//   `agt runs list 1 --json | jq -r '.runs[0].correlation_id' | xargs agt runs show`
// with a single command. Identical exit codes + rendering to
// `runs show`; the only difference is "which correlation".
func cmdRunsLast(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs last [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "render the most-recent run as a task arc (shorthand for `runs show`)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s runs last: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Limit=1 because CmdRunsList sorts newest-first — the head of
	// the slice is exactly what `last` wants.
	listRes, err := c.Call(ctx, controlplane.CmdRunsList, map[string]any{"limit": 1})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs last: %v\n", brand.CLI, err)
		return 1
	}
	runs, _ := listRes["runs"].([]any)
	if len(runs) == 0 {
		fmt.Fprintln(stderr, "no runs yet (journal has no task.received events)")
		return 1
	}
	row, _ := runs[0].(map[string]any)
	corr, _ := row["correlation_id"].(string)
	if corr == "" {
		fmt.Fprintln(stderr, "runs last: most-recent run has no correlation_id (corrupt journal?)")
		return 1
	}

	// Delegate to runs show so the rendering path is identical.
	// Pass the JSON flag through verbatim.
	delegateArgs := []string{corr}
	if asJSON {
		delegateArgs = append(delegateArgs, "--json")
	}
	return cmdRunsShow(delegateArgs, stdout, stderr)
}

// failedByReasonStr renders the M36 failed_by_reason map as a compact,
// stably-ordered string like "timeout=2, error=1". Known reasons come
// first in a fixed order (so the line is stable run-to-run); any unknown
// reasons follow. Returns "" for an empty/absent map. JSON decodes the
// counts as float64.
func failedByReasonStr(raw any) string {
	m, _ := raw.(map[string]any)
	if len(m) == 0 {
		return ""
	}
	var parts []string
	seen := map[string]bool{}
	for _, reason := range []string{"error", "timeout", "max_iters", "canceled", "unknown"} {
		if n := intOfStatus(m[reason]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", reason, n))
			seen[reason] = true
		}
	}
	// Any reason tags we didn't anticipate — append in sorted order so the
	// output stays deterministic.
	var extras []string
	for reason := range m {
		if !seen[reason] {
			extras = append(extras, reason)
		}
	}
	sort.Strings(extras)
	for _, reason := range extras {
		if n := intOfStatus(m[reason]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", reason, n))
		}
	}
	return strings.Join(parts, ", ")
}

// renderTaskArc prints a human-friendly view of a single run.
// Layout (rough):
//
//	correlation: run-01H...
//	intent     : do the thing
//	status     : completed (5 iters, 1.2s)
//
//	round 1
//	  llm.request   → 1234 input tokens
//	  tool: shell   input={...}
//	    result      output=...
//	  llm.response  → 567 output tokens
//
//	round 2 ...
//
//	final answer:
//	  ...
//
// Falls back to a minimal one-event-per-line view if the chain
// doesn't fit the canonical task arc (e.g. an old run from a
// schema that didn't emit llm.request/response).
// arcPreviewRunes bounds a tool input/output excerpt on a task-arc line (M68) —
// long enough to read a short command or error, short enough to keep one event
// per line. Mirrors the server-side toolOutputPreviewRunes for `agt tool log`.
const arcPreviewRunes = 80

// arcPreview renders a journaled tool input/output value as a compact one-line
// excerpt (M68): strings pass through, anything else (the raw-JSON input) is
// re-marshaled, then whitespace is collapsed and the result truncated. Returns
// "" for empty/unrenderable values so the caller simply omits the detail.
func arcPreview(v any) string {
	if v == nil {
		return ""
	}
	var s string
	switch t := v.(type) {
	case string:
		s = t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		s = string(b)
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) > arcPreviewRunes {
		return string(r[:arcPreviewRunes]) + "…"
	}
	return s
}

// synthesizePlanSummary builds a runTaskArc header summary for a plan-execution
// correlation (M82), which has no task-run row in collectRuns. It reads
// plan.started for the name/intent and the terminal plan.completed/plan.failed
// for status, so `agt runs show <plan-corr>` renders a header like a task run's.
// Falls back to a minimal "completed" summary if the chain has events but no
// recognisable plan lifecycle (an unknown-but-present correlation).
func synthesizePlanSummary(chain []map[string]any) map[string]any {
	summary := map[string]any{"status": "running"}
	for _, e := range chain {
		kind, _ := e["kind"].(string)
		payload, _ := e["payload"].(map[string]any)
		switch kind {
		case "plan.started":
			if name, _ := payload["plan_name"].(string); name != "" {
				summary["intent"] = "plan: " + name
			}
		case "plan.completed":
			summary["status"] = "completed"
		case "plan.failed":
			summary["status"] = "failed"
			if r, _ := payload["reason"].(string); r != "" {
				summary["reason"] = r
			}
		}
	}
	return summary
}

func renderTaskArc(w io.Writer, corr string, summary map[string]any, events []map[string]any, outcomes map[string]childOutcome) {
	intent, _ := summary["intent"].(string)
	status, _ := summary["status"].(string)
	iters := intOfStatus(summary["iters"])
	duration := intOfStatus(summary["duration_ms"])

	fmt.Fprintf(w, "correlation: %s\n", corr)
	if intent != "" {
		fmt.Fprintf(w, "intent     : %s\n", intent)
	}
	reason, _ := summary["reason"].(string)
	switch status {
	case "completed":
		fmt.Fprintf(w, "status     : completed (%d iters, %s)\n", iters, fmtDuration(duration))
	case "running":
		fmt.Fprintf(w, "status     : running (no task.completed yet — abandoned?)\n")
	case "failed":
		// A failed run's arc must say WHY (M70): the reason is the first thing an
		// operator wants, and it was silently dropped before. duration shown when
		// the fold could compute it (FailedUnixMS − StartedUnixMS).
		line := "status     : failed"
		if reason != "" {
			line += " (" + reason + ")"
		}
		if duration > 0 {
			line += " after " + fmtDuration(duration)
		}
		fmt.Fprintln(w, line)
	default:
		fmt.Fprintf(w, "status     : %s\n", status)
	}
	// This run's own spend (M50) — shown only when it cost something. For a lead
	// this is its DIRECT spend; each delegation's cost is on its ↳ line below.
	if spent := mcFromAny(summary["spent_mc"]); spent > 0 {
		fmt.Fprintf(w, "spend      : %s\n", fmtUSD(spent))
	}
	fmt.Fprintln(w)

	// Group events into rounds. A "round" starts at llm.request
	// and ends at the next llm.response. Tool events between them
	// belong to that round. Events outside the round structure
	// (task.received, task.completed, policy.decision, etc.)
	// render inline at the position they appear.
	round := 0
	inRound := false
	var finalAnswer string
	// Per-call tool latency (M72): stash each tool.invoked timestamp by call_id
	// so the matching tool.result can show its wall-clock — the same
	// invoked→result span `agt tool log` reports, here inline on the arc.
	invokedAt := map[string]int64{}
	// Arc-footer tallies (M81): a long arc gets a one-line summary at the end so
	// an operator doesn't have to scroll back and count.
	toolCalls := 0
	toolErrors := 0

	for _, e := range events {
		kind, _ := e["kind"].(string)
		payload, _ := e["payload"].(map[string]any)
		seq := intOfStatus(e["seq"])
		ts := intOfStatus(e["ts_unix_ms"])
		switch kind {
		case "task.received":
			// Intent is shown in the header; surface image attachments (M93) as
			// run provenance so a vision run is self-evident in the arc.
			if n := intOfStatus(payload["images"]); n > 0 {
				fmt.Fprintf(w, "inputs     : %d image attachment(s)\n", n)
			}
		case "llm.request":
			round++
			fmt.Fprintf(w, "round %d (seq=%d)\n", round, seq)
			fmt.Fprintf(w, "  llm.request\n")
			inRound = true
		case "llm.response":
			fmt.Fprintf(w, "  llm.response")
			if usage, _ := payload["usage"].(map[string]any); usage != nil {
				in := intOfStatus(usage["input_tokens"])
				out := intOfStatus(usage["output_tokens"])
				fmt.Fprintf(w, "  (input=%d, output=%d tokens)", in, out)
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w)
			inRound = false
		case "tool.invoked":
			tool, _ := payload["tool"].(string)
			if callID, _ := payload["call_id"].(string); callID != "" {
				invokedAt[callID] = ts // M72: anchor for the result's latency
			}
			indent := "  "
			if inRound {
				indent = "    "
			}
			line := fmt.Sprintf("%stool.invoked: %s", indent, tool)
			// Show a compact input excerpt so the arc says WHAT was called, not
			// just that something was (M68). input is journaled as raw JSON.
			if in := arcPreview(payload["input"]); in != "" {
				line += "  " + in
			}
			fmt.Fprintln(w, line)
		case "tool.result":
			indent := "  "
			if inRound {
				indent = "    "
			}
			// The agent journals this flag as "error" (agent.go); reading
			// "is_error" here meant the arc always said "ok", even for a failed
			// tool call (M68 fix). Honour the real field name.
			isErr, _ := payload["error"].(bool)
			toolCalls++
			if isErr {
				toolErrors++
			}
			tag := "ok"
			if isErr {
				tag = "ERROR"
			}
			line := fmt.Sprintf("%stool.result : %s", indent, tag)
			// Per-call latency (M72): the invoked→result span, joined by call_id.
			if callID, _ := payload["call_id"].(string); callID != "" {
				if it, ok := invokedAt[callID]; ok && ts >= it {
					line += fmt.Sprintf(" (%s)", fmtDuration(ts-it))
				}
			}
			if out := arcPreview(payload["output"]); out != "" {
				line += "  " + out
			}
			fmt.Fprintln(w, line)
		case "task.completed":
			// The run's final answer is journaled on task.completed (M51):
			// {iters, chars, stopped, answer}. Prefer it over the last
			// llm.response's content (the older, often-empty path) — it's the
			// authoritative end-of-run text and comes after every llm.response,
			// so it wins. Pre-M51 runs without the field fall back below.
			if a, _ := payload["answer"].(string); a != "" {
				finalAnswer = a
			}
		case "budget.consumed":
			// Per-round cost (M69): the governor stamps {model, cost_microcents,
			// input/output_tokens} on each priced LLM call. The header shows the
			// run's TOTAL spend (M50); this shows where it accrued, round by round.
			// Rendered inside the round's indent when we're mid-round.
			indent := "  "
			if inRound {
				indent = "    "
			}
			model, _ := payload["model"].(string)
			cost := mcFromAny(payload["cost_microcents"])
			line := indent + "budget: "
			if model != "" {
				line += model + " "
			}
			line += fmtUSD(cost)
			in := intOfStatus(payload["input_tokens"])
			out := intOfStatus(payload["output_tokens"])
			if in > 0 || out > 0 {
				line += fmt.Sprintf(" (in=%d, out=%d tokens)", in, out)
			}
			fmt.Fprintln(w, line)
		case "plan.started":
			// Plan-execution runs (M82): `agt runs show <plan-corr>` walks the
			// scheduler's plan.started + node.* events. Render them legibly instead
			// of as generic default-branch lines, so a plan run reads like a task arc.
			name, _ := payload["plan_name"].(string)
			nodeCount := intOfStatus(payload["node_count"])
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Fprintf(w, "plan: %s (%d node(s))\n", name, nodeCount)
		case "node.started":
			nodeID, _ := payload["node_id"].(string)
			nodeKind, _ := payload["node_kind"].(string)
			fmt.Fprintf(w, "  node %s [%s] started\n", nodeID, nodeKind)
		case "node.completed":
			nodeID, _ := payload["node_id"].(string)
			nodeKind, _ := payload["node_kind"].(string)
			line := fmt.Sprintf("  node %s [%s] completed", nodeID, nodeKind)
			if ob := intOfStatus(payload["output_bytes"]); ob > 0 {
				line += fmt.Sprintf(" (%dB)", ob)
			}
			fmt.Fprintln(w, line)
		case "node.failed":
			nodeID, _ := payload["node_id"].(string)
			nodeKind, _ := payload["node_kind"].(string)
			nodeErr, _ := payload["error"].(string)
			line := fmt.Sprintf("  node %s [%s] FAILED", nodeID, nodeKind)
			if nodeErr != "" {
				line += ": " + nodeErr
			}
			fmt.Fprintln(w, line)
		case "task.failed":
			// The terminal failure, inline at the round it occurred (M70). The
			// header summarises it; this marks WHERE in the arc the run died.
			fr, _ := payload["reason"].(string)
			line := "  task.failed"
			if fr != "" {
				line += ": " + fr
			}
			fmt.Fprintln(w, line)
		case "policy.decision":
			// The agent journals {capability, allow, hard_denied, reason} — there
			// is no "decision" string, so the old read rendered a blank verdict on
			// every policy line (M68 fix). Derive the verdict from the real fields.
			capName, _ := payload["capability"].(string)
			allow, _ := payload["allow"].(bool)
			hard, _ := payload["hard_denied"].(bool)
			reason, _ := payload["reason"].(string)
			verdict := "allow"
			if !allow {
				verdict = "DENY"
				if hard {
					verdict = "DENY(hard)"
				}
			}
			line := fmt.Sprintf("  policy: %-10s %s", verdict, capName)
			if reason != "" {
				line += "  (" + reason + ")"
			}
			fmt.Fprintln(w, line)
		case "approval.requested", "approval.granted", "approval.denied", "approval.timeout":
			fmt.Fprintf(w, "  %s\n", kind)
		case "subagent.spawned":
			// Call out the delegation prominently with the child correlation
			// (drill in with `agt runs show <child>`) and the delegated task
			// (M41) — instead of the generic "subagent.spawned (seq=N)" line.
			child, _ := payload["child_correlation"].(string)
			task, _ := payload["task"].(string)
			if len(task) > 60 {
				task = task[:59] + "…"
			}
			fmt.Fprintf(w, "  delegated → %s", child)
			if task != "" {
				fmt.Fprintf(w, "  (task: %s)", task)
			}
			fmt.Fprintln(w)
			// Show the sub-agent's terminal outcome inline (M44), so the
			// lead's arc tells the whole story without `agt runs show <child>`.
			if oc, ok := outcomes[child]; ok && oc.status != "" {
				statusStr := oc.status
				if oc.status == "failed" && oc.reason != "" {
					statusStr = "failed (" + oc.reason + ")"
				}
				durStr := ""
				if oc.status == "completed" || oc.status == "failed" {
					durStr = ", " + fmtDuration(oc.durationMS)
				}
				spendStr := ""
				if oc.spentMC > 0 {
					spendStr = ", " + fmtUSD(oc.spentMC) // M50: what this delegation cost
				}
				fmt.Fprintf(w, "    ↳ %s (%d iters%s%s)", statusStr, oc.iters, durStr, spendStr)
				// One-line preview of what the sub-agent answered (M52), so the
				// lead's arc shows the delegation's RESULT, not just its outcome.
				if oc.answerPreview != "" {
					fmt.Fprintf(w, ": %q", oc.answerPreview)
				}
				fmt.Fprintln(w)
			}
		default:
			// Surface unknown kinds at minimal verbosity so a future
			// kind doesn't silently vanish from the arc view.
			fmt.Fprintf(w, "  %s (seq=%d)\n", kind, seq)
		}

		// Capture the last assistant message content for the final
		// answer line. The agent loop's last llm.response carries
		// it.
		if kind == "llm.response" && payload != nil {
			if msg, _ := payload["message"].(map[string]any); msg != nil {
				if content, _ := msg["content"].(string); content != "" {
					finalAnswer = content
				}
			}
		}
	}

	// Summary footer (M81): collapse the arc into one glanceable line — rounds,
	// tool calls (with error count), spend, duration — so a long arc doesn't need
	// scrolling back to tally. Spend/duration come from the same folded summary
	// the header uses, so the footer never disagrees with it.
	footer := fmt.Sprintf("summary    : %d round(s), %d tool call(s)", round, toolCalls)
	if toolErrors > 0 {
		footer += fmt.Sprintf(" (%d error(s))", toolErrors)
	}
	if spent := mcFromAny(summary["spent_mc"]); spent > 0 {
		footer += ", " + fmtUSD(spent)
	}
	if duration > 0 {
		footer += ", " + fmtDuration(duration)
	}
	fmt.Fprintln(w, footer)

	if finalAnswer != "" {
		fmt.Fprintln(w, "final answer:")
		fmt.Fprintf(w, "  %s\n", finalAnswer)
	}
}

// fmtDuration renders milliseconds as a human-readable duration.
// 0 → "—"; <1s → "Nms"; <60s → "N.Ns"; otherwise "MmNs".
// Distinct from fmtUptime (which uses seconds + always emits at
// least seconds-level granularity) — runs typically last under
// a minute so sub-second precision matters.
func fmtDuration(ms int64) string {
	switch {
	case ms <= 0:
		return "—"
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		m := ms / 60_000
		s := (ms % 60_000) / 1000
		return fmt.Sprintf("%dm%ds", m, s)
	}
}

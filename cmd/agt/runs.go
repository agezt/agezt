// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
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
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s runs <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [N] [--json]            show the last N agent runs (default 20)\n")
		fmt.Fprintf(stdout, "  show <correlation> [--json]  render one run as a task arc\n")
		fmt.Fprintf(stdout, "  last [--json]                shorthand for show <newest correlation>\n")
		fmt.Fprintf(stdout, "  stats [--json]               aggregate run health (counts, success rate, durations)\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s runs: unknown subcommand %q (list|show|last|stats)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdRunsStats implements `agt runs stats [--json]`. Asks the
// daemon to fold the whole journal into a single health summary
// (counts, success rate, duration percentiles) and renders it.
// Different from `runs list` (one row per run) — this is the
// fleet-level view: "how are my runs doing overall?". Purely
// additive, read-only observability.
func cmdRunsStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs stats [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate run health over the whole journal:\n")
			fmt.Fprintf(stdout, "  total / completed / running / abandoned counts,\n")
			fmt.Fprintf(stdout, "  success rate, and completed-run duration avg/min/max/p50/p95\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s runs stats: unexpected arg %q (expected --json)\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdRunsStats, nil)
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

	total := intOfStatus(res["total"])
	if total == 0 {
		fmt.Fprintln(stdout, "no runs yet (journal has no task.received events)")
		return 0
	}
	completed := intOfStatus(res["completed"])
	running := intOfStatus(res["running"])
	abandoned := intOfStatus(res["abandoned"])
	terminal := intOfStatus(res["terminal"])

	fmt.Fprintf(stdout, "run stats (over %d run(s)):\n\n", total)
	fmt.Fprintf(stdout, "  completed : %d\n", completed)
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
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs list [N] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the last N agent runs (default 20, max 1000)\n")
			return 0
		default:
			n, err := strconv.Atoi(a)
			if err != nil {
				fmt.Fprintf(stderr, "%s runs list: unexpected arg %q (expected N or --json)\n", brand.CLI, a)
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
	res, err := c.Call(ctx, controlplane.CmdRunsList, map[string]any{"limit": limit})
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
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		corr, _ := r["correlation_id"].(string)
		intent, _ := r["intent"].(string)
		status, _ := r["status"].(string)
		started := intOfStatus(r["started_unix_ms"])
		duration := intOfStatus(r["duration_ms"])
		iters := intOfStatus(r["iters"])

		startedStr := "—"
		if started > 0 {
			startedStr = time.UnixMilli(started).Format("2006-01-02 15:04:05")
		}
		durationStr := "—"
		if status == "completed" {
			durationStr = fmtDuration(duration)
		}
		intentDisplay := intent
		if intentDisplay == "" {
			intentDisplay = "(no intent recorded)"
		}
		if len(intentDisplay) > 70 {
			intentDisplay = intentDisplay[:69] + "…"
		}

		fmt.Fprintf(stdout, "  %s\n", corr)
		fmt.Fprintf(stdout, "    started : %s   status: %-9s  duration: %s   iters: %d\n",
			startedStr, status, durationStr, iters)
		fmt.Fprintf(stdout, "    intent  : %s\n\n", intentDisplay)
	}
	return 0
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
	for _, raw := range runs {
		r, _ := raw.(map[string]any)
		if s, _ := r["correlation_id"].(string); s == corr {
			matchedRow = r
			break
		}
	}
	if matchedRow == nil {
		fmt.Fprintf(stderr, "%s runs show: no run with correlation %q (try `%s runs list`)\n",
			brand.CLI, corr, brand.CLI)
		return 1
	}

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
		fmt.Fprintf(stderr, "%s runs show: no journaled events for correlation %q\n",
			brand.CLI, corr)
		return 1
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

	renderTaskArc(stdout, corr, matchedRow, chain)
	return 0
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
func renderTaskArc(w io.Writer, corr string, summary map[string]any, events []map[string]any) {
	intent, _ := summary["intent"].(string)
	status, _ := summary["status"].(string)
	iters := intOfStatus(summary["iters"])
	duration := intOfStatus(summary["duration_ms"])

	fmt.Fprintf(w, "correlation: %s\n", corr)
	if intent != "" {
		fmt.Fprintf(w, "intent     : %s\n", intent)
	}
	switch status {
	case "completed":
		fmt.Fprintf(w, "status     : completed (%d iters, %s)\n", iters, fmtDuration(duration))
	case "running":
		fmt.Fprintf(w, "status     : running (no task.completed yet — abandoned?)\n")
	default:
		fmt.Fprintf(w, "status     : %s\n", status)
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

	for _, e := range events {
		kind, _ := e["kind"].(string)
		payload, _ := e["payload"].(map[string]any)
		seq := intOfStatus(e["seq"])
		switch kind {
		case "task.received":
			// Already shown in the header.
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
			indent := "  "
			if inRound {
				indent = "    "
			}
			fmt.Fprintf(w, "%stool.invoked: %s\n", indent, tool)
		case "tool.result":
			indent := "  "
			if inRound {
				indent = "    "
			}
			isErr, _ := payload["is_error"].(bool)
			tag := "ok"
			if isErr {
				tag = "ERROR"
			}
			fmt.Fprintf(w, "%stool.result : %s\n", indent, tag)
		case "task.completed":
			// The final-answer text isn't on task.completed itself
			// (that carries iters/chars/stopped). The last
			// llm.response payload's content is the answer; we
			// stash it as we walked.
		case "policy.decision":
			cap, _ := payload["capability"].(string)
			dec, _ := payload["decision"].(string)
			fmt.Fprintf(w, "  policy: %s %s\n", cap, dec)
		case "approval.requested", "approval.granted", "approval.denied", "approval.timeout":
			fmt.Fprintf(w, "  %s\n", kind)
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

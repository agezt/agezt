// SPDX-License-Identifier: MIT
//
// cmd/agt runs show sub-command + childOutcome type. Split from runs.go
// during Day 211 god-file refactor (#36). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

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

	c := dialpkg.New(stderr)
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

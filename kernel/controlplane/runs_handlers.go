// SPDX-License-Identifier: MIT

// Run-listing handlers: handleRunsList, handleRunsStats, plus durationStats and percentileNearestRank helpers.
// Code extracted from runs.go during the Day-37 god-file split. Public API unchanged.
package controlplane


import (
	"net"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/journal"
)


func (s *Server) handleRunsList(conn net.Conn, req Request) {
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}

	// Cursor pagination (M-pending): the client passes back an opaque token
	// from the previous page; we return entries strictly OLDER than the
	// (ms, seq) pair the token encodes. Cursor is optional — a missing or
	// unparseable cursor falls back to the newest page, which is what the
	// first call of a session does anyway.
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(req.Args["cursor"])

	// Optional filters: status (M61) completed|failed|running|abandoned;
	// intent substring (M77) and model substring (M123), both case-insensitive
	// contains, so an operator can find "that deploy run" / "which runs used
	// claude-opus" without scanning the whole list.
	filters, err := argStrings(req.Args, "status", "intent", "model")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	statusFilter := filters["status"]
	intentQuery := strings.ToLower(filters["intent"])
	modelQuery := strings.ToLower(filters["model"])
	// Optional cost band (M125), in microcents: keep runs whose folded spend is
	// >= min and (when set) <= max — "which runs cost at least $X / blew the
	// budget". A run that never spent (0) is excluded once a positive min is set.
	minCostMC := int64Arg(req.Args["min_cost_mc"])
	maxCostMC := int64Arg(req.Args["max_cost_mc"])

	// Tenant-scoped (M39): an empty tenant reads the primary journal; a named
	// tenant reads its own isolated journal, so a tenant sees only its runs.
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	runs, err := s.collectRuns(k)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	// Sort by StartedUnixMS DESC. Entries with zero start time (the
	// "completed without received" edge case) sort to the bottom.
	entries := make([]*runEntry, 0, len(runs))
	for _, r := range runs {
		// Status filter (M61): keep only runs matching the requested status,
		// applied BEFORE the limit so `list 5 --failed` returns 5 failed runs,
		// not "failed runs among the last 5".
		if statusFilter != "" && runEntryStatus(r) != statusFilter {
			continue
		}
		// Intent substring filter (M77), also before the limit.
		if intentQuery != "" && !strings.Contains(strings.ToLower(r.Intent), intentQuery) {
			continue
		}
		// Model substring filter (M123), also before the limit.
		if modelQuery != "" && !strings.Contains(strings.ToLower(r.Model), modelQuery) {
			continue
		}
		// Cost band (M125), also before the limit.
		if minCostMC > 0 && r.SpentMicrocents < minCostMC {
			continue
		}
		if maxCostMC > 0 && r.SpentMicrocents > maxCostMC {
			continue
		}
		entries = append(entries, r)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].StartedUnixMS != entries[j].StartedUnixMS {
			return entries[i].StartedUnixMS > entries[j].StartedUnixMS
		}
		// Same wall-clock millisecond: fall back to journal seq so
		// the newer-arrived run still sorts first.
		return entries[i].StartedSeq > entries[j].StartedSeq
	})
	// Cursor filter (M-pending): when the client passed a cursor, drop
	// every entry that sorts at or ABOVE the cursor's (ms, seq) pair —
	// that is, keep only entries STRICTLY OLDER than the cursor. The
	// equality clause is critical: without it, the cursor's own row
	// would re-appear on the next page.
	//
	// The cursor encodes the (ms, seq) of the FIRST entry on the previous
	// page (the newest of the descending sort), so we want to drop
	// anything that is EQUAL to that pair — that's the row the client
	// already has.
	if cursorOK {
		filtered := entries[:0]
		for _, r := range entries {
			if journal.KeepBeforeCursor(r.StartedUnixMS, r.StartedSeq, cursorMS, cursorSeq) {
				filtered = append(filtered, r)
			}
		}
		entries = filtered
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}

	out := make([]map[string]any, 0, len(entries))
	for _, r := range entries {
		status := "running"
		reason := ""
		duration := int64(0)
		switch {
		case r.Completed:
			status = "completed"
			if r.StartedUnixMS > 0 {
				duration = r.CompletedUnixMS - r.StartedUnixMS
			}
		case r.Failed:
			// Errored out live (M30) — provider error, max iters, or a
			// cancelled/timed-out context. The reason tag drills down.
			status = "failed"
			reason = r.FailReason
			if r.StartedUnixMS > 0 && r.FailedUnixMS >= r.StartedUnixMS {
				duration = r.FailedUnixMS - r.StartedUnixMS
			}
		case r.Abandoned:
			// Reconciled at boot: received but never completed in a prior
			// session. Not "running" — the daemon that owned it is gone.
			status = "abandoned"
		}
		row := map[string]any{
			"correlation_id":     r.CorrelationID,
			"intent":             r.Intent,
			"status":             status,
			"reason":             reason,
			"started_unix_ms":    r.StartedUnixMS,
			"completed_unix_ms":  r.CompletedUnixMS,
			"duration_ms":        duration,
			"iters":              r.Iters,
			"parent_correlation": r.ParentCorrelation, // "" for top-level runs (M41)
			"spent_mc":           r.SpentMicrocents,   // this run's spend in microcents (M50; 0 = none/unpriced)
			"model":              r.Model,             // primary (first-routed) model (M123; "" if unpriced/mock)
			"answer_preview":     r.AnswerPreview,     // one-line excerpt of the final answer (M52; "" if none)
			"agent":              r.Agent,             // roster agent slug (M73; "" for chat runs)
		}
		// Live activity phase — authoritative (folded from the full journal), so the
		// monitors show what a running run is doing even on a fresh load. Only for
		// running runs; a terminal run's status already says how it ended.
		if status == "running" && r.Phase != "" {
			row["phase"] = r.Phase
			if r.Tool != "" {
				row["tool"] = r.Tool
			}
		}
		out = append(out, row)
	}

	// Compute the cursor for the next page: the (ms, seq) of the LAST
	// emitted entry (the OLDEST in the descending sort). The client passes
	// this back on the next request to skip past it. We use the LAST entry,
	// not the FIRST — the cursor filter (above) removes entries that sort
	// STRICTLY GREATER THAN the cursor's (ms, seq) pair, so the cursor
	// needs to be the oldest row we've already emitted. Encoding the FIRST
	// (newest) entry would re-emit everything on the next page, since the
	// cursor would only "skip" the single newest row.
	var nextCursor string
	if n := len(entries); n > 0 {
		last := entries[n-1]
		nextCursor = journal.NextCursor(last.StartedUnixMS, last.StartedSeq, n, limit)
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"runs":        out,
			"count":       len(out),
			"next_cursor": nextCursor,
		},
	})
}

// handleRunsStats aggregates the whole journal into a single
// summary of agent-run health. Pure read-only fold over the same
// runEntry records handleRunsList builds, so the two can never
// disagree about a run's status. No limit/sort — stats are over
// ALL runs in the journal by definition (a "last N" window would
// make success-rate/percentiles meaningless).
//
// Duration percentiles (p50/p95) are computed over COMPLETED runs
// only — running/abandoned runs have no end time, so including
// them would either skew the distribution (treat now-start as the
// duration) or require a placeholder. Operators reading p95 want
// "how long do finished runs take", so completed-only is the
// honest denominator. The completed/abandoned/running split is
// reported separately so nothing is hidden.
//
// Optional time window (M33): args.since_ms restricts the stats to
// runs that STARTED within the last since_ms (server clock). 0 or
// absent = all-time (the original behaviour). A windowed view is
// "how have runs done in the last hour" — the failure/timeout/
// canceled terminal terms make that rate meaningful.
func (s *Server) handleRunsStats(conn net.Conn, req Request) {
	// Tenant-scoped (M39): empty tenant → primary journal; named tenant →
	// its own isolated journal.
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	runs, err := s.collectRuns(k)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	// Resolve the optional window. cutoff==0 means "no filter". We compute
	// the cutoff against the server's clock, which is the same clock that
	// stamped the events' TSUnixMS — so the comparison is apples-to-apples.
	sinceMS := int64Arg(req.Args["since_ms"])
	var cutoff int64
	if sinceMS > 0 {
		cutoff = time.Now().UnixMilli() - sinceMS
	}

	// Optional intent substring scope (M78): aggregate only runs whose intent
	// matches, so an operator can ask "how reliable are my deploy runs?".
	// Case-insensitive contains, mirroring `agt runs list --intent` (M77).
	intentArg, _, err := argString(req.Args, "intent")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	intentQuery := strings.ToLower(intentArg)

	var total, completed, failed, running, abandoned int
	var itersSum int
	durations := make([]int64, 0, len(runs)) // completed runs only, for percentiles
	// failedByReason buckets failures by their M30 reason tag (error /
	// max_iters / canceled / timeout), so an operator sees WHY runs fail,
	// not just how many (M36). A failure with no recorded reason buckets
	// under "unknown" rather than vanishing.
	failedByReason := map[string]int{}
	// Delegation metrics (M45): a sub-agent run carries the lead's
	// correlation in ParentCorrelation (M41). Folding those over the SAME
	// windowed set surfaces the SCALE of delegation — invisible until now:
	// how many sub-agents were spawned, how many leads delegated, and the
	// widest fan-out from a single lead. fanout maps a lead's correlation to
	// the number of sub-agents it spawned within the window.
	delegations := 0
	fanout := map[string]int{}
	// Spend attribution (M47): sum each run's folded budget.consumed cost.
	// spentTotal is the window's whole spend; spentDelegated is the share
	// attributable to sub-agent runs — so an operator sees not just how many
	// delegations happened (M45) but what they cost.
	var spentTotal, spentDelegated int64
	// Per-run spend distribution (M60): the microcents each priced run cost,
	// for an avg/p50/p95 breakdown mirroring the duration block — so an operator
	// sees not just total spend (M47) but how it's distributed (a few expensive
	// runs vs many cheap ones). Only runs that actually spent are included.
	spends := make([]int64, 0, len(runs))
	// Per-model attribution (M124): run count and spend grouped by the run's
	// folded model (M123), so an operator sees WHERE the money goes across a
	// multi-provider mix — "$X on opus over N runs, $Y on haiku". Runs with no
	// journaled model (free/local/mock — they didn't spend) are not attributed.
	modelRuns := map[string]int{}
	modelSpent := map[string]int64{}
	for _, r := range runs {
		// Windowed: keep only runs that started at/after the cutoff. A run
		// with no recorded start (the completed-without-received edge) can't
		// be placed on the timeline, so it's excluded from a windowed view.
		if cutoff > 0 && (r.StartedUnixMS == 0 || r.StartedUnixMS < cutoff) {
			continue
		}
		// Intent scope (M78), applied alongside the window.
		if intentQuery != "" && !strings.Contains(strings.ToLower(r.Intent), intentQuery) {
			continue
		}
		total++
		spentTotal += r.SpentMicrocents
		if r.SpentMicrocents > 0 {
			spends = append(spends, r.SpentMicrocents)
		}
		if r.Model != "" {
			modelRuns[r.Model]++
			modelSpent[r.Model] += r.SpentMicrocents
		}
		if r.ParentCorrelation != "" {
			delegations++
			fanout[r.ParentCorrelation]++
			spentDelegated += r.SpentMicrocents
		}
		switch {
		case r.Completed:
			completed++
			itersSum += r.Iters
			if r.StartedUnixMS > 0 && r.CompletedUnixMS >= r.StartedUnixMS {
				durations = append(durations, r.CompletedUnixMS-r.StartedUnixMS)
			}
		case r.Failed:
			failed++
			reason := r.FailReason
			if reason == "" {
				reason = "unknown"
			}
			failedByReason[reason]++
		case r.Abandoned:
			abandoned++
		default:
			running++
		}
	}

	// success_rate is completed / (completed + failed + abandoned): runs
	// still running are in-flight and shouldn't count against the rate
	// (they haven't failed — they just haven't finished), but failed and
	// abandoned runs are non-success terminal states and DO count against
	// it (M30 makes failures first-class here). When no run has reached a
	// terminal state yet the rate is undefined; we report 0 and the
	// renderer shows "n/a".
	terminal := completed + failed + abandoned
	successRate := 0.0
	if terminal > 0 {
		successRate = float64(completed) / float64(terminal)
	}

	// Duration aggregates over completed runs. avgIters is over
	// completed runs too (only they carry an iters count).
	dstats := durationStats(durations)
	// Spend distribution over priced runs (M60) — reuses the same nearest-rank
	// percentile helper as duration, in microcents.
	sstats := durationStats(spends)
	avgIters := 0.0
	if completed > 0 {
		avgIters = float64(itersSum) / float64(completed)
	}

	// Derive the delegation aggregates from the fanout map. delegatingRuns
	// is the number of distinct leads that delegated at least once;
	// maxFanout is the widest single lead's sub-agent count. Both are 0 when
	// nothing was delegated in the window — the renderer then omits the line.
	delegatingRuns := len(fanout)
	maxFanout := 0
	for _, n := range fanout {
		if n > maxFanout {
			maxFanout = n
		}
	}

	// Per-model breakdown (M124): {model → {runs, spent_microcents}}. Empty map
	// when no run carried a model (free/local/mock) — the CLI omits the block.
	byModel := make(map[string]any, len(modelRuns))
	for m, n := range modelRuns {
		byModel[m] = map[string]any{"runs": n, "spent_microcents": modelSpent[m]}
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":        total,
			"completed":    completed,
			"failed":       failed,
			"running":      running,
			"abandoned":    abandoned,
			"terminal":     terminal,
			"success_rate": successRate,
			"avg_iters":    avgIters,
			// Per-reason failure breakdown (M36): {error|max_iters|canceled|
			// timeout|unknown → count}. Empty map when there are no failures.
			"failed_by_reason": failedByReason,
			// 0 = all-time; >0 = the window width in ms the stats cover (M33).
			"window_ms": sinceMS,
			// Delegation scale (M45): total sub-agent runs spawned within the
			// window, the number of distinct leads that delegated, and the
			// widest fan-out from a single lead. All 0 when no delegation
			// occurred — the CLI omits the line in that case.
			"delegations":     delegations,
			"delegating_runs": delegatingRuns,
			"max_fanout":      maxFanout,
			// Spend over the window (M47), in microcents: the whole spend and
			// the share attributable to sub-agent runs. 0 when no priced usage
			// was journaled (e.g. a free/local model or the offline mock).
			"spent_microcents":           spentTotal,
			"delegated_spent_microcents": spentDelegated,
			// Per-model run-count + spend breakdown (M124). Empty when unpriced.
			"by_model": byModel,
			// Per-run spend distribution over priced runs (M60), in microcents.
			"spend_microcents": map[string]any{
				"count": len(spends),
				"avg":   sstats.avg,
				"min":   sstats.min,
				"max":   sstats.max,
				"p50":   sstats.p50,
				"p95":   sstats.p95,
			},
			"duration_ms": map[string]any{
				"count": len(durations),
				"avg":   dstats.avg,
				"min":   dstats.min,
				"max":   dstats.max,
				"p50":   dstats.p50,
				"p95":   dstats.p95,
			},
		},
	})
}

type durStats struct {
	avg, min, max, p50, p95 int64
}

// durationStats computes summary statistics over a slice of
// completed-run durations (milliseconds). Returns a zero-value
// durStats for an empty input so the caller doesn't special-case
// the no-completed-runs path. Percentiles use the nearest-rank
// method on a sorted copy (sort is in-place on a copy to avoid
// mutating the caller's slice ordering, which it doesn't rely on
// but a future caller might).
func durationStats(ms []int64) durStats {
	if len(ms) == 0 {
		return durStats{}
	}
	sorted := make([]int64, len(ms))
	copy(sorted, ms)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum int64
	for _, d := range sorted {
		sum += d
	}
	return durStats{
		avg: sum / int64(len(sorted)),
		min: sorted[0],
		max: sorted[len(sorted)-1],
		p50: percentileNearestRank(sorted, 50),
		p95: percentileNearestRank(sorted, 95),
	}
}

// percentileNearestRank returns the p-th percentile of an
// ascending-sorted slice using the nearest-rank method:
// rank = ceil(p/100 * N), 1-based, clamped to [1, N]. Chosen over
// linear interpolation because it always returns an actual
// observed duration (operators trust "p95 = 1200ms" more when
// 1200ms is a real run, not an interpolated phantom).
func percentileNearestRank(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	// ceil(p/100 * N) without floats: (p*N + 99) / 100.
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// extractIntent pulls "intent" out of a task.received payload.
// Returns "" if missing or malformed — operator-facing rendering
// gracefully shows "(no intent)" rather than crashing.
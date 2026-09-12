// SPDX-License-Identifier: MIT

// Control-plane run-listing handler (handleRunsList).
// Code extracted from runs_handlers.go during the Day-85 god-file split.
// Public API unchanged.
package controlplane



import (
	"net"
	"sort"
	"strings"

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

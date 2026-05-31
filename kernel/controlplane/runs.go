// SPDX-License-Identifier: MIT

package controlplane

// Past-runs enumeration. Walks the journal once, pairs
// task.received and task.completed events by correlation_id,
// and emits a sorted summary. Read-only; the journal is the
// single source of truth so no caching/snapshot is needed —
// operators always see the latest.

import (
	"encoding/json"
	"net"
	"sort"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

const (
	defaultRunsLimit = 20
	maxRunsLimit     = 1_000
)

type runEntry struct {
	CorrelationID   string
	Intent          string
	StartedUnixMS   int64
	// StartedSeq is the journal seq of the task.received event;
	// used as a tie-break for the sort when two runs share the
	// same TSUnixMS (the bus's wall-clock resolution is 1ms, so
	// fast back-to-back submissions collide).
	StartedSeq      int64
	CompletedUnixMS int64
	Iters           int
	Completed       bool
	// Failed is set when a task.failed event terminated this run — it
	// started but errored out (provider error, max iters, cancel/timeout)
	// instead of completing (M30). FailedUnixMS / FailReason carry the
	// terminal timestamp and the classified reason for rendering.
	Failed       bool
	FailedUnixMS int64
	FailReason   string
	// Abandoned is set when a task.abandoned event reconciled this run at
	// boot — it was received but never completed in a prior session (M28).
	// Status precedence is Completed > Failed > Abandoned > running, so a
	// run that somehow carries several terminal markers reports the most
	// authoritative one.
	Abandoned bool
}

// collectRuns walks the journal once and folds task.received /
// task.completed / task.abandoned events into per-correlation
// runEntry records. Shared by handleRunsList (which sorts +
// limits + renders) and handleRunsStats (which aggregates). The
// fold is identical in both so the two surfaces never disagree
// about a run's status.
func (s *Server) collectRuns() (map[string]*runEntry, error) {
	// Single forward walk: build per-correlation entry on
	// task.received, update on task.completed. We don't try to
	// stream early-stop after N — limit is applied post-sort, since
	// "last N runs" requires knowing all runs first (journal order
	// is by seq, not by run start time, and the same run's events
	// are interleaved with others under concurrency).
	runs := map[string]*runEntry{}
	err := s.k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindTaskReceived:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.StartedUnixMS = e.TSUnixMS
			entry.StartedSeq = e.Seq
			// Pull intent out of the payload — agent.go writes it as
			// {"intent": "..."} on KindTaskReceived (see kernel/agent).
			if intent := extractIntent(e.Payload); intent != "" {
				entry.Intent = intent
			}
		case event.KindTaskCompleted:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				// Completed without received? Only possible if the
				// journal was rotated mid-run; record the half we
				// have so the operator at least sees the chain id.
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.CompletedUnixMS = e.TSUnixMS
			entry.Completed = true
			if iters := extractIters(e.Payload); iters > 0 {
				entry.Iters = iters
			}
		case event.KindTaskFailed:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.Failed = true
			entry.FailedUnixMS = e.TSUnixMS
			entry.FailReason = extractReason(e.Payload)
		case event.KindTaskAbandoned:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.Abandoned = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

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

	runs, err := s.collectRuns()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Sort by StartedUnixMS DESC. Entries with zero start time (the
	// "completed without received" edge case) sort to the bottom.
	entries := make([]*runEntry, 0, len(runs))
	for _, r := range runs {
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
		out = append(out, map[string]any{
			"correlation_id":    r.CorrelationID,
			"intent":            r.Intent,
			"status":            status,
			"reason":            reason,
			"started_unix_ms":   r.StartedUnixMS,
			"completed_unix_ms": r.CompletedUnixMS,
			"duration_ms":       duration,
			"iters":             r.Iters,
		})
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"runs":  out,
			"count": len(out),
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
	runs, err := s.collectRuns()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Resolve the optional window. cutoff==0 means "no filter". We compute
	// the cutoff against the server's clock, which is the same clock that
	// stamped the events' TSUnixMS — so the comparison is apples-to-apples.
	sinceMS := int64(0)
	switch v := req.Args["since_ms"].(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}
	var cutoff int64
	if sinceMS > 0 {
		cutoff = time.Now().UnixMilli() - sinceMS
	}

	var total, completed, failed, running, abandoned int
	var itersSum int
	durations := make([]int64, 0, len(runs)) // completed runs only, for percentiles
	for _, r := range runs {
		// Windowed: keep only runs that started at/after the cutoff. A run
		// with no recorded start (the completed-without-received edge) can't
		// be placed on the timeline, so it's excluded from a windowed view.
		if cutoff > 0 && (r.StartedUnixMS == 0 || r.StartedUnixMS < cutoff) {
			continue
		}
		total++
		switch {
		case r.Completed:
			completed++
			itersSum += r.Iters
			if r.StartedUnixMS > 0 && r.CompletedUnixMS >= r.StartedUnixMS {
				durations = append(durations, r.CompletedUnixMS-r.StartedUnixMS)
			}
		case r.Failed:
			failed++
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
	avgIters := 0.0
	if completed > 0 {
		avgIters = float64(itersSum) / float64(completed)
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
			// 0 = all-time; >0 = the window width in ms the stats cover (M33).
			"window_ms": sinceMS,
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
func extractIntent(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Intent
}

// extractIters pulls "iters" out of a task.completed payload.
// Returns 0 on parse failure for the same reason as extractIntent.
func extractIters(payload json.RawMessage) int {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		// JSON numbers decode as float64 by default; accept either
		// shape so the field's wire type can evolve without breaking.
		Iters float64 `json:"iters"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int(p.Iters)
}

// extractReason pulls "reason" out of a task.failed payload (M30) —
// the classified failure tag (error|max_iters|canceled|timeout). Returns
// "" on parse failure or absence, so the renderer falls back gracefully.
func extractReason(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Reason
}

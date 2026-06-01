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
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
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
	// ParentCorrelation links a sub-agent run to the lead run that
	// delegated it (M41), derived from the parent's subagent.spawned event.
	// Empty for top-level runs. Lets `agt runs` show the delegation tree.
	ParentCorrelation string
	// SpentMicrocents is the sum of this run's budget.consumed cost (M47),
	// folded from the governor's per-call spend events now that they carry
	// the spending run's correlation. Lets `agt runs stats` cost a run — and,
	// via ParentCorrelation, cost a delegation.
	SpentMicrocents int64
	// AnswerPreview is a one-line excerpt of the run's final answer (M52),
	// folded from the M51 task.completed `answer` field. Lets `agt runs show`
	// show what a delegation RESULTED IN inline on its ↳ line, not just its
	// status/cost. Empty for a run that didn't complete with text.
	AnswerPreview string
}

// runEntryStatus reports a run's terminal status (M61), the single source of
// truth shared by handleRunsList, handleScheduleFires, and the status filter so
// they never disagree. Precedence: completed > failed > abandoned > running.
func runEntryStatus(r *runEntry) string {
	switch {
	case r.Completed:
		return "completed"
	case r.Failed:
		return "failed"
	case r.Abandoned:
		return "abandoned"
	default:
		return "running"
	}
}

// collectRuns walks the given kernel's journal once and folds
// task.received / task.completed / task.failed / task.abandoned
// events into per-correlation runEntry records. Shared by
// handleRunsList (which sorts + limits + renders) and
// handleRunsStats (which aggregates). The fold is identical in
// both so the two surfaces never disagree about a run's status.
// Takes an explicit kernel so the run views can be tenant-scoped
// (M39): the primary kernel for an empty tenant, else the tenant's
// own isolated journal — a tenant sees only its own runs.
func (s *Server) collectRuns(k *runtime.Kernel) (map[string]*runEntry, error) {
	// Single forward walk: build per-correlation entry on
	// task.received, update on task.completed. We don't try to
	// stream early-stop after N — limit is applied post-sort, since
	// "last N runs" requires knowing all runs first (journal order
	// is by seq, not by run start time, and the same run's events
	// are interleaved with others under concurrency).
	runs := map[string]*runEntry{}
	err := k.Journal().Range(func(e *event.Event) error {
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
			if prev := extractAnswerPreview(e.Payload); prev != "" {
				entry.AnswerPreview = prev
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
		case event.KindSubAgentSpawned:
			// The spawn event lives under the PARENT correlation; its payload
			// names the CHILD. We attach the parent link to the child's entry
			// (creating it if the spawn is seen before the child's
			// task.received), so a sub-agent run knows its lead (M41).
			child, parent := extractSpawnLink(e.Payload)
			if child != "" && parent != "" {
				entry, ok := runs[child]
				if !ok {
					entry = &runEntry{CorrelationID: child}
					runs[child] = entry
				}
				entry.ParentCorrelation = parent
			}
		case event.KindBudgetConsumed:
			// Attribute spend to its run (M47). The governor stamps each
			// budget.consumed with the spending run's correlation; fold its
			// cost into the EXISTING entry only — a budget event for an
			// unknown correlation (an out-of-run governor call) must not
			// conjure a phantom run that would then count as "running" in
			// stats. task.received always precedes a run's spend, so the
			// entry exists by the time we see its budget events.
			if e.CorrelationID == "" {
				return nil
			}
			if entry, ok := runs[e.CorrelationID]; ok {
				entry.SpentMicrocents += extractCostMicrocents(e.Payload)
			}
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

	// Optional status filter (M61): completed|failed|running|abandoned.
	statusFilter, _ := req.Args["status"].(string)
	// Optional intent substring filter (M77): case-insensitive contains, so an
	// operator can find "that deploy run" without scanning the whole list.
	intentQuery, _ := req.Args["intent"].(string)
	intentQuery = strings.ToLower(intentQuery)

	// Tenant-scoped (M39): an empty tenant reads the primary journal; a named
	// tenant reads its own isolated journal, so a tenant sees only its runs.
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	runs, err := s.collectRuns(k)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
			"correlation_id":     r.CorrelationID,
			"intent":             r.Intent,
			"status":             status,
			"reason":             reason,
			"started_unix_ms":    r.StartedUnixMS,
			"completed_unix_ms":  r.CompletedUnixMS,
			"duration_ms":        duration,
			"iters":              r.Iters,
			"parent_correlation": r.ParentCorrelation, // "" for top-level runs (M41)
			"spent_mc":           r.SpentMicrocents,    // this run's spend in microcents (M50; 0 = none/unpriced)
			"answer_preview":     r.AnswerPreview,      // one-line excerpt of the final answer (M52; "" if none)
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
	// Tenant-scoped (M39): empty tenant → primary journal; named tenant →
	// its own isolated journal.
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	runs, err := s.collectRuns(k)
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
	for _, r := range runs {
		// Windowed: keep only runs that started at/after the cutoff. A run
		// with no recorded start (the completed-without-received edge) can't
		// be placed on the timeline, so it's excluded from a windowed view.
		if cutoff > 0 && (r.StartedUnixMS == 0 || r.StartedUnixMS < cutoff) {
			continue
		}
		total++
		spentTotal += r.SpentMicrocents
		if r.SpentMicrocents > 0 {
			spends = append(spends, r.SpentMicrocents)
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

// extractCostMicrocents pulls cost_microcents out of a budget.consumed
// payload (M47). Returns 0 on parse failure or absence, so an unparseable
// spend event contributes nothing rather than crashing the fold. JSON
// numbers decode as float64; int64(...) truncates the fractional part the
// integer microcents never has.
func extractCostMicrocents(payload json.RawMessage) int64 {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		Cost float64 `json:"cost_microcents"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int64(p.Cost)
}

// answerPreviewRunes bounds the one-line answer excerpt folded into a run row
// (M52). Short enough for a single inline ↳ line; the full answer is a
// `agt runs show <child>` away.
const answerPreviewRunes = 80

// extractAnswerPreview pulls the M51 `answer` out of a task.completed payload
// and returns a one-line excerpt (M52): newlines/tabs collapsed to single
// spaces, trimmed, truncated to answerPreviewRunes with an ellipsis. Returns ""
// on parse failure or an empty/whitespace answer so the renderer simply omits it.
func extractAnswerPreview(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	one := strings.Join(strings.Fields(p.Answer), " ") // collapse all whitespace runs
	if one == "" {
		return ""
	}
	r := []rune(one)
	if len(r) > answerPreviewRunes {
		return string(r[:answerPreviewRunes]) + "…"
	}
	return one
}

// extractSpawnLink pulls child + parent correlation ids out of a
// subagent.spawned payload (M41). Returns ("","") on parse failure so the
// fold simply skips an unparseable delegation rather than crashing.
func extractSpawnLink(payload json.RawMessage) (child, parent string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.Child, p.Parent
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

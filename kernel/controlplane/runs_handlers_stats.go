// SPDX-License-Identifier: MIT

// Control-plane run-stats handler + duration helpers (handleRunsStats + durStats + percentileNearestRank).
// Code extracted from runs_handlers.go during the Day-85 god-file split.
// Public API unchanged.
package controlplane



import (
	"net"
	"sort"
	"strings"
	"time"
)

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
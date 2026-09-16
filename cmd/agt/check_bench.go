// SPDX-License-Identifier: MIT
//
// cmd/agt provider check bench machinery (the actual probe loop +
// latency stats + benchResult flattening). The render/output
// machinery (emit* + render* + checkRow) lives in
// check_render.go. Carved out of check.go during Day-211 god-file
// refactor (#39). Public API unchanged.
package main

import (
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/agezt/agezt/kernel/catalog"
)


type benchResult struct {
	entry      *catalog.Provider
	modelID    string
	iterations int
	successes  int
	failures   int
	latencies  []time.Duration // successful probes only — failures don't time meaningfully
	totalCost  int64
	lastErr    error
	stats      latencyStats // computed once via finalise
}

// latencyStats holds min/p50/p95/max for a successful-probe batch.
type latencyStats struct {
	min, p50, p95, max time.Duration
}

// runBench executes N probes against one provider. Sequential by
// design — concurrent probes would skew latencies for providers with
// per-key rate limits (Groq, Anthropic). N is small (typically 3–10);
// sequential is fine.
//
// When progress != nil and showProgress is true, prints one dot per
// completed probe so operators see progress on slow providers.
func runBench(entry *catalog.Provider, lookup func(string) string, n int, progress io.Writer, showProgress bool) benchResult {
	b := benchResult{entry: entry, iterations: n}
	if showProgress && progress != nil {
		fmt.Fprintf(progress, "  bench %s ×%d: ", entry.ID, n)
	}
	for range n {
		res := runProbe(entry, lookup)
		if b.modelID == "" {
			b.modelID = res.modelID
		}
		if res.err != nil {
			b.failures++
			b.lastErr = res.err
			if showProgress && progress != nil {
				fmt.Fprint(progress, "x")
			}
			continue
		}
		b.successes++
		b.latencies = append(b.latencies, res.latency)
		b.totalCost += res.costMicrocents
		if showProgress && progress != nil {
			fmt.Fprint(progress, ".")
		}
	}
	if showProgress && progress != nil {
		fmt.Fprintln(progress)
	}
	b.stats = computeLatencyStats(b.latencies)
	return b
}

// computeLatencyStats returns min/p50/p95/max for a slice of
// durations. Uses nearest-rank percentile (sort ascending, index
// ⌈p·n⌉) — exact median for odd n, lower-median for even, and
// matches `numpy.percentile(method="nearest")`. Sufficient for the
// small N (≤ ~30) operators will actually run.
func computeLatencyStats(lats []time.Duration) latencyStats {
	if len(lats) == 0 {
		return latencyStats{}
	}
	sorted := make([]time.Duration, len(lats))
	copy(sorted, lats)
	slices.Sort(sorted)
	pick := func(p float64) time.Duration {
		// nearest-rank: index = ⌈p·n⌉ − 1, clamped to [0, n-1]
		rank := int(float64(len(sorted))*p + 0.5)
		rank = max(rank, 1)
		rank = min(rank, len(sorted))
		return sorted[rank-1]
	}
	return latencyStats{
		min: sorted[0],
		p50: pick(0.50),
		p95: pick(0.95),
		max: sorted[len(sorted)-1],
	}
}

// toCheckRow projects a benchmark into a row the --all table can
// render. Uses p50 as the headline latency and total accumulated
// cost across all iterations.
func (b benchResult) toCheckRow() checkRow {
	r := checkRow{
		id:      b.entry.ID,
		family:  string(b.entry.Family()),
		model:   b.modelID,
		latency: b.stats.p50,
		cost:    b.totalCost,
		ok:      b.failures == 0,
	}
	if b.lastErr != nil {
		r.err = truncate(fmt.Sprintf("%d/%d failed: %v", b.failures, b.iterations, b.lastErr), 80)
	}
	return r
}

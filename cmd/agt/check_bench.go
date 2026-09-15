// SPDX-License-Identifier: MIT
//
// cmd/agt provider check bench + human-render machinery.
// Split from check.go during Day 211 god-file refactor (#39).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
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

// --- human renderers ----------------------------------------------------

func emitSingleHuman(entry *catalog.Provider, res probeResult, stdout, stderr io.Writer) {
	fmt.Fprintf(stdout, "checking provider=%s model=%s family=%s …\n",
		entry.ID, res.modelID, entry.Family())
	if res.err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", res.err)
		if res.latency > 0 {
			fmt.Fprintf(stderr, "(latency: %s — failure before/during HTTP)\n", res.latency.Truncate(time.Millisecond))
		}
		return
	}
	fmt.Fprintf(stdout, "\nOK\n")
	fmt.Fprintf(stdout, "  reply           : %q\n", truncate(strings.TrimSpace(res.reply), 80))
	fmt.Fprintf(stdout, "  latency         : %s\n", res.latency.Truncate(time.Millisecond))
	fmt.Fprintf(stdout, "  stop_reason     : %s\n", res.stopReason)
	fmt.Fprintf(stdout, "  tokens in / out : %d / %d\n", res.usage.InputTokens, res.usage.OutputTokens)
	if res.model != nil && res.model.Cost != nil {
		fmt.Fprintf(stdout, "  model pricing   : $%.2f in / $%.2f out per MTok\n",
			res.model.Cost.Input, res.model.Cost.Output)
	}
	if res.costMicrocents > 0 {
		fmt.Fprintf(stdout, "  this call cost  : $%s (%d microcents)\n",
			formatMicrocentsUSD(res.costMicrocents), res.costMicrocents)
	} else if res.model == nil || res.model.Cost == nil {
		fmt.Fprintf(stdout, "  this call cost  : (no pricing in catalog for this model)\n")
	}
}

func emitBenchHuman(b benchResult, stdout io.Writer) {
	fmt.Fprintf(stdout, "\nbench result for %s (model=%s)\n", b.entry.ID, b.modelID)
	fmt.Fprintf(stdout, "  iterations      : %d (%d ok, %d failed)\n", b.iterations, b.successes, b.failures)
	if b.successes > 0 {
		fmt.Fprintf(stdout, "  min / p50 / p95 / max : %s / %s / %s / %s\n",
			b.stats.min.Truncate(time.Millisecond),
			b.stats.p50.Truncate(time.Millisecond),
			b.stats.p95.Truncate(time.Millisecond),
			b.stats.max.Truncate(time.Millisecond))
	}
	if b.totalCost > 0 {
		fmt.Fprintf(stdout, "  total cost      : $%s (%d microcents across %d successful probes)\n",
			formatMicrocentsUSD(b.totalCost), b.totalCost, b.successes)
	}
	if b.lastErr != nil {
		fmt.Fprintf(stdout, "  last error      : %s\n", truncate(b.lastErr.Error(), 100))
	}
}

// checkRow is one provider's outcome in the --all summary table.
type checkRow struct {
	id      string
	family  string
	model   string
	latency time.Duration
	cost    int64
	ok      bool
	err     string
}

// renderCheckAllTable lays out one row per probed provider. Columns
// are sized to the widest cell so misaligned terminals still scan
// cleanly. Cost is rendered via formatMicrocentsUSD for parity with
// the single-provider view.
func renderCheckAllTable(rows []checkRow) string {
	headers := []string{"STATUS", "PROVIDER", "FAMILY", "MODEL", "LATENCY", "COST"}
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		status := "OK"
		if !r.ok {
			status = "FAIL"
		}
		lat := "-"
		if r.latency > 0 {
			lat = r.latency.Truncate(time.Millisecond).String()
		}
		cost := "-"
		if r.cost > 0 {
			cost = "$" + formatMicrocentsUSD(r.cost)
		} else if r.ok {
			cost = "(no price)"
		}
		cells = append(cells, []string{status, r.id, r.family, r.model, lat, cost})
	}
	return renderTable(headers, cells, rows)
}

// renderBenchAllTable lays out per-provider latency stats from a
// multi-probe benchmark. Each row shows MIN/P50/P95/MAX in addition
// to the basic identifying columns.
func renderBenchAllTable(benches []benchResult) string {
	headers := []string{"STATUS", "PROVIDER", "FAMILY", "MODEL", "MIN", "P50", "P95", "MAX", "OK/N", "TOTAL_COST"}
	cells := make([][]string, 0, len(benches))
	rows := make([]checkRow, 0, len(benches))
	for _, b := range benches {
		row := b.toCheckRow()
		rows = append(rows, row)

		status := "OK"
		if b.failures > 0 {
			status = "FAIL"
		}
		fmtLat := func(d time.Duration) string {
			if d == 0 {
				return "-"
			}
			return d.Truncate(time.Millisecond).String()
		}
		cost := "-"
		if b.totalCost > 0 {
			cost = "$" + formatMicrocentsUSD(b.totalCost)
		} else if b.successes > 0 {
			cost = "(no price)"
		}
		cells = append(cells, []string{
			status, b.entry.ID, string(b.entry.Family()), b.modelID,
			fmtLat(b.stats.min), fmtLat(b.stats.p50), fmtLat(b.stats.p95), fmtLat(b.stats.max),
			fmt.Sprintf("%d/%d", b.successes, b.iterations),
			cost,
		})
	}
	return renderTable(headers, cells, rows)
}

// renderTable is the shared column-aligned table renderer. `rows` is
// only used for the error-trailer lines below the table — separating
// it from `cells` lets the bench table reuse the layout without
// duplicating the alignment logic.
func renderTable(headers []string, cells [][]string, rows []checkRow) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, c := range cells {
		for i, v := range c {
			if len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}
	var b strings.Builder
	writeRow := func(vals []string) {
		for i, v := range vals {
			if i > 0 {
				b.WriteString("  ")
			}
			fmt.Fprintf(&b, "%-*s", widths[i], v)
		}
		b.WriteByte('\n')
	}
	writeRow(headers)
	for _, c := range cells {
		writeRow(c)
	}
	for _, r := range rows {
		if r.err != "" {
			fmt.Fprintf(&b, "  ! %s: %s\n", r.id, r.err)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- JSON output --------------------------------------------------------

// jsonProbe is the per-provider JSON record. Field names use
// snake_case to match conventions in the existing controlplane API
// surface. Optional fields omit when zero so a successful probe and
// a failed one don't carry each other's fields.

// SPDX-License-Identifier: MIT
package journalview

import "sort"

type DurationStats struct {
	Avg int64 `json:"avg"`
	Min int64 `json:"min"`
	Max int64 `json:"max"`
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
}

// SummarizeDurations computes summary statistics over a slice of
// completed-run durations (milliseconds). Returns a zero-value
// DurationStats for an empty input so the caller doesn't special-case
// the no-completed-runs path. Percentiles use the nearest-rank
// method on a sorted copy (sort is in-place on a copy to avoid
// mutating the caller's slice ordering, which it doesn't rely on
// but a future caller might).
func SummarizeDurations(ms []int64) DurationStats {
	if len(ms) == 0 {
		return DurationStats{}
	}
	sorted := make([]int64, len(ms))
	copy(sorted, ms)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum int64
	for _, d := range sorted {
		sum += d
	}
	return DurationStats{
		Avg: sum / int64(len(sorted)),
		Min: sorted[0],
		Max: sorted[len(sorted)-1],
		P50: PercentileNearestRank(sorted, 50),
		P95: PercentileNearestRank(sorted, 95),
	}
}

// PercentileNearestRank returns the p-th percentile of an
// ascending-sorted slice using the nearest-rank method:
// rank = ceil(p/100 * N), 1-based, clamped to [1, N]. Chosen over
// linear interpolation because it always returns an actual
// observed duration (operators trust "p95 = 1200ms" more when
// 1200ms is a real run, not an interpolated phantom).
func PercentileNearestRank(sorted []int64, p int) int64 {
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

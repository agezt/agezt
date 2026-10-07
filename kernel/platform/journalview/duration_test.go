// SPDX-License-Identifier: MIT

package journalview

import "testing"

// White-box tests for the pure stats helpers. Kept in-package so the
// exact percentile/aggregate math is pinned without routing through a
// live journal (whose durations we can't control to the millisecond).

func TestDurationStats_Empty(t *testing.T) {
	got := SummarizeDurations(nil)
	if got != (DurationStats{}) {
		t.Errorf("SummarizeDurations(nil) = %+v want zero value", got)
	}
}

func TestDurationStats_KnownDistribution(t *testing.T) {
	// 100,200,...,1000 — ten evenly-spaced durations.
	in := []int64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	got := SummarizeDurations(in)

	if got.Min != 100 {
		t.Errorf("min = %d want 100", got.Min)
	}
	if got.Max != 1000 {
		t.Errorf("max = %d want 1000", got.Max)
	}
	if got.Avg != 550 {
		t.Errorf("avg = %d want 550", got.Avg)
	}
	// nearest-rank p50: rank=ceil(0.5*10)=5 → sorted[4]=500.
	if got.P50 != 500 {
		t.Errorf("p50 = %d want 500", got.P50)
	}
	// nearest-rank p95: rank=ceil(0.95*10)=ceil(9.5)=10 → sorted[9]=1000.
	if got.P95 != 1000 {
		t.Errorf("p95 = %d want 1000", got.P95)
	}
}

func TestDurationStats_DoesNotMutateInput(t *testing.T) {
	in := []int64{300, 100, 200}
	_ = SummarizeDurations(in)
	// Input order must be preserved — the sort is on a copy.
	if in[0] != 300 || in[1] != 100 || in[2] != 200 {
		t.Errorf("input mutated: %v", in)
	}
}

func TestPercentileNearestRank(t *testing.T) {
	sorted := []int64{10, 20, 30, 40, 50}
	cases := []struct {
		p    int
		want int64
	}{
		{0, 10},   // clamps rank to 1
		{1, 10},   // ceil(0.05)=1
		{50, 30},  // ceil(2.5)=3 → sorted[2]=30
		{95, 50},  // ceil(4.75)=5 → sorted[4]=50
		{100, 50}, // rank=5 → sorted[4]=50
	}
	for _, c := range cases {
		if got := PercentileNearestRank(sorted, c.p); got != c.want {
			t.Errorf("PercentileNearestRank(p=%d) = %d want %d", c.p, got, c.want)
		}
	}
}

func TestPercentileNearestRank_Empty(t *testing.T) {
	if got := PercentileNearestRank(nil, 95); got != 0 {
		t.Errorf("PercentileNearestRank(nil) = %d want 0", got)
	}
}

func TestPercentileNearestRank_Single(t *testing.T) {
	one := []int64{42}
	for _, p := range []int{0, 50, 95, 100} {
		if got := PercentileNearestRank(one, p); got != 42 {
			t.Errorf("PercentileNearestRank([42], p=%d) = %d want 42", p, got)
		}
	}
}

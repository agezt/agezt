// SPDX-License-Identifier: MIT

package controlplane

import "testing"

// White-box tests for the pure stats helpers. Kept in-package so the
// exact percentile/aggregate math is pinned without routing through a
// live journal (whose durations we can't control to the millisecond).

func TestDurationStats_Empty(t *testing.T) {
	got := durationStats(nil)
	if got != (durStats{}) {
		t.Errorf("durationStats(nil) = %+v want zero value", got)
	}
}

func TestDurationStats_KnownDistribution(t *testing.T) {
	// 100,200,...,1000 — ten evenly-spaced durations.
	in := []int64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	got := durationStats(in)

	if got.min != 100 {
		t.Errorf("min = %d want 100", got.min)
	}
	if got.max != 1000 {
		t.Errorf("max = %d want 1000", got.max)
	}
	if got.avg != 550 {
		t.Errorf("avg = %d want 550", got.avg)
	}
	// nearest-rank p50: rank=ceil(0.5*10)=5 → sorted[4]=500.
	if got.p50 != 500 {
		t.Errorf("p50 = %d want 500", got.p50)
	}
	// nearest-rank p95: rank=ceil(0.95*10)=ceil(9.5)=10 → sorted[9]=1000.
	if got.p95 != 1000 {
		t.Errorf("p95 = %d want 1000", got.p95)
	}
}

func TestDurationStats_DoesNotMutateInput(t *testing.T) {
	in := []int64{300, 100, 200}
	_ = durationStats(in)
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
		if got := percentileNearestRank(sorted, c.p); got != c.want {
			t.Errorf("percentileNearestRank(p=%d) = %d want %d", c.p, got, c.want)
		}
	}
}

func TestPercentileNearestRank_Empty(t *testing.T) {
	if got := percentileNearestRank(nil, 95); got != 0 {
		t.Errorf("percentileNearestRank(nil) = %d want 0", got)
	}
}

func TestPercentileNearestRank_Single(t *testing.T) {
	one := []int64{42}
	for _, p := range []int{0, 50, 95, 100} {
		if got := percentileNearestRank(one, p); got != 42 {
			t.Errorf("percentileNearestRank([42], p=%d) = %d want 42", p, got)
		}
	}
}

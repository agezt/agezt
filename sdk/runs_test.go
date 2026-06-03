// SPDX-License-Identifier: MIT

package sdk

import (
	"testing"
	"time"
)

func TestParseRuns(t *testing.T) {
	res := map[string]any{"runs": []any{
		map[string]any{
			"correlation_id":     "c1",
			"intent":             "do a thing",
			"status":             "completed",
			"started_unix_ms":    float64(1_700_000_000_000),
			"duration_ms":        float64(2500),
			"iters":              float64(4),
			"spent_mc":           float64(2.5e8), // 0.25 USD
			"model":              "m1",
			"parent_correlation": "lead-1",
		},
		map[string]any{
			"correlation_id": "c2",
			"intent":         "failed thing",
			"status":         "failed",
			"reason":         "timeout",
		},
	}}

	runs := parseRuns(res)
	if len(runs) != 2 {
		t.Fatalf("want 2 runs, got %d", len(runs))
	}

	a := runs[0]
	if a.CorrelationID != "c1" || a.Intent != "do a thing" || a.Status != "completed" || a.Model != "m1" {
		t.Errorf("run[0] strings wrong: %+v", a)
	}
	if a.ParentCorrelation != "lead-1" {
		t.Errorf("parent = %q", a.ParentCorrelation)
	}
	if a.Iterations != 4 {
		t.Errorf("iters = %d", a.Iterations)
	}
	if a.CostUSD != 0.25 {
		t.Errorf("cost = %v, want 0.25", a.CostUSD)
	}
	if !a.Started.Equal(time.UnixMilli(1_700_000_000_000)) {
		t.Errorf("started = %v", a.Started)
	}
	if a.Duration != 2500*time.Millisecond {
		t.Errorf("duration = %v, want 2.5s", a.Duration)
	}

	b := runs[1]
	if b.Status != "failed" || b.Reason != "timeout" {
		t.Errorf("run[1] failure fields wrong: %+v", b)
	}
	// Missing time fields are zero values.
	if !b.Started.IsZero() || b.Duration != 0 {
		t.Errorf("run[1] missing times should be zero: %+v", b)
	}
}

func TestParseRuns_Empty(t *testing.T) {
	if got := parseRuns(map[string]any{}); len(got) != 0 {
		t.Errorf("no runs key → empty slice, got %v", got)
	}
	if got := parseRuns(map[string]any{"runs": []any{}}); len(got) != 0 {
		t.Errorf("empty runs → empty slice, got %v", got)
	}
}

// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"testing"
)

// statSeq returns a HealthStatFunc that yields the given snapshots in order,
// repeating the last one once exhausted.
func statSeq(snaps ...HealthStat) HealthStatFunc {
	i := 0
	return func(context.Context) (HealthStat, error) {
		s := snaps[i]
		if i < len(snaps)-1 {
			i++
		}
		return s, nil
	}
}

func poll(t *testing.T, o *HealthObserver) []Delta {
	t.Helper()
	d, err := o.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll error: %v", err)
	}
	return d
}

func TestHealth_BaselineFirstPollSilent(t *testing.T) {
	// Even an unhealthy first poll establishes a baseline and emits nothing —
	// there's no transition to report yet.
	o := NewHealthObserver(statSeq(HealthStat{ToolCalls: 10, ToolErrors: 9}), 0, 0)
	if d := poll(t, o); len(d) != 0 {
		t.Fatalf("first poll should be silent, got %v", d)
	}
}

func TestHealth_EmitsOnDegradeTransition(t *testing.T) {
	o := NewHealthObserver(statSeq(
		HealthStat{ToolCalls: 10, ToolErrors: 0},  // healthy baseline
		HealthStat{ToolCalls: 10, ToolErrors: 4},  // 40% → degraded
	), 0, 0)
	poll(t, o) // baseline
	d := poll(t, o)
	if len(d) != 1 {
		t.Fatalf("want 1 delta on degrade, got %d", len(d))
	}
	if d[0].Kind != "health_degraded" {
		t.Errorf("kind = %q, want health_degraded", d[0].Kind)
	}
	if d[0].After != "degraded" || d[0].Before != "healthy" {
		t.Errorf("transition = %s→%s, want healthy→degraded", d[0].Before, d[0].After)
	}
	if d[0].Hints["severity"] != string(SevHigh) {
		t.Errorf("severity = %q, want high", d[0].Hints["severity"])
	}
}

func TestHealth_CriticalSeverityOnSevereErrors(t *testing.T) {
	o := NewHealthObserver(statSeq(
		HealthStat{ToolCalls: 10, ToolErrors: 0}, // healthy
		HealthStat{ToolCalls: 10, ToolErrors: 7}, // 70% → critical
	), 0, 0)
	poll(t, o)
	d := poll(t, o)
	if len(d) != 1 || d[0].After != "critical" {
		t.Fatalf("want critical transition, got %v", d)
	}
	if d[0].Hints["severity"] != string(SevCritical) {
		t.Errorf("severity = %q, want critical", d[0].Hints["severity"])
	}
}

func TestHealth_RunFailureAxis(t *testing.T) {
	// No tool errors at all, but half the runs failed → degraded/critical from
	// the run axis alone.
	o := NewHealthObserver(statSeq(
		HealthStat{Runs: 10, FailedRuns: 0},
		HealthStat{Runs: 10, FailedRuns: 5}, // 50% → critical
	), 0, 0)
	poll(t, o)
	d := poll(t, o)
	if len(d) != 1 || d[0].After != "critical" {
		t.Fatalf("run-failure axis should drive critical, got %v", d)
	}
}

func TestHealth_RecoveryEmitsMediumSilentNoFlap(t *testing.T) {
	o := NewHealthObserver(statSeq(
		HealthStat{ToolCalls: 10, ToolErrors: 0}, // healthy baseline
		HealthStat{ToolCalls: 10, ToolErrors: 5}, // degraded
		HealthStat{ToolCalls: 10, ToolErrors: 5}, // still degraded → silent
		HealthStat{ToolCalls: 10, ToolErrors: 0}, // recovered
	), 0, 0)
	poll(t, o) // baseline
	if d := poll(t, o); len(d) != 1 || d[0].Kind != "health_degraded" {
		t.Fatalf("want degrade, got %v", d)
	}
	if d := poll(t, o); len(d) != 0 {
		t.Fatalf("unchanged level must be silent, got %v", d)
	}
	d := poll(t, o)
	if len(d) != 1 || d[0].Kind != "health_recovered" {
		t.Fatalf("want recovery, got %v", d)
	}
	if d[0].Hints["severity"] != string(SevMedium) {
		t.Errorf("recovery severity = %q, want medium", d[0].Hints["severity"])
	}
}

func TestHealth_ThinSampleCannotAlert(t *testing.T) {
	// 2/2 tool errors is 100% but below minSample — must not alert.
	o := NewHealthObserver(statSeq(
		HealthStat{ToolCalls: 10, ToolErrors: 0}, // healthy baseline (enough sample)
		HealthStat{ToolCalls: 2, ToolErrors: 2},  // 100% but thin → stays healthy
	), 0, 0)
	poll(t, o)
	if d := poll(t, o); len(d) != 0 {
		t.Fatalf("thin sample must not alert, got %v", d)
	}
}

func TestHealth_NilStatFuncIsSafe(t *testing.T) {
	o := NewHealthObserver(nil, 0, 0)
	if d := poll(t, o); d != nil {
		t.Fatalf("nil stat func should yield no deltas, got %v", d)
	}
	if o.Name() != "self:health" {
		t.Errorf("name = %q", o.Name())
	}
}

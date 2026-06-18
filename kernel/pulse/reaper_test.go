// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"strings"
	"testing"
)

func TestReaperObserver_FiresOnGrowthOnly(t *testing.T) {
	var dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) {
		return dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art
	})
	ctx := context.Background()

	// Baseline beat establishes state silently.
	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}

	// The pile grows → one low-severity delta naming the counts.
	dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art = 2, 1, 1, 1, 1, 1, 1, 1, 1, 5
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 dead agent") ||
		!strings.Contains(d[0].Summary, "1 degraded agent") ||
		!strings.Contains(d[0].Summary, "1 misconfigured agent") ||
		!strings.Contains(d[0].Summary, "1 retry-pressure agent") ||
		!strings.Contains(d[0].Summary, "1 routing-pressure agent") ||
		!strings.Contains(d[0].Summary, "1 forced-chain probation agent") ||
		!strings.Contains(d[0].Summary, "1 forced-chain-failed agent") ||
		!strings.Contains(d[0].Summary, "1 forced-chain-exhausted agent") ||
		!strings.Contains(d[0].Summary, "1 unstable-routing agent") ||
		!strings.Contains(d[0].Summary, "5 stale artifact") {
		t.Errorf("summary = %q", d[0].Summary)
	}
	if d[0].Hints["degraded_agents"] != "1" {
		t.Errorf("degraded hint = %q, want 1", d[0].Hints["degraded_agents"])
	}
	if d[0].Hints["misconfigured_agents"] != "1" {
		t.Errorf("misconfigured hint = %q, want 1", d[0].Hints["misconfigured_agents"])
	}
	if d[0].Hints["retry_pressure_agents"] != "1" {
		t.Errorf("retry hint = %q, want 1", d[0].Hints["retry_pressure_agents"])
	}
	if d[0].Hints["routing_pressure_agents"] != "1" {
		t.Errorf("routing hint = %q, want 1", d[0].Hints["routing_pressure_agents"])
	}
	if d[0].Hints["routing_forced_probation_agents"] != "1" {
		t.Errorf("forced hint = %q, want 1", d[0].Hints["routing_forced_probation_agents"])
	}
	if d[0].Hints["routing_forced_failed_agents"] != "1" {
		t.Errorf("forced-failed hint = %q, want 1", d[0].Hints["routing_forced_failed_agents"])
	}
	if d[0].Hints["routing_forced_exhausted_agents"] != "1" {
		t.Errorf("forced-exhausted hint = %q, want 1", d[0].Hints["routing_forced_exhausted_agents"])
	}
	if d[0].Hints["routing_unstable_agents"] != "1" {
		t.Errorf("unstable hint = %q, want 1", d[0].Hints["routing_unstable_agents"])
	}
	if d[0].Hints["severity"] != string(SevLow) {
		t.Errorf("severity = %q, want low", d[0].Hints["severity"])
	}

	// Stable → silent (no repeat spam).
	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Errorf("stable counts should be silent, got %v", d)
	}

	// Shrink (operator cleaned up) → silent.
	dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art = 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Errorf("shrinking counts should be silent, got %v", d)
	}
}

func TestReaperObserver_FiresWhenDegradedGrows(t *testing.T) {
	var degraded int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) { return 0, degraded, 0, 0, 0, 0, 0, 0, 0, 0 })
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	degraded = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("degraded growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 degraded agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_FiresWhenMisconfiguredGrows(t *testing.T) {
	var misconf int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) { return 0, 0, misconf, 0, 0, 0, 0, 0, 0, 0 })
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	misconf = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("misconfigured growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 misconfigured agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_FiresWhenRetryPressureGrows(t *testing.T) {
	var retryPressure int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) {
		return 0, 0, 0, retryPressure, 0, 0, 0, 0, 0, 0
	})
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	retryPressure = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("retry-pressure growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 retry-pressure agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_FiresWhenRoutingPressureGrows(t *testing.T) {
	var routing int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) { return 0, 0, 0, 0, routing, 0, 0, 0, 0, 0 })
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	routing = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("routing growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 routing-pressure agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_FiresWhenForcedProbationGrows(t *testing.T) {
	var forced int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) { return 0, 0, 0, 0, 0, forced, 0, 0, 0, 0 })
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	forced = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("forced probation growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 forced-chain probation agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_FiresWhenForcedFailedGrows(t *testing.T) {
	var forcedFailed int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) {
		return 0, 0, 0, 0, 0, 0, forcedFailed, 0, 0, 0
	})
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	forcedFailed = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("forced-failed growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 forced-chain-failed agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_FiresWhenRoutingUnstableGrows(t *testing.T) {
	var unstable int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) { return 0, 0, 0, 0, 0, 0, 0, 0, unstable, 0 })
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	unstable = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("unstable growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 unstable-routing agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_FiresWhenForcedExhaustedGrows(t *testing.T) {
	var forcedExhausted int
	o := NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) {
		return 0, 0, 0, 0, 0, 0, 0, forcedExhausted, 0, 0
	})
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}
	forcedExhausted = 2
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("forced-exhausted growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 forced-chain-exhausted agent") {
		t.Errorf("summary = %q", d[0].Summary)
	}
}

func TestReaperObserver_NilScanNoOp(t *testing.T) {
	o := NewReaperObserver(nil)
	if d, err := o.Poll(context.Background()); d != nil || err != nil {
		t.Fatalf("nil scan should no-op, got %v / %v", d, err)
	}
}

// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"strings"
	"testing"
)

func TestReaperObserver_FiresOnGrowthOnly(t *testing.T) {
	var dead, art int
	o := NewReaperObserver(func() (int, int) { return dead, art })
	ctx := context.Background()

	// Baseline beat establishes state silently.
	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatalf("baseline beat should be silent, got %v", d)
	}

	// The pile grows → one low-severity delta naming the counts.
	dead, art = 2, 5
	d, err := o.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("growth should fire exactly one delta, got %d", len(d))
	}
	if !strings.Contains(d[0].Summary, "2 dead agent") || !strings.Contains(d[0].Summary, "5 stale artifact") {
		t.Errorf("summary = %q", d[0].Summary)
	}
	if d[0].Hints["severity"] != string(SevLow) {
		t.Errorf("severity = %q, want low", d[0].Hints["severity"])
	}

	// Stable → silent (no repeat spam).
	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Errorf("stable counts should be silent, got %v", d)
	}

	// Shrink (operator cleaned up) → silent.
	dead, art = 0, 0
	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Errorf("shrinking counts should be silent, got %v", d)
	}
}

func TestReaperObserver_NilScanNoOp(t *testing.T) {
	o := NewReaperObserver(nil)
	if d, err := o.Poll(context.Background()); d != nil || err != nil {
		t.Fatalf("nil scan should no-op, got %v / %v", d, err)
	}
}

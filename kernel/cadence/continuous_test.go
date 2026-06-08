// SPDX-License-Identifier: MIT

package cadence

import (
	"testing"
	"time"
)

// TestContinuous_CompletionAnchoredLoop proves a continuous entry fires
// immediately, stays due until its run completes (no advance in Due), then
// re-anchors `cooldown` after completion — a non-overlapping perpetual loop.
func TestContinuous_CompletionAnchoredLoop(t *testing.T) {
	s := mustStore(t)
	now := time.Unix(1_000_000, 0)

	e, err := s.AddContinuous("watch the world", 30*time.Second, SourceOperator, "", now)
	if err != nil {
		t.Fatalf("AddContinuous: %v", err)
	}
	if e.Mode != ModeContinuous || e.IntervalSec != 30 {
		t.Fatalf("entry shape wrong: %+v", e)
	}

	// Due immediately (NextRun == now), and Due must NOT advance it (stays due
	// until the run completes — the engine's in-flight guard prevents overlap).
	if due := s.Due(now); len(due) != 1 {
		t.Fatalf("want 1 due at creation, got %d", len(due))
	}
	if again := s.Due(now); len(again) != 1 {
		t.Fatal("continuous entry must stay due until CompleteFiring re-anchors it")
	}

	// The run completes 5s later → next cycle is anchored at completion+cooldown.
	complete := now.Add(5 * time.Second)
	if removed, err := s.CompleteFiring(e.ID, complete); err != nil || removed {
		t.Fatalf("CompleteFiring: removed=%v err=%v (continuous must NOT be removed)", removed, err)
	}
	got, _ := s.Get(e.ID)
	wantNext := complete.Add(30 * time.Second).Unix()
	if got.NextRunUnix != wantNext {
		t.Errorf("next run = %d, want completion+cooldown = %d", got.NextRunUnix, wantNext)
	}

	// Not due during the cooldown; due again once it elapses → the loop continues.
	if len(s.Due(complete.Add(10*time.Second))) != 0 {
		t.Error("must not be due during the cooldown")
	}
	if len(s.Due(complete.Add(31*time.Second))) != 1 {
		t.Error("must be due again after the cooldown — the loop never tires")
	}
}

// TestContinuous_CooldownFloored: a sub-second cooldown is clamped to MinInterval
// so a continuous agent can't busy-loop the daemon.
func TestContinuous_CooldownFloored(t *testing.T) {
	s := mustStore(t)
	e, err := s.AddContinuous("tight", 100*time.Millisecond, SourceOperator, "", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("AddContinuous: %v", err)
	}
	if e.IntervalSec != int64(MinInterval/time.Second) {
		t.Errorf("cooldown not floored to MinInterval: %ds", e.IntervalSec)
	}
}

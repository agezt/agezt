// SPDX-License-Identifier: MIT

package governor

import (
	"testing"
	"time"
)

func TestBreaker_TripsAfterThreshold(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	b := newBreaker(3, 30*time.Second, clock)

	if !b.allow("p") {
		t.Fatal("fresh breaker should allow")
	}
	b.failure("p")
	b.failure("p")
	if !b.allow("p") {
		t.Fatal("should still allow below threshold")
	}
	if b.state("p") != "closed" {
		t.Fatalf("state = %s, want closed", b.state("p"))
	}
	b.failure("p") // third → trips
	if b.allow("p") {
		t.Fatal("should be open after threshold failures")
	}
	if b.state("p") != "open" {
		t.Fatalf("state = %s, want open", b.state("p"))
	}
}

func TestBreaker_HalfOpenThenClose(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	b := newBreaker(1, 30*time.Second, clock)

	b.failure("p") // trips immediately (threshold 1)
	if b.allow("p") {
		t.Fatal("should be open")
	}
	// Advance past cooldown → half-open, allow one probe.
	now = now.Add(31 * time.Second)
	if !b.allow("p") {
		t.Fatal("should allow probe after cooldown")
	}
	if b.state("p") != "half-open" {
		t.Fatalf("state = %s, want half-open", b.state("p"))
	}
	b.success("p") // probe succeeds → closed
	if b.state("p") != "closed" {
		t.Fatalf("state = %s, want closed", b.state("p"))
	}
	if !b.allow("p") {
		t.Fatal("closed breaker should allow")
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	b := newBreaker(1, 30*time.Second, clock)

	b.failure("p")
	now = now.Add(31 * time.Second) // half-open
	if !b.allow("p") {
		t.Fatal("probe should be allowed")
	}
	b.failure("p") // probe fails → reopen
	if b.allow("p") {
		t.Fatal("should be open again after probe failure")
	}
}

func TestBreaker_TransitionSignals(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	b := newBreaker(2, 30*time.Second, clock)

	if b.failure("p") {
		t.Fatal("first failure (below threshold) should not report a trip")
	}
	if !b.failure("p") {
		t.Fatal("second failure (hits threshold) should report a trip")
	}
	if b.failure("p") {
		t.Fatal("a failure while already open should not re-report a trip")
	}
	// Recover.
	now = now.Add(31 * time.Second) // half-open
	if !b.success("p") {
		t.Fatal("success after open should report recovery")
	}
	if b.success("p") {
		t.Fatal("success on an already-closed breaker should not report recovery")
	}
}

func TestBreaker_DisabledAllowsEverything(t *testing.T) {
	b := newBreaker(0, 0, nil) // threshold 0 → disabled
	for i := 0; i < 100; i++ {
		b.failure("p")
	}
	if !b.allow("p") {
		t.Fatal("disabled breaker must always allow")
	}
	if b.state("p") != "disabled" {
		t.Fatalf("state = %s, want disabled", b.state("p"))
	}
}

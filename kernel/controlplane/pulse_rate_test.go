// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPulse_ReplayRate_PacesEvents verifies that --replay-rate
// actually slows replay. Setting rate=10eps with several events
// should take a measurable fraction of a second; without the
// rate cap the same replay finishes in microseconds.
func TestPulse_ReplayRate_PacesEvents(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(
		mock.FinalText("a"),
		mock.FinalText("b"),
	))
	if _, _, err := k.Run(context.Background(), "x"); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if _, _, err := k.Run(context.Background(), "y"); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	head, _ := k.Journal().Head()
	if head < 4 {
		t.Skipf("not enough events to test pacing (head=%d)", head)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 20 events/sec → ≥50ms between events; with at least 4 events
	// minimum elapsed should be ≥150ms (3 inter-event gaps).
	var (
		mu    sync.Mutex
		count int
	)
	start := time.Now()
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern":     ">",
				"since":       0,
				"until":       head,
				"replay_rate": 20.0, // events/sec
			},
			func(e *event.Event) {
				if e.IsEphemeral() {
					return
				}
				mu.Lock()
				count++
				mu.Unlock()
			})
	}()
	select {
	case <-errCh:
	case <-time.After(4 * time.Second):
		cancel()
		<-errCh
		t.Fatal("rate-limited replay didn't terminate (probably too slow or hung)")
	}
	elapsed := time.Since(start)

	mu.Lock()
	defer mu.Unlock()
	if count < 4 {
		t.Skipf("got only %d events; not enough to assert pacing", count)
	}
	// With 4 events at 20 eps the floor is ~3*50ms = 150ms. Allow a
	// little slack for scheduler jitter; the unrated version finishes
	// in <10ms, so any positive result here is meaningful.
	minExpected := 100 * time.Millisecond
	if elapsed < minExpected {
		t.Errorf("replay finished in %v with rate=20eps over %d events — expected >=%v",
			elapsed, count, minExpected)
	}
}

// TestPulse_ReplayRate_ZeroIsUnlimited verifies that omitting (or
// passing zero) replay_rate doesn't introduce any artificial
// delay — preserves existing v2/v3 behaviour.
func TestPulse_ReplayRate_ZeroIsUnlimited(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("a")))
	if _, _, err := k.Run(context.Background(), "x"); err != nil {
		t.Fatalf("run: %v", err)
	}
	head, _ := k.Journal().Head()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern": ">",
				"since":   0,
				"until":   head,
				// no replay_rate
			},
			func(e *event.Event) {})
	}()
	select {
	case <-errCh:
	case <-time.After(1500 * time.Millisecond):
		cancel()
		<-errCh
		t.Fatal("untimed replay didn't terminate")
	}
	elapsed := time.Since(start)
	// Should finish in well under 100ms for a handful of events.
	if elapsed > 500*time.Millisecond {
		t.Errorf("untimed replay took %v — expected <500ms (rate cap leaked?)", elapsed)
	}
}

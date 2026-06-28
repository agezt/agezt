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

// TestPulse_ReplayRate_AbortsOnContextCancel is the regression guard
// for BUG ctx-unaware-sleep (audit M1.nn fix): a rate-limited replay
// MUST abort promptly when its context is cancelled, even if the next
// rate-limit sleep has not elapsed. Before the fix, the replay used a
// bare `time.Sleep(minInterval - elapsed)` which ignored ctx.Done();
// for a low replayRateEPS a daemon shutdown could be delayed by up to
// `minInterval` per event.
func TestPulse_ReplayRate_AbortsOnContextCancel(t *testing.T) {
	// 8 scripted responses — each kernel.Run below will get a
	// fresh response. We don't care what the responses are; we
	// only need enough journal events for a 1eps replay to have
	// at least one rate-limit sleep in flight when we cancel.
	k, _, c, _ := startPair(t, mock.New(
		mock.FinalText("a"), mock.FinalText("b"),
		mock.FinalText("c"), mock.FinalText("d"),
		mock.FinalText("e"), mock.FinalText("f"),
		mock.FinalText("g"), mock.FinalText("h"),
	))
	// Stuff the journal with enough events that a low replay rate
	// guarantees a sleep is in flight when we cancel.
	for i := 0; i < 4; i++ {
		if _, _, err := k.Run(context.Background(), "warm-up"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	head, _ := k.Journal().Head()
	if head < 8 {
		t.Skipf("not enough events to test pacing (head=%d)", head)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 1 event/sec → minInterval = 1s. We cancel halfway through the
	// first inter-event sleep, so a buggy implementation would take
	// ~1s to return; a correct one returns in well under 200ms.
	var (
		mu    sync.Mutex
		count int
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern":     ">",
				"since":       0,
				"until":       head,
				"replay_rate": 1.0, // events/sec → 1s between events
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

	// Give the subscription a moment to register, then cancel
	// mid-replay.
	time.Sleep(100 * time.Millisecond)
	cancelStart := time.Now()
	cancel()

	select {
	case <-errCh:
		elapsed := time.Since(cancelStart)
		// We slept ~100ms before cancel; the buggy code would then
		// sleep another ~900ms before waking. A correct ctx-aware
		// sleep wakes within ~50ms of cancel.
		if elapsed > 500*time.Millisecond {
			t.Errorf("replay honoured %v after ctx cancel — expected <500ms (ctx-unaware sleep leaked)", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rate-limited replay did not abort within 2s of ctx cancel")
	}
}

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

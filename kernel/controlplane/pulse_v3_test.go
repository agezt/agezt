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

// TestPulse_SinceTSMs_ReplaysOnlyRecent verifies the timestamp
// cutoff: events older than the cutoff are skipped during replay,
// newer events arrive. Sleeps just long enough between the two
// runs to guarantee monotonic TSUnixMS distinguishes them.
func TestPulse_SinceTSMs_ReplaysOnlyRecent(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(
		mock.FinalText("old"),
		mock.FinalText("new"),
	))

	// First run — these events should be EXCLUDED by the cutoff.
	if _, _, err := k.Run(context.Background(), "old task"); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	// Sleep so the cutoff falls strictly between runs.
	time.Sleep(50 * time.Millisecond)
	cutoff := time.Now().UnixMilli()
	time.Sleep(50 * time.Millisecond)

	// Second run — these events should be INCLUDED.
	if _, _, err := k.Run(context.Background(), "new task"); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu     sync.Mutex
		seenTS []int64
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern":     ">",
				"since_ts_ms": cutoff,
			},
			func(e *event.Event) {
				if e.IsEphemeral() {
					return
				}
				mu.Lock()
				seenTS = append(seenTS, e.TSUnixMS)
				mu.Unlock()
			})
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-errCh

	mu.Lock()
	defer mu.Unlock()
	if len(seenTS) == 0 {
		t.Fatal("got 0 events; expected events from second run")
	}
	for _, ts := range seenTS {
		if ts < cutoff {
			t.Errorf("event with ts=%d arrived but cutoff=%d (should have been skipped)", ts, cutoff)
		}
	}
}

// TestPulse_SinceTSMs_FutureCutoffSkipsAll verifies a cutoff in
// the future replays nothing and goes straight to live — matches
// the --since semantics where a past-the-head value disables
// replay rather than misbehaving.
func TestPulse_SinceTSMs_FutureCutoffSkipsAll(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok"), mock.FinalText("ok2")))
	if _, _, err := k.Run(context.Background(), "before"); err != nil {
		t.Fatalf("run 1: %v", err)
	}

	// Cutoff one hour in the future — nothing in the journal qualifies.
	future := time.Now().Add(1 * time.Hour).UnixMilli()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var (
		mu      sync.Mutex
		seenSeq []int64
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern":     ">",
				"since_ts_ms": future,
			},
			func(e *event.Event) {
				if e.IsEphemeral() {
					return
				}
				mu.Lock()
				seenSeq = append(seenSeq, e.Seq)
				mu.Unlock()
			})
	}()
	time.Sleep(100 * time.Millisecond)

	// Live event after subscribe — must still arrive (the future
	// cutoff disables replay only; the live stream is unaffected
	// by `since_ts_ms` filtering).
	if _, _, err := k.Run(ctx, "after"); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-errCh

	mu.Lock()
	defer mu.Unlock()
	if len(seenSeq) == 0 {
		t.Error("live events after subscribe didn't arrive; future cutoff should affect only replay")
	}
}

// TestPulse_SinceTSMs_ComposesWithSince verifies the AND
// semantics: both filters must be satisfied for an event to
// replay. Using a sinceTSMs of 0 (forces "all in journal") and a
// since that's > 0 should NOT widen the result to the union — the
// since cutoff still applies.
func TestPulse_SinceTSMs_ComposesWithSince(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(
		mock.FinalText("a"),
		mock.FinalText("b"),
	))
	if _, _, err := k.Run(context.Background(), "run a"); err != nil {
		t.Fatalf("run a: %v", err)
	}
	if _, _, err := k.Run(context.Background(), "run b"); err != nil {
		t.Fatalf("run b: %v", err)
	}

	// Combined: since_ts_ms=0 (every journal entry passes the ts
	// filter) AND since=999999 (no journal entry passes the seq
	// filter) → intersection should replay NOTHING.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var (
		mu      sync.Mutex
		seenSeq []int64
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern":     ">",
				"since":       999999,
				"since_ts_ms": 0,
			},
			func(e *event.Event) {
				if e.IsEphemeral() {
					return
				}
				mu.Lock()
				seenSeq = append(seenSeq, e.Seq)
				mu.Unlock()
			})
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-errCh

	mu.Lock()
	defer mu.Unlock()
	for _, s := range seenSeq {
		if s < 999999 {
			t.Errorf("event seq=%d replayed despite since=999999 (AND composition broken — got OR)", s)
		}
	}
}

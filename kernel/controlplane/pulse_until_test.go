// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ersinkoc/agezt/kernel/controlplane"
	"github.com/ersinkoc/agezt/kernel/event"
	"github.com/ersinkoc/agezt/plugins/providers/mock"
)

// TestPulse_Until_TerminatesAfterReplay verifies the M1.ii
// replay-only semantics: when `until` is set, the stream completes
// after the bounded window drains rather than continuing live.
func TestPulse_Until_TerminatesAfterReplay(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(
		mock.FinalText("run1"),
		mock.FinalText("run2"),
	))

	if _, _, err := k.Run(context.Background(), "run 1"); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if _, _, err := k.Run(context.Background(), "run 2"); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	// Subscribe with since=0, until=<some seq mid-stream>. Stream
	// should finish on its own — we don't cancel.
	head, _ := k.Journal().Head()
	cutoff := head / 2 // exclusive upper bound somewhere in the middle
	if cutoff < 1 {
		t.Fatalf("not enough journal events to test (head=%d)", head)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var (
		mu      sync.Mutex
		seenSeq []int64
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern": ">",
				"since":   0,
				"until":   cutoff,
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

	// Server should close the connection on its own once the
	// window drains. StreamUntilCancel returns when the conn
	// closes (or on ctx timeout, which would be a test failure).
	select {
	case <-errCh:
		// Expected — stream completed naturally.
	case <-time.After(2 * time.Second):
		cancel()
		<-errCh
		t.Fatal("stream did not terminate after bounded replay (until cutoff ignored?)")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenSeq) == 0 {
		t.Fatal("got 0 events despite since=0 and a positive cutoff")
	}
	for _, s := range seenSeq {
		if s >= cutoff {
			t.Errorf("event seq=%d arrived but cutoff=%d (exclusive upper bound violated)", s, cutoff)
		}
	}
}

// TestPulse_Until_ExclusiveBound verifies the upper bound is
// exclusive: until=5 means seqs 0..4 arrive, NOT seq 5.
func TestPulse_Until_ExclusiveBound(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(
		mock.FinalText("a"),
		mock.FinalText("b"),
		mock.FinalText("c"),
	))
	for i := range 3 {
		if _, _, err := k.Run(context.Background(), "run"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	head, _ := k.Journal().Head()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var maxSeq int64 = -1
	var mu sync.Mutex
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern": ">",
				"since":   0,
				"until":   head, // exclusive: head itself should NOT appear
			},
			func(e *event.Event) {
				if e.IsEphemeral() {
					return
				}
				mu.Lock()
				if e.Seq > maxSeq {
					maxSeq = e.Seq
				}
				mu.Unlock()
			})
	}()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		cancel()
		<-errCh
		t.Fatal("stream did not terminate")
	}

	mu.Lock()
	defer mu.Unlock()
	if maxSeq >= head {
		t.Errorf("got seq=%d, want max < head=%d (until is exclusive)", maxSeq, head)
	}
	if maxSeq < head-1 {
		t.Errorf("got max seq=%d, expected head-1=%d (under-bounded?)", maxSeq, head-1)
	}
}

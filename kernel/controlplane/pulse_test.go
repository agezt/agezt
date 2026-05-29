// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPulse_StreamsEvents verifies the happy path: subscribe with
// the default pattern, then trigger a run on the kernel — pulse
// observes the run's events on the live stream.
func TestPulse_StreamsEvents(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Subscribe in a goroutine; we'll cancel the context to end it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		seen   atomic.Int64
		mu     sync.Mutex
		kinds  []event.Kind
		ready  = make(chan struct{})
		closed sync.Once
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{"pattern": ">"},
			func(e *event.Event) {
				if seen.Add(1) == 1 {
					closed.Do(func() { close(ready) })
				}
				mu.Lock()
				kinds = append(kinds, e.Kind)
				mu.Unlock()
			})
	}()

	// Give the subscribe a moment to register with the bus, then run.
	// Without this, the run can publish before the subscription is
	// active and pulse misses the events.
	time.Sleep(50 * time.Millisecond)
	if _, _, err := k.Run(ctx, "say hi via pulse"); err != nil {
		t.Fatalf("kernel.Run: %v", err)
	}

	// Wait for at least one event to arrive.
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("no events arrived on pulse stream (saw=%d)", seen.Load())
	}

	// Let any trailing events drain, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("pulse exited with: %v (want nil for ctx cancel)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pulse did not exit after ctx cancel")
	}

	mu.Lock()
	defer mu.Unlock()
	if seen.Load() < 3 {
		t.Errorf("pulse observed only %d events; expected at least 3 (task.received, llm.response, task.completed) — kinds=%v", seen.Load(), kinds)
	}
	// One of the run's standard kinds must have appeared.
	found := false
	for _, k := range kinds {
		if k == event.KindTaskCompleted || k == event.KindLLMResponse {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to see task.completed or llm.response; got %v", kinds)
	}
}

// TestPulse_FiltersByKind exercises the server-side kinds filter.
// Events whose Kind is not in the filter must never cross the socket.
func TestPulse_FiltersByKind(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu       sync.Mutex
		kinds    []event.Kind
		anyEv    = make(chan struct{}, 1)
		signaled bool
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern": ">",
				"kinds":   []any{string(event.KindTaskCompleted)},
			},
			func(e *event.Event) {
				mu.Lock()
				kinds = append(kinds, e.Kind)
				if !signaled {
					signaled = true
					anyEv <- struct{}{}
				}
				mu.Unlock()
			})
	}()
	time.Sleep(50 * time.Millisecond)
	if _, _, err := k.Run(ctx, "filter test"); err != nil {
		t.Fatalf("kernel.Run: %v", err)
	}

	select {
	case <-anyEv:
	case <-time.After(2 * time.Second):
		t.Fatal("no filtered events arrived (expected task.completed)")
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-errCh

	mu.Lock()
	defer mu.Unlock()
	if len(kinds) == 0 {
		t.Fatal("expected at least one task.completed event")
	}
	for _, k := range kinds {
		if k != event.KindTaskCompleted {
			t.Errorf("pulse delivered Kind=%q which was not in filter; only task.completed should pass", k)
		}
	}
}

// TestPulse_ContextCancelExitsCleanly verifies cancelling the
// client-side ctx returns nil from StreamUntilCancel (i.e. it's
// treated as an operator-initiated termination, not a failure).
// Distinguishes the Ctrl+C path from real read errors.
func TestPulse_ContextCancelExitsCleanly(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{"pattern": ">"},
			func(*event.Event) {})
	}()

	// Cancel after a tick; the goroutine should return nil.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("StreamUntilCancel = %v, want nil after ctx cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamUntilCancel did not return after ctx cancel")
	}
}

// TestPulse_DefaultPatternMatchesAll proves omitting the `pattern`
// arg falls through to ">". Catches a regression where a future
// change requires the field.
func TestPulse_DefaultPatternMatchesAll(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("default")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			nil, // no args at all → server defaults pattern=">"
			func(*event.Event) {
				select {
				case got <- struct{}{}:
				default:
				}
			})
	}()
	time.Sleep(50 * time.Millisecond)
	if _, _, err := k.Run(ctx, "default pattern test"); err != nil {
		t.Fatalf("kernel.Run: %v", err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("no events arrived with default pattern")
	}
	cancel()
	<-errCh
}

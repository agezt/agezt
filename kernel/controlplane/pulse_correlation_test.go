// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ersinkoc/agezt/kernel/controlplane"
	"github.com/ersinkoc/agezt/kernel/event"
	"github.com/ersinkoc/agezt/plugins/providers/mock"
)

// TestPulse_FiltersByCorrelation exercises the server-side
// correlation filter. With two correlation chains active on the
// bus, the subscription requesting one of them must only see
// events from that chain.
func TestPulse_FiltersByCorrelation(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wantCorr := "corr-A"

	var (
		mu       sync.Mutex
		seenCorr []string
		ready    = make(chan struct{})
		readyOne atomic.Bool
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{
				"pattern":     ">",
				"correlation": wantCorr,
			},
			func(e *event.Event) {
				mu.Lock()
				seenCorr = append(seenCorr, e.CorrelationID)
				mu.Unlock()
				if readyOne.CompareAndSwap(false, true) {
					close(ready)
				}
			})
	}()

	// Let the subscription register, then publish to two chains.
	time.Sleep(80 * time.Millisecond)
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "test.a",
			Kind:          event.Kind("test.event"),
			Actor:         "test",
			CorrelationID: "corr-A",
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "test.b",
			Kind:          event.Kind("test.event"),
			Actor:         "test",
			CorrelationID: "corr-B",
		})
	}

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("no events arrived")
	}

	// Drain trailing events, then end the stream.
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("pulse did not exit after cancel")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenCorr) == 0 {
		t.Fatal("no events delivered to filtered subscription")
	}
	for _, c := range seenCorr {
		if c != wantCorr {
			t.Errorf("filter leaked: saw correlation %q, want only %q", c, wantCorr)
		}
	}
}

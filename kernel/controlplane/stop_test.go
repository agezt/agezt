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

// TestStop_ReleasesInFlightStream pins M461: a direct Stop() (with the Start ctx
// still live) must release in-flight streaming handlers, not leave them blocked on
// ctx.Done() until the per-connection deadline. A pulse subscription is held open;
// Stop() must return promptly. Before the fix, Stop closed only the listener and
// the streaming handler waited on the (uncancelled) Start ctx, so Stop's wg.Wait()
// hung.
func TestStop_ReleasesInFlightStream(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var once sync.Once
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe,
			map[string]any{"pattern": ">"},
			func(*event.Event) { once.Do(func() { close(ready) }) })
	}()

	// Give the subscription a moment to register server-side (mirrors the other
	// pulse tests, which rely on the same brief settle before publishing).
	time.Sleep(100 * time.Millisecond)

	// Stop WITHOUT cancelling the Start ctx. It must return promptly.
	done := make(chan struct{})
	go func() { _ = srv.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("srv.Stop() did not return: an in-flight streaming handler is not released by Stop")
	}

	// Tidy: the client's stream ends when the server closed its conn.
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Error("client stream did not unwind after Stop")
	}
	_ = ready
}

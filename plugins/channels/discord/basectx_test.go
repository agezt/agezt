// SPDX-License-Identifier: MIT

package discord

import (
	"context"
	"testing"
	"time"
)

// TestStart_TiesAsyncRunsToDaemonCtx verifies the wiring that lets a clean
// shutdown cancel in-flight async inbound runs: New defaults baseCtx non-nil (so
// a handler driven in tests never gets a nil ctx), and Start binds baseCtx to the
// daemon context — the context the async run spawn uses instead of
// context.Background, so shutdown (after the drain window) cancels stragglers.
func TestStart_TiesAsyncRunsToDaemonCtx(t *testing.T) {
	c := New(Config{}) // empty Addr → Start blocks on ctx.Done()
	if c.baseCtx == nil {
		t.Fatal("New must default baseCtx to a non-nil context")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = c.Start(ctx); close(done) }()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after the daemon ctx was cancelled")
	}
	// After Start returns, baseCtx is the daemon ctx (now cancelled): an in-flight
	// async run on baseCtx would observe this cancellation.
	if c.baseCtx.Err() == nil {
		t.Error("baseCtx was not tied to the daemon ctx (async runs would not cancel on shutdown)")
	}
}

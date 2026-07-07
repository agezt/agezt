// SPDX-License-Identifier: MIT

package anomaly

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// TestStart_NilBus covers the `b == nil` half of the disabled guard.
func TestStart_NilBus(t *testing.T) {
	if Start(context.Background(), nil, Config{MaxToolCalls: 5, Window: time.Second}, nil) {
		t.Fatalf("Start with a nil bus should return false")
	}
}

// TestStart_SubscribeErrorOnClosedBus covers the Subscribe-error return: an
// enabled config on an already-closed bus can't subscribe, so Start returns
// false.
func TestStart_SubscribeErrorOnClosedBus(t *testing.T) {
	b := newBus(t)
	b.Close()
	if Start(context.Background(), b, Config{MaxToolCalls: 5, Window: time.Second}, nil) {
		t.Fatalf("Start should return false when Subscribe fails on a closed bus")
	}
}

// TestStart_ContextCancelStopsWatcher covers the ctx.Done() branch and the
// non-tool-invoked `continue` branch: a non-tool event is ignored, then
// cancelling the context stops the watcher goroutine cleanly.
func TestStart_ContextCancelStopsWatcher(t *testing.T) {
	b := newBus(t)
	ctx, cancel := context.WithCancel(context.Background())
	if !Start(ctx, b, Config{MaxToolCalls: 100, Window: time.Second}, nil) {
		t.Fatalf("Start returned false for an enabled config")
	}
	// A non-tool event exercises the `continue` branch without tripping.
	if _, err := b.Publish(event.Spec{Subject: "agent.spawned", Kind: event.KindAgentSpawned, Actor: "kernel"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	cancel() // watcher must observe ctx.Done() and return
	time.Sleep(50 * time.Millisecond)
}

// TestStart_BusCloseStopsWatcher covers the channel-closed (`!ok`) branch:
// closing the bus closes the subscription channel, so the watcher returns.
func TestStart_BusCloseStopsWatcher(t *testing.T) {
	b := newBus(t)
	if !Start(context.Background(), b, Config{MaxToolCalls: 100, Window: time.Second}, nil) {
		t.Fatalf("Start returned false for an enabled config")
	}
	time.Sleep(20 * time.Millisecond)
	b.Close() // closes sub.C → watcher hits the !ok branch and returns
	time.Sleep(50 * time.Millisecond)
}

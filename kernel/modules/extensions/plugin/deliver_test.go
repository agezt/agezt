// SPDX-License-Identifier: MIT

package plugin

// White-box tests for the read loop's terminal-response delivery (M179):
// the send must be non-blocking and race-safe against a concurrent
// teardown (markDead/Close) that closes pending channels — otherwise a
// hostile plugin could provoke a send-on-closed-channel panic that, in
// the unrecovered read loop, crashes the whole daemon.

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func newTestPlugin() *Plugin {
	return &Plugin{
		pending:  make(map[string]chan *Response),
		progress: make(map[string]func(string)),
	}
}

// A normal frame reaches its waiter; a second terminal frame for the
// same id (a malicious double-send) is dropped without blocking the
// caller of deliver.
func TestDeliver_DropsDuplicateWithoutBlocking(t *testing.T) {
	p := newTestPlugin()
	ch := make(chan *Response, 1)
	p.pending["q-1"] = ch

	p.deliver(inboundFrame{ID: "q-1", Result: json.RawMessage(`{"output":"a"}`)})
	select {
	case r := <-ch:
		if r.ID != "q-1" {
			t.Fatalf("delivered id = %q want q-1", r.ID)
		}
	default:
		t.Fatal("first frame not delivered")
	}

	// Fill the buffer, then a duplicate terminal frame must not block.
	p.deliver(inboundFrame{ID: "q-1", Result: json.RawMessage(`{"output":"b"}`)}) // fills cap-1 buffer
	done := make(chan struct{})
	go func() {
		p.deliver(inboundFrame{ID: "q-1", Result: json.RawMessage(`{"output":"c"}`)}) // must hit default
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deliver blocked on a full buffer (duplicate terminal frame)")
	}

	// A frame for an unknown id is a silent no-op.
	p.deliver(inboundFrame{ID: "nobody-waiting"})
}

// The core H3 fix: deliver racing markDead must never panic
// (send-on-closed-channel). markDead closes+deletes the pending channel
// under the same lock deliver holds, so the two are mutually exclusive.
func TestDeliver_RaceWithMarkDeadNoPanic(t *testing.T) {
	for iter := 0; iter < 500; iter++ {
		p := newTestPlugin()
		ch := make(chan *Response, 1)
		p.pending["q-1"] = ch

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			p.deliver(inboundFrame{ID: "q-1", Result: json.RawMessage(`{}`)})
		}()
		go func() {
			defer wg.Done()
			p.markDead(errors.New("boom"))
		}()
		wg.Wait()
		// Reaching here without a panic is the assertion. The outcome is
		// either "caller received the response" or "channel closed by
		// teardown" — both are valid; neither crashes the daemon.
	}
}

// blockingStdin is an io.WriteCloser whose Write blocks until release is closed,
// modelling a child whose stdin pipe buffer is full because it stopped reading.
type blockingStdin struct {
	entered chan struct{}
	release chan struct{}
}

func (w *blockingStdin) Write(p []byte) (int, error) {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	<-w.release
	return len(p), nil
}
func (w *blockingStdin) Close() error { return nil }

// TestDeliver_NotBlockedByStuckStdinWrite pins the M460 fix: the read loop's
// response router (deliver) must not be blocked while a write to the child's
// stdin is stuck. A plugin that floods stdout without draining its stdin makes a
// host-side writeResponse block on the full stdin pipe; if that write held the
// same mutex deliver needs, the read loop would wedge — and since it then stops
// draining stdout, the child's stdout pipe fills and the write never completes
// (a deadlock). Writes are serialized on writeMu, not mu, so deliver stays live.
func TestDeliver_NotBlockedByStuckStdinWrite(t *testing.T) {
	release := make(chan struct{})
	bw := &blockingStdin{entered: make(chan struct{}, 1), release: release}
	p := newTestPlugin()
	p.stdin = bw
	p.pending["q-1"] = make(chan *Response, 1)

	// A host-initiated response write that gets stuck on the full stdin pipe.
	go func() { _ = p.writeResponse(Response{ID: "cb-1"}) }()
	select {
	case <-bw.entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("writeResponse never reached the stdin write")
	}

	// deliver (read-loop path) must complete despite the stuck writer.
	done := make(chan struct{})
	go func() {
		p.deliver(inboundFrame{ID: "q-1", Result: json.RawMessage(`{}`)})
		close(done)
	}()
	select {
	case <-done:
		// good — the read loop can route responses while a write is stuck.
	case <-time.After(time.Second):
		close(release)
		t.Fatal("deliver blocked by a stuck stdin write — read-loop/writer deadlock")
	}
	close(release)
}

// TestCallWithProgress_DeadDuringRegistrationFailsFast pins the M464 fix for the
// registration TOCTOU: a caller that passes the lock-free liveness check, then has
// the plugin marked dead and `pending` drained before it registers, must fail fast
// (death error) — not register an orphan channel and block until its ctx deadline.
// p.mu is the seam: holding it parks the caller exactly between the lock-free check
// and the under-lock registration while we simulate a concurrent teardown.
func TestCallWithProgress_DeadDuringRegistrationFailsFast(t *testing.T) {
	p := newTestPlugin()
	rel := make(chan struct{})
	close(rel) // stdin writes return immediately
	p.stdin = &blockingStdin{entered: make(chan struct{}, 1), release: rel}

	p.mu.Lock() // park the caller at its under-lock registration

	callDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := p.callWithProgress(ctx, "do", nil, nil)
		callDone <- err
	}()

	// Let the caller pass its lock-free dead-check and block on p.mu.
	time.Sleep(50 * time.Millisecond)

	// Simulate Close/markDead running under the lock: mark dead and drain pending.
	p.dead.Store(true)
	p.setDeathErr(errors.New("closed mid-registration"))
	for id, ch := range p.pending {
		close(ch)
		delete(p.pending, id)
	}
	p.mu.Unlock()

	select {
	case err := <-callDone:
		if err == nil {
			t.Fatal("expected a death error after a concurrent close, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callWithProgress blocked after a concurrent close: it registered an orphan channel (TOCTOU)")
	}
}

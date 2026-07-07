// SPDX-License-Identifier: MIT

package bus

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/redact"
)

// TestRedactor_Getter covers the Redactor accessor: nil by default, and the
// installed instance after SetRedactor.
func TestRedactor_Getter(t *testing.T) {
	b := newTestBus(t)
	if b.Redactor() != nil {
		t.Fatalf("Redactor() should be nil by default")
	}
	r := redact.New()
	b.SetRedactor(r)
	if got := b.Redactor(); got == nil {
		t.Fatalf("Redactor() should return the installed redactor")
	}
}

// TestSubscribe_AfterClose covers the b.closed branch in Subscribe.
func TestSubscribe_AfterClose(t *testing.T) {
	b := newTestBus(t)
	b.Close()
	if _, err := b.Subscribe("agent.>", 1); err != ErrClosed {
		t.Fatalf("Subscribe after Close = %v, want ErrClosed", err)
	}
}

// TestPublish_DropsWhenFull covers the drop-on-full branch of Publish: a
// subscriber with a tiny buffer that is never drained increments Dropped.
func TestPublish_DropsWhenFull(t *testing.T) {
	b := newTestBus(t)
	sub, err := b.Subscribe(">", 1)
	if err != nil {
		t.Fatal(err)
	}
	// First publish fills the buffer (1), subsequent ones are dropped.
	for i := 0; i < 5; i++ {
		if _, err := b.Publish(event.Spec{Subject: "agent.tick", Kind: "test", Actor: "kernel"}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	if sub.Dropped.Load() == 0 {
		t.Fatalf("expected at least one dropped event")
	}
}

// TestPublishStreaming_AfterClose covers the b.closed branch of PublishStreaming.
func TestPublishStreaming_AfterClose(t *testing.T) {
	b := newTestBus(t)
	b.Close()
	if _, err := b.PublishStreaming(event.Spec{Subject: "llm.token", Kind: "test", Actor: "kernel"}); err != ErrClosed {
		t.Fatalf("PublishStreaming after Close = %v, want ErrClosed", err)
	}
}

// TestPublishStreaming_DeliversAndDrops covers the match+deliver path and the
// drop-on-full branch of PublishStreaming.
func TestPublishStreaming_DeliversAndDrops(t *testing.T) {
	b := newTestBus(t)
	sub, err := b.Subscribe(">", 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := b.PublishStreaming(event.Spec{Subject: "llm.token", Kind: "test", Actor: "kernel"}); err != nil {
			t.Fatalf("PublishStreaming: %v", err)
		}
	}
	if sub.Dropped.Load() == 0 {
		t.Fatalf("expected at least one dropped streaming event")
	}
}

// TestPublishStreaming_NewEphemeralFailure covers the error return path in
// PublishStreaming when event.NewEphemeral fails (e.g. un-marshalable payload).
func TestPublishStreaming_NewEphemeralFailure(t *testing.T) {
	b := newTestBus(t)
	// A channel cannot be JSON-marshaled, so NewEphemeral will return an error.
	_, err := b.PublishStreaming(event.Spec{
		Subject: "llm.token",
		Kind:    event.KindLLMToken,
		Actor:   "agent-1",
		Payload: make(chan int),
	})
	if err == nil {
		t.Fatal("PublishStreaming with channel payload: expected error, got nil")
	}
}

// TestClose_Idempotent covers the second-call early return in Close.
func TestClose_Idempotent(t *testing.T) {
	b := newTestBus(t)
	b.Close()
	b.Close() // second call must hit the `if b.closed { return }` branch
}

// TestPublish_AfterClose covers the b.closed branch of Publish.
func TestPublish_AfterClose(t *testing.T) {
	b := newTestBus(t)
	b.Close()
	if _, err := b.Publish(event.Spec{Subject: "agent.tick", Kind: "test", Actor: "kernel"}); err != ErrClosed {
		t.Fatalf("Publish after Close = %v, want ErrClosed", err)
	}
}

// TestSubscribe_CancelTwice exercises the cancel closure and its once.Do guard,
// including a second Cancel that must be a no-op.
func TestSubscribe_CancelTwice(t *testing.T) {
	b := newTestBus(t)
	sub, err := b.Subscribe("agent.>", 1)
	if err != nil {
		t.Fatal(err)
	}
	sub.Cancel()
	sub.Cancel() // second Cancel must be safe (once.Do guard)
	if _, ok := <-sub.C; ok {
		t.Fatalf("channel should be closed after Cancel")
	}
}

// TestSubscribe_CancelAfterClose exercises the cancel closure's guard when the
// subscription was already removed by Close: the `s == sub` lookup misses, so
// the close/delete inside cancel is skipped without panicking on a double-close.
func TestSubscribe_CancelAfterClose(t *testing.T) {
	b := newTestBus(t)
	sub, err := b.Subscribe("agent.>", 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Close() // removes+closes the sub channel
	sub.Cancel()
}

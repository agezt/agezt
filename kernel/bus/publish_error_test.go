// SPDX-License-Identifier: MIT

package bus

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// TestSubscribe_BadPattern covers the parsePattern error branch in Subscribe:
// a malformed pattern (empty token) is rejected before any subscription state
// is created.
func TestSubscribe_BadPattern(t *testing.T) {
	b := newTestBus(t)
	if _, err := b.Subscribe("a..b", 1); err == nil {
		t.Fatalf("Subscribe(bad pattern): expected parse error, got nil")
	}
}

// TestPublish_NonMatchingSubscriber covers the non-match `continue` branch in
// Publish: a subscriber whose pattern does not match the published subject is
// skipped, and the matching subscriber still receives the event.
func TestPublish_NonMatchingSubscriber(t *testing.T) {
	b := newTestBus(t)
	// This subscriber's pattern will NOT match "x.y" — exercises the continue.
	if _, err := b.Subscribe("a.b", 1); err != nil {
		t.Fatal(err)
	}
	match, err := b.Subscribe("x.>", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Publish(event.Spec{Subject: "x.y", Kind: "test", Actor: "kernel"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case <-match.C:
	default:
		t.Fatalf("matching subscriber should have received the event")
	}
}

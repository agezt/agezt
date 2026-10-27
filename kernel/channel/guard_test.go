// SPDX-License-Identifier: MIT

package channel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func newGuardBus(t *testing.T) (*bus.Bus, *bus.Subscription) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(b.Close)
	sub, err := b.Subscribe("channel.>", 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Cancel)
	return b, sub
}

func TestGuard_RecoversPanicAndJournals(t *testing.T) {
	b, sub := newGuardBus(t)

	// A panicking handler must NOT propagate — Guard returns normally (if the panic
	// escaped, the test goroutine would crash).
	Guard(b, "telegram", func() { panic("malformed message") })

	select {
	case ev := <-sub.C:
		if ev.Kind != event.KindChannelError {
			t.Errorf("event kind = %q, want %q", ev.Kind, event.KindChannelError)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a channel.error event for the recovered panic")
	}
}

func TestGuard_RunsFnAndStaysQuietWhenNoPanic(t *testing.T) {
	b, sub := newGuardBus(t)

	ran := false
	Guard(b, "slack", func() { ran = true })
	if !ran {
		t.Error("Guard did not run fn")
	}
	select {
	case ev := <-sub.C:
		t.Errorf("no-panic path must not journal, got %q", ev.Kind)
	case <-time.After(50 * time.Millisecond):
		// expected: nothing
	}
}

func TestGuard_NilBusStillRecovers(t *testing.T) {
	// Must not itself panic when no bus is configured.
	if Guard(nil, "discord", func() { panic("boom") }) {
		t.Error("EXPECTED:no journal attempt without a publisher")
	}
}

// TestGuard_TypedNilPublisherStillRecovers: a nil *bus.Bus handed to an
// interface parameter is a NON-nil interface.
//
// The publish would panic inside the very recover that is supposed to contain
// the damage — and Guard exists precisely so that one bad message cannot end
// the process. The guard still reports the attempt: it tried, and the
// publisher was broken.
func TestGuard_TypedNilPublisherStillRecovers(t *testing.T) {
	var b *bus.Bus
	if !Guard(b, "discord", func() { panic("owned boom") }) {
		t.Error("EXPECTED:the journal attempt is reported")
	}
}

// TestGuard_JournalsTheChannelAndThePanicValue: the journalled event stays
// diagnosable — it names the channel, the actor and the recovered value.
func TestGuard_JournalsTheChannelAndThePanicValue(t *testing.T) {
	b, sub := newGuardBus(t)
	Guard(b, "telegram", func() { panic("owned malformed message") })
	select {
	case ev := <-sub.C:
		if ev.Subject != "channel.telegram.error" {
			t.Errorf("subject = %q, want %q", ev.Subject, "channel.telegram.error")
		}
		if ev.Actor != "channel-telegram" {
			t.Errorf("actor = %q, want %q", ev.Actor, "channel-telegram")
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("payload is not JSON: %v (%s)", err, ev.Payload)
		}
		if payload["channel"] != "telegram" {
			t.Errorf("payload channel = %v, want %q", payload["channel"], "telegram")
		}
		if payload["panic"] != "owned malformed message" {
			t.Errorf("payload panic = %v, want %q", payload["panic"], "owned malformed message")
		}
	case <-time.After(time.Second):
		t.Fatal("expected a channel.error event")
	}
}

// panickingPublisher is a publisher that is broken in its own right.
type panickingPublisher struct{}

func (panickingPublisher) Publish(_ event.Spec) (*event.Event, error) {
	panic("owned publisher panic")
}

// TestGuard_ABrokenPublisherDoesNotTakeTheDaemonDown: the error path is
// panic-safe on its own.
func TestGuard_ABrokenPublisherDoesNotTakeTheDaemonDown(t *testing.T) {
	if !Guard(panickingPublisher{}, "matrix", func() { panic("owned boom") }) {
		t.Error("EXPECTED:the journal attempt is reported")
	}
}

// TestGuard_ReportsTheJournalAttempt: a daemon with a publisher records the
// failure; a daemon without one says so instead of pretending.
func TestGuard_ReportsTheJournalAttempt(t *testing.T) {
	b, sub := newGuardBus(t)
	if !Guard(b, "telegram", func() { panic("owned boom") }) {
		t.Error("EXPECTED:the journal attempt is reported")
	}
	select {
	case <-sub.C:
	case <-time.After(time.Second):
		t.Fatal("expected a channel.error event")
	}
	if Guard(b, "telegram", func() {}) {
		t.Error("EXPECTED:no attempt when there is no panic")
	}
}

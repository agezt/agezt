// SPDX-License-Identifier: MIT

package channel

import (
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
	Guard(nil, "discord", func() { panic("boom") })
}

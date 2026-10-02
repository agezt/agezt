// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// startFunc is a channel whose Start is scripted.
type startFunc struct {
	name  string
	start func(ctx context.Context) error
}

func (s startFunc) Name() string                                 { return s.name }
func (s startFunc) Start(ctx context.Context) error              { return s.start(ctx) }
func (s startFunc) Send(context.Context, channel.Outbound) error { return nil }

func channelTestBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return bus.New(j)
}

// liveFor registers key as a live instance, the way boot does.
func liveFor(t *testing.T, key string) {
	t.Helper()
	channel.SetLive([]string{strings.SplitN(key, "#", 2)[0]})
	channel.SetLiveInstances([]string{key})
	t.Cleanup(func() { channel.SetLive(nil); channel.SetLiveInstances(nil) })
}

func countChannelErrors(t *testing.T, b *bus.Bus, sub *bus.Subscription) int {
	t.Helper()
	n := 0
	for {
		select {
		case ev := <-sub.C:
			if ev.Kind == event.KindChannelError {
				n++
			}
		case <-time.After(150 * time.Millisecond):
			return n
		}
	}
}

// A Start error while the daemon runs (port in use, bad token) used to vanish
// (`go ch.Start(ctx)`): the channel was dead and still reported live.
func TestRunChannel_StartErrorMarksDeadAndJournals(t *testing.T) {
	b := channelTestBus(t)
	sub, _ := b.Subscribe(">", 16)
	defer sub.Cancel()
	liveFor(t, "zzwebhook#a")
	var stderr bytes.Buffer

	runChannel(context.Background(), b, &stderr, "zzwebhook#a", startFunc{name: "zzwebhook", start: func(context.Context) error {
		return errors.New("listen tcp :8443: bind: address already in use")
	}})

	if channel.IsLiveInstance("zzwebhook#a") {
		t.Error("a channel whose Start failed is still reported live")
	}
	if !strings.Contains(stderr.String(), "address already in use") {
		t.Errorf("stderr = %q, want the start error", stderr.String())
	}
	if n := countChannelErrors(t, b, sub); n != 1 {
		t.Errorf("channel.error events = %d, want 1", n)
	}
}

// A panic inside Start used to crash the daemon for channels whose loop had
// no guard of its own. It is now contained, journaled once, and fatal only to
// that channel.
func TestRunChannel_PanicIsContainedAndMarksDead(t *testing.T) {
	b := channelTestBus(t)
	sub, _ := b.Subscribe(">", 16)
	defer sub.Cancel()
	liveFor(t, "zzirc")
	var stderr bytes.Buffer

	runChannel(context.Background(), b, &stderr, "zzirc", startFunc{name: "zzirc", start: func(context.Context) error {
		panic("handler bug")
	}})

	if channel.IsLiveInstance("zzirc") {
		t.Error("a panicked channel is still reported live")
	}
	if n := countChannelErrors(t, b, sub); n != 1 {
		t.Errorf("channel.error events = %d, want exactly 1 (Guard's panic event)", n)
	}
}

// Shutdown and a clean return are not deaths.
func TestRunChannel_ShutdownAndCleanReturnStayLive(t *testing.T) {
	b := channelTestBus(t)
	liveFor(t, "zzslack")
	var stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runChannel(ctx, b, &stderr, "zzslack", startFunc{name: "zzslack", start: func(ctx context.Context) error {
		return ctx.Err() // a listener reporting the shutdown that stopped it
	}})
	runChannel(context.Background(), b, &stderr, "zzslack", startFunc{name: "zzslack", start: func(context.Context) error {
		return nil // handed its work to its own goroutines
	}})

	if !channel.IsLiveInstance("zzslack") {
		t.Error("shutdown or a clean return marked the channel dead")
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected warning: %q", stderr.String())
	}
}

// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"reflect"
	"testing"
	"time"
)

type streamJournal struct {
	events     []*event.Event
	cause      error
	rangeStart func()
	reads      int
}

func (p *streamJournal) Range(fn func(*event.Event) error) error {
	p.reads++
	if p.rangeStart != nil {
		p.rangeStart()
	}
	for _, ev := range p.events {
		if err := fn(ev); err != nil {
			return err
		}
	}
	return p.cause
}
func streamEvent(seq int64, ephemeral bool) *event.Event {
	hash := "durable"
	if ephemeral {
		hash = ""
	}
	return &event.Event{Seq: seq, Hash: hash, Subject: "owned.one", Kind: event.KindWorkflowStarted, CorrelationID: "owned"}
}
func TestPulseStreamSubscribesBeforeReplayDeduplicatesDurableAndPassesEphemeral(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *event.Event, 8)
	subscribed, canceled, stopped, watchers := false, 0, 0, 0
	j := &streamJournal{events: []*event.Event{streamEvent(0, false), streamEvent(1, false)}}
	j.rangeStart = func() {
		if !subscribed {
			t.Fatal("replay before subscribe")
		}
		events <- streamEvent(0, false)
		events <- streamEvent(1, false)
		events <- streamEvent(0, true)
		wrong := streamEvent(2, false)
		wrong.Kind = event.KindWorkflowCompleted
		events <- wrong
		foreign := streamEvent(3, false)
		foreign.CorrelationID = "foreign"
		events <- foreign
		events <- streamEvent(4, false)
	}
	s := NewStream(j, func(pattern string, buffer int) (Subscription, error) {
		if pattern != "owned.*" || buffer != 4096 {
			t.Fatal(pattern, buffer)
		}
		subscribed = true
		return Subscription{Events: events, Dropped: func() uint64 { return 0 }, Cancel: func() { canceled++ }}, nil
	}, func() DropTicker { return DropTicker{Stop: func() { stopped++ }} })
	in := unboundedReplay()
	in.Pattern = "owned.*"
	in.Since = 0
	in.Kinds = map[event.Kind]struct{}{event.KindWorkflowStarted: {}}
	in.Correlation = "owned"
	var got []*event.Event
	err := s.Stream(ctx, in, func(ev *event.Event) error {
		got = append(got, ev)
		if ev.Seq == 4 {
			cancel()
		}
		return nil
	}, func() <-chan struct{} {
		watchers++
		if j.reads != 1 {
			t.Fatal("watcher before replay")
		}
		return nil
	})
	if err != nil || len(got) != 4 || got[0].Seq != 0 || got[1].Seq != 1 || !got[2].IsEphemeral() || got[3].Seq != 4 || canceled != 1 || stopped != 1 || watchers != 1 {
		t.Fatal(err, got, canceled, stopped, watchers)
	}
}
func TestPulseStreamReplayOnlyErrorsAndCleanupRetainNativePolicies(t *testing.T) {
	rangeCause := errors.New("owned range cause")
	writeCause := errors.New("owned write cause")
	subscribeCause := errors.New("owned subscribe cause")
	for _, mode := range []string{"bounded", "subscribe-error", "range-error", "replay-write-error", "closed", "live-write-error", "client-close", "server-cancel"} {
		t.Run(mode, func(t *testing.T) {
			events := make(chan *event.Event, 1)
			gone := make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			canceled, stopped, watchers, tickers := 0, 0, 0, 0
			j := &streamJournal{events: []*event.Event{streamEvent(0, false)}}
			in := unboundedReplay()
			switch mode {
			case "bounded":
				in.Until = 1
			case "range-error":
				in.Since = 0
				j.cause = rangeCause
			case "replay-write-error":
				in.Since = 0
			case "closed":
				close(events)
			case "live-write-error":
				events <- streamEvent(3, false)
			case "client-close":
				close(gone)
			case "server-cancel":
				cancel()
			}
			s := NewStream(j, func(string, int) (Subscription, error) {
				if mode == "subscribe-error" {
					return Subscription{}, subscribeCause
				}
				return Subscription{Events: events, Dropped: func() uint64 { return 0 }, Cancel: func() { canceled++ }}, nil
			}, func() DropTicker { tickers++; return DropTicker{Stop: func() { stopped++ }} })
			writes := 0
			err := s.Stream(ctx, in, func(*event.Event) error {
				writes++
				if mode == "live-write-error" || mode == "replay-write-error" {
					return writeCause
				}
				return nil
			}, func() <-chan struct{} { watchers++; return gone })
			switch mode {
			case "subscribe-error":
				if err != subscribeCause || canceled != 0 || j.reads != 0 {
					t.Fatal(err, canceled, j.reads)
				}
			case "range-error", "replay-write-error":
				var replayErr ReplayError
				cause := rangeCause
				if mode == "replay-write-error" {
					cause = writeCause
				}
				if !errors.As(err, &replayErr) || !errors.Is(err, cause) || err.Error() != "pulse replay: "+cause.Error() || canceled != 1 || watchers != 0 || tickers != 0 {
					t.Fatal(err, canceled, watchers, tickers)
				}
			case "bounded":
				if err != nil || writes != 1 || canceled != 1 || watchers != 0 || tickers != 0 {
					t.Fatal(err, writes, canceled, watchers, tickers)
				}
			case "closed":
				if err != ErrSubscriptionClosed || canceled != 1 || stopped != 1 {
					t.Fatal(err, canceled, stopped)
				}
			default:
				if err != nil || canceled != 1 || stopped != 1 || watchers != 1 {
					t.Fatal(err, canceled, stopped, watchers)
				}
			}
		})
	}
}
func TestPulseStreamDropNoticeIsPerStreamEphemeralUnfilteredAndDeltaTracked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time, 4)
	events := make(chan *event.Event)
	dropReads := 0
	canceled, stopped := 0, 0
	j := &streamJournal{}
	s := NewStream(j, func(string, int) (Subscription, error) {
		return Subscription{Events: events, Dropped: func() uint64 {
			dropReads++
			if dropReads < 3 {
				return 3
			}
			return 5
		}, Cancel: func() { canceled++ }}, nil
	}, func() DropTicker {
		ticks <- time.Now()
		ticks <- time.Now()
		ticks <- time.Now()
		return DropTicker{C: ticks, Stop: func() { stopped++ }}
	})
	in := unboundedReplay()
	in.Pattern = "unmatched.*"
	in.Kinds = map[event.Kind]struct{}{event.KindWorkflowStarted: {}}
	in.Correlation = "unmatched"
	var payloads []map[string]any
	err := s.Stream(ctx, in, func(ev *event.Event) error {
		if ev.Kind != event.KindPulseDropped || ev.Subject != "agezt.pulse.dropped" || ev.Actor != "agezt" || !ev.IsEphemeral() || ev.Seq != 0 || ev.Hash != "" || ev.CorrelationID != "" {
			t.Fatal(ev)
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, payload)
		if len(payloads) == 1 {
			// Equal second sample produces no notice; the third advances to5.
		} else {
			cancel()
		}
		return nil
	}, nil)
	if err != nil || !reflect.DeepEqual(payloads, []map[string]any{{"dropped_since_last_notice": float64(3), "dropped_total": float64(3)}, {"dropped_since_last_notice": float64(2), "dropped_total": float64(5)}}) || canceled != 1 || stopped != 1 || j.reads != 0 || dropReads != 3 {
		t.Fatal(err, payloads, canceled, stopped, j.reads)
	}
}
func TestPulseStreamNoMatchCheckpointAndNilEmptyKindsRemainDistinct(t *testing.T) {
	for _, emptyKinds := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		events := make(chan *event.Event, 1)
		events <- streamEvent(1, false)
		j := &streamJournal{events: []*event.Event{streamEvent(0, false)}}
		s := NewStream(j, func(string, int) (Subscription, error) {
			return Subscription{Events: events, Dropped: func() uint64 { return 0 }, Cancel: func() {}}, nil
		}, func() DropTicker { return DropTicker{Stop: func() {}} })
		in := unboundedReplay()
		in.Since = 100
		if emptyKinds {
			in.Kinds = map[event.Kind]struct{}{}
		}
		writes := 0
		if emptyKinds {
			cancel()
		}
		err := s.Stream(ctx, in, func(*event.Event) error { writes++; cancel(); return nil }, nil)
		if err != nil || !emptyKinds && writes != 1 || emptyKinds && writes != 0 {
			t.Fatal(err, writes, emptyKinds)
		}
		cancel()
	}
}
func TestPulseStreamOwnedBusSubscriptionBeforeActualReplay(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	b := bus.New(j)
	defer b.Close()
	for i := 0; i < 2; i++ {
		if _, err := b.Publish(event.Spec{Subject: "owned.event", Kind: event.KindWorkflowStarted, Actor: "fixture", CorrelationID: "owned"}); err != nil {
			t.Fatal(err)
		}
	}
	canceled := 0
	s := NewStream(j, func(pattern string, buffer int) (Subscription, error) {
		sub, err := b.Subscribe(pattern, buffer)
		if err != nil {
			return Subscription{}, err
		}
		return Subscription{Events: sub.C, Dropped: sub.Dropped.Load, Cancel: func() { canceled++; sub.Cancel() }}, nil
	}, nil)
	in := unboundedReplay()
	in.Pattern = "owned.*"
	in.Since = 0
	in.Until = 2
	var got []int64
	err = s.Stream(context.Background(), in, func(ev *event.Event) error { got = append(got, ev.Seq); return nil }, func() <-chan struct{} { t.Fatal("bounded started watcher"); return nil })
	if err != nil || !reflect.DeepEqual(got, []int64{0, 1}) || canceled != 1 {
		t.Fatal(err, got, canceled)
	}
}

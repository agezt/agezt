// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"time"
)

type Subscription struct {
	Events  <-chan *event.Event
	Dropped func() uint64
	Cancel  func()
}
type DropTicker struct {
	C    <-chan time.Time
	Stop func()
}
type Stream struct {
	replay    *Replay
	subscribe func(string, int) (Subscription, error)
	ticker    func() DropTicker
}

func NewStream(journal Journal, subscribe func(string, int) (Subscription, error), ticker func() DropTicker) *Stream {
	if ticker == nil {
		ticker = func() DropTicker { t := time.NewTicker(time.Second); return DropTicker{C: t.C, Stop: t.Stop} }
	}
	return &Stream{replay: NewReplay(journal), subscribe: subscribe, ticker: ticker}
}

type ReplayError struct{ Cause error }

func (e ReplayError) Error() string { return "pulse replay: " + e.Cause.Error() }
func (e ReplayError) Unwrap() error { return e.Cause }

var ErrSubscriptionClosed = errors.New("pulse: subscription closed")

// Stream keeps the native event-only lifetime: clean cancellation/disconnect or
// live write failure ends silently; replay/subscribe/closed-subscription errors
// retain their separate framing policy in the transport adapter.
func (s *Stream) Stream(ctx context.Context, in ReplayInput, emit func(*event.Event) error, clientGone func() <-chan struct{}) error {
	sub, err := s.subscribe(in.Pattern, 4096)
	if err != nil {
		return err
	}
	defer sub.Cancel()
	replayOnly := in.Until >= 0 || in.UntilTSMS >= 0
	lastReplayed := int64(-1)
	if in.Since >= 0 || in.SinceTSMS >= 0 || replayOnly {
		out, err := s.replay.Replay(ctx, in, emit)
		if err != nil {
			return ReplayError{Cause: err}
		}
		lastReplayed = out.LastWritten
	}
	if replayOnly {
		return nil
	}
	var gone <-chan struct{}
	if clientGone != nil {
		gone = clientGone()
	}
	ticker := s.ticker()
	defer ticker.Stop()
	var lastDropped uint64
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				return ErrSubscriptionClosed
			}
			if lastReplayed >= 0 && !ev.IsEphemeral() && ev.Seq <= lastReplayed {
				continue
			}
			if in.Kinds != nil {
				if _, want := in.Kinds[ev.Kind]; !want {
					continue
				}
			}
			if in.Correlation != "" && ev.CorrelationID != in.Correlation {
				continue
			}
			if err := emit(ev); err != nil {
				return nil
			}
		case <-ticker.C:
			now := sub.Dropped()
			if now > lastDropped {
				delta := now - lastDropped
				lastDropped = now
				payload, _ := json.Marshal(map[string]any{"dropped_since_last_notice": delta, "dropped_total": now})
				notice := &event.Event{Subject: "agezt.pulse.dropped", Kind: event.KindPulseDropped, Actor: "agezt", Payload: payload}
				if err := emit(notice); err != nil {
					return nil
				}
			}
		case <-gone:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

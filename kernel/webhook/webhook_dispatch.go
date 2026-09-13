// SPDX-License-Identifier: MIT

package webhook

// Webhook delivery mechanics: run + deliver + post +
// newDeliveryRequest + journal + backoff. Carved out of
// webhook.go during the Day 190 god-file split so the main file
// can stay focused on types + lifecycle (Sink/Dispatcher/Option/
// NewDispatcher/Start) and the helpers file can stay focused on
// signing + parsing + probing.
// Public API unchanged.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

func (d *Dispatcher) run(ctx context.Context, sink Sink, sub *bus.Subscription) {
	defer sub.Cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			// Never deliver our own delivery-result events — that would loop.
			if strings.HasPrefix(string(ev.Kind), "webhook.") {
				continue
			}
			d.deliver(ctx, sink, ev)
		}
	}
}

// deliver POSTs one event to one sink, retrying on error/non-2xx up to
// MaxAttempts, then journals the outcome.
func (d *Dispatcher) deliver(ctx context.Context, sink Sink, ev *event.Event) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	max := d.MaxAttempts
	if max <= 0 {
		max = DefaultMaxAttempts
	}
	deliveryID := ev.ID // stable per (event); receivers dedupe on it

	var lastErr string
	var status int
	for attempt := 1; attempt <= max; attempt++ {
		status, err = d.post(ctx, sink, body, ev, deliveryID)
		if err == nil && status >= 200 && status < 300 {
			d.journal(event.KindWebhookDelivered, ev, map[string]any{
				"url": sink.URL, "subject": ev.Subject, "event_id": ev.ID,
				"event_kind": string(ev.Kind), "status": status, "attempts": attempt,
			})
			fmt.Fprintf(d.log, "webhook: delivered %s (%s) → %s [%d]\n", ev.Kind, ev.ID, sink.URL, status)
			return
		}
		if err != nil {
			lastErr = err.Error()
		} else {
			lastErr = fmt.Sprintf("status %d", status)
		}
		if attempt < max {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.backoff(attempt)):
			}
		}
	}
	d.journal(event.KindWebhookFailed, ev, map[string]any{
		"url": sink.URL, "subject": ev.Subject, "event_id": ev.ID,
		"event_kind": string(ev.Kind), "error": lastErr, "attempts": max,
	})
	fmt.Fprintf(d.log, "webhook: FAILED %s (%s) → %s after %d attempts: %s\n", ev.Kind, ev.ID, sink.URL, max, lastErr)
}

func (d *Dispatcher) post(ctx context.Context, sink Sink, body []byte, ev *event.Event, deliveryID string) (int, error) {
	req, err := newDeliveryRequest(ctx, sink, body, ev, deliveryID)
	if err != nil {
		return 0, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10)) // drain to reuse conn
	return resp.StatusCode, nil
}

// newDeliveryRequest builds the signed POST a delivery sends. Shared by the live
// dispatcher (post) and the daemon-free Probe so a `agt webhook test` carries the
// byte-identical body, headers, and HMAC signature a real delivery would — the
// test is only meaningful if it mirrors reality exactly.
func newDeliveryRequest(ctx context.Context, sink Sink, body []byte, ev *event.Event, deliveryID string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sink.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "agezt-webhook/1")
	req.Header.Set("X-Agezt-Event", string(ev.Kind))
	req.Header.Set("X-Agezt-Subject", ev.Subject)
	req.Header.Set("X-Agezt-Delivery", deliveryID)
	if sink.Secret != "" {
		req.Header.Set("X-Agezt-Signature", "sha256="+sign(sink.Secret, body))
	}
	return req, nil
}

func (d *Dispatcher) journal(kind event.Kind, ev *event.Event, payload map[string]any) {
	if d.pub == nil {
		return
	}
	_, _ = d.pub.Publish(event.Spec{
		Subject:       "webhook." + verb(kind),
		Kind:          kind,
		Actor:         "webhook",
		CorrelationID: ev.CorrelationID, // tie delivery back to the originating run
		CausationID:   ev.ID,
		Payload:       payload,
	})
}

func (d *Dispatcher) backoff(attempt int) time.Duration {
	if d.Backoff != nil {
		return d.Backoff(attempt)
	}
	return time.Duration(attempt) * 250 * time.Millisecond
}

// sign returns the hex HMAC-SHA256 of body under secret.

// SPDX-License-Identifier: MIT

// Package webhook is the outbound-webhook dispatcher (ROADMAP P7-API-02): it
// subscribes to the journal bus and POSTs matching events to operator-configured
// HTTP endpoints, so external systems learn what Agezt is doing in real time
// (a run completed, an approval is pending, the system halted). This is the
// proactive-to-the-outside-world counterpart of the inbound API surfaces.
//
// Each sink is a (URL, subject-pattern, secret) triple. The subject pattern is a
// normal bus pattern, so matching reuses the bus verbatim. When a secret is set,
// the POST body is signed with HMAC-SHA256 (X-Agezt-Signature: sha256=<hex>) so
// the receiver can verify authenticity. Each delivery's outcome is journaled
// (webhook.delivered / webhook.failed); the dispatcher never re-delivers its own
// webhook.* events, so there is no feedback loop.
//
// Security (SPEC-06): the operator chooses the endpoints; bodies carry whatever
// the journal already holds. Secrets are never logged.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// DefaultMaxAttempts bounds delivery retries per event.
const DefaultMaxAttempts = 3

// DefaultTimeout caps one HTTP POST.
const DefaultTimeout = 10 * time.Second

// Sink is one configured webhook destination.
type Sink struct {
	URL     string // http(s) endpoint
	Subject string // bus subject pattern to match (default ">")
	Secret  string // HMAC-SHA256 signing key; empty = unsigned
}

// Publisher is the slice of the bus the dispatcher needs to journal delivery
// outcomes. *bus.Bus satisfies it.
type Publisher interface {
	Publish(spec event.Spec) (*event.Event, error)
}

// Dispatcher fans journal events out to the configured sinks.
type Dispatcher struct {
	bus    *bus.Bus
	pub    Publisher
	sinks  []Sink
	client *http.Client
	log    io.Writer

	// MaxAttempts overrides DefaultMaxAttempts when > 0.
	MaxAttempts int
	// Backoff returns the delay before retry attempt n (1-based: the delay
	// after the first failure is Backoff(1)). Default: 250ms * n. Tests set a
	// zero backoff.
	Backoff func(attempt int) time.Duration
}

// NewDispatcher builds a Dispatcher. b is both the event source (Subscribe) and,
// via Publisher, the audit sink (Publish). log receives one line per delivery
// result (nil = discard).
func NewDispatcher(b *bus.Bus, sinks []Sink, log io.Writer) *Dispatcher {
	if log == nil {
		log = io.Discard
	}
	return &Dispatcher{
		bus:    b,
		pub:    b,
		sinks:  sinks,
		client: &http.Client{Timeout: DefaultTimeout},
		log:    log,
	}
}

// Start subscribes each sink and dispatches until ctx is done. One goroutine per
// sink reads its subscription in publish order, so a sink's deliveries are
// serialized (natural backpressure; the bus drops if a sink falls far behind).
func (d *Dispatcher) Start(ctx context.Context) {
	for _, s := range d.sinks {
		sub, err := d.bus.Subscribe(s.Subject, 256)
		if err != nil {
			fmt.Fprintf(d.log, "webhook: bad subject %q for %s: %v\n", s.Subject, s.URL, err)
			continue
		}
		go d.run(ctx, s, sub)
	}
}

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
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func verb(kind event.Kind) string {
	s := string(kind)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// ParseSinks parses the AGEZT_WEBHOOKS spec: a comma-separated list of sinks,
// each "url|subject|secret" (subject and secret optional; subject defaults to
// ">"). Whitespace around fields is trimmed. URLs must be http(s) and contain no
// comma. A malformed entry is a hard error so a misconfigured webhook is caught
// at startup, not silently dropped.
func ParseSinks(spec string) ([]Sink, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var sinks []Sink
	for _, raw := range strings.Split(spec, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		s := Sink{Subject: ">"}
		s.URL = strings.TrimSpace(parts[0])
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			s.Subject = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			s.Secret = strings.TrimSpace(parts[2])
		}
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("webhook: invalid URL %q (need http(s)://host…)", s.URL)
		}
		sinks = append(sinks, s)
	}
	return sinks, nil
}

// TestEventKind is the Kind carried by a probe delivery (agt webhook test). A
// receiver sees it in the X-Agezt-Event header and can recognize a ping rather
// than acting on it as a real event.
const TestEventKind = "webhook.test"

// testEventID is the stable, obviously-synthetic delivery id a probe sends, so a
// receiver that dedupes on X-Agezt-Delivery can tell test pings apart.
const testEventID = "00000000000000000000000000"

// ProbeResult reports the outcome of a single test delivery.
type ProbeResult struct {
	URL     string        `json:"url"`
	Subject string        `json:"subject"`
	Signed  bool          `json:"signed"`
	Status  int           `json:"status"`
	Latency time.Duration `json:"latency_ns"`
	Err     string        `json:"error,omitempty"`
}

// OK reports whether the probe received a 2xx response.
func (r ProbeResult) OK() bool { return r.Err == "" && r.Status >= 200 && r.Status < 300 }

// Probe sends a single synthetic webhook.test event to sink and reports the
// outcome, using the byte-identical body, headers, and HMAC signature the live
// dispatcher uses — so a 2xx here means real deliveries will be accepted too. It
// is the daemon-free verification behind `agt webhook test`: an operator who just
// configured a sink can confirm it is reachable, accepts the format, and (if
// signed) validates the signature, without waiting for a real event to fire.
// Unlike a real delivery it does NOT retry — a test wants the immediate truth,
// not a transient masked by backoff. now stamps the synthetic event; client may
// be nil (a DefaultTimeout client is used).
func Probe(ctx context.Context, sink Sink, now time.Time, client *http.Client) ProbeResult {
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	subject := sink.Subject
	if subject == "" {
		subject = ">"
	}
	res := ProbeResult{URL: sink.URL, Subject: subject, Signed: sink.Secret != ""}
	ev := &event.Event{
		ID:       testEventID,
		TSUnixMS: now.UnixMilli(),
		Subject:  subject,
		Actor:    "agezt",
		Kind:     TestEventKind,
		Payload:  json.RawMessage(`{"test":true,"message":"agezt webhook test probe"}`),
	}
	body, err := json.Marshal(ev)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	req, err := newDeliveryRequest(ctx, sink, body, ev, ev.ID)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	start := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	res.Status = resp.StatusCode
	return res
}

// Describe renders a one-line banner summary of the sinks (secrets redacted).
func Describe(sinks []Sink) string {
	if len(sinks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sinks))
	for _, s := range sinks {
		sig := ""
		if s.Secret != "" {
			sig = " (signed)"
		}
		parts = append(parts, fmt.Sprintf("%s → %s%s", s.Subject, s.URL, sig))
	}
	return fmt.Sprintf("%d sink(s): %s", len(sinks), strings.Join(parts, ", "))
}

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
	"context"
	"fmt"
	"io"
	"net/http"
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

// Option configures a Dispatcher at construction.
type Option func(*Dispatcher)

// WithClient sets the HTTP client used for delivery. The daemon passes a
// netguard-guarded client so a configured sink cannot reach the host's internal
// network / cloud-metadata endpoint (SPEC-06 egress model) unless the operator
// opts that range back in. A nil client is ignored (the default is kept). Keeping
// the guard in the caller leaves this package stdlib-only (no netguard import).
func WithClient(c *http.Client) Option {
	return func(d *Dispatcher) {
		if c != nil {
			d.client = c
		}
	}
}

// NewDispatcher builds a Dispatcher. b is both the event source (Subscribe) and,
// via Publisher, the audit sink (Publish). log receives one line per delivery
// result (nil = discard). By default deliveries use a plain DefaultTimeout client;
// pass WithClient to supply a netguard-guarded one.
func NewDispatcher(b *bus.Bus, sinks []Sink, log io.Writer, opts ...Option) *Dispatcher {
	if log == nil {
		log = io.Discard
	}
	d := &Dispatcher{
		bus:    b,
		pub:    b,
		sinks:  sinks,
		client: &http.Client{Timeout: DefaultTimeout},
		log:    log,
	}
	for _, o := range opts {
		o(d)
	}
	return d
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


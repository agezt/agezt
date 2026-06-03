// SPDX-License-Identifier: MIT

// Package bus is the in-process event bus.
//
// Publishing semantics: every Publish persists the event to the journal
// (which fsyncs) BEFORE notifying any subscriber. This is the
// "durable-before-publish" invariant from TASKS P0-BUS-03 / DECISIONS B0c —
// no subscriber ever sees an event that is not in the chain.
//
// Subscription patterns are dot-separated tokens with two wildcards:
//
//	"*"   matches exactly one token.
//	">"   matches one or more remaining tokens; MUST be the last token.
//
// Examples:
//
//	"agent.spawned"  matches "agent.spawned" only.
//	"agent.*"        matches "agent.spawned", not "agent.01H.tool".
//	"agent.>"        matches "agent.spawned" AND "agent.01H.tool".
//	">"              matches everything.
//
// Subscriber channels are bounded; when a subscriber falls behind, the bus
// drops events for that subscriber and increments its Dropped counter
// rather than blocking publishers. Subscribers must monitor Dropped and
// either widen their buffer or process faster.
package bus

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// DefaultSubBuffer is used when Subscribe is called with buf <= 0.
const DefaultSubBuffer = 256

// Redactor scrubs secrets from a string / byte slice. A *redact.Redactor
// satisfies it; the bus takes the narrow interface so this package stays free of
// a redact dependency.
type Redactor interface {
	Redact(string) string
	RedactBytes([]byte) []byte
}

// Bus routes events from publishers to subscribers, persisting through a
// Journal before notification.
type Bus struct {
	j *journal.Journal

	mu        sync.Mutex
	subs      map[uint64]*subscription
	nextSubID uint64
	closed    bool
	redactor  Redactor // optional; scrubs secrets from payloads before journaling
}

// New creates a Bus that durably-publishes through j.
func New(j *journal.Journal) *Bus {
	return &Bus{j: j, subs: make(map[uint64]*subscription)}
}

// SetRedactor installs a secret redactor applied to every durably-published
// event's payload and tags BEFORE it is hashed and written to the journal, so a
// leaked secret never enters the permanent record. Nil (the default) disables
// redaction. Set once at startup, before the first Publish.
func (b *Bus) SetRedactor(r Redactor) {
	b.mu.Lock()
	b.redactor = r
	b.mu.Unlock()
}

// Redactor returns the installed secret redactor, or nil when redaction is
// disabled. Used by `agt redact test` to exercise the LIVE redactor (built-in
// patterns + configured literal secrets) without leaking which literal matched.
func (b *Bus) Redactor() Redactor {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.redactor
}

// redactSpec returns spec with its payload and tag values scrubbed when a
// redactor is configured. The original payload/tags are not mutated: payload is
// re-marshaled to redacted JSON and tags are copied into a fresh map. Caller
// holds b.mu (so the redactor pointer read is race-free).
func (b *Bus) redactSpecLocked(spec event.Spec) event.Spec {
	if b.redactor == nil {
		return spec
	}
	if spec.Payload != nil {
		// Marshal WITHOUT HTML-escaping (M170): json.Marshal escapes < > & to
		// < > &, which would hide those characters from the literal
		// scrubber — a configured secret containing & / < / > (common in generated
		// passwords and connection strings) would survive into the permanent
		// journal. json.Encoder.SetEscapeHTML(false) keeps them literal so the
		// scrubber sees the real bytes. Encode appends a newline; trim it so the
		// stored payload is byte-identical to the marshaled form.
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(spec.Payload); err == nil {
			raw := bytes.TrimRight(buf.Bytes(), "\n")
			spec.Payload = json.RawMessage(b.redactor.RedactBytes(raw))
		}
	}
	if len(spec.Tags) > 0 {
		nt := make(map[string]string, len(spec.Tags))
		for k, v := range spec.Tags {
			nt[k] = b.redactor.Redact(v)
		}
		spec.Tags = nt
	}
	return spec
}

// subscription is the bus-internal record per subscriber.
type subscription struct {
	pattern []string
	ch      chan *event.Event
	dropped atomic.Uint64
}

// Subscription is the public handle returned to callers.
type Subscription struct {
	// C delivers matched events in publish order. It closes when Cancel
	// is called or the bus is closed.
	C <-chan *event.Event
	// Dropped counts events the bus could not deliver because C was full.
	Dropped *atomic.Uint64
	cancel  func()
}

// Cancel unsubscribes and closes C. Safe to call multiple times.
func (s *Subscription) Cancel() { s.cancel() }

// ErrPattern is returned by Subscribe for malformed patterns.
var ErrPattern = errors.New("bus: bad pattern")

// ErrClosed is returned by Publish and Subscribe after Close.
var ErrClosed = errors.New("bus: closed")

// Subscribe returns a handle for events matching pattern. If buf <= 0,
// DefaultSubBuffer is used.
func (b *Bus) Subscribe(pattern string, buf int) (*Subscription, error) {
	tokens, err := parsePattern(pattern)
	if err != nil {
		return nil, err
	}
	if buf <= 0 {
		buf = DefaultSubBuffer
	}
	sub := &subscription{
		pattern: tokens,
		ch:      make(chan *event.Event, buf),
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrClosed
	}
	id := b.nextSubID
	b.nextSubID++
	b.subs[id] = sub
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if s, ok := b.subs[id]; ok && s == sub {
				delete(b.subs, id)
				close(sub.ch)
			}
			b.mu.Unlock()
		})
	}
	return &Subscription{C: sub.ch, Dropped: &sub.dropped, cancel: cancel}, nil
}

// Publish appends the event to the journal (fsyncs) and then notifies every
// subscriber whose pattern matches spec.Subject. The append is the
// durable-before-publish point; if the journal returns an error, no
// subscriber sees the event.
func (b *Bus) Publish(spec event.Spec) (*event.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}

	spec = b.redactSpecLocked(spec)
	e, err := b.j.Append(spec)
	if err != nil {
		return nil, err
	}
	subjectTokens := strings.Split(spec.Subject, ".")
	for _, sub := range b.subs {
		if !matches(sub.pattern, subjectTokens) {
			continue
		}
		select {
		case sub.ch <- e:
		default:
			sub.dropped.Add(1)
		}
	}
	return e, nil
}

// PublishStreaming fans out an ephemeral event to matching subscribers
// WITHOUT persisting it to the journal. Use only for high-rate
// display-only signals (LLM token chunks via KindLLMToken) where
// the durable record lives elsewhere — the full assembled text and
// usage land in the regular llm.response event published right
// after the stream completes.
//
// Ephemeral events have Hash="" — subscribers that care about the
// durable chain can filter them out with `if !ev.IsEphemeral() { ... }`.
// The CLI's `agt run` renderer special-cases ev.Kind==KindLLMToken
// to print payload text inline.
//
// Why not durable? Streaming a 5KB response can produce 200+ token
// chunks. Persisting each as a chain-linked journal entry would
// 5× the journal volume for no audit benefit; the assembled
// llm.response already carries the canonical output. This method
// is the explicit escape hatch.
func (b *Bus) PublishStreaming(spec event.Spec) (*event.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	e, err := event.NewEphemeral(spec)
	if err != nil {
		return nil, err
	}
	subjectTokens := strings.Split(spec.Subject, ".")
	for _, sub := range b.subs {
		if !matches(sub.pattern, subjectTokens) {
			continue
		}
		select {
		case sub.ch <- e:
		default:
			sub.dropped.Add(1)
		}
	}
	return e, nil
}

// Close cancels every subscription, prevents future Publish calls, and
// returns. Idempotent.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, sub := range b.subs {
		close(sub.ch)
		delete(b.subs, id)
	}
}

// MatchSubject reports whether subject matches pattern using the
// bus's NATS-style wildcard rules. Exported for callers (notably
// the controlplane's pulse historical-replay path in M1.aa) that
// need to filter journal events by the same pattern the live
// subscription uses, without going through the full Subscribe
// machinery. Returns false on a malformed pattern rather than
// erroring; pulse's caller validates patterns at subscribe time.
func MatchSubject(pattern, subject string) bool {
	pat, err := parsePattern(pattern)
	if err != nil {
		return false
	}
	return matches(pat, strings.Split(subject, "."))
}

// matches reports whether subject matches pattern. Both are pre-tokenized.
func matches(pattern, subject []string) bool {
	pi, si := 0, 0
	for pi < len(pattern) && si < len(subject) {
		tok := pattern[pi]
		if tok == ">" {
			return true // consumes all remaining subject tokens
		}
		if tok != "*" && tok != subject[si] {
			return false
		}
		pi++
		si++
	}
	if pi == len(pattern) && si == len(subject) {
		return true
	}
	// Pattern still has tokens but subject exhausted: only "> with nothing
	// behind it" doesn't make sense, and "*" requires at least one segment.
	// So only acceptable case is pattern fully consumed already (handled).
	return false
}

// ValidatePattern reports whether p is a well-formed NATS-style subject pattern
// (non-empty, no empty tokens, '>' only as the final token), returning ErrPattern-
// wrapped detail otherwise. Exported so config parsers (e.g. webhook sink subject
// filters) can reject a malformed pattern at startup instead of silently never
// matching it at delivery time.
func ValidatePattern(p string) error {
	_, err := parsePattern(p)
	return err
}

func parsePattern(p string) ([]string, error) {
	if p == "" {
		return nil, fmt.Errorf("%w: empty", ErrPattern)
	}
	tokens := strings.Split(p, ".")
	for i, t := range tokens {
		if t == "" {
			return nil, fmt.Errorf("%w: empty token in %q", ErrPattern, p)
		}
		if t == ">" && i != len(tokens)-1 {
			return nil, fmt.Errorf("%w: %q: '>' must be the last token", ErrPattern, p)
		}
	}
	return tokens, nil
}

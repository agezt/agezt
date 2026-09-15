// SPDX-License-Identifier: MIT
//
// Bus publish methods: Publish (durable-before-notify, the canonical entry
// point) and PublishStreaming (ephemeral fan-out for high-rate signals like
// LLM token chunks). Split from bus.go during Day 211 god-file refactor (#33).
// Public API unchanged.
package bus

import (
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

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
// Secret redaction (M418): streaming deltas are scrubbed with the same redactor
// as Publish before fan-out. They never hit the journal, but they DO reach every
// subscriber — including the outbound webhook dispatcher (default `>` subject),
// the pulse stream, the OpenAI-compat relay, and the web UI — so a credential the
// model echoes mid-stream must not egress unredacted. Redaction is a pure
// deterministic transform, harmless to display.
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
	spec = b.redactSpecLocked(spec)
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

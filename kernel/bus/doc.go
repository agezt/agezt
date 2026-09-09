// SPDX-License-Identifier: MIT

// Package bus is the in-process pub/sub that fans events out to
// subscribers. It enforces the project's durable-before-publish
// invariant: every Publish call writes to the journal (which fsyncs)
// BEFORE notifying any subscriber, so a subscriber can never see an
// event that isn't in the chain. Subjects are NATS-style
// dot-separated with `*` (one token) and `>` (tail) wildcards.
// PublishStreaming is the ephemeral, non-journaled path used for
// LLM token deltas. A pluggable Redactor scrubs secrets from both
// payload and tag values before they hit the journal or the wire.
package bus

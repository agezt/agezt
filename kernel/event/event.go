// SPDX-License-Identifier: MIT

// Package event defines the canonical Agezt Event type, deterministic
// encoding for hashing, and the BLAKE3 hash-chain primitive.
//
// "Everything is an event" (BUILD-GUIDE §0): every meaningful kernel action
// is journaled here. Events are immutable; corrections are inverse events
// appended later (DECISIONS B0c — log is the audit/replay/revert truth).
//
// Hash chain (DECISIONS B3):
//
//	hash = BLAKE3-256( prev_hash_bytes || canonical_json_bytes )
//
// where canonical_json_bytes is the deterministic JSON encoding of the
// event with its Hash field empty (omitempty drops it). The first event
// in a journal uses GenesisHash for prev_hash.
//
// Note on DECISIONS B3 wording: B3 was written against the now-superseded
// protobuf wire format ("protobuf serialized with deterministic field
// ordering"). Per the foundational revision DECISIONS B0, the wire format
// is JSON; the deterministic-bytes requirement carries over. Go's
// encoding/json sorts map keys and respects struct field declaration order,
// giving us a deterministic encoding for free as long as struct field order
// is treated as part of the contract.
package event

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"lukechampine.com/blake3"
)

// HashSize is the byte length of a BLAKE3-256 digest.
const HashSize = 32

// HashHexLen is the length of a hex-encoded BLAKE3-256 digest.
const HashHexLen = HashSize * 2

// GenesisHash is the prev_hash value for the first event in a chain — 32
// zero bytes, hex-encoded.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Spec is the caller-provided portion of an event. The kernel (specifically
// the journal) supplies ID, Seq, TSUnixMS, PrevHash and computes Hash.
type Spec struct {
	Subject       string            // hierarchical routing key (e.g. "agent.01H...tool")
	Kind          Kind              // event-kind discriminator
	Actor         string            // who emitted (agent ID, plugin ID, "kernel", ...)
	CorrelationID string            // ties a multi-event causal chain (optional)
	CausationID   string            // the event that caused this one (optional)
	Payload       any               // JSON-marshalable; nil → omitted
	Tags          map[string]string // free-form labels (optional)
}

// Event is the canonical kernel event.
//
// FIELD ORDER IS PART OF THE WIRE CONTRACT: the canonical JSON encoding
// follows struct field declaration order, and the hash depends on that
// encoding. Reordering fields would break replay across versions. New
// fields MUST be appended at the end with omitempty (additive, never
// reorder; DECISIONS B1).
type Event struct {
	ID            string            `json:"id"`
	Seq           int64             `json:"seq"`
	TSUnixMS      int64             `json:"ts_unix_ms"`
	PrevHash      string            `json:"prev_hash"`
	Hash          string            `json:"hash,omitempty"`
	Subject       string            `json:"subject"`
	Actor         string            `json:"actor"`
	Kind          Kind              `json:"kind"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	CausationID   string            `json:"causation_id,omitempty"`
	Payload       json.RawMessage   `json:"payload,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// ErrHashMismatch is returned by VerifyHash when the stored hash does not
// match what the canonical bytes hash to.
var ErrHashMismatch = errors.New("event: hash mismatch")

// New constructs a fresh event from a Spec, assigning identity fields and
// computing Hash. The caller (typically the journal) supplies id, seq,
// ts, and the chain-tail prevHash.
//
// Validation: subject, kind, actor, and prevHash are required; prevHash
// must be 64 hex characters.
func New(spec Spec, id string, seq int64, ts time.Time, prevHash string) (*Event, error) {
	if spec.Subject == "" {
		return nil, errors.New("event: subject required")
	}
	if spec.Kind == "" {
		return nil, errors.New("event: kind required")
	}
	if spec.Actor == "" {
		return nil, errors.New("event: actor required")
	}
	if len(prevHash) != HashHexLen {
		return nil, fmt.Errorf("event: prev_hash must be %d hex chars, got %d", HashHexLen, len(prevHash))
	}
	if _, err := hex.DecodeString(prevHash); err != nil {
		return nil, fmt.Errorf("event: prev_hash not hex: %w", err)
	}

	payload, err := marshalPayload(spec.Payload)
	if err != nil {
		return nil, fmt.Errorf("event: payload: %w", err)
	}

	e := &Event{
		ID:            id,
		Seq:           seq,
		TSUnixMS:      ts.UnixMilli(),
		PrevHash:      prevHash,
		Subject:       spec.Subject,
		Actor:         spec.Actor,
		Kind:          spec.Kind,
		CorrelationID: spec.CorrelationID,
		CausationID:   spec.CausationID,
		Payload:       payload,
		Tags:          spec.Tags,
	}
	h, err := e.computeHash()
	if err != nil {
		return nil, err
	}
	e.Hash = h
	return e, nil
}

// NewEphemeral constructs an event suitable for display-only fan-out
// via bus.PublishStreaming. It bypasses the hash chain entirely:
// Seq=0, PrevHash="", Hash="". Subscribers can detect ephemeral
// events with `if ev.Seq == 0 { ... }`.
//
// Used for high-rate signals (LLM token chunks via KindLLMToken)
// where the durable record lives in a sibling event. Do NOT use
// for anything an auditor or replay would need; ephemeral events
// disappear when the daemon restarts.
//
// Validation mirrors New() except no prevHash check: subject, kind,
// actor are still required. ID is generated by the caller (typically
// via ulid.New) so tests can pin it.
func NewEphemeral(spec Spec) (*Event, error) {
	if spec.Subject == "" {
		return nil, errors.New("event: subject required")
	}
	if spec.Kind == "" {
		return nil, errors.New("event: kind required")
	}
	if spec.Actor == "" {
		return nil, errors.New("event: actor required")
	}
	payload, err := marshalPayload(spec.Payload)
	if err != nil {
		return nil, fmt.Errorf("event: payload: %w", err)
	}
	return &Event{
		// Seq=0, PrevHash="", Hash="" all left at zero values —
		// the "ephemeral" signal.
		TSUnixMS:      time.Now().UnixMilli(),
		Subject:       spec.Subject,
		Actor:         spec.Actor,
		Kind:          spec.Kind,
		CorrelationID: spec.CorrelationID,
		CausationID:   spec.CausationID,
		Payload:       payload,
		Tags:          spec.Tags,
	}, nil
}

// IsEphemeral reports whether the event was created via NewEphemeral
// (i.e. never journaled). Hash=="" is the discriminator: every event
// that flows through New() — and therefore through journal.Append —
// has a computed BLAKE3 hash; only ephemerals skip that step.
// (Don't use Seq for this — the journal's first durable event has
// Seq=0, so it's not a reliable ephemeral signal.)
func (e *Event) IsEphemeral() bool {
	return e.Hash == ""
}

// Canonical returns the deterministic JSON encoding used for hashing. The
// Hash field is forced empty so it is dropped by omitempty.
func (e *Event) Canonical() ([]byte, error) {
	clone := *e
	clone.Hash = ""
	return json.Marshal(&clone)
}

// computeHash returns the hex-encoded BLAKE3-256 hash chained from PrevHash.
func (e *Event) computeHash() (string, error) {
	canonical, err := e.Canonical()
	if err != nil {
		return "", err
	}
	prevBytes, err := hex.DecodeString(e.PrevHash)
	if err != nil {
		return "", fmt.Errorf("event: prev_hash not hex: %w", err)
	}
	h := blake3.New(HashSize, nil)
	_, _ = h.Write(prevBytes)
	_, _ = h.Write(canonical)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyHash recomputes the hash and compares it to e.Hash. Returns
// ErrHashMismatch (wrapped) on disagreement.
func (e *Event) VerifyHash() error {
	want, err := e.computeHash()
	if err != nil {
		return err
	}
	if want != e.Hash {
		return fmt.Errorf("%w: event %s: got %s, want %s", ErrHashMismatch, e.ID, e.Hash, want)
	}
	return nil
}

// Decode parses a JSON-encoded event line (one event per line, JSONL
// segment format used by the journal).
func Decode(line []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(line, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func marshalPayload(p any) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	if rm, ok := p.(json.RawMessage); ok {
		// Copy: returning the caller's slice directly would let a later mutation of
		// it silently diverge e.Payload from the Hash already computed over these
		// bytes, breaking VerifyHash on a journal round-trip. (M482)
		return append(json.RawMessage(nil), rm...), nil
	}
	return json.Marshal(p)
}

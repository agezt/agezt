// SPDX-License-Identifier: MIT

// Package event is the canonical event spec for every subsystem in the
// kernel: identity (ULID, sequence, BLAKE3 hash), routing (subject,
// actor, correlation/cause), and a kind-tagged payload. The event type
// is the single shape that flows through both the durable append-only
// journal (kernel/journal) and the in-process bus (kernel/bus); every
// other kernel package's event types are projected from it. The Kind
// enum (event/kinds.go) is append-only and is the only place new event
// types are introduced.
//
// "Everything is an event" (BUILD-GUIDE §0): every meaningful kernel action
// is journaled here. Events are immutable; corrections are inverse events
// appended later (DECISIONS B0c — the log is the audit / replay / revert
// truth).
//
// Hash chain (DECISIONS B3): hash = BLAKE3-256( prev_hash_bytes ||
// canonical_json_bytes ), where canonical_json_bytes is the deterministic
// JSON encoding of the event with its Hash field empty (omitempty drops
// it). The first event in a journal uses GenesisHash for prev_hash.
//
// A note on DECISIONS B3's wording: B3 was written against the
// now-superseded protobuf wire format ("protobuf serialized with
// deterministic field ordering"). Per the foundational revision DECISIONS
// B0 the wire format is JSON, and the deterministic-bytes requirement
// carries over. Go's encoding/json sorts map keys and respects struct
// field declaration order, which gives a deterministic encoding for free
// as long as struct field order is treated as part of the contract.
package event

// SPDX-License-Identifier: MIT

// Package event is the canonical event spec for every subsystem in the
// kernel: identity (ULID, sequence, BLAKE3 hash), routing (subject,
// actor, correlation/cause), and a kind-tagged payload. The event type
// is the single shape that flows through both the durable append-only
// journal (kernel/journal) and the in-process bus (kernel/bus); every
// other kernel package's event types are projected from it. The Kind
// enum (event/kinds.go) is append-only and is the only place new event
// types are introduced.
package event

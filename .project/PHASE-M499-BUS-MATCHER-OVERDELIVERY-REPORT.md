# M499 — Mutation testing the bus: pin the subject-matcher over-delivery edge

## Context
Tenth package in the mutation pass: `kernel/bus`, the event-sourcing backbone
(publish, durable journaling + redaction, subscriber fan-out, NATS-style subject
pattern matching). Run with `GOMAXPROCS=3` (CPU-capped per operator feedback). Score
0.722. Tag-value redaction is already covered (`TestRedactor_ScrubsBeforeJournal`); the
genuine gap was in subject matching.

## The genuine gap (closed)
`matches(pattern, subject)` ends with
`if pi == len(pattern) && si == len(subject) { return true }` — a match requires BOTH
the pattern AND the subject to be fully consumed. The mutation
`pi == len(pattern) → true` (dropping the "pattern fully consumed" half) **survived**.

Effect: a pattern with **more tokens than the subject**, where the subject is a prefix
of the pattern, would wrongly match — e.g. `matches("a.b.c", "a.b")` returns true. In
delivery terms a subscriber to a *more specific* subject (`agent.spawned.detail`) would
receive *shorter* events (`agent.spawned`) — **over-delivery / cross-subscription
leakage**.

The existing `TestPatternMatching` table covered exact-length, shorter-pattern, and
`>`/`*` wildcard cases, but had **no case where the pattern is longer than the
subject**, so the mutant went undetected.

## Fix
Extended `TestPatternMatching` with the missing direction:
- `{"agent.spawned.detail", "agent.spawned", false}` (literal, longer pattern)
- `{"agent.*.tool", "agent.01H", false}` (wildcard, longer pattern)
- `{"a.b.c", "a.b", false}` (generic prefix case)

## Negative control (manual, CPU-capped)
Applying the survivor (`pi == len(pattern) && … → true && …`) makes the table fail
with `matches("a.b.c", "a.b") = true, want false`; restored byte-for-byte
(`git diff --ignore-all-space` on bus.go empty); passes again. First confirmed on clean
code that the match is correctly false (throwaway probe, removed).

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — ten packages (M490–M499)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus —
plus the controlplane primary-token auth gate verified solid. Genuine gaps closed where
they existed (redact, journal, edict, creds-legacy-KDF, warden-blank-argv0,
governor-spend-boundary, scheduler-correlation-id, bus-matcher-over-delivery); the rest
verified solid. The event routing/fan-out core is now pinned against the over-delivery
edge.

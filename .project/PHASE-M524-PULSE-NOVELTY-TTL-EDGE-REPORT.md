# M524 — Mutation testing pulse: pin the novelty-suppression TTL upper edge

## Context
Follow-up on the `kernel/pulse` pass (M523), closing the survivor explicitly noted there:
`seenRecently`'s novelty-window upper boundary. Run with `GOMAXPROCS=3` (CPU-capped).

## The genuine gap (closed)
```
age := s.now().UnixMilli() - e.LastMS
return age >= 0 && age < s.noveltyTTL.Milliseconds()
```

Novelty suppression drops a delta if the same issue was surfaced within `noveltyTTL`. The
engine test only proves an *immediate* repeat is suppressed, never the edge — so
`age < noveltyTTL → <=` survived: under it an entry whose age is *exactly* the TTL would
be kept suppressed for one extra millisecond instead of being treated as stale and allowed
to re-surface. (The `age >= 0` lower guard, which rejects a clock that went backwards, is
already pinned by the engine novelty test.)

## Fix
Added `TestSeenRecently_TTLBoundary` (state + injected clock): `MarkSeen` at `base`, then
- at `base + TTL` (age == TTL) → NOT suppressed (stale, may re-surface);
- at `base + TTL - 1ms` (age == TTL-1) → still suppressed.

## Negative control (manual, CPU-capped)
`age < noveltyTTL → <=`: FAIL (the exactly-at-TTL entry stays suppressed). Restored
byte-for-byte (`git diff --ignore-all-space` on salience.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Result
`kernel/pulse` salience now has both its score→band mapping (M523) and its novelty TTL
window (M524) pinned at every inclusive edge. Mutation pass: thirty-three packages
(M490–M524); pulse counted once.

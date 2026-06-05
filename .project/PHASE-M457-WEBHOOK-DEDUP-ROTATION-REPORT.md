# M457 — Webhook replay-guard: two-generation rotation (no wholesale flush)

## Context
The inbound webhook channel guards against replay of a captured signed body with
two layers (`webhook.go` handleInbound):
1. **Freshness** — reject a stale `ts_ms`. This is **conditional**: it only runs
   when the client sends a timestamp (`if env.TSMS != 0`).
2. **Dedup** — reject a repeated `channel_id:id` via `dedup.seenBefore`.

When the client omits `ts_ms` (TSMS == 0), layer 1 is skipped entirely, so the
dedup set is the **sole** replay protection.

## The weakness
`dedup.seenBefore` bounded memory by clearing the whole set when it filled:

```go
if len(d.seen) >= d.cap {
    d.seen = make(map[string]struct{}, d.cap) // forgets every recent id at once
}
```

A wholesale clear forgets every recently-seen id in one shot. A captured, validly
signed body whose id was already seen can then be replayed immediately after a
flush and is accepted as new — within the freshness window if `ts_ms` was sent, or
unconditionally if it was not. The flush can be reached by normal traffic filling
the 4096-entry set. Low severity (the body must carry a valid HMAC, i.e. a true
replay rather than a forgery), but it is a real weakening of the replay guard, of
the same class as the governor usage-index drop fixed in M456.

## The fix
Two-generation rotation (live + previous), memory bounded at 2×cap:

- `seenBefore` reports a hit if the key is in **either** generation.
- When the live set fills, it rotates to become the previous generation and a
  fresh live set starts (no wholesale forget).
- A key is dropped only after it ages out of BOTH generations — roughly doubling
  the number of distinct recent ids the guard remembers, with bounded memory.

No protocol or API change; purely a stronger, still-bounded eviction.

## Test + negative control
`plugins/channels/webhook/webhook_test.go`:
`TestDedup_ReplaySurvivesRotation` — sees ids `a,b,c` (fills cap=3), a fourth id
`d` forces a rotation, then asserts replaying `a` (seen before the rotation) is
still caught, and a brand-new id `z` is not a false positive. Existing
`TestInbound_DedupesRepeatedID` still passes.

**Negative control:** removing the `d.prev = d.seen` rotation line (restoring the
wholesale clear) made the test report `a was seen before the rotation; a replay
must still be caught` — `--- FAIL`. Restored; test passes.

## Verification / gate
- `plugins/channels/webhook` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.

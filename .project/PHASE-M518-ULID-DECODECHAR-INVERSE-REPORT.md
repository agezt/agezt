# M518 — Mutation testing ulid: pin decodeChar as the exact inverse of the alphabet

## Context
Twenty-eighth package in the mutation pass: `kernel/ulid` (the kernel's ULID source —
128-bit sortable IDs, Crockford base32, generate/validate/timestamp-extract). Run with
`GOMAXPROCS=3` (CPU-capped). go-mutesting score 0.583, 284 survivors (the bulk in the
dense bit-shifting `encode`, much of it equivalent); working tree restored clean.

## The genuine gap (closed)
`decodeChar` is the Crockford reverse map that `Timestamp` relies on to recover the 48-bit
millisecond prefix:

```
case c >= 'A' && c <= 'H': return int(c-'A') + 10, true
case c == 'J': return 18, true   // K=19, M=20, N=21
case c >= 'P' && c <= 'T': return int(c-'P') + 22, true
case c == 'V': return 27, true
case c >= 'W' && c <= 'Z': return int(c-'W') + 28, true
```

The existing tests only ever decode the handful of characters that appear in their fixed
timestamp vectors (`TestTimestamp_Roundtrip` uses one timestamp; `TestValidate` checks
only accept/reject, never the value). So most of decodeChar's *return values* were
unpinned. Hand-applied negative control against the existing suite confirmed survivors:
- `'P'..'T' +22 → +23` — survived.
- `'W'..'Z' +28 → +29` — survived.
- `J 18 → 19`, `V 27 → 26` — survived.
(The `'A'..'H' +10` offset and the range edges like `<= 'H'` were already killed, because
those characters appear in fresh-ULID validation / the test timestamp.)

A wrong decode value silently corrupts `Timestamp()` for any ULID whose 48-bit timestamp
encodes one of those characters — i.e. a large fraction of real IDs, depending on the
millisecond — so `agt why` / time-based queries would report the wrong instant.

## Fix
Added `TestDecodeChar_InverseOfAlphabet` (`package ulid`, internal): for every index `i`,
`decodeChar(crockfordAlphabet[i])` must return `(i, true)` — the decode table is the exact
inverse of the encode alphabet. Also asserts the Crockford exclusions (`I L O U`, lowercase,
symbols) are rejected, never aliased.

## Negative control (manual, CPU-capped)
`P–T +22→+23`, `W–Z +28→+29`, `J 18→19`, `V 27→26`, and an `I → accepted` alias each FAIL
under the new test. Restored byte-for-byte (`git diff --ignore-all-space` on ulid.go
empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-eight packages (M490–M518)
…restapi, acp, state, planner, ulid — plus the controlplane primary-token auth gate
verified solid. The encode path's residual survivors are the dense bit-mask shifts
(largely equivalent); the closeable gap was the decode table's value correctness, which
feeds timestamp extraction.

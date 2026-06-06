# M519 — Mutation testing artifact: content-addressed store verified solid (no gap)

## Context
Twenty-ninth package in the mutation pass: `kernel/artifact` (the content-addressed
BLAKE3 blob store — `Put`/`Get`/`Has`/`Size`, atomic temp+rename write, content
re-verification on read, and the `validRef` path-traversal guard). Run with `GOMAXPROCS=3`
(CPU-capped). go-mutesting score 0.596, 31 survivors; working tree restored clean.

## Finding: no genuine gap — survivors are equivalent
Every semantically-meaningful operator was mutated by hand and run against the existing
tests (the reliable negative-control method). **All were killed:**

`validRef` (the security core — only thing turning a caller string into a path):
- `len(ref) != refLen → ==` — killed (wrong-length refs in `TestBadRefRejected`; valid
  64-char refs in the roundtrip).
- hex range edges `c < '0' → <=`, `c > '9' → >=`, `c < 'a' → <=`, `c > 'f' → >=` — all
  killed: the content-derived test refs collectively contain `0`/`9`/`a`/`f`, so rejecting
  any edge char breaks a roundtrip. (`&& → ||` and the inner `|| → &&` are killed by the
  valid-hex roundtrip and the `ZZ…`/traversal rejection cases.)

Other meaningful logic:
- `Get` corrupt-detection `ConstantTimeCompare(...) != 1 → == 1` — killed
  (`TestGetCorruptIsDetected` + roundtrip).
- `Put` dedup skip `os.Stat(path); err == nil → err != nil` — killed (first Put would skip
  the write → `Get` ErrNotFound).
- `pathFor` sharding `ref[:2] → ref[:1]` — killed (a test pins the on-disk sharded layout,
  so the shard width is not a free parameter).

The 31 go-mutesting survivors are therefore **equivalent mutants**: removal of error-path
cleanup (`os.Remove(tmpName)`) and `fmt.Errorf` wrapping where the error still propagates
with the same outcome — unkillable by construction, as on anomaly / netguard / event.

## Why no new test
Adding a test here would pad an already-covered property, not close a gap. Consistent with
M512 (anomaly) and M493 (netguard), this package is recorded as **verified solid**. No
production or test code changed.

## Verification / gate
- No code change; existing `go test ./kernel/artifact/` passes (`GOMAXPROCS=3`).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-nine packages (M490–M519)
…state, planner, ulid, artifact — plus the controlplane primary-token auth gate verified
solid. artifact joins anomaly/netguard/event/controlplane as a package where the security
core and all meaningful logic are already pinned and the residual survivors are equivalent.

# M523 — Mutation testing pulse: pin dispositionForValue band boundaries

## Context
Thirty-third package in the mutation pass: `kernel/pulse` (the proactive resident —
salience scoring, dial routing, novelty suppression, briefings). Larger package
(~1200 LOC across 8 files). Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting score
0.499, 181 survivors (much of it in the engine/observer plumbing); tree restored clean.

## Triage
`Route` (the dial × disposition × quiet-hours matrix) is exhaustively pinned by
`route_test.go`. `seenRecently`'s `age >= 0` lower guard is killed by the engine novelty
test. The clean, high-value pure function with unpinned edges is `dispositionForValue`.

## The genuine gap (closed)
`dispositionForValue` maps an LLM 0..1 score onto a delivery band:

```
case v >= 0.85: return DispAlert
case v >= 0.45: return DispNotify
case v >= 0.20: return DispDigest
default:        return DispDrop
```

The salience/route tests use dispositions directly or via the relevance boost, never
calling `dispositionForValue` at its exact thresholds. So all three `>=` edges survived
`→ >` (confirmed by hand-applied negative control): a score landing *exactly* on a band
edge would drop a notch — an alert silently demoted to a notify, a notify to a digest, a
digest dropped entirely. For the salience component whose whole job is "neither annoying
nor useless," a silent band demotion at the boundary is a real misclassification.

## Fix
Added `TestDispositionForValue_BandBoundaries`: a table hitting each exact edge
(`0.85→Alert`, `0.45→Notify`, `0.20→Digest`) plus the value just below each
(`0.84→Notify`, `0.44→Digest`, `0.19→Drop`) and the extremes.

## Negative control (manual, CPU-capped)
`v >= 0.85 → >`, `v >= 0.45 → >`, `v >= 0.20 → >` each FAIL under the new test. Restored
byte-for-byte (`git diff --ignore-all-space` on salience.go empty); passes again.

## Known remaining survivor (honest note)
`seenRecently`'s upper edge `age < noveltyTTL.Milliseconds()` survives `→ <=` (an entry
exactly at the TTL age would still be suppressed rather than treated as stale) — a
one-millisecond off-by-one in the novelty window. Closing it cleanly needs a state +
clock fixture; deferred as a candidate follow-up rather than rushed here.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-three packages (M490–M523)
…reflect, meshctx, tenantctx, pulse — plus the controlplane primary-token auth gate
verified solid. The salience delivery-band mapping is now pinned at every edge.

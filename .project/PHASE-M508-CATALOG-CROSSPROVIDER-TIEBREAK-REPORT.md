# M508 — Mutation testing catalog: pin the cross-provider down-route tie-break

## Context
Nineteenth package in the mutation pass: `kernel/catalog` (the model catalog —
API/local merge, discovery, tool-capable down-routing). Run with `GOMAXPROCS=3`
(CPU-capped). Score 0.519.

## The genuine gap (closed)
`ToolCapableAlternativeAmong` (types.go) finds a tool-capable substitute for a model.
Pass 2 widens to other eligible providers and selects the best by context, tie-broken
by id ascending:

```
if ctx > bestCtx || (ctx == bestCtx && id < bestID) { bestID, bestCtx = id, ctx }
```

The single-provider tie-break (line 448) is covered by
`TestToolCapableAlternative_TieBreaksByID`, but the **cross-provider** selection
(line 420) was only tested for largest-context — no case had two eligible cross
providers with EQUAL context. So six mutants on that line survived, including the
tie-break direction (`id < bestID` → `>`) and the context comparison
(`ctx > bestCtx` → `>=`) — either makes the cross-provider down-route non-deterministic
or pick the wrong substitute model.

## Fix
Extended `downroute_test.go` with
`TestToolCapableAlternativeAmong_TieBreaksByIDAcrossProviders`: two eligible cross
providers each offering a tool-capable model of equal context (64000), run in two
arrangements — the lowest model id in the earlier provider and in the later provider
(`ProviderList()` is sorted by provider id, so both are deterministic). Both must select
the lowest id (`alpha`).

## Negative control (manual, CPU-capped)
- `id < bestID → id > bestID`: FAIL (picks the larger id).
- `ctx > bestCtx → ctx >= bestCtx`: FAIL (a later equal-context provider overrides).
Restored byte-for-byte (`git diff --ignore-all-space` on types.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — nineteen packages (M490–M508)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog — plus
the controlplane primary-token auth gate verified solid. The single-provider down-route
path was already tie-break-tested; this closes the same property on the cross-provider
path.

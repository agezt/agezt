# M532 — Pin the `runs list` cost-band floor inclusive edge

## Context
Targeted mutation of `kernel/controlplane/runs.go` — the `runs list` cost-band filter
(M125, `--min-cost` / `--max-cost`). `GOMAXPROCS=3` (CPU-capped).

## The genuine gap (closed)
The filter keeps runs whose spend is `≥ min and (when set) ≤ max`:

```
if minCostMC > 0 && r.SpentMicrocents < minCostMC { continue }   // floor
if maxCostMC > 0 && r.SpentMicrocents > maxCostMC { continue }   // ceiling
```

`TestRunsList_CostBandFilter` tests the **ceiling** at its exact edge (a run that spent
exactly 100 against `max=100` is kept — which is why `> → >=` is killed there), but the
**floor** is only tested strictly below a run's spend (`floor 1000` vs runs at 5000 / 1M).
So `SpentMicrocents < minCostMC` was unpinned at the boundary: `< → <=` survived, which
would drop a run that spent *exactly* its floor — an asymmetry where `--min-cost X` would
silently exclude the X-spend runs the operator is asking for.

## Fix
Extended `TestRunsList_CostBandFilter`: with `min_cost_mc=100`, the run that spent exactly
100 (`cheap`) must be included (inclusive floor), and the free (0-spend) run excluded.

## Negative control (manual, CPU-capped)
`SpentMicrocents < minCostMC → <=`: FAIL (the exactly-at-floor run is dropped). Restored
byte-for-byte (`git diff --ignore-all-space` on runs.go empty); passes again. (The limit
clamps `limit < 1` / `limit > maxRunsLimit` have equivalent boundary mutants — clamping to
the boundary value when already at it is a no-op — so they were left unpinned honestly.)

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Control-plane targeted sweep so far
tokenIsPrimary (M529), tenantTokenAllows (M530), readBoundedLine cap (M531), and now the
runs cost-band floor (M532) — four control-plane primitives pinned/verified by negative
control. The remaining handlers are read-only journal-fold renderers covered by the 71
test files.

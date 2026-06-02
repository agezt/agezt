# M167 — `agt run --dry-run` shows the cost cap (+ unpriced-cap advisory)

## Why
M166 added `--max-cost`, the fifth per-run override. M159's dry-run previews the
others (model/system/timeout/tools) but didn't show the cost cap — a completeness
gap. Worse, M166 has a real footgun: a cost cap only binds if the run accrues
**priced** spend. On an unpriced model (unknown to the catalog, or a free/local
model with no cost) the computed spend is $0, so the cap never trips — a run the
operator believes is money-bounded actually isn't. A dry-run is exactly where that
should be caught, before committing.

## What
- `runPlanInput` gains `MaxCostMC int64` (the cap, microcents) and `ModelPriced
  bool` (catalog has a price for the effective model).
- `buildRunPlan` adds a `cost_cap` field (`formatMicrocentsUSD` → `$0.50
  (per-run)`, or `none`) and a new advisory: when `MaxCostMC > 0 && !ModelPriced`,
  it warns the cap "will not bind (spend is computed as $0)" and points to `agt
  catalog sync`.
- `formatMicrocentsUSD` + `microcentsPerUSD` ($1 = 1e9) helpers added (the dry-run
  side of the CLI's `parseUSDToMicrocents`).
- `handleRun`'s dry-run branch fills `MaxCostMC` from the parsed `maxCost` and
  `ModelPriced` from `m.Cost != nil`.
- `runDryRunMode` renders a `cost cap : …` line after `timeout`.

No control-plane change; reuses the dry-run path.

## Tests (+1, all passing)
`TestBuildRunPlan_CostCap`: no cap → `none`; cap on a priced model → `$0.50
(per-run)` with no bind warning; cap on a catalog-known-but-unpriced model →
`$0.10 (per-run)` **with** the "will not bind" advisory.

## Live proof (offline mock + custom catalog)
A daemon with a priced `priced-model` (cost block) and the unpriced offline mock:
- `agt run --dry-run --max-cost 0.50 "x"` (default mock, unpriced) →
  `cost cap : $0.50 (per-run)` **and** the warning
  `--max-cost $0.50 is set, but model "mock" has no known pricing — the cap will not
  bind …`.
- `agt run --dry-run --model priced-model --max-cost 0.50 "x"` →
  `cost cap : $0.50 (per-run)` with **no** bind warning.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or event kind.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1540 tests** (was 1539; +1), 61 packages.

## Result
`agt run --dry-run` now previews all five per-run overrides, and catches the
specific case where a `--max-cost` would silently fail to bind — so an operator
learns the cap is inert (and runs `agt catalog sync`) before relying on it.

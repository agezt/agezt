# M169 — Cost cap inert-on-unpriced: authoritative detection + run-time advisory

## Why
The M168 review flagged that the M166 per-run cost cap is **silently inert** for any
model the Governor can't price: spend computes as $0, so a positive cap never trips.
That's exactly the class of model an operator reaches for to save money
(openai-compatible, local, just-released), so a believed spend guardrail can be a
no-op with no signal. M167 surfaced this in `--dry-run`, but (a) it used a
catalog-only `m.Cost != nil` check that mis-classifies models priced via the
*fallback table* (or unknown to the catalog but in it), and (b) a direct run got no
signal at all.

## What
- **Authoritative pricing check** — `modelPriced(model)` =
  `governor.CostMicrocents(model, 1e6, 1e6) > 0`, which consults the live catalog
  *then* the fallback price table (the same path real billing uses). Replaces the
  catalog-only `m.Cost != nil` test. A model that prices to $0 (unknown, or
  free/local) can never exceed a positive cap.
- **Dry-run accuracy** — `handleRun`'s dry-run branch now sets
  `runPlanInput.ModelPriced` from `modelPriced(effModel)` for *every* model (moved
  out of the catalog-known block), so the "`--max-cost` … will not bind" advisory is
  correct for fallback-priced and catalog-unknown models too.
- **Run-time advisory** — at run submission, when `max_cost > 0 &&
  !modelPriced(effModel)`, a `budget.cap_inert` event is journaled, tied to the
  run's correlation (so `agt why <run>` shows the cap was inert). New event kind
  `KindBudgetCapInert = "budget.cap_inert"` (registered in `knownKinds`); payload
  `{model, cap_microcents}`. Best-effort, never blocks the run.

No new env var (M127 guard unaffected); no protocol command. One new event kind.

## Tests (+4, all passing)
- `TestModelPriced` (internal) — `claude-sonnet-4-6` priced (fallback table) →
  true; `mock` / `llama3.2` / an unknown model → false.
- `TestRun_CostCapInertAdvisory` (server-level, 2 subtests) — a cap on the unpriced
  offline mock journals `budget.cap_inert`; a run with no cap journals none.
- (`TestBuildRunPlan_CostCap` from M167 still passes — the pure plan logic is
  unchanged; only the `ModelPriced` *source* in `handleRun` changed.)

## Live proof
`TestRun_CostCapInertAdvisory` drives a real `controlplane.Server` + `Kernel`: a
`--max-cost` run against the unpriced mock journals exactly one `budget.cap_inert`
advisory (verified by folding the journal), and an uncapped run journals none. The
M166 end-to-end test (cap firing against a priced model) continues to pass, so
priced models still bind.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var (one new event
  kind, registered).
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1546 tests** (was 1542; +4), 61 packages.

## Result
A `--max-cost` that can't bind is no longer silent: the operator learns it ahead of
time (`--dry-run`, now accurate across catalog + fallback pricing) and on the run
itself (a journaled `budget.cap_inert` advisory on the run's chain). The cost
guardrail is now honest about when it isn't one.

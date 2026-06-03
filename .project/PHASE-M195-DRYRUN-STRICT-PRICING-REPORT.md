# M195 — Dry-run advisory for strict-pricing refusal

## Why
M193/M194 added strict pricing (`AGEZT_PRICING_STRICT=on`): a run on a model with no
known price is refused before any provider call. But an operator only discovers that at
submit time, as a "model has no known price" failure. `agt run --dry-run` already
surfaces preventive advisories (unknown model, tool-use mismatch, cost-cap-won't-bind —
M160/M167/M169); a strict-pricing refusal belongs in the same place, so the operator
learns up front.

## What
- **Exported `governor.ModelIsPriced(model) bool`** — wraps the internal `modelIsPriced`,
  reporting whether the governor has a KNOWN price (catalog or fallback table) for the
  model. Crucially distinct from `CostMicrocents(...) > 0`: a known-FREE model
  (local/mock, priced 0) IS priced (would NOT be refused), while a genuinely unknown
  model is not. The existing `modelPriced` (cost > 0) is the right check for "could a
  cost cap trip"; `ModelIsPriced` is the right check for "would strict pricing refuse."
- **`Governor.StrictPricingEnabled() bool`** — lock-free read of the (immutable) config
  flag, so the dry-run can ask whether the daemon is in strict mode.
- **`runPlanInput.StrictPricing` / `ModelHasPrice`** + a warning in `buildRunPlan`: when
  strict pricing is on and the model has no known price (and is non-empty), the plan
  carries "…would be REFUSED before any provider call; `agt catalog sync`…".
- **Wiring**: `handleRun` fills the two fields via a small `strictPricingPlan` helper in
  dryrun.go (so server.go needn't import governor) from `k.Provider()` and the effective
  model.

## Tests
- `kernel/controlplane/dryrun_strict_test.go` — `buildRunPlan` warns on strict + unpriced;
  does NOT warn on strict + priced (incl. known-free), strict-off + unpriced, or an empty
  model. Pure, no daemon needed.

### Live proof
Offline daemon (`AGEZT_PROVIDER=mock`, `AGEZT_PRICING_STRICT=on`),
`agt run --dry-run --model totally-unknown-model "hello"` prints, among its warnings:
`strict pricing is on (AGEZT_PRICING_STRICT) and model "totally-unknown-model" has no
known price — this run would be REFUSED before any provider call; agt catalog sync to
load prices, or unset the flag`.

## Verification
- `go test ./...` — 1608 passing, 0 failing.
- `go vet` clean on touched packages.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/governor/pricing.go` — exported `ModelIsPriced`.
- `kernel/governor/governor.go` — `StrictPricingEnabled()` accessor.
- `kernel/controlplane/dryrun.go` — `StrictPricing`/`ModelHasPrice` input, the warning,
  and the `strictPricingPlan` helper.
- `kernel/controlplane/server.go` — wire the two fields in `handleRun`.
- `kernel/controlplane/dryrun_strict_test.go` — new.

## Governor arc — operator-complete
M191 (overflow-safe cost), M192 (deterministic price match), M193 (strict-pricing gate),
M194 (env + `agt budget` posture), M195 (dry-run advisory). The unpriced-model budget
bypass is now fixed, configurable, visible in budget output, AND previewable per-run.
The only remaining review item is M1 (the deliberate soft-cap TOCTOU).

# M193 — Strict-pricing gate (refuse unpriced models)

## Why
The governor review (H1) flagged a fail-open budget bypass. When a model has no price
entry — missing from the live catalog AND the fallback table — `priceFor` returns a
zero price and the call costs **0 microcents**. `spentToday` never advances, so neither
the daily ceiling nor any per-task cap ever fires for that model. An operator (or a
`compat` endpoint configured with a novel model id, or a task-model override pointing at
an unpriced id) can make real, billed provider calls that the governor records as free —
the "$20/day" guarantee silently does not hold for any unpriced model.

The behavior is documented as intentional (don't block on a missing price), and there's
a real tension: blocking unknown models by default would break a working setup the
moment the catalog lags. But for a budget *security* control, fail-open is the dangerous
default, so it should at least be operator-selectable.

## What
An opt-in strict-pricing mode, defaulting OFF (no behavior change unless enabled).

- **`priceForOk(model) (modelPrice, bool)`** (pricing.go) — `priceFor` refactored to
  expose a `found` flag, so callers can distinguish a **known-free** model (`{0,0}`,
  found — e.g. `llama3.2`, `mock`) from a **genuinely unknown** one (`{0,0}`, not found).
  `priceFor` is now a thin wrapper; `modelIsPriced(model)` reports found-ness.
- **`Config.StrictPricing bool`** (governor.go) — when true, `Complete` refuses a request
  whose `req.Model` is non-empty and unpriced, with `ErrUnpricedModel`, BEFORE routing or
  calling any provider. A `budget.unpriced` event (`event.KindBudgetUnpriced`, new) is
  journaled with the model name so the refusal is auditable. Known-free models and an
  empty `req.Model` (provider picks its default — no id to price ahead of the call) still
  pass.

The gate sits with the other pre-flight gates (rate → budget → task-budget →
**strict-pricing** → route), mirroring the existing `StrictModelCapabilities` pattern.

## Tests
`kernel/governor/strict_pricing_test.go`:
- `TestStrictPricing_RefusesUnknownModel` — strict on + unknown model → `ErrUnpricedModel`,
  provider called 0 times, `budget.unpriced` event journaled.
- `TestStrictPricing_AllowsKnownFreeAndPriced` — strict on + `llama3.2` (known-free) and
  `claude-sonnet-4-6` (priced) both proceed (provider called once).
- `TestStrictPricing_EmptyModelNotGated` — strict on + empty model proceeds.
- `TestStrictPricing_OffByDefaultAllowsUnknown` — strict OFF + unknown model proceeds
  (charged $0), guarding the unchanged default.

## Verification
- `go test ./...` — 1606 passing, 0 failing.
- `go vet ./kernel/governor/ ./kernel/event/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/governor/pricing.go` — `priceForOk` / `modelIsPriced`; `priceFor` wraps them.
- `kernel/governor/governor.go` — `Config.StrictPricing`, `ErrUnpricedModel`, the gate.
- `kernel/event/kinds.go` — `KindBudgetUnpriced` + registration.
- `kernel/governor/strict_pricing_test.go` — new.

## Status — governor review
C1/C2 (M191), H2 (M192), H1 (M193) are now addressed. The remaining item is **M1** (the
budget pre-check and charge are separate critical sections — a deliberate soft cap; a
hard reservation under one lock would close the concurrent-overshoot window). It's a
design trade-off the code documents explicitly, so it's lower priority than the
fail-open/overflow/nondeterminism bugs now fixed.

# M191 — Overflow/negative-safe budget cost math

## Why
A security review of the governor (the cost/budget enforcement layer that bounds how
much money an autonomous agent can spend) found a CRITICAL pair of bugs in the cost
math. `costMicrocents` computed:

```go
totalMicromicrocents := int64(inputTokens)*p.InputMicrocentsPerMTok +
    int64(outputTokens)*p.OutputMicrocentsPerMTok
return totalMicromicrocents / 1_000_000
```

The token counts come from the provider's usage response, which is **untrusted**: a
`compat`/`ollama` provider can be operator-configured to an arbitrary URL, and a buggy
or hostile endpoint can report any `int`. Two failure modes, both disabling the budget:

- **C2 — negative tokens (trivial).** A response reporting `InputTokens: -1_000_000`
  yields a *negative* cost. `recordUsage` does `spentToday += cost`, so a negative cost
  *credits* the ledger — the agent manufactures budget headroom. Repeated, `spentToday`
  goes arbitrarily negative.
- **C1 — overflow (absurd counts).** Per-MTok prices reach `7_500_000_000`
  (`claude-opus-4-7`). `int64(2_000_000_000) * 7_500_000_000 = 1.5e19` exceeds
  `MaxInt64` (9.22e18) and **wraps to a negative int64** → negative cost again.

Once `spentToday` is negative, the daily-ceiling check `spentToday >= DailyCeilingMicrocents`
evaluates `(negative) >= ceiling` → **false for the rest of the UTC day**. The budget
gate is silently disabled and every subsequent call passes the pre-check regardless of
real cost. A single hostile usage response thus both under-charges and opens the gate.

## What
Both fixes live where the untrusted counts enter the cost path.

- **Clamp negatives at the sink** (`governor.go recordUsage`): `inputTokens`/`outputTokens`
  are clamped to ≥0 before costing, and the clamped values are used for BOTH the cost
  and the `budget.consumed` audit event (so the journal can't be poisoned with negative
  counts either).
- **Saturating cost math** (`pricing.go costMicrocents`): the multiply-add is done with
  `saturatingMul` / `saturatingAdd` helpers built on `math/bits.Mul64`. Non-positive
  operands yield 0; any product or sum that would exceed `MaxInt64` saturates to
  `MaxInt64` instead of wrapping. This is **fail-closed**: an absurd usage report
  produces a huge cost (~$9.2M-equivalent after the /1e6) that trips the budget gate
  immediately, which is exactly what you want from a hostile provider.

Normal-range arithmetic is bit-for-bit unchanged (`bits.Mul64` returns the exact product
in `lo` with `hi==0`, and the sum doesn't overflow), so existing pricing values and tests
are preserved.

## Tests
`kernel/governor/overflow_test.go`:
- `TestCostMicrocents_ClampsAndSaturates` — negative tokens → 0; a single overflowing
  term and a both-terms-overflow case → large positive (saturated), never ≤0; and the
  normal `claude-sonnet-4-6` 1 MTok input = `$0.30` is unchanged.
- `TestComplete_NegativeUsageDoesNotCreditLedger` — a provider reporting negative usage
  leaves `SpentMicrocents() ≥ 0` (no ledger credit).
- `TestComplete_OverflowUsageTripsBudget` — a provider reporting 2e9 output tokens makes
  the first call's cost saturate positive, and the *next* call is blocked with
  `ErrBudgetExceeded` (fail-closed). Before the fix the cost wrapped negative and the
  second call would NOT be blocked.

## Verification
- `go test ./...` — 1601 passing, 0 failing.
- `go vet ./kernel/governor/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged (`math`, `math/bits` are stdlib).
- Local commit only (no push); standard trailer.

## Files
- `kernel/governor/pricing.go` — saturating `costMicrocents` + `saturatingMul`/`saturatingAdd`.
- `kernel/governor/governor.go` — negative-token clamp in `recordUsage` (cost + audit).
- `kernel/governor/overflow_test.go` — new.

## Follow-ups (same governor review)
- **H2** — `priceFor`'s case-insensitive PREFIX match can bind a billed model to a
  cheaper/free entry (e.g. `mistral-large-…` → `mistral` = $0) and is nondeterministic
  under Go map iteration; prefer exact-match (the catalog path already is) or
  longest-deterministic match.
- **H1** — an unknown/arbitrary model id costs $0 (fail-open), so an unpriced model dodges
  the budget; make it operator-selectable (strict pricing / floor) and distinguish
  "found, price 0" from "not found".
- **M1** — the budget pre-check and the charge are separate critical sections (a
  deliberate soft cap); after this fix a single saturating call is bounded, but a hard
  reservation would close the concurrent-overshoot window.

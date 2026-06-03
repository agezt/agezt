# M192 — Deterministic longest-prefix price match

## Why
The governor review (H2) flagged the fallback price-table lookup:

```go
lower := strings.ToLower(model)
for k, v := range modelPriceTable {
    if strings.HasPrefix(lower, strings.ToLower(k)) {
        return v          // first match Go's randomized iteration hits
    }
}
```

Go map iteration order is randomized, so when a model id is a prefix-match for more
than one table key, *which* price is returned varies run to run. For money math that is
a latent correctness bug: the same `(model, tokens)` can cost different amounts on
different daemon boots. It can also bind a model to a **less specific (cheaper)** entry
than the best available (e.g. matching `gpt` instead of `gpt-4` for `gpt-4-turbo`).

(The live, synced catalog is the primary source and is exact-match — pricing.go:90 — so
this only affects the bootstrap fallback table before `agt catalog sync`. But the
fallback is used offline and in tests, and the nondeterminism is real.)

## What
The prefix fallback now selects the **longest** matching key instead of the first:

```go
bestLen := -1
var best modelPrice
for k, v := range modelPriceTable {
    lk := strings.ToLower(k)
    if strings.HasPrefix(lower, lk) && len(lk) > bestLen {
        bestLen = len(lk)
        best = v
    }
}
if bestLen >= 0 { return best }
```

This is fully deterministic (no two distinct keys of equal length can both be a prefix
of the same string, so there are no ties) and always prefers the most specific price.
Versioned-suffix matching still works (a new dated snapshot prices like its base model),
but now picks the closest base rather than a random shorter one.

Exact-match (the common case, including the `[1m]` and dated variants already listed in
the table) is unchanged — it is checked before the prefix loop.

## Tests
`kernel/governor/pricing_internal_test.go` (white-box) —
`TestPriceFor_LongestPrefixWinsDeterministically` injects two overlapping fallback keys
with different prices (`zztest-base` = 100, `zztest-base-pro` = 999) and asserts a model
id matching both resolves to the longer key's price across 100 iterations (the old
first-match code would intermittently return the cheaper base price), and that a name
matching only the shorter key still resolves to it. Keys are removed in a defer.

## Verification
- `go test ./...` — 1602 passing, 0 failing.
- `go vet ./kernel/governor/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/governor/pricing.go` — longest-prefix selection in `priceFor`.
- `kernel/governor/pricing_internal_test.go` — new white-box test.

## Remaining governor follow-ups
- **H1** — an unknown/unpriced model still costs $0 (fail-open), dodging the budget; a
  strict-pricing option (floor or refuse unpriced models) is the next hardening. The
  residual cross-family prefix-to-free case (`mistral-large` → free `mistral`) is really
  this same missing-price issue and is best addressed there.
- **M1** — budget pre-check and charge are separate critical sections (deliberate soft
  cap); a hard reservation would close the concurrent-overshoot window.

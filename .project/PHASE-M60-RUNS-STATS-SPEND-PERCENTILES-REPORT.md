# Phase Report — Milestone M60 (`agt runs stats` spend percentiles)

> Status: **shipped** · Date: 2026-06-01 · SPEC-12 multi-agent.

## Why

M47 added total + delegated spend to `agt runs stats`, but not the DISTRIBUTION:
is the spend a few expensive runs or many cheap ones? `agt runs stats` already
shows a duration distribution (avg/min/p50/p95/max); M60 brings the same to spend.

## What shipped

- **Spend distribution in `handleRunsStats`** — collects per-run `SpentMicrocents`
  over priced runs in the windowed set and runs the existing nearest-rank
  `durationStats` over them, returning `spend_microcents: {count,avg,min,max,p50,p95}`.
- **`cmdRunsStats` renders a `spend dist` block** (over N priced runs), reusing
  `fmtUSD`. Omitted when nothing was priced.

## Design decisions

- **Reuse `durationStats`.** The nearest-rank percentile helper is unit-agnostic
  (int64); spend in microcents folds through it unchanged. No new math.
- **Priced runs only.** Free/local runs ($0) are excluded from the distribution so
  the percentiles describe actual cost, not a sea of zeros.

## Tests

- `TestRunsStats_SpendDistribution` — three runs spending 100/200/300 → count 3,
  min 100, max 300, avg 200; a free run is excluded.

Test count: **1297 → 1298**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (delegate demo, priced usage)

```
$ agt runs stats
  spend dist (over 3 priced run(s)):
    avg : $0.0042
    min : $0.0021
    p50 : $0.0021
    p95 : $0.0084
    max : $0.0084
```

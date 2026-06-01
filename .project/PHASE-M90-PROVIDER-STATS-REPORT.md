# Phase Report — Milestone M90 (`agt provider stats`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

M89 added `agt provider log` (per-event routing timeline). The aggregate was
missing: "how reliable is my primary — what fraction of calls fell back, and
which provider keeps failing?". M90 adds `agt provider stats`, completing the
provider **log / stats** pair and giving a provider-reliability metric.

## What shipped

- **Server `handleProviderStats`** — folds `routing.decision` + `provider.fallback`
  into routed-call count, fallback count + rate, a calls-by-primary breakdown
  (which provider the governor chose), and a fallbacks-by-failed-provider
  breakdown. `since_ms` windows by event time.
- **CLI `agt provider stats [--since <dur>] [--json]`** — renders the fallback
  rate and both name-sorted breakdowns.

## Design decisions

- **Fallback rate = fallbacks / routed.** A direct "how often does the primary
  fail?" signal; one routing.decision per call is the denominator.
- **Two breakdowns.** calls-by-primary shows the routing mix; fallbacks-by-failed
  pinpoints the flaky provider — together they answer "what's my provider posture,
  and what's breaking it?".

## Tests

- `TestProviderStats_Aggregates` — 3 routes (2 openai, 1 anthropic) + 1 openai
  fallback → routed 3, fallbacks 1, rate ≈ 0.333, by_primary[openai] = 2.

Test count: **1333 → 1334**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_DEMO_FAIL_PRIMARY=1 agt run "summarize"
$ agt provider stats
  provider routing (over 2 routed call(s)):
    fallbacks : 2
    fallback  : 100.0%
    calls by primary:
      mock-failshim      2
    fallbacks by failed provider:
      mock-failshim      2
```

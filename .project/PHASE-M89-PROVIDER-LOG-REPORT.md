# Phase Report — Milestone M89 (`agt provider log`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt provider check` probes whether a provider WORKS (a synthetic call). Nothing
showed what the governor actually DID at request time: which provider handled
each call, and — critically — when the primary errored and it fell back. The
journal records `routing.decision` (per call) and `provider.fallback` (on each
primary error), but neither had a surface. "Is my primary provider flaky?" was
unanswerable without grepping the raw journal. M89 adds `agt provider log`, the
provider-layer analogue of `agt tool log`.

## What shipped

- **Server `handleProviderLog` (`provider_log.go`)** — folds `routing.decision`
  (primary, fallback chain, task_type) and `provider.fallback` (failed → next,
  reason) into one timeline, newest-first, limited, with a `--fallbacks` filter
  (only the actionable fallback events) and the shared `--since` window.
- **CLI `agt provider log [N] [--fallbacks] [--since <dur>] [--json]`** — renders
  `route → <provider> (chain: …)` and `FALLBACK <failed> → <next> (reason)`.

## Design decisions

- **Two event kinds, one timeline.** Routing and fallback interleave in the order
  they happened, so the log reads as the provider's actual decision stream;
  `--fallbacks` isolates the problems when routing volume is noise.
- **Chain shown only when it differs from primary.** A single-provider route
  doesn't repeat the name — the chain annotation appears only when there's an
  actual fallback option.

## Tests

- `TestProviderLog_RoutingAndFallbacks` — a routing decision + a fallback: both
  listed newest-first (fallback first), `--fallbacks` returns just the fallback.

Test count: **1332 → 1333**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_DEMO_FAIL_PRIMARY=1 agt run "summarize"   # primary fails → falls back
$ agt provider log
  2026-06-01 14:58:55  FALLBACK  mock-failshim → mock  (demo-shim: simulated primary failure)
  2026-06-01 14:58:55  route     → mock-failshim  (chain: mock-failshim,mock)
$ agt provider log --fallbacks
  2026-06-01 14:58:55  FALLBACK  mock-failshim → mock  (demo-shim: simulated primary failure)
```

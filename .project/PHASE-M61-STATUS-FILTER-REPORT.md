# Phase Report — Milestone M61 (Status filter on runs list & schedule fires)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

No quick way to find FAILED runs/firings — `agt runs list` and `agt schedule
fires` showed everything, and an operator hunting failures had to eyeball the
whole list. M61 adds a status filter to both.

## What shipped

- **`runEntryStatus(r)` helper (`kernel/controlplane/runs.go`)** — the single
  source of truth for a run's terminal status (completed>failed>abandoned>running),
  shared by the list, the fires fold, and the filter so they never disagree.
- **`handleRunsList` status filter** — `args.status` keeps only matching runs,
  applied BEFORE the limit so `list 5 --failed` returns 5 failed runs, not "failed
  among the last 5".
- **`handleScheduleFires` status filter** — same, matched against each firing's run
  outcome, before sort/limit.
- **CLI flags** — `agt runs list` and `agt schedule fires` gain `--status <s>` and
  `--failed` (shorthand), documented in `--help`.

## Design decisions

- **Filter before limit.** Applying the status filter during entry collection (not
  in the render loop) means the limit counts matching rows — the intuitive
  behaviour for `list N --failed`.
- **One status helper.** Extracted `runEntryStatus` so list/fires/filter share the
  exact precedence, eliminating the inline switch duplicated across handlers.

## Tests

- `TestRunsList_StatusFilter` — 2 completed + 1 failed → `--status failed` returns
  just the failed run.
- `TestScheduleFires_StatusFilter` — 1 completed + 1 failed firing → `--status
  failed` returns just the failed firing.

Test count: **1298 → 1300**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof

```
$ agt runs list --status completed   # shows the completed run
$ agt runs list --failed             # no failed runs
```

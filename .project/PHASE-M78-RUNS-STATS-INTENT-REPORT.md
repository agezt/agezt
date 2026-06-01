# Phase Report — Milestone M78 (`agt runs stats --intent <substr>`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

M77 added `--intent` to `agt runs list`; M78 brings the same scope to `agt runs
stats`, so an operator can ask aggregate health questions about a SUBSET of runs:
"what's the success rate / p95 duration / spend of my deploy runs?". Without it,
stats were all-or-(time)-window; you couldn't slice by what the run was for.
Completes the list/stats symmetry for the intent filter (as M76 did for edict).

## What shipped

- **Server intent scope** — `handleRunsStats` skips runs whose intent doesn't
  contain the (case-insensitive) query, alongside the existing `--since` window,
  so every reported figure (counts, success rate, duration/spend distributions,
  delegation metrics) is computed over the matching subset.
- **CLI `--intent <substr>` / `--intent=`** — documented in `--help`,
  composes with `--since` and `--tenant`.

## Design decisions

- **Same matcher as M77.** Case-insensitive `strings.Contains` on `r.Intent`, so
  `runs list --intent deploy` and `runs stats --intent deploy` select the same set
  — list shows them, stats aggregates them.
- **Inside the fold.** The filter runs before any counting, so the success rate
  is the deploy runs' own rate, not the global one masked by a filtered count.

## Tests

- `TestRunsStats_IntentScope` — unscoped total 3; `--intent deploy` → total 2
  (case-insensitive over "deploy staging" / "DEPLOY prod").

Test count: **1320 → 1321**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt runs stats --intent deploy
  run stats (over 1 run(s)):
    completed : 1
    failed    : 0
    ...
```

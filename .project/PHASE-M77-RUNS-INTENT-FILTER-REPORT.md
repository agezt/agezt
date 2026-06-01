# Phase Report — Milestone M77 (`agt runs list --intent <substr>`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt runs list` filters by status (M61) and reads a tenant's runs (M39), but to
find a SPECIFIC run an operator had to eyeball the intent column or pipe to grep.
With many runs, "where's that deploy run?" meant scrolling. M77 adds
`--intent <substr>` — a case-insensitive substring match on the run's intent —
so an operator can name what they're looking for.

## What shipped

- **Server `intent` substring filter** — `handleRunsList` keeps only runs whose
  (lower-cased) intent contains the (lower-cased) query, applied BEFORE the limit
  so `runs list 5 --intent deploy` returns 5 matching runs, not "matches among
  the last 5". Composes with `--status`/`--failed`.
- **CLI `--intent <substr>` / `--intent=`** — documented in `--help`.

## Design decisions

- **Case-insensitive contains, server-side.** Matches how an operator
  half-remembers an intent ("deploy", not the exact phrasing), and runs in the
  fold so the limit applies to matches.
- **Filter-before-limit, like `--status`.** Consistent with M61 so the two
  filters stack predictably.

## Tests

- `TestRunsList_IntentFilter` — three runs ("deploy the staging cluster",
  "summarize the README", "DEPLOY production now"); `--intent deploy` → 2,
  proving the case-insensitive match.

Test count: **1319 → 1320**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt runs list --intent deploy
  last 1 run(s):
    intent  : deploy the staging cluster
$ agt runs list --intent zzznope
  no runs yet (journal has no task.received events)
```

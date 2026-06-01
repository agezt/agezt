# Phase Report — Milestone M80 (`agt schedule fires --intent <substr>`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

M77 gave `agt runs list` an `--intent` substring filter. `agt schedule fires` —
the autonomy analogue of `runs list` — was the last list surface without it.
With many schedules firing, "show me the deploy firings" meant scrolling. M80
adds `--intent`, completing the list-surface filter symmetry (runs list and
schedule fires now share status/since/intent).

## What shipped

- **Server intent filter** — `handleScheduleFires` skips firings whose intent
  doesn't contain the (case-insensitive) query, applied before sort/limit
  (composes with `--id` / `--status` / `--since`).
- **CLI `--intent <substr>` / `--intent=`** — documented in `--help`.

## Design decisions

- **Same matcher as M77.** Case-insensitive `strings.Contains` on the firing's
  intent (pulled from the `schedule.fired` payload), so `runs list --intent X`
  and `schedule fires --intent X` select alike.

## Tests

- `TestScheduleFires_IntentFilter` — three firings ("nightly DEPLOY", "hourly
  summary", "deploy canary"); `--intent deploy` → 2 (case-insensitive).

Test count: **1322 → 1323**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt schedule fires --intent deploy   # after two schedules fired
  2026-06-01 14:13:32  failed (error)  (20ms)  run-…  "deploy nightly build"
  # the "summarize weekly" firing is correctly excluded
```

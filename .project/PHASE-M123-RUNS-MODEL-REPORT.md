# M123 — Model-aware `agt runs`

## Why
Agezt does per-request model routing across 9 provider families with fallback
chains (M99 surfaces the fallback *rates*). But at the per-run level the model was
invisible: `agt runs list` showed intent, status, duration, iters, spend — never
*which model served the run*. So a multi-provider operator couldn't answer the
everyday questions:
- "Which runs used `claude-opus` (the expensive one)?"
- "Did the cheap-model route actually take, or is everything hitting the premium
  model?"
- "Show me only the runs on the new model I'm dogfooding."

The data already existed — every `budget.consumed` event the governor journals
carries `model` (and `provider`) — but it was never folded into the run
projection or exposed. This is a pure surfacing of data already on disk.

## What
- `runEntry.Model` — folded **first-wins** from a run's `budget.consumed` events
  (the model the run was initially routed to). A later fallback to another model
  does *not* overwrite it: "what model was this run" is settled by the first call,
  and fallback rates live in `agt provider stats` (M99).
- `handleRunsList` now emits `"model"` on every row and accepts an optional
  `model` arg — a case-insensitive substring filter, applied before the limit
  like the existing `--status` (M61) and `--intent` (M77) filters.
- `agt runs list --model <substr>` (CLI), and the model is rendered inline on the
  status line (`… iters: N   model: claude-opus-4`) when journaled — quiet for an
  unpriced/mock run that never recorded a model.

No new control-plane command, no schema change, no new event — `runEntry` gained
one field, `CmdRunsList` one optional arg.

## Files
- `kernel/controlplane/runs.go` — `runEntry.Model`; fold in the
  `KindBudgetConsumed` case; `extractModel` helper; `model` filter + row output
  in `handleRunsList`.
- `kernel/controlplane/runs_test.go` — `TestRunsList_ModelFoldAndFilter`
  (first-wins fold, empty for no-spend, substring + case-insensitive filter).
- `cmd/agt/runs.go` — `--model` flag (+ `--model=`), pass-through, inline render,
  help text.

## Live proof (offline mock provider)
```
=== runs list (model shown inline) ===
  status: completed   duration: 22ms   iters: 2   model: mock

=== runs list --json ===
  "model": "mock"        (priced run)
  "model": ""            (run with no spend journaled)

=== filter ===
  runs list --model MOCK   → matches the run (case-insensitive)
  runs list --model claude → 0 matches (actual model is "mock")
  runs list --model zzz    → no runs
```

## Verification
- 55 packages `ok`, **FAIL 0**; **1410 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: my added lines are clean; the two pre-existing format-drift complaints in
  `runs.go` (the `runEntry` top-field alignment and the row-map comment spacing)
  are present in the HEAD blob unchanged — not introduced here, left untouched per
  the standing rule.

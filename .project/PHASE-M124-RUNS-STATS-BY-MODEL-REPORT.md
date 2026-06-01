# M124 — Per-model cost attribution in `agt runs stats`

## Why
M123 folded each run's model into the run projection and made it visible/filterable
in `agt runs list`. The natural next question is the *aggregate* one — the
defining question for a multi-provider deployment with per-model pricing:

> "Where is my money going, by model?"

`agt runs stats` already reports totals, success rate, duration/spend
distributions, and delegation scale — but spend was a single lump sum. An operator
running opus + sonnet + haiku with per-request routing couldn't see that, say, 80%
of spend is on opus across a handful of runs while haiku serves the bulk cheaply.
The data was already on disk (M123's per-run model); this groups it.

## What
- The stats fold now accumulates `modelRuns[model]` and `modelSpent[model]` over
  the same windowed/intent-scoped set as every other aggregate.
- Response gains `by_model: {model → {runs, spent_microcents}}`. Empty map when no
  run carried a model (free/local/mock) — runs with no journaled model didn't
  spend and aren't attributed.
- `agt runs stats` renders a `by model:` block sorted by spend descending (ties by
  name), showing `<model>  N run(s), $X` — the `$X` omitted for a free model that
  ran but spent nothing. `--json` carries `by_model` verbatim.

No new control-plane command, no schema change, no new event — one map in the
existing `CmdRunsStats` response.

## Files
- `kernel/controlplane/runs.go` — `modelRuns`/`modelSpent` accumulation in the
  stats fold; `by_model` in the response.
- `kernel/controlplane/runs_test.go` — `TestRunsStats_ByModel` (two opus + one
  haiku → correct per-model run count + spend; model-less run not attributed).
- `cmd/agt/runs.go` — `by model:` render block (sorted by spend desc).

## Live proof (offline mock provider)
```
=== runs stats (text) ===
  by model:
    mock                         1 run(s)

=== runs stats --json ===
  by_model = {"mock": {"runs": 1, "spent_microcents": 0}}
```
The mock reports a single model name and spends $0, so the breakdown shows the
run count without a `$` figure (correct). The multi-model + nonzero-spend path —
opus {runs:2, spent:400}, haiku {runs:1, spent:50} — is covered by the unit test,
which the single-model offline mock cannot reproduce.

## Verification
- 55 packages `ok`, **FAIL 0**; **1411 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: HEAD and working tree each carry exactly one identical pre-existing
  format-drift complaint per touched file — my added lines are clean and
  introduced none (verified by HEAD-vs-working complaint-count parity). A stray
  `gofmt -w` had rewritten one pre-existing doc-comment; that was reverted so only
  my lines changed.

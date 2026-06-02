# M125 — Cost-band filter for `agt runs list`

## Why
M124 answers "where does my money go, by model?" at the aggregate level. The
natural next move is to **drill into the runs themselves**: an operator who sees
opus dominating spend wants to pull up *the specific expensive runs* to see what
they were doing — a feedback loop, a misrouted model, a runaway delegation. Per
the scout, "why is my bill high / show me the expensive runs" is the single most
common run-list operator question, and `agt runs list` could filter by status
(M61), intent (M77), and model (M123) — but not by **cost**.

The per-run spend was already folded (`runEntry.SpentMicrocents`, M47); it just
wasn't a filter axis.

## What
- `agt runs list --min-cost <usd>` / `--max-cost <usd>` — keep only runs whose
  folded spend falls in the band. Both combine; a positive `--min-cost` excludes
  free ($0) runs, while `--max-cost` alone keeps them.
- New `usdToMicrocents` helper (in `budget.go`, beside `fmtUSD`) — the inverse of
  the display scale ($1 = 1e9 microcents), tolerant of a leading `$` and
  surrounding whitespace, rejecting negative/unparseable input. So the operator
  types dollars, never the internal microcents unit.
- Server applies the band before the limit, like the existing status/intent/model
  filters. New `int64Arg` helper decodes the JSON-number args.

No new control-plane command, no schema change, no new event — `CmdRunsList`
gained two optional numeric args.

## Files
- `cmd/agt/budget.go` — `usdToMicrocents`.
- `cmd/agt/budget_test.go` (new) — `TestUsdToMicrocents` (incl. `$`/whitespace/
  negative/non-numeric), `TestUsdMicrocentsRoundTrip`.
- `cmd/agt/runs.go` — `--min-cost` / `--max-cost` flags, pass-through, help text.
- `kernel/controlplane/runs.go` — `int64Arg` helper; `min_cost_mc` / `max_cost_mc`
  band filter in `handleRunsList`.
- `kernel/controlplane/runs_test.go` — `TestRunsList_CostBandFilter` (floor, band,
  ceiling; free run handling).

## Live proof (offline mock provider)
The mock spends $0, so:
```
runs list                 → 1 run
runs list --min-cost 0.01 → no runs   (the $0 mock run is below the floor)
runs list --max-cost 1.00 → 1 run     ($0 ≤ $1.00)
runs list --min-cost abc  → error: want a dollar amount like 0.01, got "abc"
```
The positive-band path (cheap 100mc / mid 5_000mc / dear 1_000_000mc → floor,
band, ceiling selections) is covered by the unit test, which the single-cost
offline mock cannot reproduce.

## Verification
- 55 packages `ok`, **FAIL 0**; **1414 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: HEAD-vs-working complaint-count parity preserved on every touched file
  (1/1 on the two run files' pre-existing drift, 0/0 on budget.go, 0 on the new
  test) — my added lines are clean and introduced none.

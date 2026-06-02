# M146 — `agt run` reports what the run cost

## Why
`agt run "<intent>"` printed the final answer and the correlation id, but never
what the run *cost* — how many iterations it took, which model it used, or the
dollar spend. To learn that, an operator had to note the correlation id and run a
second `agt runs show <corr>`. The data is already in the journal (the per-run
fold behind `agt runs` computes iterations, model, and folded spend); it just
wasn't surfaced on the one command people run most. Showing it inline closes the
loop: you see the price of what you just ran, immediately.

## What
- **Handler enrichment** (`kernel/controlplane/server.go`, `handleRun`) — when a run
  completes, the handler folds this run's events via `collectRuns(k)` (the SAME fold
  `agt runs list/show/stats` use, so the numbers can't disagree) and adds the run's
  `iters`, `spent_mc` (microcents), and `model` to the result map. Best-effort: a
  fold error or an unpriced run simply omits the fields. No new event, no second
  round-trip.
- **CLI render** (`cmd/agt/main.go`, `cmdRun`) — after the final answer, a `usage:`
  line is printed from those fields: model · iteration count · USD cost
  (reusing the existing `mcFromAny` / `fmtUSD` / `intOfStatus` helpers). Each piece
  is shown only when present, so an unpriced run (e.g. the offline mock) shows
  `usage: mock · 2 iteration(s)` with no bogus `$0`. `agt run --json` carries the
  three fields in its result object for scripting.

## Files
- `kernel/controlplane/server.go` — enrich the run result with iters/spent_mc/model.
- `cmd/agt/main.go` — render the `usage:` line in `cmdRun`.
- `kernel/controlplane/controlplane_test.go` — `TestRun_ResultCarriesUsage`.

## Tests (+1, all passing)
- `TestRun_ResultCarriesUsage` — a streamed run's result carries the folded `iters`
  (≥ 1), confirming the enrichment path.

## Live proof (offline mock, real booted daemon)
```
$ agt run "list the files and say what this is"
  --- final answer ---
  [offline-mock] I ran a directory listing via the shell tool. …
  (correlation_id: run-01KT46…; use `agt why <event_id>` to walk the chain)
  usage: mock · 2 iteration(s)            ← new line (cost omitted: mock is unpriced)

$ agt run --json "hello"   # result object
  iters=2  spent_mc=0  model=mock         ← fields present for scripting
```

On a priced provider the line reads e.g. `usage: claude-sonnet-4-6 · 4 iteration(s)
· $0.0123`.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1470 tests** (was 1469; +1), 61 packages.

## Result
The command operators run most now answers "what did that cost?" inline — model,
iterations, and dollars — folded from the same source as `agt runs`, with the data
also exposed in `--json` for automation.

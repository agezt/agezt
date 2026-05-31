# Phase Report — Milestone M29 (`agt runs stats`)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/observability). Second step on the resilience/observability
> axis opened by M28: where M28 *reconciles* orphaned runs, M29 *summarizes* the
> whole run population into a single health view.

## Why

Operators had two run views, both per-run: `agt runs list` (one row per run) and
`agt runs show`/`last` (one run's task arc). Neither answers the fleet-level
question an operator actually asks first: **"how are my runs doing overall?"** —
how many completed vs. abandoned, what fraction succeed, and how long a run
typically takes (and the slow tail). Answering that today meant piping
`agt runs list 1000 --json` through `jq` and doing the arithmetic by hand.

The journal already holds everything needed; this is a pure, additive
aggregation. Zero risk to the hot path — read-only, no new events, no agent-loop
involvement. It also lays the denominator groundwork for the next candidates
(a `task.failed` terminal event would slot straight into the success-rate split).

## What shipped

- **`CmdRunsStats` (`runs_stats`) control-plane verb** (`kernel/controlplane/protocol.go`,
  dispatch in `server.go`) — no args; returns the aggregate document.
- **`collectRuns` (`kernel/controlplane/runs.go`)** — extracted the journal fold
  that `handleRunsList` already did into a shared helper, so list and stats walk
  the journal **identically**. The two surfaces can never disagree about a run's
  status because they read the same `runEntry` records.
- **`handleRunsStats`** — folds the runs into: `total`, `completed`, `running`,
  `abandoned`, `terminal` (= completed + abandoned), `success_rate`, `avg_iters`,
  and a `duration_ms` block (`count`, `avg`, `min`, `max`, `p50`, `p95`).
- **`durationStats` + `percentileNearestRank`** — pure helpers. Percentiles use
  the **nearest-rank** method (`rank = ceil(p/100 · N)`, clamped) so every
  reported value is a real observed duration, not an interpolated phantom. The
  sort is on a copy — the caller's slice is never mutated.
- **`cmdRunsStats` (`cmd/agt/runs.go`)** — `agt runs stats [--json]` renderer,
  wired into the `runs` dispatcher and `--help`.

## Design decisions

- **Stats are over ALL runs, never a "last N" window.** A success rate or p95
  computed over a sliding window is meaningless; the command deliberately takes
  no `limit`.
- **Success rate = `completed / (completed + abandoned)`.** Runs still in-flight
  (`running`) are *not* failures — they just haven't finished — so they're
  excluded from the denominator. When no run has reached a terminal state the
  rate is undefined; the renderer shows `n/a` rather than a misleading `0%`.
- **Duration percentiles are over completed runs only.** Running/abandoned runs
  have no end time; including them would require a placeholder that skews the
  distribution. The completed/running/abandoned split is reported separately so
  nothing is hidden — `duration_ms.count` tells the operator exactly how many
  runs the percentiles are computed from.
- **Empty journal renders cleanly** (`total=0`, zero-valued duration block) so
  `jq` pipelines and the human renderer both no-op gracefully instead of
  crashing.

## Tests

- `kernel/controlplane/runs_test.go` (black-box, via the live control plane):
  empty journal; a mixed population (3 completed / 1 abandoned / 1 running →
  `terminal=4`, `success_rate=0.75`, `avg_iters=4`); and a percentile-ordering
  invariant (`min ≤ p50 ≤ p95 ≤ max`, `min ≤ avg ≤ max`).
- `kernel/controlplane/runs_internal_test.go` (white-box) — pins the **exact**
  percentile/aggregate math without a live journal: a known 100..1000 ms
  distribution (avg=550, p50=500, p95=1000), nearest-rank edge cases
  (p0/p1/p50/p95/p100), empty + single-element inputs, and that the input slice
  is not mutated.

Test count: **1202 → 1212**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged. My added lines are gofmt-clean (`runs.go`/`runs_test.go` show only the
pre-existing CRLF/blank-line artifacts noted in the standing constraints).

## Live proof (mock provider)

```
$ agt runs stats
run stats (over 3 run(s)):

  completed : 1
  running   : 2
  abandoned : 0
  success   : 100.0% (1/1 terminal)
  avg iters : 2.0

  duration (over 1 completed run(s)):
    avg : 22ms
    min : 22ms
    p50 : 22ms
    p95 : 22ms
    max : 22ms
```

The split matches `agt runs list` exactly (same 1 completed / 2 running) —
confirming the shared `collectRuns` fold. `--json` emits the same document for
pipelines.

## What's next

The resilience/observability axis stays the least-saturated. Natural follow-ons,
unchanged from the M28 report and now better supported:

1. **`task.failed` terminal event** on the agent error path — today an errored
   run looks identical to an orphan (`received`, no completion). A real terminal
   `task.failed` would let `agt runs` and this command tell a *failure* apart
   from an in-flight run, and split `success_rate` into completed / failed /
   abandoned.
2. **Per-run wall-clock timeout** (`MaxDuration` around the agent loop) — caps a
   hung run *within* a live session (M28 only covers across-restart).
3. **`agt runs stats --since <dur>`** — a time-bounded variant once the above
   lands, for "how have runs done in the last hour".

# Phase Report — Milestone M36 (Failure-reason breakdown in `agt runs stats`)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/observability). Ninth step on the resilience/observability
> axis (M28 → … → M36). A small, purely-additive cap on the run-stats surface:
> turn the `failed` count into a *why* breakdown.

## Why

M29 reports how many runs failed; M30–M35 made failures rich (`error`,
`max_iters`, `canceled`, `timeout`). But `agt runs stats` collapsed them all into
a single `failed` count. "10% of runs fail" is an alarm; "…and they're all
timeouts" is an action (raise `AGEZT_RUN_TIMEOUT`, fix the slow provider). M36
surfaces the distribution that the journal already records.

## What shipped

- **`failed_by_reason` aggregation (`kernel/controlplane/runs.go`)** —
  `handleRunsStats` buckets each failed run by its `FailReason` (already captured
  by `collectRuns` from the `task.failed` payload), defaulting a missing reason to
  `"unknown"` so nothing vanishes. Emitted as a `{reason→count}` map; empty (not
  null) when there are no failures, so jq pipelines stay safe.
- **CLI rendering (`cmd/agt/runs.go`)** — the `failed` line gains an inline
  breakdown: `failed : 3 (timeout=2, canceled=1)`. A new `failedByReasonStr`
  helper renders known reasons in a fixed order (`error, timeout, max_iters,
  canceled, unknown`) with any unanticipated tags sorted after, so the line is
  deterministic run-to-run. `--json` exposes the raw map.

## Design decisions

- **Reuse the existing fold; no new state.** `collectRuns` already parses the
  reason; M36 only aggregates it. The change is a map increment + a render line.
- **`unknown` bucket, never drop.** A failure that predates M30's reason tag (or a
  malformed payload) buckets under `unknown` rather than silently disappearing —
  the breakdown always sums to the `failed` count.
- **Deterministic ordering.** Maps iterate randomly in Go; the renderer imposes a
  fixed reason order (then sorted extras) so the line doesn't shuffle between
  invocations — important for eyeballing diffs and for any test that asserts it.
- **Respects the M33 window.** The breakdown is computed inside the same
  windowed loop, so `agt runs stats --since 1h` shows the last hour's failure mix.

## Tests

- `kernel/controlplane/runs_test.go`:
  - `TestRunsStats_FailedByReason` — four failures (2×timeout, 1×error,
    1×no-reason) bucket as `{timeout:2, error:1, unknown:1}`.
  - `TestRunsStats_NoFailuresEmptyBreakdown` — no failures → present, empty map.
- `cmd/agt/runs_stats_test.go`:
  - `TestFailedByReasonStr` — fixed ordering, unknown tags sorted after known,
    zero counts dropped, nil/empty → "".

Test count: **1231 → 1240**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (black-hole endpoint + run timeout + manual cancel)

```
$ AGEZT_RUN_TIMEOUT=2s agezt …      # provider dials a black hole
$ agt run one ; agt run two         # both time out at 2s
$ agt run three & ; agt runs cancel <corr>   # cancelled

$ agt runs stats
  completed : 0
  failed    : 3 (timeout=2, canceled=1)
  running   : 0
  abandoned : 0

$ agt runs stats --json | jq .failed_by_reason
  { "canceled": 1, "timeout": 2 }
```

Two timed-out runs and one cancelled run produced exactly the expected breakdown,
inline and in JSON — end-to-end through the real fold and renderer.

## What's next

The resilience/observability axis is now very mature (nine milestones; `agt runs`
covers list/show/last/stats/cancel, windowed, with a failure breakdown). The
honest call is that further deepening here has diminishing returns. Options:

1. **`agt runs list --since <dur>`** (LOW) — last symmetry gap with M33; lift the
   `since_ms`/cutoff filter into a shared helper.
2. **Open a fresh axis** — per-tenant *authenticated* policy management (tenant
   token vs primary token, deferred from M22); capability **down-routing** (route
   a tools request to a tool-capable fallback, deferred from M23–M27);
   `policy.changed` compaction. These break new ground rather than adding a tenth
   coat to the runs surface.

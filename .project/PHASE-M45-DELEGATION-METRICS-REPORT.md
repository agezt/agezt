# Phase Report — Milestone M45 (Delegation metrics in `agt runs stats`)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Fifth step on the multi-agent axis:
> from "see *a* delegation" (M41–M44) to "see the *scale* of delegation".

## Why

M41–M44 made an individual delegation fully observable: the link, the backlink,
the tree, and the child's outcome are all rendered in `agt runs show` / `agt runs
list --tree`. But the *aggregate* was invisible. In `agt runs stats` a sub-agent
run is just another row in the `total` / `completed` counts — indistinguishable
from a top-level run. An operator could not answer "how much delegation is
happening in this system, and how wide does any single lead fan out?" without
manually walking the tree. Fan-out is also currently **unbounded** (only depth is
capped, via `SubAgentMaxDepth`) — so the scale metric is the first step toward
governing it: you can't bound what you can't see.

## What shipped

- **Server fold (`handleRunsStats`, `kernel/controlplane/runs.go`)** — inside the
  existing windowed loop, a run carrying a non-empty `ParentCorrelation` (the
  sub-agent marker M41 added) is counted into three new aggregates:
  - `delegations` — total sub-agent runs spawned within the window.
  - `delegating_runs` — distinct leads that delegated at least once
    (`len(fanout)`).
  - `max_fanout` — the widest single lead's sub-agent count.
  All three are computed over the **same** windowed set, so `--since` applies to
  them for free, and all are `0` when nothing delegated.
- **CLI render (`cmdRunsStats`, `cmd/agt/runs.go`)** — after the duration block,
  a single line prints only when `delegations > 0`:
  `delegations: 3 (from 2 run(s), max fan-out 2)`. Single-agent operators never
  see the line, so it adds zero noise to the common case. `--help` updated to
  mention delegation fan-out.

## Design decisions

- **Reuse the M41 parent link, not a new scan.** `collectRuns` already sets
  `ParentCorrelation` on each sub-agent run (folded from the lead's
  `subagent.spawned` event). Counting runs with that field set is the entire
  metric — no second journal walk, no new event kind, no protocol field beyond
  the three result keys. Consistent with the M29/M33/M36 stats lineage: every
  stat is a fold over the run set, never a bespoke query.
- **Window-respecting by construction.** Placing the counters *inside* the
  `cutoff` loop (not before it) means a windowed `--since 1h` view reports the
  delegations *in that hour*, matching every other line in the block. A child
  whose lead started before the window still counts as a delegation (it has a
  parent link); `delegating_runs` counts the distinct parent correlations the
  in-window children reference, which is the honest "how many leads fed this
  window".
- **Omit, don't zero-print.** A `delegations: 0` line on every single-agent
  deployment would be clutter. The line appears only when delegation actually
  happened — the JSON always carries the keys (jq-safe), the human view stays
  clean.
- **Scale, not spend (yet).** Sub-agents share the lead's governor budget (one
  `k.cfg.Provider` instance — `subagent.go:141`), and spend is journaled at the
  provider-call level, not per delegation. Attributing cost to a delegation is a
  larger, separate increment; M45 ships the *structural* scale (count, breadth)
  that's a pure fold, and leaves spend attribution as the next frontier.

## Tests

`kernel/controlplane/runs_test.go`:
- `TestRunsStats_DelegationMetrics` — lead p1 spawns c1+c2, lead p2 spawns c3,
  s1 is standalone; asserts `delegations=3`, `delegating_runs=2`, `max_fanout=2`.
- `TestRunsStats_NoDelegationsZeroed` — a journal of only top-level runs reports
  all three aggregates as `0` (the CLI then omits the line).

Test count: **1265 → 1267**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=1)

```
$ agt runs stats
run stats (over 3 run(s)):

  completed : 2
  failed    : 1 (error=1)
  …
  delegations: 1 (from 1 run(s), max fan-out 1)

$ agt runs stats --json | grep -E 'delegat|fanout'
  "delegating_runs": 1
  "delegations": 1
  "max_fanout": 1
```

The aggregate now distinguishes the sub-agent run from the two top-level ones —
the scale of fan-out is visible without walking the tree.

## What's next

The delegation surface is now observable both per-run (M41–M44) and in aggregate
(M45). The remaining multi-agent frontiers, sharpest first:

1. **Bound fan-out** (MED) — a `SubAgentMaxFanout` ceiling (mirroring
   `SubAgentMaxDepth`) the lead can't exceed in one run, enforced in
   `runSubAgent`, surfaced as a `policy.decision` deny. M45 makes the metric
   visible; this acts on it. The first *governance* (not just observability) step.
2. **Per-delegation spend** (MED-HIGH) — attribute provider cost to the spawning
   delegation so `agt budget` / `runs stats` can show "sub-agents consumed X% of
   this run's spend". Requires threading the child's correlation into the
   governor's budget-consumed events (schema-adjacent).
3. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` (M39
   pattern) + tenant-token allowlist, closing the last non-tenant-aware surface.

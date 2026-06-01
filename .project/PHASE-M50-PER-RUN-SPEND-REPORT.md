# Phase Report — Milestone M50 (Per-run spend in `agt runs list` / `show`)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Tenth step on the multi-agent axis —
> completes the spend story from aggregate (M47) down to the single run and the
> individual delegation.

## Why

M47 attributed spend per run and surfaced it *in aggregate* (`agt runs stats`:
`spend: $X (delegated: $Y)`). But the per-run views — `agt runs list` and `agt
runs show` — showed status, duration, and iters, never cost. An operator could
see the window's total spend but not *which run* drove it, nor what a *specific
delegation* cost. The data was already computed (`runEntry.SpentMicrocents`, M47)
and already on the wire for `stats`; it just wasn't exposed on the per-run path.
M50 surfaces it everywhere a run is shown.

## What shipped

- **`spent_mc` per row (`handleRunsList`, `kernel/controlplane/runs.go`)** — each
  runs-list row's JSON now carries the run's spend in microcents, straight from
  the M47 fold. The only server change.
- **Per-row spend in `renderRunRow` (`cmd/agt/runs.go`)** — appends
  `   spend: $0.0021` to a run's status line, only when it cost something. Flows
  through `agt runs list` (flat and `--tree`) since both render via `renderRunRow`.
- **Lead spend in the `agt runs show` header (`renderTaskArc`)** — a
  `spend      : $0.0084` line under the status, showing the lead's own direct
  spend (its delegations' costs appear on their `↳` lines).
- **Per-delegation cost on the `↳` line (M44 extension)** — `childOutcome` gains
  `spentMC`; the inline outcome becomes
  `↳ completed (1 iters, 42ms, $0.0021)`, so the lead's arc shows what each
  delegation cost without drilling into the child.

## Design decisions

- **Surface, don't recompute.** `collectRuns` already folds `budget.consumed` into
  `runEntry.SpentMicrocents` (M47). M50 is almost entirely *rendering* over data
  already produced — one new JSON field server-side (`spent_mc`), the rest client
  formatting. No new endpoint, fold, or event.
- **Quiet at $0.** A free/local model or the offline mock spends nothing; every
  spend surface (row, header, `↳`) is omitted when the figure is zero, so
  single-model / unpriced deployments see no `$0.0000` noise — matching the M47
  stats line's behaviour.
- **`fmtUSD` everywhere.** Reuses the `agt budget` formatter, so a dollar figure
  reads identically across `budget`, `runs stats`, `runs list`, and `runs show`.
- **Header = direct spend, `↳` = delegation spend.** A lead's header line is its
  *own* llm-call spend; each delegation's cost is attributed to its own `↳` line.
  The two don't double-count — the header is the lead correlation's spend, the `↳`
  figures are the child correlations' — so an operator can read both the lead's
  direct cost and each delegation's cost separately.

## Tests

- `cmd/agt/runs_show_test.go::TestRenderTaskArc_ShowsSpend` — summary spend $0.0084
  and a child outcome of $0.0021 render as `spend      : $0.0084` (header) and
  `↳ completed (1 iters, 42ms, $0.0021)`.
- `TestRenderTaskArc_NoSpendLineWhenZero` — a $0 run shows no header spend line and
  no dollar figure anywhere (quiet at zero).
- `kernel/controlplane/runs_test.go::TestRunsList_RowCarriesSpend` — a run with two
  `budget.consumed` events (100+50) reports `spent_mc=150` on its list row.
- The four existing M44 `renderTaskArc` tests are unchanged (spend renders only
  when > 0, which they don't set) and still pass.

Test count: **1275 → 1278**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=3 + AGEZT_SUBAGENT_FANOUT=2)

```
$ agt runs list 5
    started : … status: completed … iters: 1   spend: $0.0021
    …

$ agt runs show <lead>
  status     : completed (4 iters, 13ms)
  spend      : $0.0084
    delegated → run-…  (task: subtask 1)
      ↳ completed (1 iters, 2ms, $0.0021)
    delegated → run-…  (task: subtask 2)
      ↳ completed (1 iters, 1ms, $0.0021)
```

Spend is now visible at every level: the row, the lead's header, and each
delegation — completing the path from `runs stats`' aggregate down to the
individual run.

## What's next

The multi-agent axis is observable (M41–M45), governed (M46–M48), legible (M49),
and now costed end-to-end (M47 aggregate → M50 per-run). Sharpest remaining
frontiers:

1. **Journal the run answer** (MED) — `llm.response`/`task.completed` carry
   `text_chars`/`usage`, not the body, so the M44 outcome and the "final answer:"
   arc section can't show real text. Adding it (weigh journal size + redaction)
   lights up both. Touches `kernel/agent/agent.go` (publish) + the renderers.
2. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` (M39
   pattern) + a tenant-token allowlist; the last non-tenant-aware control surface.
3. **Boot-banner the delegation caps** (LOW) — echo the active depth / fan-out /
   spend ceilings at daemon startup (next to the model-advisory / recovery
   banners), so they're visible in the daemon log too, not only via `agt status`.
4. **`agt runs stats` spend percentiles** (LOW) — extend the M47 spend aggregate
   with a per-run cost distribution (avg/p50/p95), mirroring the duration block.

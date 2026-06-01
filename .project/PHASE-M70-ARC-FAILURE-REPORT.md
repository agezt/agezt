# Phase Report — Milestone M70 (failure reason in the task arc)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

When a run fails, the first question is "why?". `agt runs show` rendered a failed
run's arc with a bare `status: failed` — the reason (already folded onto the
summary row, and carried on the `task.failed` event) was silently dropped, and
`task.failed` itself fell to the arc's generic default branch. An operator
debugging a failure had to leave the arc and cross-reference `agt runs list` (or
`why`) to learn what every other surface already knew.

## What shipped (client-side, in `renderTaskArc`)

- **Header states the reason** — a failed run now renders
  `status: failed (<reason>) after <duration>`, drawing the reason from the
  summary row (the same M30/M61 fold `agt runs list --failed` uses) and the
  duration when the fold could compute it.
- **`task.failed` body case** — marks WHERE in the arc the run died
  (`task.failed: <reason>`), instead of the generic `task.failed (seq=N)` line.

## Design decisions

- **Reason from the summary, not re-derived.** The header reads `summary["reason"]`
  — the authoritative folded value — so the arc agrees with `runs list` and
  `runs stats`'s failed-by-reason breakdown by construction.
- **Header summarises, body locates.** The header answers "why did it fail?"; the
  inline `task.failed` line answers "at which round?" — the same
  header-summary / body-detail split M69 used for budget.

## Tests

- `TestRenderTaskArc_FailedRunShowsReason` — header renders
  `failed (max iterations exceeded) after 1.5s`; the body renders
  `task.failed: max iterations exceeded`.

Test count: **1312 → 1313**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt runs show <failed>
  correlation: run-01KT1B9T3GSPTMXNG42TMQZ6S9
  intent     : second exhausts the mock
  status     : failed (error) after 1ms

  round 1 (seq=16)
    llm.request
    task.failed: error
```
(Before M70: `status: failed` with no reason, and a generic `task.failed (seq=16)`.)

# Phase Report — Milestone M83 (`agt plan history`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-02/SPEC-08 observability.

## Why

M82 made a single plan run readable via `agt runs show <plan-corr>` — but you had
to already KNOW the correlation (printed once at execution). There was no way to
discover "what plans have run, and how did they turn out?". Plan runs aren't task
runs, so `collectRuns` / `agt runs list` never list them. M83 adds `agt plan
history`, the plan analogue of `runs list`.

## What shipped

- **Server `handlePlanHistory` (`plan_history.go`)** — folds `plan.started`
  joined with the terminal `plan.completed` / `plan.failed` (all under the same
  `plan-…` correlation) into one row per plan: name, node count, status, start
  time, duration. Newest-first, limited, with an optional `status` filter
  (completed|failed|running). Tenant routing via `kernelFor` (primary-only, like
  `CmdPlan`).
- **CLI `agt plan history [N] [--status <s>|--failed] [--json]`** (aliases
  `runs`/`ls`) — renders each execution with its outcome; drill in with
  `agt runs show <correlation>` (M82).

## Design decisions

- **Lifecycle fold, same shape as runs list.** A plan with a `plan.started` but
  no terminal event is `running`; the row layout (correlation / started·status·
  nodes / name) mirrors `runs list` so the two read alike.
- **Primary-only.** Plan execution (`CmdPlan`) isn't tenant-allowlisted, so its
  history isn't either — consistent boundary.

## Tests

- `TestPlanHistory_ListsAndJoinsOutcome` — three plans (completed / failed /
  running): all three listed newest-first (running plan-3 first), `--status
  failed` returns just the failed one with its name.

Test count: **1326 → 1327**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt plan history          # after running a plan twice
  last 2 plan execution(s):
    plan--142855.031
      started : 2026-06-01 14:28:55   status: failed in 2ms   nodes: 1
      plan    : summarize-plan
    plan--142854.960
      started : 2026-06-01 14:28:54   status: completed in 20ms   nodes: 1
      plan    : summarize-plan
```

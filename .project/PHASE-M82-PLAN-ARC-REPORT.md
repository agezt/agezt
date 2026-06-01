# Phase Report — Milestone M82 (plan-execution runs in `agt runs show`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-02/SPEC-08 observability.

## Why

`agt runs show` renders a task run's arc, but a PLAN run (`agt plan <file.json>`,
the SPEC-02 scheduler executing gate/loop nodes) was unreachable and unreadable:
its correlation ("plan-…") isn't a task run, so `collectRuns` never sees it and
`runs show` errored "no run with correlation"; and even the events that WERE
walked (plan.started, node.started/completed/failed) fell to the arc's generic
default branch. A plan run — a first-class execution path — had no legible view.

## What shipped (client-side, in `runs show`)

- **Reachable** — when no task-run summary matches the correlation, `runs show`
  no longer errors immediately; it walks the journal chain and, if that chain
  carries plan events, synthesises a header from them (`synthesizePlanSummary`:
  intent from `plan.started`'s name, status from `plan.completed`/`plan.failed`).
- **Legible** — the arc renders `plan: <name> (<n> node(s))` and each node as
  `node <id> [<kind>] started|completed (<bytes>B)|FAILED: <err>`, instead of
  generic `kind (seq=N)` lines.

## Design decisions

- **Synthesise, don't special-case the server.** The server already returns the
  full event chain (CmdJournalTail); the client just builds a minimal header from
  the plan lifecycle events it already has — no new control-plane command, no
  change to `collectRuns`.
- **Degrade gracefully.** A correlation with events but no recognisable plan
  lifecycle still renders (minimal "running"/"completed" header) rather than
  erroring — the arc never refuses a correlation that has journaled events.

## Tests

- `TestRenderTaskArc_PlanNodeEvents` — plan.started + node.started/completed/
  failed render legibly (name, node id/kind, output bytes, error).
- `TestSynthesizePlanSummary` — a plan chain yields `intent: "plan: deploy"`,
  `status: completed`.

Test count: **1324 → 1326**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt plan plan.json          # a one-node loop plan, mock provider
$ agt runs show plan--142504.340
  correlation: plan--142504.340
  intent     : plan: summarize-plan
  status     : completed (0 iters, —)

  plan: summarize-plan (1 node(s))
    node summarize [loop] started
    node summarize [loop] completed (377B)
  summary    : 0 round(s), 0 tool call(s)
```
(Before M82: `runs show` errored "no run with correlation".)

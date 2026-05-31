# Phase Report — Milestone M41 (Sub-agent delegation links in `agt runs`)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). **Opens a fresh axis.** The mature axes
> (runs observability, capability arc, tenant isolation) were essentially done, so
> this milestone turns to the least-explored layer: sub-agent delegation.

## Why

The `delegate` tool (P6-MULTI-01) lets a lead agent spawn a bounded sub-agent
(`kernel/runtime/subagent.go`). The sub-agent runs under its **own** correlation
and emits its own `task.received`/`task.completed`, so it already shows up as a
separate row in `agt runs list`. But the rows were **unlinked**: nothing told an
operator that `run-Y` was a sub-agent of `run-X`. The only record of the
relationship was the `subagent.spawned` event's payload, reachable solely by
reading the journal with `agt why --payload`. For a multi-agent system, "who
delegated to whom" is first-order information that was effectively invisible.

A read-only recon of the layer (subagent.go, the `delegate` tool,
`subagent.spawned` consumers) confirmed the architecture is complete but
observability is the gap — and that the run-link is the sharpest, smallest,
highest-value first increment.

## What shipped

- **`runEntry.ParentCorrelation` + fold (`kernel/controlplane/runs.go`)** —
  `collectRuns` now also handles `event.KindSubAgentSpawned`. The spawn event lives
  under the **parent** correlation and its payload names the **child**
  (`child_correlation`) and the parent (`parent`); the fold attaches the parent
  link to the *child's* run entry (creating it if the spawn is seen before the
  child's `task.received`). New `extractSpawnLink` payload helper.
- **`CmdRunsList` output** gains `parent_correlation` (empty for top-level runs).
- **CLI rendering (`cmd/agt/runs.go`)**:
  - `runs list` marks a sub-agent row `<corr>  ↳ sub-agent of <lead>`;
  - `runs show <lead>` renders the spawn as `delegated → <child> (task: …)` (a new
    `case "subagent.spawned"` in `renderTaskArc`) instead of the generic
    `subagent.spawned (seq=N)` fallthrough.
- **`AGEZT_DEMO_DELEGATE=1` escape hatch (`cmd/agezt/main.go`, `newDemoMock`)** —
  scripts the offline mock to delegate once (lead → sub-agent → lead final), so the
  multi-agent path is demoable with zero external services. Mirrors the existing
  `AGEZT_DEMO_FAIL_PRIMARY` precedent; demo-only.

## Design decisions

- **Reuse the existing fold, attach to the child.** The relationship is one extra
  event kind in `collectRuns`; no new endpoint, no new walk. The link is stored on
  the *child* entry (the natural owner — a child has exactly one parent), keyed off
  the spawn payload's `child_correlation` rather than the event's correlation (which
  is the parent).
- **Tolerate ordering.** The spawn is published before the child's `task.received`,
  so the fold get-or-creates the child entry — the link survives regardless of walk
  order.
- **Surface in both views.** `list` answers "which runs are sub-agents and of
  whom"; `show` answers "what did this run delegate". Both read the same
  `subagent.spawned` data, so they can't disagree.
- **Demo hook, not production behaviour.** The daemon's scripted mock can't call
  `delegate` on its own; rather than leave M41 un-demoable end-to-end, a tiny
  env-gated demo script (off by default) makes the path observable — consistent
  with the project's demo-gated culture.

## Tests

- `kernel/controlplane/runs_test.go` — `TestRunsList_SubAgentParentLink`: a
  synthetic parent (`task.received` + `subagent.spawned{child,parent}` +
  `task.completed`) and child (`task.received` + `task.completed`); the child row's
  `parent_correlation` is the parent, the parent row's is empty.
- Existing `subagent_test.go` (the delegation runtime itself) unchanged and green.

Test count: **1259 → 1260**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=1)

```
$ agt run "describe this project"     # the lead delegates once
  [offline-mock lead] I delegated the kernel-layout summary to a sub-agent …

$ agt runs list
  run-…W3RJG…  ↳ sub-agent of run-…05EN7G
    status: completed   iters: 1   intent: summarize the kernel package layout
  run-…05EN7G
    status: completed   iters: 2   intent: describe this project

$ agt runs show run-…05EN7G
  correlation: run-…05EN7G
  intent     : describe this project
  …
  delegated → run-…W3RJG…  (task: summarize the kernel package layout)
```

A single live run produced a parent and a sub-agent; `runs list` linked the child
to its lead and `runs show` called out the delegation with the child correlation
and the delegated task — the delegation tree is now visible end-to-end.

## What's next

The multi-agent axis is now open with one observability win; clear follow-ons:

1. **`agt why <child>` → parent backlink** (MED) — from a child correlation,
   discover its lead (search `subagent.spawned` by `child_correlation`) and print
   `spawned by <parent>`. Closes the child→parent discovery gap (today only
   parent→child is walkable).
2. **`agt runs list --tree`** (MED) — render the delegation hierarchy (lead with
   indented sub-agents) rather than a flat list; `parent_correlation` is now in the
   data, so this is pure rendering.
3. **Per-delegation outcome on the lead's arc** (LOW-MED) — in `runs show`, after
   `delegated → <child>`, show the child's terminal status/answer inline (fetch the
   child's run summary), so the lead's arc tells the whole story.
4. **Sub-agent budget/policy surfacing** (MED) — show how `delegate` is gated by
   Edict and whether sub-agents share the lead's governor budget.

# Phase Report — Milestone M42 (`agt why` sub-agent → parent backlink)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Second step on the fresh multi-agent axis;
> closes the exact gap M41 left open.

## Why

M41 made the delegation visible parent→child: a lead's `agt runs`/`why` already
shows its `subagent.spawned` event (it lives under the parent's correlation), and
`agt runs list` now links a child back to its lead. But the **inverse** wasn't
walkable: standing on a *sub-agent's* own event chain (`agt why <child-event>` or
`agt runs show <child>`), there was no way to discover *who delegated it*. The
spawn event isn't in the child's chain — it's in the parent's — so the child's
events have no backreference. An operator drilling into a sub-agent run hit a dead
end.

## What shipped

- **`Kernel.ParentOf(childCorr string) string` (`kernel/runtime/runtime.go`)** — a
  single forward journal scan for a `subagent.spawned` event whose payload names
  `childCorr` as its `child_correlation`; returns the spawn's `parent`, or "" if
  the correlation was never delegated (a top-level run) or is empty. This is the
  only way to walk child→parent, since the spawn lives under the parent's chain.
- **`handleWhy` (`kernel/controlplane/server.go`)** now returns `correlation` (the
  chain's shared id) and `parent_correlation` (via `ParentOf`) alongside `events`.
- **`agt why` (`cmd/agt/why.go`)** prints `spawned by <lead>  (try: agt runs show
  <lead>)` when the chain is a sub-agent's, and carries both new fields in `--json`.
  The hint points at `agt runs show` (which takes a correlation and renders the
  lead's arc — including M41's `delegated → <child>` line back to this child),
  rather than `agt why` (which needs an event id, not a correlation).

## Design decisions

- **Scan, don't index.** `Why` already does a full journal walk; `ParentOf` is one
  more bounded scan. An in-memory child→parent index (rebuilt at boot) is the
  optimization if delegation graphs ever get large, but it's premature now — the
  handoff flags it as a later option.
- **Resolve from the chain's correlation, not the event id.** `handleWhy` takes an
  event id; the returned chain all shares one correlation (`events[0].Correlation
  ID`). `ParentOf` keys off that correlation, so the backlink works from *any*
  event in the sub-agent's chain, not just its `task.received`.
- **Point the hint at `runs show`, not `why`.** A correlation isn't an event id, so
  `agt why <parent-correlation>` wouldn't resolve. `agt runs show <correlation>` is
  the right drill-up and closes the loop with M41's parent-side rendering.
- **Reuses the M41 event shape.** Both directions read the same
  `subagent.spawned{child_correlation, parent}` payload, so they can't disagree
  about the relationship.

## Tests

- `kernel/runtime/runtime_test.go` — `TestParentOf`: resolves a child's lead from a
  published spawn event; unknown and empty correlations return "".
- `kernel/controlplane/runs_test.go` — `TestWhy_SubAgentParentBacklink`: `CmdWhy`
  on an event in a sub-agent's chain returns `parent_correlation` = the lead; on a
  top-level run's event it returns "".

Test count: **1260 → 1262**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=1)

```
$ agt run "describe this project"        # lead delegates once (M41 demo hook)
$ agt why <a-sub-agent-event-id>
  … chain events …
  spawned by run-…BXSYAT…  (try: agt runs show run-…BXSYAT…)

$ agt why <same-event> --json
  "correlation": "run-…BZ6V…",            ← the sub-agent's chain
  "parent_correlation": "run-…BXSYAT…"    ← its lead
```

Standing on a sub-agent's event, `agt why` now names its lead and points to the
drill-up command — the delegation tree is walkable in both directions (M41
parent→child, M42 child→parent).

## What's next

The multi-agent axis now has both-direction navigation; remaining increments:

1. **`agt runs list --tree`** (MED value, LOW risk) — render the delegation
   hierarchy (lead with indented sub-agents) rather than a flat list;
   `parent_correlation` is already in the data (M41), so this is pure rendering in
   `cmdRunsList` (build a parent→children map, DFS).
2. **Per-delegation outcome on the lead's arc** (LOW-MED) — in `runs show`, after
   `delegated → <child>`, show the child's terminal status/answer inline.
3. **Sub-agent budget/policy surfacing** (MED) — show how `delegate` is gated by
   Edict and whether sub-agents share the lead's governor budget/ceiling.
4. **Tenant-scoped `why`** (LOW-MED) — `handleWhy` is primary-journal-only; route it
   via `kernelFor` like M39 did for runs, and allowlist it for tenant tokens.

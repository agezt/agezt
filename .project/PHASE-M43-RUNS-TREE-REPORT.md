# Phase Report — Milestone M43 (`agt runs list --tree`)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Third step on the multi-agent axis;
> completes the delegation-observability trio (M41 link → M42 backlink → M43 tree).

## Why

M41 added `parent_correlation` to every runs-list row and M42 made the
parent↔child relationship walkable both ways. But `agt runs list` still rendered a
**flat**, newest-first list — a lead and its sub-agents appeared as sibling rows,
their relationship conveyed only by a `↳ sub-agent of <lead>` tag. For a run that
delegates several subtasks (or nests delegations), the operator had to mentally
reassemble the tree. M43 renders it directly.

## What shipped

- **`--tree` flag (`cmd/agt/runs.go`, `cmdRunsList`)** — opt-in; the flat
  newest-first list stays the default and `--json` is untouched.
- **`renderRunRow` (factored)** — the existing three-line per-row renderer
  extracted to take a base indent and a `showParentTag` flag, so the flat and tree
  views share exactly one row formatter. The flat view keeps the `↳ sub-agent of`
  tag; the tree view suppresses it (the nesting already shows parentage).
- **`renderRunsTree`** — builds a `parent → children` map and a `corr → row`
  index from the fetched rows, picks the roots (a run with no parent, **or** whose
  parent isn't in the fetched set — so a window-trimmed sub-agent still shows),
  then depth-first walks each root, indenting two spaces per level.

No server change: `parent_correlation` was already in the `CmdRunsList` output
(M41), so M43 is pure client rendering.

## Design decisions

- **One row formatter, two views.** Extracting `renderRunRow` means the flat and
  tree outputs can't drift in how a run is summarised — only the indent and the
  parent-tag differ.
- **Absent-parent → root.** The list is windowed (`limit`), so a sub-agent's lead
  may be outside the fetched rows. Rather than drop such a sub-agent (hiding a real
  run) or crash, it's promoted to a root — the tree degrades gracefully to a
  forest.
- **DFS, server order within a level.** Children are rendered in the order the
  server returned them (newest-first), and a node's whole subtree prints before its
  next sibling — so the structure reads top-down without reordering surprises.
- **Opt-in.** The flat list is the right default for "what ran recently"; the tree
  is for "how did this delegated task decompose". `--tree` keeps both cheap.

## Tests

- `cmd/agt/runs_tree_test.go` — `TestRenderRunsTree_HierarchyAndIndent`: a root +
  two children + a grandchild + an orphan-child (parent absent) render with the
  correct per-level indent (2/4/6 spaces), the orphan promotes to a root (indent
  2), and a child's whole subtree precedes its sibling (DFS order).

Test count: **1262 → 1263**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=1)

```
$ agt runs list                       # flat (default)
  run-…AY13…  ↳ sub-agent of run-…AHBV…   intent: summarize the kernel package layout
  run-…AHBV…                              intent: describe this project

$ agt runs list --tree                # hierarchy
  run-…AHBV…                              intent: describe this project
      run-…AY13…                          intent: summarize the kernel package layout
```

A live delegation renders flat as two tagged sibling rows and, under `--tree`, as
the lead with its sub-agent nested beneath it — the delegation tree made visible.

## What's next

The delegation-observability trio (link/backlink/tree) is complete. Remaining
multi-agent increments:

1. **Per-delegation outcome on the lead's arc** (LOW-MED) — in `runs show`, after
   `delegated → <child>`, show the child's terminal status/answer inline (the
   server already has the child's run summary via `collectRuns`).
2. **Sub-agent budget/policy surfacing** (MED) — how `delegate` is gated by Edict,
   and whether sub-agents share the lead's governor budget/ceiling
   (`kernel/runtime/subagent.go` + governor). Deeper, more product value.
3. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` like M39
   did for runs, and allowlist it for tenant tokens.
4. **`agt runs stats` delegation metrics** (LOW) — count of runs that delegated,
   avg sub-agents per run, etc.

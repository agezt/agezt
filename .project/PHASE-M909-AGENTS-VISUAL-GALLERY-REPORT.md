# Phase M909 — Visual Agents gallery (replace the dropdown)

## Ask (owner)
> "agents sayfasını daha görsel yapman lazım — dropdown agent seçmek değil, daha
> bol her şeyi göreceğim bir ekran isterim."

The Agents view (the multi-agent / delegation monitor) forced the operator to
pick a lead run from a **`<select>` dropdown** before anything appeared. The owner
wants a rich, visual screen that shows the whole fleet at once.

## What shipped — `frontend/src/views/Agents.tsx` (rewrite)
A two-mode view:

**Gallery (default):**
- **Summary band** — five big stats: leads, running (accented when > 0), total
  sub-agents, roster size (`/api/agents` count), total fleet spend.
- **Filter chips** (all / running / done / failed) with per-bucket counts —
  replacing the dropdown.
- **Card gallery** — one rich card per lead run (responsive 1→2→3 cols), each
  showing: a status dot (pulsing while running) + status, the agent identity badge
  (when the run carries one), start time, the intent (2-line clamp), the answer
  preview for finished runs, and a chip row of **agents / sub-agents / depth /
  iterations / tree-spend** — everything at a glance. Cards sort running-first then
  newest.

**Drill-down (click a card):** the existing live `DelegationGraph` + per-node
`RunDetailLoader` detail panel (reused unchanged), with a back button and the
tree's aggregate stats.

Live refresh (6s poll + event nudges) and the delegation aggregates are reused
from the prior version; the `<select>` is gone.

## Pure, tested helpers (exported)
- `statusKind(status)` → `running|done|failed|other` (status normalization);
- `summarizeRoots(runs)` → one `RootSummary` per lead, folding each delegation
  subtree (count/depth/spend via the existing `buildDelegationTree`), sorted
  running-first then newest, sub-agents never surfaced as their own cards;
- `filterRoots(roots, filter)`.

## Tests — `frontend/src/views/Agents.test.tsx`
`statusKind` buckets; `summarizeRoots` folds a 3-node depth-2 tree (spend summed,
sub-agents counted, agent identity + answer preview captured, running sorts
first, sub-agents excluded as cards); `filterRoots` chips. 5 tests.

## Gate
`tsc` ✓ · full vitest **535 pass** (79 files) ✓ · `vite build` → embedded dist
(LF) ✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend +
dist only (no backend change — all fields already in the `/api/runs` payload).

## Notes
Roster (the named-agent CRUD) stays its own sibling tab — this view is the live
*fleet monitor*. Reused `buildDelegationTree`, `DelegationGraph`, `RunDetailLoader`,
`money`, `fmtTime`, `clip` rather than re-deriving (the reuse-over-create rule).
Follow-ups possible if wanted: surface each run's agent identity in `/api/runs`
(today only set when a run runs `--agent`), a roster strip on the same screen.

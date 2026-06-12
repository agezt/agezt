# M923 — Runs: status summary band + click-to-filter chips

## Problem

The Runs view is the core "what has my fleet actually run?" monitoring surface,
but it was a flat expandable list with only a text search. Every other monitoring
gallery — Agents (M909), Roster (M911), Tools (M916), Schedules (M917), World
(M918) — already got a summary band + click-to-filter chips so the *shape* of the
data is visible before you scan it. Runs was the gap: to learn "how many runs
failed?" you had to type `failed` into the search box and read the match count,
and there was no at-a-glance distribution at all.

This is squarely the owner's standing webui direction ([[webui-visual-richness]]):
more visual, see-everything-at-once surfaces over type-to-discover.

## Change

Frontend-only, `frontend/src/views/Runs.tsx` (+ its test):

- **Pure helpers (exported, unit-tested):**
  - `runBucket(run)` → `"running" | "completed" | "failed" | "other"`, mirroring
    Insights' outcome buckets. `abandoned` counts as `failed` (ended without
    finishing); unknown/future statuses fall to `other` so nothing vanishes.
  - `runCounts(runs)` → `{total, running, completed, failed, other}`; the four
    buckets always sum to `total`.
- **Summary band:** a `N total` count beside the title.
- **Status chips:** a row of click-to-filter chips (running / completed / failed /
  other) with status-coloured dots and live counts. Clicking narrows the list to
  that outcome; clicking the active chip clears back to all. A zero-count chip is
  disabled. Composes with the existing text search (both filters AND together).
- **Deep-link safety:** if a ⌘K / alert deep-link focuses a run that the active
  chip would hide, the filter auto-clears so the targeted run is actually visible
  when it scrolls into view.

The existing search box, run-focus expand/scroll, and empty states are unchanged
(the empty hint now names whichever of filter/query is active).

## Verification

- `npx vitest run src/views/Runs.test.tsx` → 7 passed (added `runBucket`/
  `runCounts` unit tests + chip click/clear + disabled-zero-count interaction;
  updated the search fixtures from the non-real `"done"` status to `"completed"`).
- Full suite: `npx vitest run` → **568 passed (83 files)**.
- `npx tsc --noEmit` → clean; `npm run build` → dist rebuilt; `go build
  ./kernel/webui/` → clean; dist committed LF (`eol: lf`).

Real run statuses confirmed against the kernel: `completed / failed / running /
abandoned` (the lone `"done"` in the tree is mock final-text, not a run status).

Scope: `frontend/src/views/Runs.{tsx,test.tsx}` + rebuilt `kernel/webui/dist` +
CHANGELOG. No backend, no contested files. Shipped from an isolated worktree off
`origin/main`.

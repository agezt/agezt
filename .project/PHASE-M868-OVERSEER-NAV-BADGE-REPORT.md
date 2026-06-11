# PHASE M868 — Live active-runs badge on the Overseer nav item

**Status:** shipped
**Milestone:** M868 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** Product/live-monitoring layer — ambient supervision. After making the
Overseer live (M867), this surfaces "how many runs are in flight" from **any**
view, so the operator doesn't have to be on the Overseer tab to notice activity.

## What shipped

`frontend/src/App.tsx`: a live count badge on the **Overseer** sidebar nav item.

- `activeRunCount` folds the live event buffer (`useEvents().events`) through the
  existing `foldActivityEvent` + `summarize` (`@/lib/activity`) and takes
  `summary.running`. Events are newest-first, so the fold runs in reverse
  (chronological) order. Memoized on `events`.
- The badge renders only when `activeRunCount > 0`, styled as an accent pill
  (distinct from the red unseen-alert badge), capped at `99+`, with a
  `"{n} runs in flight"` tooltip and an aria-label.

Same cold-start semantics as the existing alert badge (M779): it reflects the
live event buffer, so it tracks runs as their events arrive.

## Why this milestone is conflict-free

Purely frontend. Touches **only** `frontend/src/App.tsx` and the rebuilt
`kernel/webui/dist`. Reuses the already-shipped `useEvents()` provider and the
tested `foldActivityEvent`/`summarize` helpers — no new route, no Go change. The
concurrent session's in-flight kernel edits are untouched.

## Verification

- **Frontend gate:** `tsc --noEmit` clean; `vite build` emits the committed-LF
  dist (1855 modules); `vitest run src/lib/activity` green (7/7) — confirms the
  fold/summarize logic the badge depends on is intact.
- No Go change → contested kernel packages not compiled; full `go build ./...`
  deliberately skipped (it would compile the concurrent in-progress Go edits).

## Notes
- Mirrors the M779 unseen-alert badge pattern, so the sidebar now carries two
  ambient monitoring signals: red = the agent flagged something (Alerts), accent =
  runs are executing (Overseer). The badge is a count (always shown while > 0),
  not an unseen-delta, because "in flight" is a live state, not a notification.

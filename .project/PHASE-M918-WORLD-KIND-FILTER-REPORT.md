# Phase M918 — World: clickable kind-filter chips

## Ask
Continuation of the webui visual / navigation arc. The World view (the agent's
entity/knowledge graph) already had a kind **breakdown bar**, a force-graph, and a
free-text search — but the breakdown was purely a visual; you couldn't *act* on it
to focus the entity list on one kind.

## What shipped — `frontend/src/views/World.tsx` (additive)
- **Kind-filter chips** below the breakdown bar — one per entity kind (with counts)
  plus "all". Click a kind to narrow the entity list to it; click again to clear.
  Composes with the existing text search (kind AND query).
- The breakdown bar now uses the shared sorted `kindBreakdown` (count desc).
- The "no entities match" message reflects both the query and the active kind.

Everything else (add/relate/edit/forget, graph, import/export) is untouched.

## Pure helpers (exported, unit-tested)
- `kindBreakdown(ents)` — per-kind counts, sorted by count then name (now exported).
- `filterEntities(ents, query, kind)` — text (via `entityMatches`) + exact-kind
  filter, composed.

## Tests — `frontend/src/views/World.test.tsx`
`kindBreakdown` (counts, sort, blank→"entity") and `filterEntities` (exact kind,
kind+query composition, empty passthrough). Full suite **552 pass**.

## Gate
`tsc` ✓ · full vitest **552 pass** (80 files) ✓ · `vite build` → embedded dist (LF)
✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend + dist
only.

## Process
Built in an isolated git worktree (`AGEZT-world`, branch
`feat/m918-world-kind-filter`) from `origin/main`. Consistent with the chip-filter
pattern from the Agents gallery (M909) and Tools gallery (M916).

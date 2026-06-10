# Phase M785 — Roster view: manage named agents from the console

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 3
(console surface for the M783 roster; routes shipped in M783, delegate-by-slug
in M784).

## What

New **Roster** view (Agents nav group, `#roster`, ⌘K-indexed automatically):
lists every named agent — slug, enabled/paused badge, model, task type,
per-run budget (`money()`), memory scope, workdir, fallbacks chain,
description, full soul — with create / edit / pause / resume / remove.

## Design

- `frontend/src/views/Roster.tsx` (~430 lines), Standing.tsx idiom verbatim:
  `getJSON`/`postJSON`/`postAction`, `useUI()` toast + confirm (remove is
  confirm-gated, danger), `act()` busy-guard, SkeletonList, EmptyState
  (teaches `agt run --agent <slug>`), 8s live refresh.
- Exported per the M714 recipe: `NewAgentForm`, `EditAgentForm`, plus pure
  helpers `slugOk` (mirrors the kernel slug regex; create disabled until
  valid, red border while invalid) and `usdToMc` ($1 = 1e9 microcents; blank
  = no cap; NaN/negative rejected client-side before any POST).
- Edit shows the slug read-only ("slug is permanent") and posts the mutable
  fields wholesale to `/api/agents/edit {ref, profile}` — matching the
  server's wholesale-apply semantics, prefilled from the current profile.
- `App.tsx`: 3-line wiring (lucide `Users` import, view import, nav item) —
  deep link + command palette come free.
- **No Go changes**; dist rebuilt and committed (LF, .gitattributes).

## Tests (7 new Vitest; 451 total green)

slugOk good/bad table · usdToMc conversions (0.50 → 5e8, "$1", blank=0,
NaN/negative=null) · create disabled until valid slug, then posts the exact
profile shape (fallbacks split, budget converted) · bad budget → onError, no
POST · list renders state/model/budget/soul from /api/agents · pause posts
`{ref, enabled:"false"}` · empty state.

## Browser verification (real browser, isolated daemon, new embedded dist)

Booted daemon with the rebuilt dist (NOTE: 18xxx ports sat in a Windows
reserved range — bind failed; moved to 23987). In Playwright: #roster deep
link rendered nav + empty state → New agent form → filled slug/name/model/
$0.50/soul → Create → card rendered every field → Pause flipped the badge to
*paused* → **CLI cross-check**: `agt agent list` showed the same agent PAUSED
(one roster, every surface). **0 console errors during the live session**
(the 7 in the final log are ERR_CONNECTION_REFUSED at 97–102s — after the
daemon was deliberately shut down; verified from the saved console log).

## Gate

`tsc --noEmit` + vite build clean (one fix: ErrorText takes children, not
text) · 67 Vitest files / 451 tests green · dist rotation committed · Go tree
untouched this milestone (suite green as of M784, same HEAD otherwise) · CI
org-billing still blocked → local battery + arc-authority merge.

## Next in the arc

memory_scope/workdir wiring into runs · Fallbacks → per-agent routing chain ·
A2A ask/reply on the board · chat: run a conversation as a named agent.

# Phase M911 — Visual Roster card grid + agent identity avatars

## Ask
Continuation of the owner's "webui'yi daha görsel yap" directive ([[webui-visual-richness]]).
After the Agents monitor (M909), its sibling tab **Roster** (the named-agent CRUD)
was still a flat vertical list of rows.

## What shipped — `frontend/src/views/Roster.tsx`
- **Agent identity avatars** — each agent gets a deterministic colored monogram
  (`agentHue(slug)` → stable HSL; `initials(name, slug)` → 1–2 char monogram),
  dimmed/grayscaled when retired. The roster now reads visually at a glance.
- **Summary band** — agents / enabled (accented) / paused / graveyard counts.
- **Card grid** — the `<ul>` of rows became a responsive grid (1→2→3 cols). A card
  whose **edit form or activity timeline** is open spans the full width, so the
  2-column form and the timeline have room (no cramped half-width forms).
- All existing functionality — create / edit / pause / resume / retire / revive /
  remove / activity, the meta line, the soul preview — is unchanged; only the
  layout and the avatar are new.

## Pure helpers (exported, tested)
- `agentHue(slug)` — deterministic 0–359 hue from a tiny string hash;
- `initials(name, slug)` — two name-word initials, else two chars, else the slug.

## Tests — `frontend/src/views/Roster.test.tsx`
- `agentHue` is deterministic + in range + differs across slugs;
- `initials` covers name/two-word/slug fallbacks;
- the existing render test updated for the summary band (`paused` now also appears
  as a stat label — asserted via `getAllByText`). 9 tests pass.

## Gate
`tsc` ✓ · full vitest **540 pass** (79 files) ✓ · `vite build` → embedded dist
(LF) ✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend +
dist only.

## Notes
Same visual language as M909 (summary band + grid) so the two "Agents" tabs feel
consistent. The avatar helpers are pure + reusable — a follow-up could show the
same monogram next to a run's agent identity in the Agents gallery once `/api/runs`
carries the slug.

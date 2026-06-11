# PHASE M878 — Per-skill last-used recency + idle hint (completes #37)

**Status:** shipped
**Milestone:** M878 (numbered above the concurrent session's M868–M876 arc to
avoid collision).
**Theme:** Backlog **#37 — skill usage tracking + cleanup (in-use vs idle)**.
Closes the last visibility gap: every skill card now shows *when it was last
used* and flags active-but-never-used skills, so "in-use vs idle" is readable
per skill — not only via the aggregate hygiene strip.

## Context: #37 was already ~90% there

- **Tracking** (kernel): `skill.Metrics` records `Uses`, `Successes`, `Failures`,
  `LastUsedMS`; the forge bumps them on each use.
- **Cleanup** (M858): `Forge.Hygiene` flags idle active skills (never-used or
  stale), surfaced as the Skills view's idle strip with one-click retire
  (quarantine).
- **API**: `skillView` already serializes `metrics.{uses,successes,failures,last_used_ms}`.

The remaining gap was purely presentational: the card showed `used N×` but never
the recency, and didn't call out an active skill that has never fired.

## What shipped (frontend only)

- `lib/utils.ts` — a reusable `fmtAgo(ms)` coarse relative-time helper
  ("3m ago" / "2d ago" / "5mo ago").
- `views/Skills.tsx` — the per-card metric line now reads
  `used N× · K ok · last <ago>` (using `metrics.successes`, falling back to the
  legacy `wins`, and `metrics.last_used_ms`). When an **active** skill has zero
  uses it instead shows an amber `idle · never used` hint. Extended the `metrics`
  TS type with `successes` / `failures` / `last_used_ms`.

## Why this milestone is conflict-free

Purely frontend. Touches **only** `frontend/src/lib/utils.ts`,
`frontend/src/views/Skills.tsx`, and the rebuilt `kernel/webui/dist`. Reuses the
existing skill-list route (no new endpoint, no Go change). The concurrent
session's kernel edits are untouched.

## Verification

- **Frontend gate:** `tsc --noEmit` clean; `vitest run src/views/Skills src/lib/utils`
  green (14/14); `vite build` emits the committed-LF dist (1855 modules).
- No Go change → contested kernel packages not compiled; full `go build ./...`
  deliberately skipped.

## Notes
- With tracking + cleanup + per-skill recency/idle visibility all in place, #37 is
  complete. `fmtAgo` is now available for other "last seen" labels (Roster,
  Overseer) to reuse.

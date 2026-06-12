# Phase M917 — Schedules: summary band + live "fires in …" countdown

## Ask
Continuation of the webui visual / situational-awareness arc. The Schedules view
listed each schedule with an absolute next-fire time, but gave no fleet-level
glance and no sense of *what's about to fire* — you had to read each timestamp and
do the math.

## What shipped — `frontend/src/views/Schedules.tsx`
Additive (the 540-line view's create/edit/forecast/import logic is untouched):
- **Summary band** — schedules / enabled / paused / **due within 1h** (the last two
  accented when non-zero), so the schedule fleet reads at a glance.
- **Live "fires in …" countdown** on each card next to the absolute next-run time —
  `now`, `in 45s`, `in 12m`, `in 3h`, `in 2d`, or `overdue`. A coarse 5s clock keeps
  it live without refetching; due-soon countdowns are accented.
- `last_status` was already badged (left as-is).

## Pure helpers (exported, unit-tested)
- `untilLabel(nextMs, nowMs)` — the coarse countdown (deterministic; `now` injected).
- `scheduleCounts(items, nowMs)` — total / enabled / paused / dueSoon (enabled-only,
  within `DUE_SOON_MS` = 1h).

## Tests — `frontend/src/views/Schedules.test.tsx`
`untilLabel` across the second/minute/hour/day buckets + overdue/now; `scheduleCounts`
tallies (paused schedules excluded from due-soon, continuous/no-next excluded). 19
tests in the file; full suite **549 pass**.

## Gate
`tsc` ✓ · full vitest **549 pass** (80 files) ✓ · `vite build` → embedded dist (LF)
✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend + dist
only (all fields already in `/api/schedules`).

## Process
Built in an isolated git worktree (`AGEZT-sched`, branch
`feat/m917-schedules-cockpit`) from `origin/main`. M917 verified free against
`git log` + open PRs (M912/M915 still in flight from the other session) right before
committing.

# Phase M921 — Doctor / diagnostics in the Health view

## Ask
Finishing the owner's "güven & maliyet" direction. The CLI has `agt doctor` (a
preflight checklist with remediation hints), but the **webui had no active
diagnosis** — the Health view showed passive vitals (gauges, sparklines), never
"here's what's wrong and how to fix it."

## What shipped — `frontend/src/views/Health.tsx`
A **Diagnostics ("doctor") card** at the top of Health that evaluates the daemon's
live state into actionable issues, each with a deep-link to the view that fixes it:
- **Daemon unreachable / halted** → fail (resume from Policy).
- **Journal verification failed** → fail (the `/api/journal/verify` endpoint
  rejects on a broken chain — that rejection IS the signal; → Search/inspect).
- **Provider failing over** → warn, with the last reason (`/api/providers`).
- **No default model set** → warn (→ Models).
- **Elevated failure rate** → warn (≥5 runs and >20% failed; → Runs).
- **Pending approvals** → info (→ Approvals).

When everything's clean it reads "all systems healthy"; the card tone + header
reflect the worst level. Added `/api/journal/verify` to Health's parallel refresh.

## Pure helpers (exported, unit-tested)
- `runDiagnostics(status, stats, journalOk)` — the check engine; returns only the
  not-ok issues (empty = healthy). `journalOk` is `null` while the verify is in
  flight (no false alarm).
- `worstLevel(diags)` — most-severe rollup for the card tone.

## Tests — `frontend/src/views/Health.test.tsx`
daemon-unreachable; clean daemon → empty; every issue type fires with the right
level + deep-link + the provider reason; the fail-rate min-sample guard;
`worstLevel` rollup. 5 tests; full suite **564 pass**.

## Gate
`tsc` ✓ · full vitest **564 pass** (82 files) ✓ · `vite build` → embedded dist (LF)
✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend + dist
only (all data from existing endpoints; no backend change).

## Direction wrap-up (all four the owner picked)
1. **Proaktif ulaşma** — M919 desktop notifications (daemon-channel push = backend
   follow-up still open).
2. **Sesle giriş** — already existed (MicButton M689).
3. **Chat aksiyonları** — already existed (copy/export/regenerate/run-as-agent/…).
4. **Güven & maliyet** — M920 budget forecast + **M921 doctor page** (this) → done.

## Process / numbering
Built in an isolated worktree from `origin/main`. NOTE: the other session's open
PR #348 is mislabeled "M919" (already taken by my merged #346) — it'll renumber on
merge. Verified M921 free against `git log origin/main` + open PRs before
committing.

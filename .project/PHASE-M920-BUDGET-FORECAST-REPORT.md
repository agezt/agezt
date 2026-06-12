# Phase M920 — Budget forecast ("at this pace")

## Ask
Part of the owner's "güven & maliyet" (trust & cost) direction. The Budget view
showed today's spend vs the daily ceiling, but only the *present* — no sense of
where the day is heading. "Am I about to blow the budget?" needed mental math.

## What shipped — `frontend/src/views/Budget.tsx`
- **`projectedDailySpend(spentMc, nowMs)`** (pure, exported, unit-tested) —
  extrapolates today's spend to UTC end-of-day from the fraction of the UTC day
  elapsed (`spent / frac`). The budget resets at UTC midnight (`utc_date`) and the
  Unix epoch is UTC-aligned, so `nowMs % dayMs` is ms since midnight. Returns
  `null` in the first ~hour (`frac < 0.04`) where extrapolation is just noise.
- A **forecast card** under the spend gauge: "Projected today · at this pace →
  $X". When a ceiling is set, it says whether you're comfortably within it or **on
  track to exceed it** (turning the card red) — and notes that the daily cap will
  halt spend before then. Hidden when nothing's been spent or it's too early.

## Tests — `frontend/src/views/Budget.test.tsx`
`projectedDailySpend`: quarter/half-day extrapolation, `null` in the noisy first
hour, projects once past the threshold. Full suite **559 pass**.

## Gate
`tsc` ✓ · full vitest **559 pass** (81 files) ✓ · `vite build` → embedded dist (LF)
✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend + dist
only (all data already in `/api/budget`).

## Direction status (the owner picked all four)
1. **Proaktif ulaşma** — desktop notifications shipped (M919); daemon-side channel
   push still a backend follow-up.
2. **Sesle giriş (STT)** — *already exists* (MicButton M689 → `/api/transcribe`);
   nothing to build.
3. **Chat aksiyonları** — *largely already exists* (copy answer, export-to-markdown,
   regenerate, run-as-agent, persona, prompt launcher); only niche gaps remain.
4. **Güven & maliyet** — this (budget forecast). A diagnostic **"doctor" page**
   (active checks + remedies for unkeyed providers / disk / daemon) is the
   remaining genuine gap here — good next milestone.

## Process
Built in an isolated git worktree from `origin/main`. M920 verified free against
`git log` + open PRs (M912/M915 still in flight).

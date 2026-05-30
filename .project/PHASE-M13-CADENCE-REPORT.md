# Phase Report — Milestone M13 (Deep autonomy: a cron-grade scheduler, operator-managed)

> Status: **shipped** · Date: 2026-05-31
> ROADMAP autonomy track · the timer companion to Pulse's event-driven
> proactivity. Where Pulse makes the system *react* to events, the cadence
> scheduler makes it *act on its own clock* — and this milestone turns that from
> a single env-seeded interval list into a full, operator-managed, cron-grade
> surface that survives restarts and flows through the same governed loop as
> `agt run`.

## Why this milestone

The frontier the operator chose was **"otonomi derinleştir"** — deepen autonomy.
A scheduler existed (env-seeded `AGEZT_SCHEDULE` interval jobs), but it was not
something an operator could manage at runtime, and its only cadence was "every N
seconds". A Jarvis-grade system needs the cadences people actually think in:
*every hour*, *every weekday at 09:30*, *weekends at 11:00*, *remind me in 30
minutes*, *at 18:00 today*. And it needs to manage them live — add, list, pause,
resume, fire-now, remove — without editing env and restarting.

M13 closes that gap, stdlib-only, demo-gated, with `go.mod` unchanged.

## What shipped

### 1. Persistent, operator-managed schedule store (`kernel/cadence.Store`)
Schedules now live in a single atomically-rewritten `schedules.json` under the
kernel base dir, opened by `runtime.Open()` and exposed as `k.Schedules()`. It is
the **one source of truth** for both operator-managed entries (`source=operator`)
and env-seeded ones (`source=env`, synced from `AGEZT_SCHEDULE` at startup via
`SyncEnv`, which replaces only env entries and leaves operator ones untouched).
Survives restarts; every mutation persists immediately.

### 2. `agt schedule` — the operator CLI + control-plane surface
A full management verb over the token-authed control plane (handlers in
`kernel/controlplane/schedule.go`, commands `schedule_{add,list,remove,run,enable}`):

- `add "<intent>" --every <dur>` — recurring interval.
- `add "<intent>" --at <HH:MM> [--days <spec>]` — daily wall-clock, optionally
  weekday-filtered.
- `add "<intent>" --in <dur>` / `--once --at <HH:MM>` — one-shot.
- `edit <id> [--intent <s>] [--model <id>] [<cadence flag>]` — change a schedule
  in place, preserving its id. A field-only edit (intent/model) leaves the
  next-run time undisturbed; a cadence change (interval ↔ daily ↔ one-shot)
  recomputes it.
- `list [--json]` — id, rendered cadence, source, enabled/paused, next run.
- `run <id>` — fire on the next tick (marks due now).
- `pause <id>` / `resume <id>` — disable/re-enable without deleting.
- `rm <id>` — reversible removal.

### 3. Three cadences

- **Interval** (`ModeInterval`): fire every `IntervalSec`. The original behavior,
  preserved as the zero-value mode for backward-compatible stores.
- **Daily wall-clock** (`ModeDaily`, `--at 09:30`): fire once a day at a local
  time-of-day, advancing by **calendar date** (DST-correct — no 24h-add drift).
  Optionally restricted to specific weekdays via a `time.Weekday` bitmask:
  `--days mon-fri`, `--days weekends`, or a case-insensitive list/range like
  `mon,wed,fri` / `fri-mon` (inclusive, wrapping). `cadence.ParseDays` /
  `FormatDays` round-trip the spec; `nextDaily` walks forward to the next
  permitted weekday.
- **One-shot** (`ModeOnce`, `--in 30m` / `--once --at 18:00`): fire exactly once
  at a wall-clock instant, then **remove itself** from the store in `Store.Due`.
  The reminder / at-job primitive. A past time is rejected.

### 4. Pause/resume + no-overlap firing
`SetEnabled` toggles an entry's `Enabled` flag; the ticker (`Engine.fireDue` →
`Store.Due`) skips disabled entries (kept in the store, not deleted). A single
ticker fires every due entry on its own goroutine; an entry whose previous run is
still in flight is skipped (`running sync.Map`) so a slow intent never stacks.

### 5. Governed + auditable
Every firing runs through the normal kernel loop — same Edict policy, same
budget, same journal — and emits a `schedule.fired` event carrying the run's
correlation id, so `agt why` / `agt journal grep schedule.fired` show exactly
what the system did on its own and link it to the resulting run.

## Proven (demo-gated, live daemon + CLI)

- **Wall-clock + pause/resume:** `add --at 09:30` → "daily at 09:30", next run
  tomorrow; `pause` → paused; `resume` → enabled.
- **Weekday filtering, against a real Sunday (2026-05-31):** `--days mon-fri`
  correctly skipped Sunday to **Monday 06-01 09:30**; `--days weekends` fired the
  **same day**. JSON exposed `days` (62 = Mon-Fri, 65 = Sat+Sun).
- **One-shot end-to-end:** `--in 3s` fired through the governed loop
  (`schedule: firing "fire and vanish"`), journaled `schedule.fired`, and
  **self-removed** (list count → 0). A past one-shot was rejected.
- Input guards verified live: exactly one of `--every`/`--at`/`--in`; `--days`
  only with `--at`; `--once` requires `--at` and rejects `--days`.

## Engineering notes

- **Stdlib only.** `time`, `encoding/json`, `os`, `sort`, `sync` — no new deps;
  `go.mod` unchanged.
- **DST correctness** came from advancing daily/weekday schedules by calendar
  date (`time.Date(y, m, d+i, …)`) rather than adding 24h, so a schedule at 09:30
  stays at 09:30 across a spring-forward / fall-back boundary.
- **Backward compatibility:** the zero-value mode is interval; existing stores and
  env-seeded jobs keep working untouched. `Days == 0` means every day.
- **Tests: 1098** across 55 packages (green), `go vet` clean, `GOOS=linux`
  builds. New coverage: `ParseDays`/`FormatDays`, daily fire+advance, weekday
  skip+advance, one-shot fire+self-remove, all validation paths, and the
  control-plane round-trips for daily/days/once/pause-resume.

## Commits (local; this arc)

- `d8ba771` — operator-managed scheduling: `agt schedule` + persistent store
- `a477700` — daily wall-clock scheduling (`--at HH:MM`) + pause/resume
- `cee731a` — weekday filtering for daily schedules (`--days mon-fri`)
- `79e4083` — one-shot schedules (`--in 30m` / `--once --at 18:00`)
- `agt schedule edit` — change a schedule in place (intent/model/cadence)

## Deferred (named for future autonomy work)

- **Catch-up semantics** for daily/once schedules missed while the daemon was
  down (currently they advance past the missed slot rather than firing late).
- **Sub-daily cron expressions** (e.g. "every 15 min between 09:00–17:00 on
  weekdays") — the day/time/interval primitives are in place to build on.
- **Timezone-per-schedule** (today: the daemon's local zone for all wall-clock
  schedules).

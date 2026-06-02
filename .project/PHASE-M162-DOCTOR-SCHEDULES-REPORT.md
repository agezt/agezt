# M162 — Scheduled-run health check in `agt doctor`

## Why
`agt doctor` is the single-pane health surface (15 checks: daemon, journal, tools,
model, sandbox, provider, approvals, budget, catalog, webhooks, disk, exposure,
channels, halt). It had no check for the **autonomy axis** — scheduled runs.

Scheduled runs are precisely where a silent failure hurts: they fire unattended
(the cadence engine ticks every 10s; `AGEZT_SCHEDULE='1s=…'` seeds a fast one), so
when a firing errors there's no operator watching. The failure is journaled
(`schedule.fired` + the run's terminal event) and visible in `agt schedule list`'s
last-outcome column (M56), but invisible unless someone thinks to look. Folding it
into the go-to diagnostic surfaces broken automation proactively — the same
rationale as the webhooks check (M121).

## What
`cmd/agt/doctor.go`:
- `checkSchedules(ctx, client)` — calls `CmdScheduleList`; a call failure is an
  informational OK (never a false FAIL), mirroring `checkWebhooks`.
- `schedulesCheckFromList(res)` — the pure, testable verdict:
  - no schedules → OK "no schedules configured".
  - only **enabled** schedules are judged (a disabled schedule the operator turned
    off must not raise an alarm).
  - a schedule whose `last_status` is `failed` or `abandoned` counts as a failure;
    a never-fired schedule (no `last_status`) is healthy-by-default.
  - any failures → WARN "N/M enabled schedule(s) last firing failed", with the hint
    naming the **most-recently-failed** schedule's id
    (`agt schedule fires --id <id>`).
  - else → OK ("N enabled schedule(s), recent firings healthy", or "N schedule(s),
    none enabled").
- `int64Of(v any) int64` helper — coerces a decoded-JSON number to int64, returning
  -1 for missing/non-numeric so it sorts below any real `last_fired_unix_ms`.
- Wired into `runDoctorChecks` after `checkWebhooks`.

Reuses the existing `schedule_list` response shape (M56), which already carries
`enabled`, `last_status`, `last_reason`, and `last_fired_unix_ms` per row — no
control-plane change. Firing statuses are `running`/`completed`/`failed`/
`abandoned` (from `latestFiringBySchedule`).

## Tests (+1, all passing)
`TestSchedulesCheckFromList`: no-schedules OK; an enabled failed firing → WARN with
the id in the hint; `abandoned`+`failed` both count and the most-recent wins the
hint; a disabled failed schedule is ignored (OK); an enabled healthy firing → OK;
an enabled never-fired schedule → OK.

## Live proof (offline mock daemon)
- **OK path:** a daemon with no schedules → `agt doctor` shows
  `[OK  ] schedules : no schedules configured`.
- **WARN path:** a daemon seeded with `AGEZT_SCHEDULE='1s=keep summarizing the repo
  status'`, left ~28s so the schedule fired repeatedly and exhausted the mock's
  finite scripted responses (the later firings error). `agt schedule list` showed
  `last: failed (error)`, and `agt doctor` showed:
  `[WARN] schedules : 1/1 enabled schedule(s) last firing failed`
  `↳ inspect with \`agt schedule fires --id sched-…\` (or \`agt runs\`)`.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1524 tests** (was 1523; +1), 61 packages.

## Result
A scheduled run that starts failing now surfaces in `agt doctor` instead of sitting
silently in the journal — the autonomy axis joins the single-pane health view, with
a hint that points straight at the failing schedule.

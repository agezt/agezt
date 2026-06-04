# M297 — Web UI: schedule next-fire time

## Why
A pivot off the cost arc to the autonomy axis. `agt schedule list` shows each
schedule's `next <date+time>` (the `next_run_unix` the control plane already
returns), but the Web UI Schedules panel showed only cadence / intent / paused /
last-outcome — not *when* the schedule next fires. This closes that CLI↔Web-UI
parity gap, so the dashboard answers "when will this autonomous task next run?".

## What
- **`kernel/webui/dashboard.html`**:
  - `fmtDateTime(ms)` — local date+time (a schedule's next run can be days out, so
    the time-only `fmtTime` isn't enough).
  - The `schedules` renderer appends `next <fmtDateTime(next_run_unix·1000)>` for
    each enabled entry (paused entries keep the `(paused)` marker instead).
    `next_run_unix` is unix seconds → ×1000 for `Date`.

## Files
- `kernel/webui/dashboard.html` — `fmtDateTime`, schedules-renderer next-fire line
  (edited).
- `kernel/webui/webui_test.go` — the dashboard-wiring test now guards
  `next_run_unix` + `function fmtDateTime` (no new top-level test func).

## Verification
- `go test ./kernel/webui/` — green; full suite (1907) green, 68 packages,
  `go test ./...` exit 0; `gofmt -l` clean; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon with `AGEZT_SCHEDULE='1h=daily health
  check'` → `agt schedule list` showed `next 2026-06-04 12:24`; `/api/schedules`
  returned `next_run_unix: 1780565051`.
- **Live-proven in a real browser** (Playwright): the Schedules panel rendered
  `every 1h0m0s · daily health check · next 6/4/2026, 12:24:11 PM` — matching the
  CLI.

## Scope notes
- Pure front-end over an existing backend field; no backend change, no new
  dependency. The field was already in the `schedule_list` response (and the CLI
  renderer) — only the Web UI panel wasn't reading it.

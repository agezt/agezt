# M276 — Web UI: a live Schedules panel

## Why
After the Runs panel (M275), the most valuable missing live view was the
autonomy surface: what the agent is scheduled to do on its own (the M54–M60
autonomy-observability arc). A Schedules panel makes the daemon's autonomy
visible — and, paired with the Runs panel + event-driven refresh, lets an
operator watch a schedule fire and produce a run entirely on its own.

## What
- **`kernel/webui/webui.go`** — added `"/api/schedules":
  controlplane.CmdScheduleList` to the read-only `apiRoutes`.
- **`kernel/webui/dashboard.html`**:
  - A **Schedules** panel section, placed after Runs.
  - A `schedules` renderer: per-schedule cadence chip (e.g. `every 5s`), the
    intent, a `(paused)` marker when disabled, and a colour-coded `statusChip` for
    the last firing's outcome (from `last_status`, M56).
  - `schedule.*` events added to the `liveRefresh` map (→ Schedules panel), and
    `schedules` added to the initial-load + periodic-refresh sets.

## Files
- `kernel/webui/webui.go` — `/api/schedules` route (edited).
- `kernel/webui/dashboard.html` — Schedules panel, renderer, live refresh (edited).
- `kernel/webui/webui_test.go` — `TestSchedulesRouteProxiesScheduleList` (new);
  the read-only allowlist gained `schedule_list`; the dashboard-wiring test now
  guards the Schedules panel.

## Verification
- `go test ./kernel/webui/` — green; full suite **1881 → 1882** (+1), 68
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on the Go files; `go vet ./kernel/webui/`
  clean; `GOOS=linux build` clean; `go.mod` / `go.sum` unchanged.
- **Live-proven in a real browser** against a daemon started with
  `AGEZT_SCHEDULE='5s=heartbeat health check'`:
  - the Schedules panel rendered the schedule with an `every 5s` chip and a green
    `completed` last-outcome chip;
  - over ~13s the schedule **fired on its own** and the Runs panel filled with 5
    autonomous "heartbeat health check" runs — no human trigger — while the Event
    Feed showed the repeating `schedule.fired → task.* → …` cycle; both panels
    updated live via the event-driven refresh.

## Scope notes
- Reuses the existing `CmdScheduleList` (with the M56 last-outcome annotation), so
  the Web UI and `agt schedule list` stay consistent; no new control-plane
  command, no new dependency.
- Demo artifacts (`webui-*.png`) remain gitignored.

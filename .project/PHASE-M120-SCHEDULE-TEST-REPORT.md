# Phase Report — Milestone M120 (`agt schedule test`)

> Status: **shipped** · Date: 2026-06-02 · autonomy / cadence.

## Why

Schedules support interval, daily-at-time, and windowed cadences with weekday
filters and IANA timezones — easy to misconfigure ("did I mean Mon-Fri or
weekends? does 09:00 mean the daemon's zone or New York?"). `agt schedule list`
shows only the single immediate next run, so an operator couldn't *forecast*
whether a complex cadence does what they intend before relying on it. Policy has
`agt edict test`; schedules now have the equivalent dry-run.

## What shipped

- **`agt schedule test <id> [--count N] [--json]`** — previews the next N (default
  5, max 100) fire times of a schedule, read-only. Renders each as a local
  date-time + weekday, and notes when the schedule is paused.
- **`cadence.Entry.Forecast(from, n)`** — the pure simulation: the first fire
  matches the engine's current `NextRunUnix` (so the forecast lines up with what
  the daemon will actually do), the rest are produced by repeatedly advancing.
  Handles `once` (single future fire or none) and is deterministic given `from`.
- **`CmdScheduleTest`** handler routes to `Forecast(time.Now(), count)`.

## Design notes

- **Reuses the real cadence math** (`advance`), so weekday filters, windows, and
  timezones are honoured exactly as at runtime — a Mon-Fri schedule's forecast
  skips the weekend, a window schedule stays within its hours.
- **No state change** — pure read; safe to run anytime.

## Tests

- `TestForecast_Interval` — hourly schedule yields evenly-spaced fires.
- `TestForecast_DailyAllDays` — all fires at 09:00, ~1 day apart, after `from`.
- `TestForecast_OnceAndZero` — future once → 1; past once → 0; n=0 → nil.
- `TestScheduleTest_PreviewsFires` (control plane) — an hourly schedule previews
  4 fires 3600s apart; unknown id → found:false.

Test count: **1396 → 1400**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt schedule add "morning brief" --at 09:00 --days mon-fri
$ agt schedule test <id> --count 5
<id> — Mon-Fri at 09:00
  2026-06-02 09:00 (tue)
  2026-06-03 09:00 (wed)
  2026-06-04 09:00 (thu)
  2026-06-05 09:00 (fri)
  2026-06-08 09:00 (mon)      # weekend correctly skipped
```

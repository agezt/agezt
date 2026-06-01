# Phase Report — Milestone M73 (`agt tool log --slow <dur>` latency filter)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

M71 gave every tool-log row a latency, and M72 put it on the arc — but there was
no way to ASK "which tool calls are slow?". `--errors` finds failures; nothing
found slowness. An operator chasing a latency regression had to eyeball the whole
log. M73 adds `--slow <dur>`, the performance-hunting counterpart to `--errors`,
completing the tool-log filter family: `--errors` (failures), `--slow`
(slowness), `--tool` (one tool), `--since` (time window).

## What shipped

- **Server `slow_ms` floor** — `handleToolLog` computes each call's
  invoked→result latency (M71) and drops rows below the floor. A call with no
  measurable latency (a policy-denied call has no `tool.invoked`) is treated as
  below any positive floor, so `--slow` shows only calls with real, measured
  wall-clock at/above the threshold.
- **CLI `--slow <dur>`** — `agt tool log --slow 500ms` / `--slow=2s`, documented
  in `--help`, with an honest empty-result message ("no tool calls at/above that
  latency").

## Design decisions

- **Floor, not sort.** A threshold filter composes with the existing newest-first
  order and limit (`tool log 5 --slow 1s` = the 5 most-recent calls ≥1s), rather
  than introducing a separate "slowest-N" mode that would fight the time ordering.
- **Server-side.** The latency is computed where the journal is walked (M71), so
  the floor is applied before the limit — `--slow 1s` returns N slow calls, not
  "N recent calls, some slow".

## Tests

- `TestToolLog_SlowFilter` — a fast call (back-to-back invoked/result) and a slow
  call (~20ms apart); no floor returns both, a 10ms floor returns only the slow one.

Test count: **1315 → 1316**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt tool log --slow 1ms
  2026-06-01 13:54:10  ok    22ms  shell  Volume in drive D … Directory of D…
$ agt tool log --slow 10s
  no tool calls at/above that latency.
```

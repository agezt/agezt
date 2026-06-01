# Phase Report — Milestone M75 (per-tool latency in `agt tool stats`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

M71 gave `agt tool stats` a GLOBAL latency distribution — "calls are slow" — but
the by-tool breakdown still showed only calls/errors. The operator's real
question is "WHICH tool is slow?". A global p95 doesn't say whether it's the
shell tool or the http tool dragging the average.

## What shipped

- **Server per-tool mean latency** — `handleToolStats` accumulates each tool's
  invoked→result spans (the same M71 join) into a per-tool sum + count and emits
  `avg_ms` on each `by_tool` entry (only when the tool has a measurable span).
- **CLI** — the by-tool line gains `, avg <dur>`:
  `shell  3 call(s), 0 error(s), avg 14ms`. Tools with no joinable span stay
  clean (no `avg` suffix).

## Design decisions

- **Mean per tool, distribution global.** The global block keeps the
  avg/min/p50/p95/max shape (matching `runs stats`); the per-tool line adds just
  the mean, which is what you need to rank tools by slowness without cluttering
  each row with five percentiles.
- **Only when measured.** A policy-denied call (no `tool.invoked`) contributes no
  span, so a tool with only denied calls shows no `avg` rather than a fake 0.

## Tests

- `TestToolStats_PerToolLatency` — a shell call ~15ms apart yields a `by_tool`
  `shell` entry carrying a non-negative `avg_ms`.

Test count: **1317 → 1318**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt tool stats
  by tool:
    shell            1 call(s), 0 error(s), avg 14ms
  latency (over 1 call(s)):
    avg : 14ms  ...
```

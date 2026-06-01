# Phase Report — Milestone M71 (tool-call latency in log & stats)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt tool log` / `tool stats` (M66/M67) answered "what ran and what broke?" but
not "how long did it take?". Tool latency is a first-order operator signal — a
slow tool is the usual suspect behind a sluggish run. The data was already in the
journal: the agent publishes `tool.invoked` immediately before `tool.Invoke` and
`tool.result` immediately after, both timestamped and sharing a `call_id`. So the
per-call wall-clock is just `result.TSUnixMS − invoked.TSUnixMS`.

## What shipped (pure fold — no agent or event-schema changes)

- **`tool log` per-row latency** — `handleToolLog` stashes each `tool.invoked`
  timestamp by `call_id` and joins it to the matching `tool.result`, emitting
  `duration_ms` per row. The CLI renders a latency column (`ok 15ms shell …`); a
  policy-denied call (no `tool.invoked`) shows `—`.
- **`tool stats` latency distribution** — `handleToolStats` collects the same
  per-call spans and runs them through the shared nearest-rank `durationStats`,
  emitting a `duration_ms: {count, avg, min, max, p50, p95}` block. The CLI
  renders it as a `latency (over N call(s))` block, identical in shape to
  `runs stats`' duration block.

## Design decisions

- **Compute from the journal, don't touch the hot path.** The agent loop and the
  event schema are unchanged; latency is derived entirely in the read-side fold
  from existing timestamps. Zero risk to the run loop, and it works retroactively
  on already-journaled runs.
- **Reuse `durationStats`.** The same percentile helper behind `runs stats` and
  the M60 spend distribution, so every latency/duration block across the CLI
  reads alike.
- **Honest gaps.** A denied call has no invoked event, so it contributes no
  latency (0 / `—`) rather than a fabricated span.

## Tests

- `TestToolLog_ReportsLatency` — a row carries a non-negative `duration_ms`
  (invoked → 3ms sleep → result), and `tool stats` carries a `duration_ms` block
  with `count == 1`.

Test count: **1313 → 1314**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt tool log
  2026-06-01 13:25:55  ok       15ms  shell  Volume in drive D … Directory of D…
$ agt tool stats
  ...
  latency (over 1 call(s)):
    avg : 15ms   min : 15ms   p50 : 15ms   p95 : 15ms   max : 15ms
```

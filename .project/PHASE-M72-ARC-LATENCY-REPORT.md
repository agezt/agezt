# Phase Report — Milestone M72 (per-tool latency inline in the task arc)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

M71 added tool-call latency to `agt tool log` / `tool stats`. But the task arc —
the primary surface for debugging a single run — still showed `tool.result : ok
<output>` with no timing. An operator reading the arc to find which step was slow
had to leave it for `agt tool log`. The arc already receives each event's
`ts_unix_ms` (it walks the journal-tail chain), so the per-call wall-clock is the
same invoked→result span, joinable by `call_id` right in the render.

## What shipped (client-side, in `renderTaskArc`)

- **Per-tool latency** — `tool.invoked` stashes its `ts_unix_ms` by `call_id`;
  the matching `tool.result` renders `(NNms)` from the delta:
  `tool.result : ok (18ms)  <output>`. A result with no preceding invoked (a
  policy-denied call) simply omits the timing.

## Design decisions

- **Same span, same surface-agnostic source.** Identical join to M71's
  `handleToolLog` (invoked→result by `call_id`), but computed from the arc's own
  event stream — so the arc and `agt tool log` report the same latency for the
  same call, by construction.
- **Reuse `fmtDuration`.** The arc header and `runs stats` already format
  durations with it; the inline tool timing reads the same.

## Tests

- `TestRenderTaskArc_ToolResultShowsLatency` — invoked at ts=1000, result at
  ts=1250 → the arc renders `tool.result : ok (250ms)`.

Test count: **1314 → 1315**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt runs last
  round 1 (seq=1)
    tool.invoked: shell  {"command":"dir"}
    tool.result : ok (18ms)  Volume in drive D … Directory of D…
```

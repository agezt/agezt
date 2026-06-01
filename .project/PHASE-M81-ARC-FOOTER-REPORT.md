# Phase Report — Milestone M81 (task-arc summary footer)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

The task arc (`agt runs show`) grew rich over M68–M72 — per-round budget, tool
input/output, latency, policy verdicts. For a long run that's a lot of lines, and
the arc had no at-a-glance summary: to answer "how many tool calls? how many
failed? what did it cost?" an operator scrolled back and counted. M81 adds a
one-line footer that collapses the arc.

## What shipped (client-side, in `renderTaskArc`)

- **Summary footer** — `summary: N round(s), M tool call(s) [(K error(s))],
  $<spend>, <duration>`. Rounds come from the existing per-`llm.request` counter;
  tool calls/errors are tallied as the arc renders each `tool.result`; spend and
  duration come from the same folded summary the header uses (so the footer never
  disagrees with the header).

## Design decisions

- **Tally while rendering, don't re-walk.** The counts come from the single pass
  the arc already makes, so the footer costs nothing extra.
- **Reconciles with the header.** Header spend/duration and the footer's are the
  same folded values; the footer just re-states them next to the round/tool
  counts so the whole run reads in one line.
- **Errors only when present.** The `(K error(s))` clause is omitted at zero, so a
  clean run's footer stays uncluttered.

## Tests

- `TestRenderTaskArc_SummaryFooter` — a 2-round run with 2 tool calls (1 error),
  $0.0084, 1.5s renders
  `summary    : 2 round(s), 2 tool call(s) (1 error(s)), $0.0084, 1.5s`.

Test count: **1323 → 1324**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt runs last           # AGEZT_DEMO_DELEGATE=3
  status     : completed (1 iters, 1ms)
  spend      : $0.0021
  round 1 (seq=37)
  summary    : 1 round(s), 0 tool call(s), $0.0021, 1ms
```

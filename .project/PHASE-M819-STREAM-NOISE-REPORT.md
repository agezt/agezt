# Phase M819 — quiet the llm.token / llm.reasoning stream flood

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "su nedir yahu
sürekli bundan görüyorum [evt seq=0 kind=llm.reasoning …]" — the CLI spammed a
line per streamed token / reasoning chunk.

## Why

`llm.token` (each output token) and `llm.reasoning` (each chain-of-thought delta
from a reasoning model like deepseek-v4-pro) are EPHEMERAL, high-rate bus events
(seq=0, never journaled). The CLI streamers dumped them raw:

- `agt run` special-cased `llm.token` (inline) but let `llm.reasoning` fall
  through to the `[evt seq=0 kind=llm.reasoning]` summary line — hundreds per run.
- `agt plan` / `agt plan run` dumped EVERY event including both — and with
  concurrent plan nodes they interleaved into an unreadable wall.

## What changed (`cmd/agt/main.go` only)

- **`agt run`:** answer tokens still stream inline; reasoning is now HIDDEN by
  default (no per-chunk line). `AGEZT_SHOW_REASONING=1` streams it inline,
  demarcated with `💭`, on its own line group. A small streamMode helper emits a
  clean newline when switching between thinking, answer, and per-event lines.
- **`agt plan` (`runPlanJSON`):** skips `llm.token` + `llm.reasoning` in the
  event dump — the structural events (node transitions, tool calls) and the
  per-node outputs at the end carry the signal.

`-q`/`--quiet` is unchanged (answer-only).

## Verification

Live run (real deepseek-v4-pro, isolated home), `agt run "What is 17*23?"`:
output is now `task.received → llm.request → 391 (inline) → llm.response →
task.completed`, **zero** `kind=llm.reasoning`/`kind=llm.token` lines (grep
count 0). The answer `391` streams inline as before.

## Gate

`GOMAXPROCS=3 go test -p 2 ./cmd/... ./kernel/...` green; vet + staticcheck
clean; linux cross-build OK; gofmt clean. No schema / behaviour changes beyond
CLI rendering; go.mod unchanged.

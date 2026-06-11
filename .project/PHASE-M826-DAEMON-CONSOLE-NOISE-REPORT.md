# Phase M826 — quiet the daemon console event flood

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "agezt çalışınca
console bunlarla dolmasın artık [evt seq=0 kind=llm.reasoning subject=…]".

## Why

M819 silenced the per-token / per-reasoning-chunk flood in the `agt run`/`agt
plan` CLI renderers, but the DAEMON itself also echoes events: `runDaemon`
subscribes to `">"` (all bus events) and prints each to stdout so the operator
sees activity. With autonomous agents running, the ephemeral high-rate
`llm.token` / `llm.reasoning` events (seq=0, one per chunk, across several
concurrent runs) buried the console.

## Change (`cmd/agezt/main.go`)

The daemon's `">"`-stream printer now skips `KindLLMToken` + `KindLLMReasoning`
(same two kinds M819 filtered). All lifecycle events (task.received, llm.request,
routing.decision, budget.consumed, llm.response, task.completed, tool.*, …) still
print.

## Verification

Isolated daemon + real deepseek run "Say hi": daemon console shows the 6
lifecycle lines (task.received → … → task.completed) and **zero**
`kind=llm.reasoning` / `kind=llm.token` lines (grep count 0).

## Gate

`go test ./cmd/agezt/` green; vet + staticcheck + linux clean. No new env/schema;
go.mod unchanged.

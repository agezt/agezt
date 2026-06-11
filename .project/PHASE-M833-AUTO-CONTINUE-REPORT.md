# Phase M833 — autonomous continue past the iteration cap

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "chat max iteration
ile tıkanıp error veriyor … iterasyon biterse continue deriz, iterasyon sayısını
artırırız … otonom şekilde bir süre bekleyip devam, continue promptu ile ikinci
request'i atabilsin işi bitene kadar."

## Gap

M824 raised the round cap to 50 and added a MANUAL chat **Continue** button. But a
run that exhausted `MaxIter` still **stopped** (`ErrMaxIter` → `task.failed`,
reason=max_iters) and waited for a human to click Continue. A long, unattended
task couldn't finish on its own.

## What shipped

The agent loop now runs in **segments** and continues itself (`kernel/agent`):

- `LoopConfig.MaxAutoContinue` (0 → `DefaultMaxAutoContinue`=5; negative →
  disabled) and `LoopConfig.AutoContinueWait` (0 → `DefaultAutoContinueWait`=2s).
- When a segment of `MaxIter` rounds elapses **without** a final answer, instead
  of returning `ErrMaxIter` the loop: journals **`task.continued`**
  `{attempt, of, iters_so_far}`, waits `AutoContinueWait` (ctx-aware — a halt
  during the wait ends the run at once), injects a `[auto-continue]` user turn
  ("keep going from where you left off; stop and answer once truly done"), and
  grants another `MaxIter` rounds. After `MaxAutoContinue` continuations it gives
  up with the usual `ErrMaxIter`. `iter` stays monotonic across segments so
  journal round-numbers keep climbing.
- The per-run **cost cap**, the **identical-call guard**, and **context
  cancellation/timeout** are unchanged and remain the real safety nets across
  continuations — auto-continue only relaxes the round cap, nothing else.

This applies to **every** run — main chat loop and sub-agents both pass the new
fields through (`kernel/runtime/runtime.go`, `subagent.go`).

## Config

- `AGEZT_MAX_AUTO_CONTINUE` (int; negative disables, high = long unattended jobs)
  and `AGEZT_AUTO_CONTINUE_WAIT` (duration) parsed in `cmd/agezt`; a new
  `auto-continue` banner line shows the effective budget. Both added to
  controlplane `configEnvVars` (guard test).

## Verification

- **Unit** (`kernel/agent`): a run needing 3 model calls with `MaxIter=2`
  auto-continues and completes (answer "done", `task.continued` emitted); an
  always-looping run with `MaxAutoContinue=2` stops after exactly 2+2×2=6 calls
  with `ErrMaxIter`. Existing max-iter / loop-guard tests pinned to
  `MaxAutoContinue:-1` so they still assert the bare cap behaviour.
- **Live** (isolated home, real deepseek, `AGEZT_MAX_ITER=2`): a 5-file
  create-then-list task finished in **7 iterations** (would have failed at 2
  before), with **3** `task.continued` events journaled
  (`{attempt:1,of:5,iters_so_far:2}` … `{attempt:3,…,iters_so_far:6}`). Banner
  showed `auto-continue : 5×2 rounds`.

## Gate

agent + runtime + controlplane + event + cmd/agezt tests green; vet + staticcheck
+ linux cross-build clean; gofmt swept. go.mod unchanged. Default-allow posture:
auto-continue is ON by default (owner asked for "until done"), bounded by a
configurable cap and the existing budget/cancel safety nets.

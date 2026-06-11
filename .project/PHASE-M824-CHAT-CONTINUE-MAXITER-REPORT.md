# Phase M824 — chat Continue past max-iteration + configurable iteration cap

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "chat max iteration
ile tıkanıp error veriyor, retry dediğimde de baştan alıyor her şeyi… iterasyon
biterse continue deriz, iterasyon sayısını artırırız."

## Why

A run that exhausted the tool-round cap returned `ErrMaxIter`, surfacing in chat
as a dead error. The only recovery, "Retry", **restarts the whole task** —
throwing away everything the agent already did. And the cap wasn't configurable.

## What shipped

**Bigger + configurable cap (backend).**
- `agent.DefaultMaxIter` raised 25 → 50 so deeper agentic tasks finish in one run.
- New `AGEZT_MAX_ITER` (positive int; malformed/≤0 is a hard startup error) sets
  `cfg.MaxIter`. Banner prints `max iterations : N per run`. Added to
  `configEnvVars` (guard) + the Config Center "Budget & Limits" section.

**Chat "Continue" (frontend).**
- New `continueRun()` in chatStore: keeps the partial/errored assistant turn as
  history (`buildHistory` folds its text in) and appends a fresh turn that asks
  the model to "Continue from where you stopped and finish… don't repeat work
  already completed." Unlike `retry()` (drop the failed turn, re-run the original
  ask), this PRESERVES the work done.
- An errored turn now shows **Continue** (accent) next to **Retry**; threaded
  through MessageRow → AssistantBubble.

## Tests

- Go: cmd/agezt + agent + controlplane(configEnvVars guard) + settings + runtime
  green; banner/override/validation verified live (default 50, AGEZT_MAX_ITER=120
  → "120 per run", `abc` → hard error).
- Frontend: full vitest 506 green; tsc clean. `continueRun` is thin glue over the
  already-tested `buildHistory` + `streamRun`.

## Gate

Targeted Go suites green; vet + staticcheck + linux clean; vitest 506; dist
rebuilt (LF). One new AGEZT_* var (`AGEZT_MAX_ITER`) in configEnvVars + schema.

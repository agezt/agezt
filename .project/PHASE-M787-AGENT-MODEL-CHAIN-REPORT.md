# Phase M787 — per-agent model fallback chain

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 5
(M783 roster → M784 delegate → M785 console → M786 memory → this).

## What

Profile `Fallbacks` (stored since M783) become live routing: run-as-agent and
delegate-by-slug carry `[primary, fallbacks…]` as the request's model chain;
the Governor walks it on retryable failure (M703 semantics, identity-keyed).

## Design

- `agent.CompletionRequest.ModelChain` + `LoopConfig.ModelChain` (additive,
  Governor-only — providers ignore it, like TaskType); loop copies cfg→req.
- `governor.completeChained`: per-request chain WINS over the task-type
  chain; fallback events scoped `agent-chain` (vs `model-chain`) so the
  Routing view / `agt why` can tell them apart.
- `runtime.WithModelChain` ctx + RunWith → LoopConfig (run-as path);
  `runSubAgent` builds the child chain directly (delegate path).
- `controlplane.agentModelChain(primary, fallbacks)`: explicit `--model`
  heads the chain; primary-duplicates skipped.

## Tests (5 new)

- governor (real registry/bus): per-request chain falls back model-a→model-b
  with an `agent-chain`-scoped `governor.fallback` journaled; per-request
  chain wins over a configured task chain.
- controlplane e2e (wire): agent run → `[agent-model, backup-1, backup-2]`;
  explicit model → `[explicit-model, backup-1, backup-2]`; plain run → none.
  Asserted on the actual provider requests.
- runtime delegate: child requests carry the chain, primary-dupe skipped.

## Gate

Full suite `-p 2 ./...` green; vet + staticcheck clean; linux cross-build OK;
go.mod unchanged; no frontend change. No daemon smoke this time: a demo-echo
daemon cannot exercise model fallback; the chain is proven end-to-end by the
wire-level control-plane test and the Governor walk by the registry-level
test (the same layering M703/M706 used). CI org-billing still blocked →
local battery + arc-authority merge.

## Next in the arc

A2A ask/reply on the board · chat: converse AS a named agent · workdir
wiring · per-agent budget ledger (daily, beyond per-run).

# M528 — Mutation testing agent: pin the per-run cost cap inclusive boundary

## Context
Thirty-fifth package in the mutation pass: `kernel/agent` (the core tool-call loop —
max-iter, loop guard, per-run cost cap, context compaction). Large (1107 LOC), 9 test
files. Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting score 0.515, 134 survivors;
tree restored clean.

## Triage — the runaway-prevention core is well covered
The loop guard `callCounts[k] > MaxIdenticalToolCalls` is pinned at its exact edge
(`TestRun_LoopGuard_CapsIdenticalCalls`: cap 3 → 3 executions, the 4th trips), distinct
inputs and the negative-disable are covered. `MaxIter`, provider-error/cancel → task.failed,
tool timeout, streaming fallback, and context compaction all have dedicated tests.

## The genuine gap (closed)
The per-run cost cap (M166) terminates when cumulative spend reaches the cap:

```
if spentMicrocents >= cfg.MaxRunCostMicrocents { return … ErrRunBudgetExceeded … }
```

`runcost_test.go` exercises this only **strictly over** the cap (`spend 2000` vs `cap
1500`) and well under — never *exactly at* it. So `>= → >` survived: a run spending
exactly its budget would NOT terminate and would run one more (over-budget) round before
the next check, defeating the cap's purpose at the boundary.

## Fix
Added `TestRun_PerRunCostCap_ExactlyAtCap`: a run whose single call spends exactly the cap
(1500 tokens, `sumCost` 1:1, cap 1500) must stop with `ErrRunBudgetExceeded`.

## Negative control (manual, CPU-capped)
`spentMicrocents >= cap → >`: FAIL (the exactly-at-cap run completes instead of being
capped). Restored byte-for-byte (`git diff --ignore-all-space` on agent.go empty); passes
again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-five packages (M490–M528)
…openaiapi, agent — plus the controlplane primary-token auth gate verified solid. The
agent loop's runaway guards (max-iter, identical-call) were already edge-pinned; the gap
was the cost cap's inclusive boundary.

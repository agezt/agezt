# M501 — Mutation testing runtime: pin foldRunTools correlation isolation

## Context
Twelfth package in the mutation pass: `kernel/runtime` (the orchestration core). Run
with `GOMAXPROCS=3` (CPU-capped per operator feedback). Score 0.549 over 494 mutants
(large package). The clearest security-relevant survivor, `WithTrustCeiling`'s
`ceiling >= edict.LevelAllow`, was triaged and found **equivalent** (clamping to the
max trust level is a no-op; the meaningful `>=`→`<=` direction, which would bypass an
L0 ceiling, is already killed by `TestPolicyHook_TrustCeiling`). The genuine gap was in
the memory-distillation fold.

## The genuine gap (closed)
`foldRunTools(corr)` builds a run's memory-distillation transcript by folding the
journal: `if e.CorrelationID != corr || e.Kind != event.KindToolResult { return nil }`
keeps only THIS run's tool.result events, counts them, and collects tool names. Nothing
tested that isolation, so three mutants on that line survived — notably `||`→`&&`,
which keeps any event matching the correlation OR being a tool result, folding **other
runs' tool results** (and this run's non-tool events) into the transcript. The distilled
memory of a run would then be contaminated with another run's activity.

## Fix
`kernel/runtime/foldruntools_internal_test.go` (internal `package runtime`, reusing the
`openCausesKernel` helper): publishes two tool.results under run-A, one under run-B, and
a non-tool event under run-A, then asserts `foldRunTools("run-A")` returns `count == 2`
and `names == [shell file]` — excluding run-B's tool and run-A's non-tool event.

## Negative control (manual, CPU-capped)
Applying the survivor (`|| → &&`) makes the test fail with
`names = [shell file http]` (run-B's `http` tool folded in); restored byte-for-byte
(`git diff --ignore-all-space` on runtime.go empty); passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twelve packages (M490–M501)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime — plus the controlplane primary-token auth gate verified solid.
Genuine gaps closed where they existed (redact, journal, edict, creds-legacy-KDF,
warden-blank-argv0, governor-spend-boundary, scheduler-correlation-id,
bus-matcher-over-delivery, cadence-due-boundary, runtime-foldRunTools-isolation); the
rest verified solid. Each was a single-token regression the existing suite missed by
construction.

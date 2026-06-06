# M498 — Mutation testing the scheduler: pin plan correlation-id generation

## Context
Ninth package in the mutation pass: `kernel/scheduler` (DAG plan execution). Run
with `GOMAXPROCS=3` (CPU-capped per operator feedback). Score **0.774** — the highest
of the packages assessed, confirming the scheduler is already well-tested (prior bug
fixes M459 gate-slot, M472 busy-wait are pinned). Most survivors are equivalent /
cosmetic-label / concurrency-timing mutants (`sem <- struct{}{}`, `wg.Wait()` — racy
to assert) and the `maxParallel <= 0` guard is effectively killed (every unset-
MaxParallel test would deadlock a size-0 semaphore).

## The genuine gap (closed)
`Run(ctx, plan, correlationID)` generates an id when the caller passes an empty one:
`correlationID = "plan-" + ulid.New()`. That id becomes `PlanResult.PlanID` and is
stamped on **every** journal event the plan emits (plan/node started/completed/failed)
and propagated through the run context. Many tests call `Run(ctx, plan, "")` but none
asserted the generated id, so the mutation run showed the generation could be removed
(`_, _ = correlationID, ulid.New`) undetected — leaving every event of an
auto-correlated plan run with an **empty correlation id**, which breaks `agt why` /
audit-trail correlation (SPEC-08).

## Fix
`kernel/scheduler/correlation_test.go` (external `package scheduler_test`):
- `TestRun_GeneratesCorrelationIDWhenEmpty`: `Run(…, "")` → `PlanID` non-empty with a
  `"plan-"` prefix.
- `TestRun_PreservesProvidedCorrelationID`: a caller-supplied id is preserved verbatim
  (also pins the `if correlationID == ""` guard so generation can't clobber a real id).

## Negative control (manual, CPU-capped)
Removing the generation (`correlationID = "plan-" + ulid.New()` →
`_, _ = correlationID, ulid.New`) makes `TestRun_GeneratesCorrelationIDWhenEmpty` fail
(`PlanID == ""`); restored byte-for-byte (`git diff --ignore-all-space` empty); passes
again.

## Verification / gate
- New tests pass; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — nine packages (M490–M498)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler — plus the
controlplane primary-token auth gate verified solid. Genuine gaps closed where they
existed (redact, journal, edict, creds-legacy-KDF, warden-blank-argv0,
governor-spend-boundary, scheduler-correlation-id); the rest verified solid. The
genuine-gap rate is now low and skewed toward observability/edge cases rather than
core security/correctness — the high-stakes paths are comprehensively pinned.

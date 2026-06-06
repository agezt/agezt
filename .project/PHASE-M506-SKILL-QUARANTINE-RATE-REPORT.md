# M506 — Mutation testing skill: pin the auto-quarantine failure-rate threshold

## Context
Seventeenth package in the mutation pass: `kernel/skill` (skill lifecycle —
draft/shadow/active/quarantined transitions, auto-promote, auto-quarantine, forge
lock + revert, where HIGH bugs were fixed in M424). Run with `GOMAXPROCS=3`
(CPU-capped). Score 0.527. The state machine and forge are well-covered
(transitions_matrix, forge_m424, autoquarantine tests); the gap was a threshold edge.

## The genuine gap (closed)
`maybeAutoQuarantine` disables a failing active skill when both the failure count and
rate cross their thresholds. The rate gate is `if rate < f.aqFailureRate { return }` —
i.e. quarantine when `rate >= aqFailureRate`. The existing tests drive a 100% rate
(quarantine) and a ~23% rate (stay active), so neither lands on the threshold exactly;
the mutation `< → <=` **survived** — under it a skill sitting at *exactly* the failure
rate would escape quarantine.

## Fix
`kernel/skill/autoquarantine_rate_boundary_test.go` (internal `package skill`):
`TestRecordOutcome_QuarantinesAtExactFailureRate` — with the defaults
(`aqMinFailures=3`, `aqFailureRate=0.5`), records 3 successes then 3 failures. The
min-failure-count guard keeps the skill active until the 3rd failure, at which point the
rate is exactly 3/6 = 0.5 (exactly representable). It asserts active at 3S/2F (40%,
below threshold) and quarantined at 3S/3F (exactly 50%).

## Negative control (manual, CPU-capped)
Applying the survivor (`rate < f.aqFailureRate → <=`) lets the skill stay active at
exactly 50%, failing the test; restored byte-for-byte (`git diff --ignore-all-space`
on forge.go empty); passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — seventeen packages (M490–M506)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill — plus the controlplane
primary-token auth gate verified solid. The recurring closeable gap remains the
inclusive threshold / unset-default that end-to-end tests skip by always passing values
clear of the boundary.

# M495 — Mutation testing the warden: pin blank-argv0 rejection

## Context
Seventh package in the mutation pass: `kernel/warden`, the command-execution
sandbox (the security boundary that runs subprocesses with setpgid/rlimit/namespace
hardening and bounds their output). `go-mutesting .` scored 0.563 over 119 mutants.

## Triage — warden is largely well-tested
- **capBuffer** (the output memory bound — DoS-relevant) is exemplary: both
  truncation branches, the tail-most-recent invariant across many writes, the
  never-exceeds-cap guarantee under a huge write, non-positive-max defaults, and the
  empty-write no-op are all pinned. Its survivors are equivalent mutants — the
  defensive `max(n-keep, 0)` where `n ≥ keep` always holds in that branch, and the
  `drop >= len(c.buf)` boundary that coincides with the default branch exactly at
  `n == max`.
- Remaining survivors are error-message, config-default (`timeout/waitDelay <= 0`),
  and platform-hardening best-effort mutants.

## The genuine gap (closed)
`Run`'s spec guard is `if len(spec.Argv) == 0 || spec.Argv[0] == ""`. The existing
`TestRun_RejectsEmptyArgv` passes `Spec{Profile: ProfileNone}` (nil Argv), exercising
only the `len(Argv) == 0` branch. The second condition — rejecting a one-element argv
whose program name is blank (`Argv: [""]`) — was unpinned, so the mutation run’s
`spec.Argv[0] == "" → false` **survived**. Without that check a blank command would
fall through to `exec.CommandContext(ctx, "", …)` — an empty binary path handed to
the OS exec layer.

## Fix
`kernel/warden/warden_test.go` — `TestRun_RejectsBlankArgv0`: `Run` with
`Spec{Argv: []string{""}}` must return `ErrBadSpec` (asserted via `errors.Is`).

## Negative control (manual)
Applying the survivor — `spec.Argv[0] == "" → false` — makes the test fail with the
exact bug it guards against: `Run` proceeds to exec and returns
`warden: start "": exec: no command` instead of `ErrBadSpec`. Restored byte-for-byte
(`git diff --ignore-all-space` on warden.go empty); test passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0; `go.mod`/`go.sum` unchanged.

## Mutation pass — seven packages (M490–M495)
redact, journal, edict, netguard, event, creds, warden. Genuine, security/integrity-
relevant gaps were found and closed where they existed (redact, journal, edict,
creds-legacy-KDF, warden-blank-argv0); the rest were verified already solid, their
survivors equivalent or error-message mutants. The `.project/HARDENING.md` mutation
criterion ("no surviving non-equivalent mutant on the highest-stakes packages") holds
across all seven.

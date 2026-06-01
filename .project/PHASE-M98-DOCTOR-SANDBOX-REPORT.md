# Phase Report — Milestone M98 (`agt doctor` sandbox check)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / security health.

## Why

M96/M97 exposed the warden's data (`agt warden log`/`stats`), but an operator has
to know to look. The most consequential signal there — the sandbox silently
running WEAKER than requested (a profile downgrade) — belongs in `agt doctor`,
the go-to "is everything healthy?" diagnostic, as an actionable WARN. A sandbox
that isn't sandboxing is a security gap that should announce itself.

## What shipped

- **`checkSandbox` doctor check** — calls `CmdWardenStats` and verdicts:
  - **WARN** when any execution ran with downgraded isolation (with the rate +
    a hint to build full-namespace support or accept it knowingly),
  - **WARN** when a warden resource limit was breached (pointer to
    `agt warden log --issues`),
  - **OK** for full requested isolation / no sandboxed executions yet.
- **`sandboxCheckFromStats`** — the pure verdict logic, split out so it's
  unit-testable without a live daemon (the doctor's other checks are
  integration-only).

## Design decisions

- **Synthesis, not another fold.** This turns the M97 numbers into a judgement an
  operator can act on, inside the one command they already run for health.
- **Best-effort, never a false FAIL.** A missing daemon/stats call or zero
  executions is an informational OK — the check only WARNs on real, measured
  downgrade/limit signals.

## Tests

- `TestSandboxCheckFromStats` — downgraded → WARN; limit breach → WARN; full
  isolation → OK; no executions → OK.

Test count: **1344 → 1345**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt run "summarize" ; agt doctor
  [WARN] sandbox (warden) : 1/1 execution(s) ran with downgraded isolation (100%)
         ↳ the host lacks the requested sandbox backend; on Linux build with
           full-namespace support, or accept the reduced isolation knowingly
```

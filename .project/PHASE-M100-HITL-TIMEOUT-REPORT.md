# Phase Report — Milestone M100 (tunable HITL timeout + doctor surfaces it)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / HITL reliability.

## Why

In `prompt` approval mode a HITL request that no operator answers auto-denies
when its timeout fires — the run silently stalls and dies. Two gaps made this a
trap:

1. **The window was hardcoded at 5 minutes.** Fine at a console, far too long for
   an unattended run that should fail fast, with no way to right-size it.
2. **Timeouts were invisible.** The count lived only in `agt approvals stats`;
   nothing told an operator "your runs are dying because nobody is answering."

This milestone closes both, as one coherent HITL-reliability story — the third
doctor-as-single-pane health check after M98 (sandbox) and M99 (provider).

## What shipped

- **`AGEZT_APPROVAL_TIMEOUT=<duration>`** — sets how long a prompt-mode approval
  blocks before auto-deny. Default stays `approval.DefaultTimeout` (5m); malformed
  is a hard startup error (fast feedback, mirrors `AGEZT_RUN_TIMEOUT`);
  non-positive = default. Plumbed `runtime.Config.ApprovalTimeout` →
  `approval.New(Config{Timeout: …})` (only when the kernel builds the default
  registry; an injected one keeps its own). Shown in the boot banner
  (`approval timeout : …`) and added to the `agt config` env inventory.
- **`checkApprovals` doctor check** — WARNs when any approval has timed out (with
  a hint to respond promptly / lengthen the window / change the mode); OK on
  no-timeouts, in-flight pending, or no approvals yet.
- **`approvalsCheckFromStats`** — the pure verdict, unit-testable without a live
  daemon (mirrors M98/M99).

## Design decisions

- **Pending is not a WARN.** An approval awaiting an answer right now is normal
  in-flight state; only an *expired* one (timeout) is a failure. The OK detail
  still surfaces the pending count for context.
- **One theme, both halves.** Making the window tunable and making its failures
  visible are the same HITL-reliability concern; shipping them together means the
  doctor's advice ("lengthen `AGEZT_APPROVAL_TIMEOUT`") points at a real knob.

## Tests

- `TestApprovalsCheckFromStats` — timeouts → WARN; pending-only → OK; all
  resolved → OK; none → OK.

Test count: **1347 → 1348**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, my lines gofmt-clean.

## Live proof

```
$ AGEZT_APPROVAL_MODE=prompt AGEZT_APPROVAL_TIMEOUT=2s agezt &
    approval timeout : 2s per HITL approval (auto-deny on overrun)
$ agt run "list the project files"   # default mock calls shell (Ask-class); nobody answers
$ agt approvals stats
    timeout   : 1
$ agt doctor
  [WARN] approvals (HITL) : 1/1 approval(s) expired with no operator response
         ↳ HITL requests are going unanswered — runs auto-deny and stall;
           respond promptly, lengthen AGEZT_APPROVAL_TIMEOUT, or change AGEZT_APPROVAL_MODE
```

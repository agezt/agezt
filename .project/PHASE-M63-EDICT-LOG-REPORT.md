# Phase Report — Milestone M63 (`agt edict log` — policy-decision audit)

> Status: **shipped** · Date: 2026-06-01 · SPEC (Edict policy) observability.

## Why

`agt edict show` lists the policy RULES, but there was no view of the DECISIONS
those rules produced. The agent loop journals a `policy.decision` event for every
tool-call gating (tool, capability, allow/deny, reason, hard_denied), but nothing
surfaced them — an operator auditing "what got denied recently?" had to grep the
raw journal. M63 adds the audit log.

## What shipped

- **`CmdEdictLog` + `handleEdictLog` (`kernel/controlplane/policy_log.go`)** —
  folds `policy.decision` events newest-first into rows (ts, actor, correlation,
  tool, capability, allow, reason, hard_denied). `args.denied` keeps only denials;
  `args.limit` bounds the count; tenant-scoped via `kernelFor`.
- **`agt edict log [N] [--denied] [--tenant <id>] [--json]`** — renders
  `<time>  allow|DENY|DENY(hard)  <capability>  <tool>  (reason)`.
- **Tenant-allowlisted** — `CmdEdictLog` joins `tenantTokenAllows` (a tenant can
  audit its own policy decisions), alongside the other read-only edict commands.

## Design decisions

- **show = rules, log = decisions.** Two distinct, complementary surfaces over the
  same engine — `show` is the configured policy, `log` is the realized history.
- **Journal fold, no new state.** Mirrors the runs/schedule observability pattern:
  a read-only walk of an event kind already journaled, newest-first + limit +
  filter. No projection, no new event.

## Tests

- `TestEdictLog_ListsAndFiltersDenied` — 1 allow + 2 denials → all 3 listed;
  `--denied` returns just the 2 denials.

Test count: **1301 → 1302**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt run "what is this project?"      # mock invokes the shell tool
$ agt edict log
  2026-06-01 12:51:50  allow      shell        shell  (level L2; AskPolicy=AskAllow …)
```

# Phase Report — Milestone M87 (`agt approvals log`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / security observability.

## Why

`agt approvals` lists PENDING human-in-the-loop requests. Once an operator
granted or denied one, it vanished — there was no audit of HITL decisions: what
was asked, how it resolved, and who decided. For a governed agent that's a
first-order security record. M87 adds `agt approvals log`, the human analogue of
`agt edict log` (which audits the AUTOMATIC policy gating) — together they cover
both halves of "was this allowed, and by whom?".

## What shipped

- **Server `handleApprovalsLog` (`approvals_log.go`)** — folds
  `approval.requested` joined with the terminal `approval.granted` /
  `approval.denied` / `approval.timeout` by `approval_id` into one row per
  request (capability, tool, reason, status, resolved_by). Newest-first, limited,
  with a `--denied` filter (denials + timeouts) and the shared `--since` window.
  A still-open request shows `pending`.
- **CLI `agt approvals log [N] [--denied] [--since <dur>] [--json]`** — routed
  from `agt approvals log`; plain `agt approvals` stays the pending-only list.

## Design decisions

- **Request anchors the row; outcome updates it.** Keyed by `approval_id` so the
  request and its resolution join even though they're separate events — the same
  invoked→result join shape `tool log` uses.
- **`--denied` includes timeouts.** A timed-out approval is a non-grant — an
  operator auditing "what was refused?" wants both denials and timeouts.

## Tests

- `TestApprovalsLog_JoinsRequestAndOutcome` — a granted (by alice) + a denied (by
  bob) + a pending: all three listed, a1 joins to granted/alice, `--denied`
  returns just a2.

Test count: **1330 → 1331**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_APPROVAL_MODE=prompt agt run "summarize"   # blocks on approval
$ agt approve <id>
$ agt approvals log
  2026-06-01 14:49:35  granted  shell  shell  (by operator)
```

# Phase Report — Milestone M88 (`agt approvals stats`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / security observability.

## Why

M87 added `agt approvals log` (per-decision audit). The aggregate was missing:
"how often do I grant vs deny, and which capabilities get refused?". M88 adds
`agt approvals stats`, completing the approval **log / stats** pair and the human
analogue of `agt edict stats`.

## What shipped

- **Server `handleApprovalsStats`** — folds the approval lifecycle into total /
  granted / denied / timeout / pending, a grant rate over RESOLVED requests, and
  a denied-by-capability breakdown (denials + timeouts). `since_ms` windows by
  request time.
- **CLI `agt approvals stats [--since <dur>] [--json]`** — routed from `agt
  approvals stats`; renders the counts, grant rate, and capability breakdown.

## Design decisions

- **Grant rate over resolved, not total.** Pending requests aren't decisions yet,
  so they're excluded from the rate (matching how `runs stats` excludes running
  from success rate).
- **Denied-by-capability includes timeouts.** A timeout is an effective refusal;
  bucketing it with denials answers "what keeps getting blocked?".

## Tests

- `TestApprovalsStats_Aggregates` — 2 granted + 1 denied + 1 pending → total 4,
  granted 2, pending 1, grant rate ≈ 0.667 (2/3 resolved), denied_by_capability
  net = 1.

Test count: **1331 → 1332**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_APPROVAL_MODE=prompt agt run "summarize"  # + agt approve <id>
$ agt approvals stats
  approvals (over 1):
    granted   : 1
    denied    : 0
    pending   : 0
    grant     : 100.0%
```

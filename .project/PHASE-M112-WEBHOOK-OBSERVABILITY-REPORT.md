# Phase Report — Milestone M112 (webhook delivery observability)

> Status: **shipped** · Date: 2026-06-02 · SPEC-08 / P7-API-02.

## Why

The outbound webhook dispatcher journals `webhook.delivered` (a 2xx) and
`webhook.failed` (exhausted retries) for every event it POSTs to an operator's
sink — but those were only reachable via `agt journal grep webhook`. A webhook
silently failing is the classic "I never got paged" outage: the operator set up
notifications, they quietly stopped working, and nothing told them. This folds
the events into a first-class surface.

## What shipped

- **`agt webhook log [N] [--failed] [--tenant <id>] [--since <dur>] [--json]`** —
  a timeline of deliveries (event kind, URL, status / error, attempts).
  `--failed` keeps only the failures (the on-call view).
- **`agt webhook stats [--tenant <id>] [--since <dur>] [--json]`** — total,
  delivered, failed, failure rate, and a per-URL breakdown ("which sink is
  down?").
- Both tenant-routed and on the tenant-token allowlist.

## Tests

- `TestWebhookLogAndStats` (control plane) — two deliveries + one failure;
  stats report 3 total / 1 failed; `log --failed` returns only the failure with
  its error; `log` returns all three.

Test count: **1374 → 1375**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_WEBHOOKS="http://127.0.0.1:8768/hook|>|" agezt &   # local receiver up
$ agt run "do a thing"
$ agt webhook stats
  delivered : 15   failed : 0   failure rate : 0.0%
$ # kill the receiver, run again:
$ agt webhook log --failed
  FAILED  task.failed  → http://127.0.0.1:8768/hook  after 3 attempt(s): connection refused
```

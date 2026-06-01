# M121 — `agt doctor` webhook-health check

## Why
M112 added outbound-webhook delivery observability (`webhook.delivered` /
`webhook.failed` events, surfaced by `agt webhook log|stats`). But a webhook sink
exists precisely so an operator gets notified out-of-band; a sink that silently
5xx's, times out, or refuses connections is the classic **"I never got paged"**
outage — and it stayed invisible unless someone *thought* to run `agt webhook
stats`. The go-to first-look diagnostic (`agt doctor`) didn't know about it.

This closes the loop the same way M98 (sandbox downgrade), M99 (provider
fallback), M100 (approval timeouts), and M110 (stale catalog) did: take an
existing silent-degradation signal and fold it into the operator's first-look
diagnostic so it surfaces proactively, not only on demand.

## What
- `checkWebhooks(ctx, client)` — calls `CmdWebhookStats`; a failing call or no
  deliveries is an informational **OK**, never a FAIL (best-effort, like its
  siblings).
- `webhookCheckFromStats(res)` — pure verdict (testable without a daemon):
  - `total == 0` → OK "no webhook deliveries yet"
  - `failed == 0` → OK "N delivery(ies), all delivered"
  - `failed > 0` → **WARN** "F/T webhook delivery(ies) failed (R%)", hint names
    the worst sink and points at `agt webhook log --failed`.
- `topFailingWebhook(by_url)` — returns the sink URL with the most failed
  deliveries (ties broken by URL for determinism; all-zero → "").
- Wired into `runDoctorChecks` after `checkCatalog`, before `checkHalt`.

No new control-plane command, no schema change — pure reuse of the M112
`webhook_stats` surface.

## Files
- `cmd/agt/doctor.go` — `checkWebhooks`, `webhookCheckFromStats`,
  `topFailingWebhook`; one wiring line.
- `cmd/agt/doctor_test.go` — `TestWebhookCheckFromStats`, `TestTopFailingWebhook`.

## Tests
- `TestWebhookCheckFromStats`: failures → WARN naming worst sink; all-delivered →
  OK; no deliveries → OK.
- `TestTopFailingWebhook`: picks max-failed url; all-zero failures → ""; nil → "".

## Live proof (offline mock provider)
Daemon booted with `AGEZT_WEBHOOKS='http://127.0.0.1:9/hook|>'` (port 9 refuses
connections → every delivery fails after 3 attempts). One `agt run "say hello"`
emitted matching events; the dispatcher journaled 4 `webhook.failed`. Then:

```
=== webhook stats ===
  delivered    : 0
  failed       : 4
  failure rate : 100.0%
  by URL:
    http://127.0.0.1:9/hook    delivered=0 failed=4

=== doctor ===
  [WARN] webhooks         : 4/4 webhook delivery(ies) failed (100%)
           ↳ "http://127.0.0.1:9/hook" is failing; check `agt webhook log --failed`
```

## Verification
- 55 packages `ok`, **FAIL 0**; **1403 tests**.
- `gofmt` clean (my files), `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod` / `go.sum` unchanged.

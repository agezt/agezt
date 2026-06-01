# M122 — `agt webhook test`

## Why
M112 gave webhook delivery *observability* (`agt webhook log|stats`) and M121
*surfaced* failures in `agt doctor`. Both are **reactive** — they tell you a sink
is failing *after* real events have already been dropped. The missing piece is an
**active probe**: an operator who just wired up a webhook sink has no way to
confirm it works short of waiting for a real event to fire (a run to complete, an
approval to land). "I configured a webhook — does it actually receive?" had no
answer. This is the same gap `agt edict test`, `agt netguard test`, and `agt
schedule test` fill for their subsystems.

## What
`agt webhook test [<url>] [--subject <pat>] [--secret <key>] [--json]` — a
daemon-free probe that POSTs **one** synthetic `webhook.test` event to a sink:
- With an explicit `<url>`, probes that sink (optional `--subject` / `--secret`).
- With no `<url>`, probes **every** sink in `AGEZT_WEBHOOKS` — the same spec the
  daemon reads, so it validates the real configuration.
- Exit `0` = all sinks returned 2xx, `3` = at least one failed, `2` = usage error.

The probe uses the **byte-identical** body, headers (`X-Agezt-Event`,
`X-Agezt-Subject`, `X-Agezt-Delivery`), and HMAC-SHA256 signature
(`X-Agezt-Signature`) a real delivery sends — so a 2xx here means real deliveries
will be accepted too, *including* signature verification on the receiver's end.
Unlike a real delivery it does **not** retry: a test wants the immediate truth,
not a transient masked by backoff.

### Design
- New exported `webhook.Probe(ctx, sink, now, client) ProbeResult` + `ProbeResult`
  (`OK()`, `Status`, `Latency`, `Signed`, `Err`) and `TestEventKind`
  (`"webhook.test"`).
- To keep the probe honest, the dispatcher's request-building was refactored into
  a shared `newDeliveryRequest` used by *both* the live `post` path and `Probe`,
  so headers/signing can never silently diverge.
- `agt webhook test` is an **operator** command POSTing to an **operator-chosen**
  URL (like `curl`), so it is intentionally not subject to the agent egress guard
  (netguard) — that guards *agent tool* egress, not operator intent.

## Files
- `kernel/webhook/webhook.go` — `newDeliveryRequest` (extracted), `Probe`,
  `ProbeResult`, `TestEventKind`, `testEventID`.
- `kernel/webhook/webhook_test.go` — `TestProbe_Delivers` (2xx, headers,
  signature, single POST), `TestProbe_Non2xxNoRetry`, `TestProbe_ConnError`.
- `cmd/agt/webhook.go` — `cmdWebhookTest`; `test` wired into the `webhook`
  dispatcher; help/usage updated.
- `cmd/agt/webhook_test.go` (new) — `TestCmdWebhookTest_OKAndFail`,
  `_UsageErrors`, `_FromEnv`.

## Live proof (offline)
A tiny local sink echoing the request it received:

```
=== signed probe (good sink) ===
  [OK]   http://127.0.0.1:8791/hook  200  in 2ms  (signed)
1 ok, 0 failed.                                            (exit 0)

=== probe a dead port (failure path) ===
  [FAIL] http://127.0.0.1:8799/nope  — dial tcp 127.0.0.1:8799: ...refused
0 ok, 1 failed.                                            (exit 3)

=== what the sink received ===
SINK got POST  Event=webhook.test Delivery=00000000000000000000000000
  Sig=sha256=adc5c32a711e5eb8...
  body={"id":"00000…","kind":"webhook.test","payload":{"test":true,...}}
```

The receiver got the exact delivery format with a valid HMAC signature.

## Verification
- 55 packages `ok`, **FAIL 0**; **1409 tests**.
- `gofmt` clean (my files), `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod` / `go.sum` unchanged.

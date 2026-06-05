# M416 — Outbound-webhook egress guard (SPEC-06 consistency)

## Context
A security review of the egress surfaces found that `kernel/netguard` is rigorously
correct and the agent-reachable `http`/`browser` tools are guarded — but the
outbound **webhook** dispatcher (`NewDispatcher`) and `Probe` built plain
`&http.Client{}` with no egress guard. A configured sink could therefore reach
loopback / RFC1918 / `169.254.169.254`, and since a webhook body carries journal
event data, that is a journal-exfiltration / internal-POST primitive.

Severity is LOW (sinks are operator-configured via `AGEZT_WEBHOOKS`, not agent- or
attacker-controllable — a defense-in-depth gap, not an agent-reachable SSRF), but it
is an inconsistency with the documented SPEC-06 secure-by-default egress model. The
user chose to close it with the established opt-in pattern.

## What
- **`kernel/webhook/webhook.go`** — new `Option`/`WithClient(c)` so the caller can
  inject the delivery client; `NewDispatcher` is now variadic (`opts ...Option`,
  backward compatible). The package stays stdlib-only — the netguard dependency lives
  in the caller, not here.
- **`cmd/agezt/main.go`** (`buildWebhooks`) — builds a `netguard`-guarded client
  (default-deny internal) and passes it via `WithClient`. Opt-in env vars
  `AGEZT_WEBHOOK_ALLOW_LOOPBACK` / `AGEZT_WEBHOOK_ALLOW_PRIVATE` relax it per range
  (private logs a warning), mirroring `AGEZT_HTTP_ALLOW_*`. The banner gains an
  `[egress=guarded|loopback-ok|private-ok|loopback+private-ok]` suffix.
- **`cmd/agt/webhook.go`** (`agt webhook test`) — builds the same guarded client from
  the same env opt-ins and passes it to `Probe`, so a test reflects what a real
  delivery will do.
- **`kernel/controlplane/config.go`** — registered both env vars in `configEnvVars`
  (alphabetical), satisfying the config-inventory test.

## Verification
- **`kernel/webhook/webhook_test.go`**:
  - `TestDispatch_EgressGuardBlocksInternalSink`: a guarded (default-deny) dispatcher
    with a sink at the loopback httptest server → the delivery is refused at dial
    time, `webhook.failed` is journaled, the server is never hit.
  - `TestDispatch_EgressGuardAllowsWhenOptedIn`: `netguard.AllowLoopback()` → the same
    sink delivers (count==1).
  - **Negative control:** making `WithClient` ignore its argument (default unguarded
    client used) → the block test's delivery succeeds, `webhook.failed` is never
    journaled, the `waitFor` times out → FAIL. Restored byte-identical.
- **Existing `agt webhook test` tests** (`TestCmdWebhookTest_OKAndFail`,
  `_FromEnv`) updated to set `AGEZT_WEBHOOK_ALLOW_LOOPBACK=1` (they probe loopback
  servers) — which also exercises the opt-in path end-to-end.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged, config-inventory test passes. Full suite **2265**
  passing (was 2263; +2). CHANGELOG Security entry added.

## Behaviour change (documented)
Operators who currently deliver webhooks to an internal sink (localhost dev sink,
internal dashboard/alerting) must now set `AGEZT_WEBHOOK_ALLOW_LOOPBACK=1` or
`AGEZT_WEBHOOK_ALLOW_PRIVATE=1`. This was the user's chosen trade-off (secure-by-
default + opt-in) over leaving the gap or guarding only the metadata IP.

## Review status
This closes the one finding from the egress-surface review. `kernel/netguard`,
inbound channel authorization (fail-closed allowlist + constant-time HMAC/Ed25519
signature checks), and webhook HMAC signing were all found clean.

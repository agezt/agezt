# M334 — Generic webhook channel (SPEC-04)

## Why
The first session under the v1.0-conformance goal. SPEC-04 §1 lists a vendor-
neutral **webhook channel** alongside the platform channels, but only Telegram,
Slack, and Discord were built. The generic webhook channel is the offline-
verifiable, genuinely-missing first-party piece (priority B): it lets *any*
external system (a custom app, an automation tool, an IFTTT/n8n/Zapier hook, a
homegrown bot) drive an Agezt agent over plain signed HTTP — no platform SDK.

## What
- **`plugins/channels/webhook/webhook.go`** (new package): a duplex
  `channel.Channel`.
  - **Inbound**: `POST {path}` with `X-Agezt-Signature: sha256=<hex HMAC-SHA256(
    secret, raw-body)>` (the same scheme as Agezt's *outbound* webhook dispatcher,
    so the two compose). Body `{channel_id, sender, text, id?, ts_ms?, images?}`.
    Returns the agent's reply synchronously: `200 {reply, correlation_id}`.
  - **Security** (mirrors Slack/Discord): empty secret fails closed (no unsigned
    inbound); constant-time `hmac.Equal`; a `ts_ms` freshness window + `id`
    de-duplication for replay protection; a fail-closed channel-id Allowlist;
    bounded request bodies (`io.LimitReader`). Inbound text is data; the agent's
    tool calls still pass through Edict. Journals `channel.inbound/outbound`.
  - **Outbound** (`Send`): POSTs a signed message to a configured `OutboundURL`
    (async/proactive — Pulse briefs, `agt send`); errors when none is set.
- **`cmd/agezt/main.go`**: `buildWebhook` wires it from env
  (`AGEZT_WEBHOOK_SECRET` / `_ADDR` / `_PATH` / `_CHANNELS` / `_OUTBOUND_URL`),
  starts it, registers it for `agt send` (`liveChannels["webhook"]`) and the
  Pulse brief tee, and lists it in `agt status` (`collectChannels`). Imported as
  `webhookchan` to avoid colliding with the existing `kernel/webhook` (outbound)
  package.
- **`kernel/controlplane/config.go`**: registered the 5 new env vars in the
  `configEnvVars` inventory (the `TestConfigEnvVars_CoversCmdAgeztReads` guard
  caught the omission — kept honest).

## Verification
- **`plugins/channels/webhook/webhook_test.go`** (7 tests): signed+allowlisted →
  handler runs and the reply returns; wrong/missing signature → 401; empty secret
  fails closed (handler never runs); non-allowlisted channel → 403; stale `ts_ms`
  → 401; repeated `id` → de-duped (handler runs once); `Send` → signed outbound
  POST (receiver re-verifies the signature) and errors with no URL.
- **Live daemon**: started the daemon with the channel enabled, POSTed a signed
  allowlisted message via `curl` → the full agent loop ran and returned a reply +
  correlation id; an unsigned POST → 401. (An initial attempt hit a port held by a
  leftover daemon — re-ran on a free port; the channel logic was already proven by
  the unit tests.)
- Full suite **2039** passing, `go test ./...` exit 0 (two clean runs); `gofmt -l`
  clean; `go vet` clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Channels now: Telegram, Slack, Discord, **generic webhook**. The remaining
  SPEC-04 channels (email/SMS/WhatsApp/Signal/Matrix/Teams/HomeAssistant) need
  live external services to verify and are separate follow-ups; email via SMTP
  (mockable) is the next offline-verifiable candidate.
- The signature scheme is intentionally identical to the outbound dispatcher, so
  an Agezt outbound webhook can loop into an Agezt inbound webhook channel.

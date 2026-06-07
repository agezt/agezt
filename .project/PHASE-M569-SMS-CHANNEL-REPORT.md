# Phase M569 — SMS (Twilio) messaging channel

**Type:** Feature (channel sweep; seventh channel after telegram/slack/discord/webhook/email/matrix)
**Date:** 2026-06-07
**Branch:** `feat-sms-channel`

## Goal

Add SMS as a messaging channel over Twilio Programmable Messaging, following the
established inbound-HTTP channel template (webhook/slack) for inbound + a REST
client for outbound, holding to the stdlib-first, no-new-dependency, fail-closed
posture and the `kernel/channel` contract.

## What shipped

### `plugins/channels/sms/sms.go` (new)
- Twilio Programmable Messaging over `net/http` + stdlib crypto only — no SDK, no
  new module.
- **Inbound** (`handleInbound`): bounded body, `url.ParseQuery` the form, then
  authenticate with `X-Twilio-Signature` = base64(HMAC-SHA1(authToken, signedURL +
  concat of sorted form key+value)) — the documented Twilio scheme. Empty auth
  token fails closed. Allowlist by sender number; build `UnifiedMessage`
  (ChannelID/Sender = `From`, Text = `Body`); emit inbound event; run handler;
  reply synchronously as **TwiML** (`<Response><Message>…</Message></Response>`,
  XML-escaped). Retried `MessageSid` de-duped (two-generation bounded set, mirrors
  the webhook guard). Refused/empty → bare `<Response/>` ack (200, so Twilio
  doesn't retry).
- **Outbound** (`Send`): `POST /2010-04-01/Accounts/{SID}/Messages.json`,
  form-encoded To/From/Body, HTTP Basic auth; `channel.SplitText` at 1500 chars
  per request. Empty text = no-op; missing From/credentials = error.
- `signedURL` uses a configured `PublicURL` (the exact URL Twilio signs behind a
  tunnel) or reconstructs from the request. Bus events
  `channel.inbound.sms`/`channel.outbound.sms`.

### `plugins/channels/sms/sms_test.go` (new — 7 tests, all pass)
Signed-request helper computes a valid `X-Twilio-Signature` against a fixed
public URL. Covers: allowed sender → handler runs + TwiML reply + inbound/outbound
events; bad signature → 401, handler skipped; empty token → fail-closed 401;
non-allowlisted → empty TwiML, no handler, refusal journaled; `MessageSid` dedup
(handler runs once); outbound posts to a mock Twilio with Basic auth + To/From/Body
and splits a long body into 2 requests; empty-noop + unconfigured-errors.

### Daemon wiring
- `cmd/agezt/main.go`: `buildSMS(ctx, k)` (gated on `AGEZT_SMS_ACCOUNT_SID` +
  `AGEZT_SMS_AUTH_TOKEN`; inbound on `AGEZT_SMS_ADDR`; outbound from
  `AGEZT_SMS_FROM`; allowlist `AGEZT_SMS_NUMBERS`; `AGEZT_SMS_PATH` /
  `AGEZT_SMS_PUBLIC_URL`); banner, `combineSinks`, `liveChannels["sms"]`,
  `collectChannels()`, pulse brief sink.
- `kernel/controlplane/config.go`: 7 `AGEZT_SMS_*` env vars added to
  `configEnvVars` (keeps `TestConfigEnvVars_CoversCmdAgeztReads` green).

## Verification

- **Unit:** `plugins/channels/sms` — 7/7 pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` exit 0 (76 packages).
  gofmt (staged blobs) / vet / staticcheck clean. `go.mod`/`go.sum` unchanged.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon with
  SMS configured, then over HTTP:
  - banner `sms channel : inbound at 127.0.0.1:18855/sms, allowlist=1 number(s)`.
  - a **genuinely Twilio-signed** POST (signature computed with `openssl` HMAC-SHA1,
    independent of the Go code) from the allowed number → TwiML
    `<Response><Message>[echo] ping</Message></Response>` (agent ran, reply
    returned) — proving the signature implementation matches Twilio's spec.
  - unsigned → 401; signed-but-not-allowlisted → empty `<Response/>` (no agent run).
  - `agt status` lists `sms (inbound @127.0.0.1:18855, allow 1)`; 2 inbound + 1
    outbound events journaled; **0 panics**.

## Counts

- Packages: 75 → **76**. Tests (funcs + subtests): 2425 → **2432**.

## Out of scope (documented follow-ups)

- MMS (image) messages — text-only for now.
- WhatsApp via the same Twilio account (different `From`/endpoint shape) — its own
  channel/follow-up.

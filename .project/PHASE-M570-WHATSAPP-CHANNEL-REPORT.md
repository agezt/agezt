# Phase M570 — WhatsApp (Meta Cloud API) messaging channel

**Type:** Feature (channel sweep; eighth channel)
**Date:** 2026-06-07
**Branch:** `feat-whatsapp-channel`

## Goal

Add WhatsApp as a messaging channel over Meta's WhatsApp Cloud API, following the
inbound-webhook channel template (sms/webhook) with Meta's two-shape inbound
(GET verify handshake + signed POST deliveries) and Graph API outbound. Stdlib-only,
no new dependency, fail-closed.

## What shipped

### `plugins/channels/whatsapp/whatsapp.go` (new)
- Meta WhatsApp Cloud API over `net/http` + stdlib crypto — no SDK.
- **Inbound GET**: verification handshake — echoes `hub.challenge` when
  `hub.mode=subscribe` and `hub.verify_token` matches (constant-time).
- **Inbound POST**: authenticate `X-Hub-Signature-256` = `sha256=` hex
  HMAC-SHA256(appSecret, raw body); empty secret fails closed. Parse the nested
  envelope (`entry[].changes[].value.messages[]`), flatten to text messages, and
  for each: dedup on message id, allowlist by sender, emit inbound, run handler,
  send the reply back via the Graph API (WhatsApp has no synchronous reply).
  Acknowledge 200 promptly (Meta retries; dedup makes that safe).
- **Outbound** (`Send`/`send`): `POST {GraphBase}/{PhoneNumberID}/messages` with
  `{messaging_product:"whatsapp", to, type:"text", text:{body}}`, Bearer access
  token, `channel.SplitText` at 4000 chars. Bus events
  `channel.inbound.whatsapp`/`channel.outbound.whatsapp`; a reply-send failure is
  journaled as `channel.error.whatsapp`.

### `plugins/channels/whatsapp/whatsapp_test.go` (new — 8 tests, all pass)
A mock Graph server records outbound message POSTs. Covers: GET handshake
(challenge echo + wrong-token 403); signed delivery from an allowed number →
handler runs + reply POSTed via Graph with a Bearer token + inbound/outbound
events; bad signature → 401, no handler/send; empty secret → fail-closed 401;
non-allowlisted → refused (no handler/send, refusal journaled); message-id dedup
(handler runs once); outbound chunking (long body → 2 sends); empty-noop +
unconfigured-errors.

### Daemon wiring
- `cmd/agezt/main.go`: `buildWhatsApp(ctx, k)` (gated on
  `AGEZT_WHATSAPP_APP_SECRET` + `AGEZT_WHATSAPP_ACCESS_TOKEN`; inbound on
  `AGEZT_WHATSAPP_ADDR`; outbound via `AGEZT_WHATSAPP_PHONE_NUMBER_ID`; verify
  `AGEZT_WHATSAPP_VERIFY_TOKEN`; allowlist `AGEZT_WHATSAPP_NUMBERS`; `_PATH`);
  banner, `combineSinks`, `liveChannels["whatsapp"]`, `collectChannels()`, pulse
  brief sink.
- `kernel/controlplane/config.go`: 7 `AGEZT_WHATSAPP_*` env vars added to
  `configEnvVars`.

## Verification

- **Unit:** `plugins/channels/whatsapp` — 8/8 pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` exit 0 (77 packages).
  gofmt (staged blobs) / vet / staticcheck clean. `go.mod`/`go.sum` unchanged.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon with
  WhatsApp configured:
  - banner `whatsapp channel : inbound at 127.0.0.1:18866/whatsapp, allowlist=1 number(s)`.
  - GET verify handshake echoed the challenge (`777`); wrong token → 403.
  - a **genuinely HMAC-SHA256-signed** POST (signature computed with `openssl`,
    independent of the Go code) from the allowed number → 200 + inbound event
    journaled — proving the X-Hub-Signature-256 implementation matches Meta's spec.
  - unsigned → 401; `agt status` lists `whatsapp (inbound @…, allow 1)`; 0 panics.

## Counts

- Packages: 76 → **77**. Tests (funcs + subtests): 2432 → **2440**.

## Out of scope (documented follow-ups)

- Non-text message types (image/audio/document/interactive) — text-only.
- Async processing (200-then-handle-in-goroutine) for very slow agent runs; the
  current synchronous handle + dedup is fine for the MVP.

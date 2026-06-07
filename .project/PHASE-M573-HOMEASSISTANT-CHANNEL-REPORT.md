# Phase M573 — Home Assistant (outbound) channel

**Type:** Feature (channel sweep; ninth channel)
**Date:** 2026-06-07
**Branch:** `feat-homeassistant-channel`

## Goal

Add Home Assistant as an outbound channel so Agezt — the agentic OS — can speak
into your home: Pulse briefs and `agt send` land as phone pushes, TTS
announcements, or dashboard notifications via HA's REST notify API. Mirror the
email outbound-only template; stdlib-only, no new dependency, fail-closed.

## What shipped

### `plugins/channels/homeassistant/homeassistant.go` (new)
- Outbound over HA's REST notify API on `net/http` only.
- `Send`: `POST {BaseURL}/api/services/notify/{service}` with a long-lived bearer
  token and `{"message": text}`. The `channel_id` is the HA notify SERVICE name
  (`mobile_app_phone`, `persistent_notification`, `tts`, …). Empty text = no-op;
  a non-allowlisted service is refused before any HTTP call (fail-closed); a
  non-2xx HA response surfaces as an error. Bus event
  `channel.outbound.homeassistant`. `Start` blocks on ctx (outbound-only, uniform
  lifecycle). Injectable HTTP client for tests.

### `plugins/channels/homeassistant/homeassistant_test.go` (new — 4 tests, all pass)
A mock HA server records notify POSTs. Covers: allowed service → correct
endpoint path + bearer token + message body + outbound event; non-allowlisted →
refused, no HTTP call, no event; empty-noop + non-2xx → error; unconfigured →
error.

### Daemon wiring
- `cmd/agezt/main.go`: `buildHomeAssistant(ctx, k)` (gated on
  `AGEZT_HOMEASSISTANT_URL` + `AGEZT_HOMEASSISTANT_TOKEN`; allowlist
  `AGEZT_HOMEASSISTANT_SERVICES`); banner, `combineSinks`,
  `liveChannels["homeassistant"]`, `collectChannels()`, pulse brief sink.
- `kernel/controlplane/config.go`: 3 `AGEZT_HOMEASSISTANT_*` vars added to
  `configEnvVars`.
- `cmd/agt/send.go`: refreshed the `--channel` help to list all nine channels.

## Verification

- **Unit:** `plugins/channels/homeassistant` — 4/4 pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` exit 0 (78 packages).
  gofmt / vet / staticcheck clean. `go.mod`/`go.sum` unchanged.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon with
  HA pointed at a mock HA server:
  - banner `homeassistant ch : outbound → http://127.0.0.1:18900, allowlist=1 service(s)`.
  - `agt status` shows `homeassistant (outbound-only, allow 1)`.
  - `agt send --channel homeassistant --to mobile_app_phone "dinner is ready"` →
    `sent to homeassistant/mobile_app_phone` (the channel POSTed to the mock and
    got a 2xx — Send only succeeds on 2xx). 0 panics.

## Counts

- Packages: 77 → **78**. Tests (funcs + subtests): 2440 → **2444**.

## Out of scope (documented follow-ups)

- Inbound (HA → Agezt): use the generic webhook channel (HA automation POSTs a
  kernel-signed message); a dedicated HA Assist/conversation inbound is a follow-up.
- `notify` extras (title, target, data) — currently sends `{"message": …}`.
- An HA *tool* (query/control entities via `/api/states` + `/api/services`) —
  distinct from this notify channel; a follow-up.

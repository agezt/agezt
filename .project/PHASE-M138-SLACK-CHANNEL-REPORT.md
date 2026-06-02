# M138 — Slack channel

## Why
Until now Agezt had exactly one chat surface: Telegram. ROADMAP M3 ("the rest of the
channels") calls for the daemon to be reachable where teams actually live — and the
single most-requested of those is Slack. The hard constraint is **stdlib-only**
(`go.mod` must stay BLAKE3 + cpuid). The naive Slack integration uses the Socket-Mode
WebSocket, which would drag in a WebSocket dependency. The way through: Slack's
**Events API** is a plain HTTPS webhook with an HMAC request signature — it fits the
existing `channel.Channel` (`Name` / `Start` / `Send`) shape with nothing but
`net/http` + `crypto/hmac`, exactly mirroring the Telegram channel's role while
inverting the transport (Slack pushes; Telegram long-polls).

## What
A new in-process duplex channel, `plugins/channels/slack`, implementing
`channel.Channel`:

- **Inbound** — `Start(ctx)` serves `POST /slack/events` on `AGEZT_SLACK_ADDR`
  (fronted by the operator's tunnel/reverse-proxy). Every request is verified:
  `v0=HMAC-SHA256(signing_secret, "v0:{ts}:{body}")` with a ±5-minute timestamp
  freshness window (replay protection). An **empty signing secret fails closed** —
  no inbound is accepted without one. The `url_verification` handshake echoes the
  challenge so Slack will accept the endpoint. Real events are **ACKed with 200
  immediately** (Slack retries if it doesn't see a 200 within 3s) and the agent runs
  **asynchronously** on a background context, posting its reply when done.
- **Loop / injection guards** — only `type:"message"` events with a real `user` and
  non-empty `text` drive the agent; `bot_id` / `subtype` (edits, joins,
  `bot_message`) and self-posts are ignored, so the agent never replies to its own
  output. A retry delivery (`X-Slack-Retry-Num`) is ACKed but not reprocessed. A
  channel-id **Allowlist** gates who may drive the agent (fail-closed: a
  non-allowlisted channel gets a "not authorized" reply, the handler never runs).
- **Outbound** — `Send` / `send` POST `chat.postMessage` with `Authorization: Bearer
  {token}`, checking Slack's `{ok,error}` envelope. Doubles as a Pulse brief sink.
- **Observability** — inbound and outbound are journaled as
  `channel.inbound.slack` / `channel.outbound.slack` (Kind
  `KindChannelInbound`/`KindChannelOutbound`) so `agt why` / `agt inbox` reconstruct
  the exchange.
- **Wiring** — `cmd/agezt/main.go` gains `buildSlack(ctx, k)` (mirrors
  `buildTelegram`: reads `AGEZT_SLACK_TOKEN` / `_SIGNING_SECRET` / `_ADDR` /
  `_CHANNELS` / `_API_BASE`; handler runs `k.RunWith`), and `combineSinks(...)` so
  Telegram + Slack Pulse sinks coexist. Five `AGEZT_SLACK_*` vars added to
  `configEnvVars` (M127 drift guard passes).

## Files
- `plugins/channels/slack/slack.go` (new, ~340 lines) — the channel.
- `plugins/channels/slack/slack_test.go` (new) — 4 tests.
- `cmd/agezt/main.go` — `buildSlack`, `combineSinks`, slack import, main() wiring.
- `kernel/controlplane/config.go` — 5 `AGEZT_SLACK_*` env vars in `configEnvVars`.

## Tests (4, all passing)
- `TestSlack_URLVerification` — challenge handshake echoes `challenge`.
- `TestSlack_BadSignatureRejected` — bad signature → 401; stale timestamp
  (>5 min) with an otherwise-valid sig → 401.
- `TestSlack_MessageDrivesAgentAndReplies` — signed `event_callback` → fast 200,
  handler sees the text, `chat.postMessage` posts the reply to the right channel
  (captured by an httptest Slack-API stand-in).
- `TestSlack_IgnoresBotAndNonAllowlisted` — a `bot_id` message drives nothing; a
  non-allowlisted channel gets a "not authorized" post but never runs the handler.

## Live proof (offline mock provider)
A real daemon was booted with `AGEZT_SLACK_TOKEN` / `_SIGNING_SECRET` / `_ADDR` /
`_CHANNELS` set and `AGEZT_SLACK_API_BASE` pointed at a local fake Slack API:

```
banner:  slack : events at 127.0.0.1:8840/slack/events, allowlist=1 channel(s)

# signed message event → fast ACK, agent runs, reply posted back to Slack:
POST /slack/events  (valid v0= signature)   → HTTP 200
SLACK-API got chat.postMessage:
  {"channel":"C1","text":"[offline-mock] I ran a directory listing via the shell
   tool. This project is Agezt..."}

# tampered signature is rejected at the door:
POST /slack/events  (X-Slack-Signature: v0=deadbeef)   → HTTP 401
```

End-to-end duplex confirmed: signature-gated inbound → async agent run → outbound
`chat.postMessage`, with the bad-signature path returning 401.

## Verification
- `go.mod` / `go.sum` unchanged (stdlib-only; no SDK).
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all 4 touched files.
- `go test ./...` — 56 ok packages, **FAIL 0**, **1437 tests** (was 1433; +4).

## Result
Agezt now speaks Slack as a first-class duplex channel alongside Telegram, with the
same security posture (HMAC authenticity + Allowlist authority + loop guards +
journaling) and zero new dependencies. ROADMAP M3 has its second channel.

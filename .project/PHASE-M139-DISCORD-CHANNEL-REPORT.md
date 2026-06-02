# M139 — Discord channel

## Why
M138 added Slack as the second chat surface; the channel abstraction
(`kernel/channel`) is explicitly meant to generalize ("telegram", "discord", …
in its own doc comment). Discord is the next obvious surface — but it poses a
sharper version of the stdlib-only constraint than Slack did. Free-form Discord
messages are only delivered over the **Gateway**, a persistent WebSocket, which
would force a WebSocket dependency and break the `go.mod`-unchanged rule. The way
through: Discord's **Interactions** model is a plain HTTPS webhook (slash
commands), and its request signature is **Ed25519** — covered by stdlib
`crypto/ed25519`. So the same `channel.Channel` shape fits, with a *different*
signature scheme than Slack's HMAC, proving the abstraction holds across crypto.

## What
A new in-process duplex channel, `plugins/channels/discord`, implementing
`channel.Channel`:

- **Inbound** — `Start(ctx)` serves `POST /discord/interactions` on
  `AGEZT_DISCORD_ADDR` (fronted by the operator's tunnel/reverse-proxy). Every
  request is verified: `Ed25519.Verify(public_key, timestamp‖body, signature)`
  with a ±5-minute timestamp freshness window (replay protection). An
  **empty/invalid public key fails closed** — no inbound is accepted without a
  valid app public key. A `PING` interaction (type 1) is answered with a `PONG`
  (Discord's endpoint-verification handshake).
- **Command → deferred → follow-up** — an `APPLICATION_COMMAND` (type 2, a slash
  command such as `/agezt prompt:<text>`) is ACKed **immediately with a DEFERRED
  response** (type 5 — Discord shows "Agezt is thinking…"; it requires a response
  within 3s) and the agent runs **asynchronously**. When the run finishes, the
  reply is delivered via a **follow-up webhook** POST to
  `webhooks/{application_id}/{interaction_token}` (the URL token authenticates, so
  no bot header on this path).
- **Authority / guards** — a channel-id **Allowlist** gates who may drive the
  agent (fail-closed). A non-allowlisted command gets an **immediate ephemeral
  "not authorized"** (type 4, flag 64) and the handler never runs. An empty prompt
  or absent handler yields an ephemeral "nothing to do".
- **Outbound** — `Send` (channel.Channel; Pulse brief sink and out-of-band
  senders) posts to `channels/{id}/messages` with `Authorization: Bot {token}` —
  distinct from the interaction-follow-up path.
- **Observability** — inbound and outbound are journaled as
  `channel.inbound.discord` / `channel.outbound.discord` (Kinds
  `KindChannelInbound`/`KindChannelOutbound`) so `agt why` / `agt inbox`
  reconstruct the exchange.
- **Wiring** — `cmd/agezt/main.go` gains `buildDiscord(ctx, k)` (mirrors
  `buildSlack`/`buildTelegram`: reads `AGEZT_DISCORD_TOKEN` / `_PUBLIC_KEY` /
  `_APP_ID` / `_ADDR` / `_CHANNELS` / `_API_BASE`; handler runs `k.RunWith`), and
  the Discord Pulse sink joins `combineSinks(tgSink, slSink, dcSink)`. Six
  `AGEZT_DISCORD_*` vars added to `configEnvVars` (M127 drift guard passes).

## Files
- `plugins/channels/discord/discord.go` (new, ~390 lines) — the channel.
- `plugins/channels/discord/discord_test.go` (new) — 4 tests.
- `cmd/agezt/main.go` — `buildDiscord`, discord import, main() wiring,
  `combineSinks` third sink.
- `kernel/controlplane/config.go` — 6 `AGEZT_DISCORD_*` env vars in `configEnvVars`.

## Tests (4, all passing)
- `TestDiscord_PingPong` — a signed PING (type 1) → `{"type":1}` PONG.
- `TestDiscord_BadSignatureRejected` — zero signature → 401; stale timestamp
  (>5 min) → 401; a *different keypair's* valid signature → 401.
- `TestDiscord_CommandDrivesAgentAndFollowsUp` — a signed command → fast deferred
  ACK (type 5), the handler sees the prompt text, and the reply lands as a
  follow-up webhook (captured by an httptest Discord-API stand-in).
- `TestDiscord_IgnoresNonAllowlisted` — a non-allowlisted command → immediate
  ephemeral "not authorized" (type 4), the handler never runs, no follow-up posts.

## Live proof (offline mock provider, real booted daemon)
A real daemon was booted with `AGEZT_PROVIDER=mock` and the Discord channel
enabled; a throwaway Ed25519 signer generated a keypair, handed the daemon its
public key, ran a fake Discord API to capture the follow-up, and signed real
requests:

```
banner:  discord : interactions at 127.0.0.1:8850/discord/interactions, allowlist=1 channel(s)

PING    (valid Ed25519 sig)  -> HTTP 200 {"type":1}      # PONG handshake
COMMAND (valid Ed25519 sig)  -> HTTP 200 {"type":5}      # deferred ACK
  follow-up webhook the daemon then posted:
  FAKE-API POST /webhooks/APP1/tok-live ->
    {"content":"[offline-mock] I ran a directory listing via the shell tool.
     This project is Agezt — an open-source, MIT-licensed agentic operating
     system written in Go. …"}
BAD-SIG (zeroed signature)   -> HTTP 401                 # rejected at the door

journal: [evt seq=0  kind=channel.inbound  subject=channel.inbound.discord]
         [evt seq=16 kind=channel.outbound subject=channel.outbound.discord]
```

End-to-end duplex confirmed: Ed25519-gated inbound → deferred ACK → async agent
run → follow-up webhook, with the bad-signature path returning 401 and both
directions journaled.

## Verification
- `go.mod` / `go.sum` unchanged (stdlib-only; `crypto/ed25519`, no SDK).
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all 4 touched files.
- M127 drift guard (`TestConfigEnvVars_CoversCmdAgeztReads`) passes with the 6 new
  `AGEZT_DISCORD_*` vars.
- `go test ./...` — 57 ok packages, **FAIL 0**, **1441 tests** (was 1437; +4).

## Result
Agezt now speaks Telegram, Slack, and Discord as first-class duplex channels,
each with the same security posture (signature authenticity + Allowlist authority
+ loop guards + journaling) and **zero new dependencies** — and across three
different transports (long-poll / HMAC webhook / Ed25519 webhook). ROADMAP M3 has
its third channel, and the abstraction is proven general.

# Phase M563 — Matrix messaging channel

**Type:** Feature (first deferred-roadmap surface; user-selected "Yeni mesajlaşma kanalı")
**Date:** 2026-06-07
**Branch:** `feat-matrix-channel`

## Goal

Add Matrix as a sixth first-class messaging channel alongside the existing five
(Telegram, Slack, Discord, webhook, email), holding to the project's stdlib-first,
zero-new-dependency, fail-closed posture and the existing `kernel/channel`
contract (`Name`/`Start`/`Send`, `UnifiedMessage`, `Allowlist`, `Guard`,
`SplitText`).

## What shipped

### `plugins/channels/matrix/matrix.go` (new)
- Matrix Client-Server API **v3** over `net/http` only — no SDK, no new module in
  `go.mod`/`go.sum`.
- `Config{Homeserver, Token, Allowlist, Bus, Handler, HTTPClient, PollTimeoutSecs}`;
  `New` trims a trailing `/` on the homeserver, defaults the poll to 30s and the
  HTTP client to a 60s timeout.
- `Start(ctx)`: `resolveWhoami` (own MXID — required to skip self-messages),
  **prime** (an initial `/sync` that discards backlog), then the long-poll loop:
  `sync` → `dispatchable` filter → `handleInbound` through `channel.Guard`.
- `dispatchable(ev)`: true only for `m.room.message` + `m.text` msgtype +
  non-empty body + `Sender != self` (no echo loop).
- `handleInbound`: normalize → `UnifiedMessage`, allowlist by room id (fail-closed
  "not authorized" notice on miss, handler never runs), run handler, reply.
- `send`: `SplitText` chunks, one `PUT .../send/m.room.message/{txn}` per chunk
  with a **fresh ulid txn** (idempotency), `Authorization: Bearer`, empty = no-op.
- Bus events `channel.inbound.matrix` / `channel.outbound.matrix`
  (`KindChannelInbound`/`KindChannelOutbound`); token scrubbed from errors.
- Scope: **text-only**. Image/file events are a documented follow-up.

### `plugins/channels/matrix/matrix_test.go` (new — 6 tests, all pass)
A `fakeHomeserver` (`httptest`) records PUT sends + auth headers and serves
scripted `whoami` + `/sync` responses. Covers: allowed-room reply via send
(asserts bearer header + inbound/outbound journal events), non-allowlisted room
refused fail-closed (handler never runs), `dispatchable` truth table
(self/non-text/empty/wrong-type all skipped), `/sync` parse + `since` cursor
advance + forward, empty-noop + multi-chunk split with distinct txns, and
`resolveWhoami`.

### Daemon wiring
- `cmd/agezt/main.go`: `buildMatrix(ctx, k)` (mirrors `buildTelegram`), gated on
  `AGEZT_MATRIX_HOMESERVER` + `AGEZT_MATRIX_TOKEN`, allowlist from
  `AGEZT_MATRIX_ROOMS`; banner, `combineSinks`, `liveChannels["matrix"]`,
  `collectChannels()`, pulse brief sink.
- `kernel/controlplane/config.go`: three env vars added to `configEnvVars`
  (`AGEZT_MATRIX_HOMESERVER` / `_ROOMS` / `_TOKEN`) so the config snapshot reports
  their presence and `TestConfigEnvVars_CoversCmdAgeztReads` (M127) stays green.

## Verification

- **Unit:** `plugins/channels/matrix` — 6/6 pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` — exit 0 (75 packages).
- **gofmt:** staged LF blobs of all changed `.go` files clean (`git show :path | gofmt -l`).
- **vet / staticcheck:** clean on changed packages.
- **Runtime smoke:** daemon boots with the three env vars set against a stub
  homeserver → banner `matrix channel : listening, allowlist=1 room(s)`,
  `agt status` shows `channels : matrix (inbound ..., allow 1)`, 0 panics,
  graceful shutdown.
- **go.mod / go.sum:** unchanged (no new dependency).

## Counts

- Packages: 69 → **75**.
- Tests (funcs + subtests): 2315 → **2421**.

## Out of scope (documented follow-ups)

- Matrix image / file (`m.image` / `m.file`) events — text-only for now.
- E2EE rooms (olm/megolm) — the C-S API path here is unencrypted rooms.

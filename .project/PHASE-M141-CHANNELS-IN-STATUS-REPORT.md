# M141 — Configured channels in `agt status`

## Why
M138–M140 brought the daemon to three duplex channels (Telegram, Slack, Discord),
but their only runtime visibility was the per-channel **boot banner** — which
scrolls off-screen the moment the daemon is running. An operator asking "is Slack
actually listening? on what address? who's allowlisted?" had no live answer short
of restarting and watching the banner, or grepping the process env. `agt status`
is the at-a-glance dashboard (it already surfaces schedules, tenants, approvals,
delegation, HTTP servers); channels belonged there too. This is the exact M137
pattern: a fact known only at boot becomes a persistent, queryable part of the
status snapshot.

## What
- **Injection** (`kernel/controlplane/server.go`) — a `ChannelInfo{Kind, Inbound,
  Addr, Allowlist}` type and a `channels []ChannelInfo` field, set by the daemon via
  `Server.SetChannels(...)`. This keeps `kernel/controlplane` from importing the
  channel plugins (the same decoupling as `SetHTTPBindings` / `SetPulse`).
- **Status surface** (`kernel/controlplane/status.go`) — `handleStatus` adds a
  `channels` array (kind / inbound / addr / allowlist) when any is configured;
  omitted entirely otherwise so a channel-less daemon shows no noise.
- **Daemon wiring** (`cmd/agezt/main.go`) — `collectChannels()` reads the same env
  the `buildTelegram`/`buildSlack`/`buildDiscord` functions consume (read-only) and
  builds the inventory. A channel is listed when its **token** is set; `Inbound`
  reflects whether it can actually receive and act on commands — Telegram always can
  (it long-polls), while Slack/Discord need a listen addr **plus** the inbound
  secret / public key. A webhook channel with an addr but no secret/key therefore
  shows as `outbound-only`, exposing a silent half-configuration rather than
  pretending it's live. Wired next to `SetHTTPBindings`.
- **CLI render** (`cmd/agt/status.go`) — a one-line `channels :` summary, e.g.
  `telegram (inbound, allow 2), slack (inbound @127.0.0.1:8840, allow 1),
  discord (outbound-only, allow 3)`. Quiet when none configured. Full structured
  data is in `agt status --json` under `channels`.

## Files
- `kernel/controlplane/server.go` — `ChannelInfo` type, `channels` field,
  `SetChannels`.
- `kernel/controlplane/status.go` — `channels` in the status result.
- `cmd/agezt/main.go` — `collectChannels()`; `srv.SetChannels(...)`.
- `cmd/agt/status.go` — the `channels :` render line.
- `kernel/controlplane/status_test.go` — `TestStatus_Channels`.
- `cmd/agezt/main_test.go` — `TestCollectChannels`.

## Tests (+2, all passing)
- `TestStatus_Channels` — absent when none configured; with two set, the array
  carries kind/inbound/addr/allowlist for each in order.
- `TestCollectChannels` — env→inventory: no tokens → empty; with all three tokens
  set and Slack's signing secret deliberately omitted, Telegram is inbound (allow 2),
  Slack is **not** inbound (addr set, no secret) with allow 1, Discord is inbound
  (addr + public key) with allow 3.

## Live proof (offline mock, real booted daemon)
Booted with all three channels configured — but Discord's `PUBLIC_KEY` deliberately
left unset:

```
$ agt status
  channels  : telegram (inbound, allow 2), slack (inbound @127.0.0.1:8840, allow 1), discord (outbound-only, allow 3)

$ agt status --json   # channels array
  telegram → inbound:true,  addr:"",               allowlist:2
  slack    → inbound:true,  addr:"127.0.0.1:8840", allowlist:1
  discord  → inbound:false, addr:"127.0.0.1:8850", allowlist:3   ← addr set but no public key
```

The Discord line proves the half-configured detection: a listen addr without a
public key renders as `outbound-only`, not a false "listening".

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1446 tests** (was 1444; +2).

## Result
The messaging channels are now first-class on the status dashboard: an operator
sees what's listening, where, and for whom — and a silently half-configured webhook
channel is called out as outbound-only instead of masquerading as live.

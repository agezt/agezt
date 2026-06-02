# M142 — `agt send` (operator-initiated channel egress)

## Why
After M138–M141 the daemon could *receive* on three channels (Telegram, Slack,
Discord), *observe* them (`agt inbox`, `agt status`), but only emit outbound in two
hard-wired ways: Pulse briefs and agent replies. There was no way for an operator,
a script, or a CI job to push a one-off message out a channel — the obvious
"build went green → ping Slack" / "nightly job done → notify the ops chat" use
case. The channels already had a `Send` method (Pulse uses it); it just wasn't
reachable from the CLI. `agt send` closes that, turning the channels into a
general notification egress.

## What
- **Protocol** (`kernel/controlplane/protocol.go`) — `CmdSend = "send"`. Args:
  `channel` (kind), `to` (chat/channel id), `text`. Returns `{sent, channel, to}`.
- **Server** (`kernel/controlplane/server.go`, `send.go`) — a `ChannelSender`
  primitive func type (`func(ctx, kind, channelID, text string) error`) and a
  `channelSend` field set via `SetChannelSender`. Kept as a primitive (not a
  `channel.Channel`) so `kernel/controlplane` still never imports the channel
  plugins — the same decoupling as `SetHTTPBindings` / `SetChannels` / `SetPulse`.
  `handleSend` validates the three args, returns a clear error when no channel is
  configured (nil sender), runs the send under a 30s timeout, and surfaces the
  sender's error (e.g. unknown channel kind) verbatim.
- **Daemon wiring** (`cmd/agezt/main.go`) — builds a `map[kind]channel.Channel` from
  the channels actually constructed at boot (`tgChan`/`slChan`/`dcChan`, each added
  only when non-nil) and injects a `SetChannelSender` closure that looks up the kind
  and calls its `Send` (which journals `channel.outbound`). An unconfigured kind
  returns `channel %q not configured`.
- **CLI** (`cmd/agt/send.go`) — `agt send --channel KIND --to ID <text...>`. Accepts
  `--channel`/`--channel=`, `--to`/`--to=`, and joins the remaining args as the
  message text. Missing channel/to/text → usage error (exit 2) before dialing.
  Documented in the top-level help.

## Authority
`agt send` is gated by control-plane authentication (the primary token); it is NOT
added to `tenantTokenAllows`, so a tenant token can't drive it. Because the caller
already holds daemon authority, there's deliberately **no** per-channel allowlist
gate on the send path (the inbound allowlist gates who may *drive the agent*, a
different threat). The outbound is journaled like any other, so it remains
auditable via `agt why` / `agt inbox`.

## Files
- `kernel/controlplane/protocol.go` — `CmdSend` constant + doc.
- `kernel/controlplane/server.go` — `ChannelSender` type, `channelSend` field,
  `SetChannelSender`, dispatch case.
- `kernel/controlplane/send.go` (new) — `handleSend` + `stringArg` helper.
- `cmd/agezt/main.go` — live-channel map + `SetChannelSender` closure.
- `cmd/agt/send.go` (new) — the `agt send` command.
- `cmd/agt/main.go` — dispatch + help line.
- `kernel/controlplane/send_test.go` (new), `cmd/agt/send_test.go` (new).

## Tests (+8, all passing)
- Server: routes to the sender with the kind lowercased and to/text passed through
  (`sent:true`); missing text → error; no sender configured → error; the sender's
  error (unknown channel) is surfaced.
- CLI: help shows usage; text without `--channel`/`--to` → exit 2; `--channel` /
  `--to` with no value → exit 2.

## Live proof (offline mock, real booted daemon + fake Discord API)
A daemon was booted with Discord configured and `AGEZT_DISCORD_API_BASE` pointed at
a local fake API:

```
$ agt send --channel discord --to D9 "deploy finished — all green"
sent to discord/D9
  fake API: POST /channels/D9/messages -> {"content":"deploy finished — all green"}

$ agt send --channel slack --to C1 "nope"      # slack not configured
agt send: controlplane: channel "slack" not configured   (exit 1)

$ agt inbox --channel discord                  # the egress is journaled
1 thread(s):
── discord/D9
   → deploy finished — all green
```

Full path confirmed: CLI → control plane → live channel `Send` → real HTTP POST →
journaled `channel.outbound` → visible in `agt inbox` (and filterable by M140).

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1454 tests** (was 1446; +8).

## Result
The channels are now a full duplex surface in both directions and from both
drivers: the agent and Pulse push automatically, and now an operator/script can
push on demand — auditable, with a clear error when a kind isn't configured.

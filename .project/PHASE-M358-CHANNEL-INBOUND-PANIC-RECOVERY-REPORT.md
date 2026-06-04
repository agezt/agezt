# M358 — Channel inbound panic recovery (untrusted-input DoS guard)

## Why
The higher-severity half of the panic-containment audit started in M357. The
Telegram, Slack, and Discord channels process **untrusted external messages** on
long-lived daemon goroutines:
- Telegram: `handleInbound` called synchronously in the long-poll loop;
- Slack: `go c.process(ctx, ev)` per event;
- Discord: `go c.runAndFollowUp(...)` per message.

None recovered from a panic. In Go an unrecovered panic in any goroutine crashes
the whole process, so a single malformed inbound message that trips a handler bug
(a nil-deref on an unexpected message shape, an edge case in normalization /
reply formatting — code paths fed by attacker-controlled JSON) would take down the
entire daemon: every run, every channel, the web UI. `agent.Run` recovers
internally, so the agent loop itself is safe — but the channel-side parsing,
allowlist, journaling, and reply handling around it were not.

(The webhook channel is already safe: it's served by `net/http`, which recovers
per request.)

## What
Production reliability fix + lock-in tests.
- **`kernel/event/kinds.go`** — new `KindChannelError = "channel.error"` (+ in the
  known-kinds map) so a recovered panic stays diagnosable rather than silent.
- **`kernel/channel/guard.go`** — `Guard(b, channelName, fn)`: runs `fn`, recovers
  a panic, and (when a bus is supplied) journals a `channel.error` event with the
  channel name + recovered value. One shared implementation for all channels.
- **Wiring** — Telegram (`handleInbound`), Slack (`process` goroutine), and Discord
  (`runAndFollowUp` goroutine) inbound handling is now wrapped in `channel.Guard`.
- **`kernel/channel/guard_test.go`** — `TestGuard_RecoversPanicAndJournals`
  (panic → no propagation + a `channel.error` event), `TestGuard_RunsFnAndStays
  QuietWhenNoPanic` (normal path runs fn, journals nothing),
  `TestGuard_NilBusStillRecovers` (recovers even without a bus).

## Verification
- `go test ./kernel/channel -run Guard -v` — all three pass.
- `gofmt -l` clean on all edited files; `go vet ./kernel/channel ./kernel/event
  ./plugins/channels/...` clean; `GOOS=linux go build ./...` exit 0. Full suite
  **2094** passing (was 2091; +3), `go test ./...` exit 0. `go.mod`/`go.sum`
  unchanged. CHANGELOG updated (Reliability).

## Scope notes
- Completes the panic-containment audit: every request/message-handling goroutine
  is now covered — `net/http` servers (REST/OpenAI API, web UI, webhook channel)
  via the stdlib, the control-plane TCP handler via M357, the agent loop internally,
  and the Telegram/Slack/Discord inbound paths via M358.
- `Guard` journals at most one event per recovered panic; a channel that panics on
  every message will emit a `channel.error` stream the operator can see in
  `agt journal` / the web UI feed, rather than silently dropping every message.

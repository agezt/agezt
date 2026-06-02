# M144 — Multi-turn conversation context for channels

## Why
The channel arc (M138–M143) made the daemon reachable on Telegram, Slack, and
Discord, observable, and proactive — but each inbound message still drove a
**fresh, memory-less** agent run: the handler was `k.RunWith(corr, msg.Text)`
with a brand-new correlation per message. So a chat like:

> user: what's the capital of France?
> agent: Paris.
> user: and Germany?

lost all thread — the second message reached the agent as a standalone "and
Germany?" with no idea what came before. That's a command-runner-over-chat, not a
chat assistant. Multi-turn context is the single biggest thing separating the two,
and it's the natural completion of the channel arc.

## What
A read-only conversation fold + a shared handler that prepends it as context:

- **`kernel/channel/history.go`** — `ConversationHistory(r EventRanger, kind,
  channelID string, limit int) string` walks the journal once (via a tiny
  `EventRanger` interface — `*journal.Journal` satisfies it, tests pass a fake),
  collects the `channel.inbound`/`channel.outbound` events for THIS conversation
  (matching channel kind + id), takes the last `limit`, and renders a compact
  oldest-first transcript with `user:` / `assistant:` labels. Each message is
  clipped (2000 chars) so one huge message can't blow the token budget. Returns
  `""` when there is no PRIOR context (≤1 message — only the just-received one) so
  the first turn of a conversation is byte-for-byte unchanged.
- **`cmd/agezt/main.go`** — `makeChannelHandler(k)` is now the single inbound
  handler shared by all three channels (replacing three identical inline closures).
  It looks up the transcript for the message's (kind, id); when non-empty it runs
  that as the intent, else the raw text. The window is `channelHistoryLimit()` ←
  `AGEZT_CHANNEL_HISTORY` (default `defaultChannelHistory` = 10; `0` disables; a
  malformed/negative value falls back to the default).

The current inbound is journaled (by `emitInbound`) before the handler runs, so it
is the final `user:` line of the transcript; the agent replies to it with the
preceding turns as context. Prior agent replies (and notify pings / Pulse briefs
to the same chat) appear as `assistant:` turns.

## Files
- `kernel/channel/history.go` (new) — the fold + transcript renderer.
- `cmd/agezt/main.go` — `defaultChannelHistory`, `channelHistoryLimit`,
  `makeChannelHandler`; the three channel handlers now share it; `AGEZT_DEMO_ECHO`
  hook (mock echoes the last user message, to observe the intent).
- `kernel/controlplane/config.go` — `AGEZT_CHANNEL_HISTORY`, `AGEZT_DEMO_ECHO` in
  `configEnvVars` (M127 drift guard).
- `kernel/channel/history_test.go` (new).

## Tests (+5, all passing)
- `ConversationHistory`: builds an oldest-first labeled transcript; returns `""` for
  a single (first) message; isolates the conversation (other kinds/ids don't leak);
  respects the `limit` (keeps only the last N); disabled at `limit<=0` / nil ranger.

## Live proof (offline mock, real booted daemon + fake Discord API)
Booted with Discord configured, `AGEZT_DEMO_ECHO=1` (mock echoes the user intent),
default history window. Two slash commands in the SAME channel `D9`:

```
turn 1 — "what is the capital of France"
  reply 1: [echo]
           what is the capital of France                  ← first turn: raw text, no context

turn 2 — "and Germany"
  reply 2: [echo]
           [recent conversation, oldest first — reply to the latest user message]
           user: what is the capital of France
           assistant: [echo] what is the capital of France
           user: and Germany                              ← second turn SEES the first
```

Reply 2 echoes the full transcript the agent received, proving the prior turn
reached the loop — multi-turn context works end-to-end, while the first turn is
unchanged.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- M127 env drift guard passes with the two new vars.
- `go test ./...` — **FAIL 0**, **1465 tests** (was 1460; +5), 61 packages.

## Result
The channels are now a real conversational surface: the agent remembers the recent
back-and-forth of each chat and answers follow-ups in context, bounded and
configurable, with the first turn unchanged and no new state — the journal it
already writes is the conversation memory.

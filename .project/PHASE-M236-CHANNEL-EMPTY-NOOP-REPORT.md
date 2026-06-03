# M236 — Empty/whitespace outbound is a no-op, not a failed send

## Why
The flip side of M234/M235 (over-long messages): an **empty or whitespace-only**
message. Chat platforms reject a blank message — Telegram with 400 "message text
is empty", Slack with "no_text", Discord similarly — so sending one is a guaranteed
failed delivery plus error-log noise, and the user sees nothing either way.

Auditing the send paths showed two real gaps:

1. The inbound reply guards check `reply == ""` (exact empty) only, so a
   **whitespace-only** answer (`"  \n "`, which a model can emit) slips through
   and fails at the API.
2. The public `Send` method — used by **Pulse**, briefs, and `agt send` — had **no
   empty guard at all**, so a blank proactive message failed the same way.

## What
A trim-and-bail guard at each channel's send chokepoint, so empty/whitespace is a
clean no-op (return nil, no POST, no journal event) for *every* caller uniformly:

- `telegram.send`, `slack.send`, `discord.Send`, and `discord.followUp` each
  return early when `strings.TrimSpace(text) == ""`.

This complements (doesn't replace) the inbound paths' existing handling
(Telegram/Slack skip on `""`, Discord substitutes "(no output)") — those run
before send and are unaffected; the chokepoint guard catches whitespace-only and
the previously-unguarded `Send` path.

## Files
- `plugins/channels/telegram/telegram.go` — guard + `strings` import (edited).
- `plugins/channels/discord/discord.go` — guard in `Send` and `followUp` +
  `strings` import (edited).
- `plugins/channels/slack/slack.go` — guard + `strings` import (edited).
- `plugins/channels/telegram/telegram_chunk_test.go` — `TestSend_EmptyIsNoOp` (new).
- `plugins/channels/discord/discord_chunk_test.go` — `TestDiscord_EmptySendIsNoOp` (new).
- `plugins/channels/slack/slack_empty_test.go` — `TestSlack_EmptySendIsNoOp` (new).

Each test drives the real `Send` against the channel's httptest API stand-in with
`""`, spaces, and newlines/tabs, and asserts nothing is POSTed and no error is
returned.

## Verification
- `go test ./plugins/channels/...` — green; full suite **1774 → 1777** (+3),
  66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/channels/...` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Together with M234/M235, the channel outbound surface now handles both extremes
  correctly: a too-long message is chunked and delivered; a blank one is dropped
  silently instead of erroring.

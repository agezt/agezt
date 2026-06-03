# M235 — Chunk Discord slash-command follow-ups

## Why
M234 chunked the channels' primary `Send` paths but explicitly left Discord's
**follow-up** path unchanged — and flagged it as the same bug. `followUp`
delivers a slash-command's answer via the interaction webhook
(`/webhooks/{app}/{token}`) and posted the whole `content` in one request. A
`/command` answer over Discord's 2000-character limit was therefore rejected and
lost, exactly like the `Send` path before M234. Same class, same channel, the
other delivery flow.

## What
- **`plugins/channels/discord/discord.go`** — `followUp` now loops over
  `channel.SplitText(content, discordMaxChars)`, posting each chunk as a separate
  follow-up message (each POST to the follow-up webhook creates a new message).
  One `channel.outbound` event per logical answer (unchanged).

## Files
- `plugins/channels/discord/discord.go` — chunk `followUp` (edited).
- `plugins/channels/discord/discord_chunk_test.go` —
  `TestDiscord_FollowUpChunksLongAnswer` (new): a `/command` whose handler returns
  a 5000-char answer is delivered as ≥3 follow-up messages, each ≤2000 chars, that
  rejoin to the original. Drives the real interaction flow (signed POST → deferred
  ACK → async handler → follow-ups) against the httptest Discord API.

## Verification
- `go test ./plugins/channels/discord/` — green; full suite **1773 → 1774** (+1),
  66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/channels/discord/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- This closes the Discord delivery surface for over-long messages: both `Send`
  (M234) and `followUp` (M235) now chunk. Slack remains as-is (its limit is
  effectively never hit by agent replies).

# M240 — Chunk over-long Slack messages (completes the channel theme)

## Why
M234/M235 chunked Telegram and Discord (incl. follow-ups); M236 made empty sends
a no-op across all three. Slack's length case was deferred as "rarely hit" — but
its `chat.postMessage` text limit is **40000 characters**, and a large agent
answer (a file/data/log dump, a long analysis) can exceed it, at which point the
single oversize send is rejected and the answer is lost, exactly like Telegram/
Discord before M234. This closes the gap so all three channels behave the same.

## What
- **`plugins/channels/slack/slack.go`** — `send` now loops over
  `channel.SplitText(out.Text, slackMaxChars)` (40000). The single-message POST +
  Slack app-level `ok` check was factored into a `postMessage` helper so the loop
  stays readable; the empty-message no-op (M236) and the journal-once behaviour
  are unchanged.

## Files
- `plugins/channels/slack/slack.go` — chunked `send` + `postMessage` helper +
  `slackMaxChars` (edited).
- `plugins/channels/slack/slack_chunk_test.go` — `TestSlack_SendChunksOverLongMessage`
  (new): a 90000-char message goes out as ≥2 messages each ≤40000 chars that
  rejoin to the original, driven against the httptest Slack API stand-in.

## Verification
- `go test ./plugins/channels/slack/` — green; full suite **1781 → 1782** (+1),
  66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/channels/slack/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- The channel outbound surface is now uniform across Telegram, Discord, and
  Slack: too-long → chunked (M234/M235/M240), blank → no-op (M236).

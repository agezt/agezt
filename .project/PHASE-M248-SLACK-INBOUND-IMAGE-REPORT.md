# M248 — Inbound images from Slack reach a vision model

## Why
M247 wired Telegram inbound photos to a vision model. Slack — the other
event-based channel — had the same gap: its inbound `message` events carried
only text, so a user sharing an image in Slack got a text-only run and the
picture was discarded.

## What
Slack mirrors Telegram's inbound shape, and the download is simpler (a single
authenticated GET rather than getFile + download).

- **`plugins/channels/slack/slack.go`**
  - `slackEvent` gains `Files []slackFile`; new `slackFile{URLPrivate, Mimetype,
    Name}`.
  - `process` — for allowlisted senders only — downloads each file whose
    `mimetype` is `image/*` via the new `fetchFileDataURL` (GET `url_private`
    with `Authorization: Bearer <bot-token>`, capped at 12 MiB vs the 16 MiB
    control-plane request limit) and appends the resulting `data:` URL to
    `msg.Images`. Slack reports the mimetype directly, so it becomes the data
    URL's media type. Non-image files and files from non-allowlisted channels
    are never fetched.

The run-side threading (`UnifiedMessage.Images` → `kernelruntime.WithImages` in
`makeChannelHandler`) was added in M247 and is reused unchanged.

## Files
- `plugins/channels/slack/slack.go` — `slackFile` + `slackEvent.Files`; image
  fetch in `process`; `fetchFileDataURL` (edited).
- `plugins/channels/slack/inbound_image_test.go` — 2 tests: image file → data
  URL (asserts the bot-token auth header on the download), non-image file
  skipped (new).

## Verification
- `go test ./plugins/channels/slack/` — green; full suite **1813 → 1815** (+2),
  66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/channels/slack/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged (stdlib `encoding/base64`).

## Scope notes
- Fetch is allowlist-gated (no file download for an unauthorized channel),
  keeping the inbound injection surface closed.
- Multiple shared images are all forwarded (Slack can attach several files to
  one message).
- Discord inbound images (slash-command ATTACHMENT options, resolved via
  `data.resolved.attachments`) are the last inbound surface — a follow-up,
  larger because it also needs the command's attachment option registered.

# M249 — Inbound images from Discord reach a vision model (inbound arc complete)

## Why
M247 (Telegram) and M248 (Slack) wired inbound photos to a vision model. Discord
was the last inbound channel. Discord is slash-command based, so an image
arrives as an `ATTACHMENT`-type command option whose value is an attachment id
resolved through `data.resolved.attachments`. The channel parsed only STRING
options, so an attached image was ignored.

## What
- **`plugins/channels/discord/discord.go`**
  - `discordData` gains `Resolved *discordResolved`; new `discordResolved`
    (`Attachments map[string]discordAttachment`) and `discordAttachment`
    (`URL`, `ContentType`, `Filename`).
  - `optionTypeAttachment = 11`; new `discordInteraction.imageAttachments()`
    returns the resolved attachments of ATTACHMENT options whose `content_type`
    is `image/*`.
  - `runAndFollowUp` downloads each image attachment (a public CDN url, no auth)
    **after** the deferred ACK — so the download never risks Discord's 3-second
    interaction deadline — via the new `fetchAttachmentDataURL` (capped at 12 MiB
    vs the 16 MiB control-plane request cap) and appends the `data:` URLs to
    `msg.Images`.
  - The "nothing to do" guard in `handleCommand` now also proceeds when the
    command carries an image attachment but no prompt text (image-only command).

The run-side threading (`UnifiedMessage.Images` → `WithImages`, M247) is reused.

## Files
- `plugins/channels/discord/discord.go` — resolved-attachment types,
  `imageAttachments`, `fetchAttachmentDataURL`, fetch in `runAndFollowUp`,
  relaxed empty-command guard (edited).
- `plugins/channels/discord/inbound_image_test.go` — 2 tests: attachment image →
  data URL (full signed-interaction → follow-up flow with a CDN stand-in),
  `imageAttachments` filter (image vs non-image vs no-resolved vs string option)
  (new).

## Verification
- `go test ./plugins/channels/discord/` — green; full suite **1815 → 1817**
  (+2), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/channels/discord/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged (stdlib `encoding/base64`).

## Scope notes
- The download happens after the ACK (async), so — unlike Telegram/Slack — the
  journaled `channel.inbound` event records the message before images are
  attached. The run still receives them. Keeping the ACK within 3s is the
  Discord-specific constraint that drives this.
- The slash command must be **registered** with an attachment option for Discord
  to send one; that registration is the operator's (out of band). This change is
  the channel correctly handling the attachment when present — forward-compatible.
- **Inbound vision is now complete on Telegram, Slack, and Discord** — together
  with the M241–M246 send-side and OpenAI-API-input work, images flow end to end
  in both directions across every surface.

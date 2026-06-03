# M247 — Inbound photos from Telegram reach a vision model

## Why
M241–M246 made Agezt deliver images to every provider and accept them on the
OpenAI-compatible API. The remaining inbound surface was the **channels**: a
user who sends a photo to the Telegram bot got a text-only run — the inbound
path only ever carried `Text`, so the picture was discarded before it could
reach a vision model. This wires the first (and most-used) channel's inbound
photos through to the model.

## What
- **`kernel/channel/channel.go`** — `UnifiedMessage` gains `Images []string`
  (data: URLs), the canonical place inbound attachments ride to the run.
- **`plugins/channels/telegram/telegram.go`**
  - `tgMessage` gains `Caption` and `Photo []tgPhotoSize`; new `tgPhotoSize`.
  - `handleInbound` uses the caption as the text when the body is empty, and —
    for allowlisted senders only — fetches the largest photo size via the new
    `fetchPhotoDataURL` (getFile → `/file/bot<token>/` download → base64 →
    `data:` URL, capped at 12 MiB vs the 16 MiB control-plane request limit) and
    sets `msg.Images`. `tgMediaType` maps the file extension (Telegram photos are
    JPEG). A non-allowlisted sender's photo is never dereferenced.
- **`cmd/agezt/main.go`** — `makeChannelHandler` threads `msg.Images` into the
  run via `kernelruntime.WithImages` (the same mechanism as the control plane
  and the OpenAI API), and gives an uncaptioned photo a default
  `"Describe the attached image(s)."` intent.

## Files
- `kernel/channel/channel.go` — `UnifiedMessage.Images` (edited).
- `plugins/channels/telegram/telegram.go` — caption fallback, photo fetch,
  `fetchPhotoDataURL` + `tgMediaType` (edited).
- `cmd/agezt/main.go` — `makeChannelHandler` threads images + default intent
  (edited).
- `plugins/channels/telegram/inbound_image_test.go` — 3 tests: photo → data URL
  (caption as text), photo not fetched for a rejected sender, `tgMediaType`
  (new).

## Verification
- `go test ./plugins/channels/telegram/ ./kernel/channel/` — green; full suite
  **1810 → 1813** (+3), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all four files; `go vet` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged (stdlib `encoding/base64`, `io`, `path/filepath`).

## Scope notes
- Fetch is gated on the allowlist, so an unauthorized sender can't make the bot
  dereference a file id — keeping the inbound injection surface closed.
- The run path doesn't pre-gate vision capability (same as the API path); a
  non-vision configured model surfaces a provider error, returned to the user as
  the bot's failure reply.
- Discord (`attachments[].url`) and Slack (`files[].url_private`) inbound images
  are the obvious follow-ups; both use a URL the channel can fetch the same way.
- `UnifiedMessage.Images` is additive + omitempty, so other channels and the
  journaled inbound event are unaffected until they populate it.

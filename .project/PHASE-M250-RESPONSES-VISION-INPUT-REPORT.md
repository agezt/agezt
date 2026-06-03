# M250 — Accept image input on the Responses API

## Why
M246 wired image input on `/v1/chat/completions` (the `image_url` part shape).
The Responses API (`/v1/responses`) was explicitly left out: its image part is
`{"type":"input_image","image_url":"<url>"}` where `image_url` is a **bare
string**, distinct from Chat Completions' `{"type":"image_url","image_url":{"url"}}`
object. So a multimodal Responses request still ran text-only. This closes the
last vision-input gap, so both OpenAI-compatible endpoints accept images.

## What
- **`kernel/openaiapi/openaiapi.go`** — new `chatMessage.inputImages()` extracts
  `input_image` part URLs, tolerating both the bare-string form (documented) and
  the `{url}` object form (some SDKs send it).
- **`kernel/openaiapi/responses.go`**
  - Factored the input → `[]chatMessage` build into a shared `responsesMessages`
    helper (used by both intent and image extraction).
  - New `imagesFromResponsesInput` collects `input_image` URLs from the user
    items.
  - `handleResponses` forwards them to `RunModel` / `streamResponses`. An
    image-only input (no `input_text`) now runs with a default
    `"Describe the attached image(s)."` intent instead of being rejected; an
    input with neither text nor image is still a 400.
  - `streamResponses` gained an `images` parameter.

The run-side threading (`kernelAPIEngine.RunModel` → `WithImages`) is the M246
path, reused unchanged.

## Files
- `kernel/openaiapi/openaiapi.go` — `inputImages` method (edited).
- `kernel/openaiapi/responses.go` — `responsesMessages` + `imagesFromResponsesInput`;
  handler + streamResponses forwarding (edited).
- `kernel/openaiapi/vision_input_test.go` — 3 tests: Responses forwards
  input_image, image-only Responses input runs with a default intent,
  `inputImages` tolerates both string and object forms (new).

## Verification
- `go test ./kernel/openaiapi/` — green; full suite **1817 → 1820** (+3), 66
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/openaiapi/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- Vision input is now complete on both OpenAI-compatible surfaces (Chat
  Completions M246, Responses M250) and all three inbound channels (M247–M249),
  matching the send-side provider emission (M241–M245).
- Remaining vision nits: the API/channel run paths still don't pre-gate vision
  capability (a non-vision model + image surfaces a provider error rather than a
  clean pre-check) — a possible shared-gate follow-up.

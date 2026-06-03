# M246 — Accept image input on Agezt's OpenAI-compatible endpoint

## Why
M241–M245 made Agezt *send* images to every provider. The mirror gap was on the
*receiving* side: Agezt's own OpenAI-compatible HTTP surface
(`kernel/openaiapi`, `/v1/chat/completions`) flattened each message's content to
text and, by its own comment, *"Non-text parts (images) are ignored."* So a
client using Agezt as an OpenAI-compatible gateway and sending a standard vision
request (`content: [{type:"image_url", image_url:{url}}]`) got a text-only run —
the image was silently discarded before it ever reached a provider. This
completes the round trip: clients can send images, and Agezt forwards them to a
vision model.

## What
- **`kernel/openaiapi/openaiapi.go`**
  - `Engine.RunModel` gains an `images []string` parameter — the attachment URLs
    parsed from a multimodal request.
  - New `chatMessage.images()` extracts `image_url` part URLs (data: or http(s));
    new `imagesFromMessages` collects them from the user messages.
  - `handleChat` forwards the images to `RunModel` / `streamChat`. An image-only
    message (no text) now runs with a default `"Describe the attached image(s)."`
    intent instead of being rejected; a message with neither text nor image is
    still a 400.
- **`cmd/agezt/main.go`** — `kernelAPIEngine.RunModel` threads the images into
  the run via `kernelruntime.WithImages(ctx, images)` — the exact mechanism the
  control plane uses (server.go) — so the agent loop attaches them to the user
  message and the (M241–M245) providers emit them natively.
- **`kernel/restapi/restapi.go`** + **`kernel/openaiapi/responses.go`** — the
  shared `Engine.RunModel` signature is updated; these paths pass `nil` (REST
  has no image-input concept; Responses-API `input_image` parts are a different
  shape, deferred).

## Files
- `kernel/openaiapi/openaiapi.go` — Engine sig; `images()`/`imagesFromMessages`;
  handleChat + streamChat forwarding (edited).
- `kernel/openaiapi/responses.go` — pass nil images (edited).
- `kernel/restapi/restapi.go` — Engine sig + nil at call sites (edited).
- `cmd/agezt/main.go` — `kernelAPIEngine.RunModel` threads `WithImages` (edited).
- `kernel/openaiapi/openaiapi_test.go`, `kernel/restapi/restapi_test.go`,
  `plugins/tools/peer/live_test.go` — fake engines updated to the new signature;
  the openaiapi fake records `ranImages` (edited).
- `kernel/openaiapi/vision_input_test.go` — 4 tests: image_url forwarded,
  image-only message runs with a default intent, empty content rejected,
  `imagesFromMessages` (user-only) (new).

## Verification
- `go test ./kernel/openaiapi/ ./kernel/restapi/ ./plugins/tools/peer/` — green;
  full suite **1806 → 1810** (+4), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all eight files; `go vet` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- The vision-capability gate lives in the control-plane CmdRun handler, not in
  `RunWith`, so this API path does not pre-gate — a non-vision model that
  receives an image surfaces a provider error (`upstream_error`), which is the
  honest outcome for a client mistake. Adding a shared pre-gate is a possible
  follow-up.
- Responses-API image input (`input_image` parts, where `image_url` is a bare
  string) is a different wire shape and remains a follow-up; that path passes no
  images today.

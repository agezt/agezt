# M245 — Vision delivery on Vertex AI (completes every first-party provider)

## Why
M241–M244 made `agt run --image` reach the model on Anthropic, OpenAI, Gemini,
and Bedrock. Vertex AI was the last first-party surface, and it has **two**
dialects, each with its own encoder that dropped image attachments:
- Anthropic-on-Vertex (`encodeAnthropicOnVertexRequest` / `canonicalToAnthVx`),
- Gemini-on-Vertex (`encodeRequest` / `canonicalToVertex`).

## What
Both encoders get the data:-URL → native-image transformation; one shared
`parseImageDataURL` helper serves both (same `package vertex`).

- **`plugins/providers/vertex/anthropic.go`** — `anthVxBlock` gains
  `Source *anthVxImageSource`; new `anthVxImageSource` type; `canonicalToAnthVx`
  RoleUser emits a `type=image` base64 block before the text block. Added the
  shared `parseImageDataURL`.
- **`plugins/providers/vertex/vertex.go`** — `vxPart` gains
  `InlineData *vxInlineData`; new `vxInlineData` type; `canonicalToVertex`
  RoleUser emits an `inlineData` part before the text part.
- Both Vertex streaming paths reuse these encoders, so streaming and
  non-streaming both benefit.

## Files
- `plugins/providers/vertex/anthropic.go` — image block + shared helper (edited).
- `plugins/providers/vertex/vertex.go` — inlineData part (edited).
- `plugins/providers/vertex/vision_test.go` — 4 tests: Anthropic-on-Vertex image
  block, Gemini-on-Vertex inlineData part, non-data-URL skipped on both,
  `parseImageDataURL` (new, `package vertex` internal).

## Verification
- `go test ./plugins/providers/vertex/` — green; full suite **1802 → 1806**
  (+4), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all three files; `go vet` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- **Vision is now end-to-end on every built-in provider**: Anthropic (M241),
  OpenAI (M242), Gemini (M243), Bedrock/Claude (M244), Vertex/both dialects
  (M245). OpenAI-compatible `compat` vendors inherit via the OpenAI encoder.
- The CLI delivery (M241: read bytes → `data:` URL, vision-gate, 12 MiB cap) is
  unchanged and shared by all of them — this milestone is purely the provider
  emission side.

# M309 — Ollama vision: local multimodal models can see images

## Why
A provider-capability-parity gap surfaced by surveying the matrix: every cloud
provider (Anthropic, OpenAI, Google, Vertex, Bedrock) supports image input, but
**Ollama** — the local/self-hosted provider — did not, even though Ollama serves
the popular *local* vision models (llava, llama3.2-vision, moondream, bakllava,
minicpm-v). The privacy-friendly "run a vision model entirely on your own
hardware" path was the one that didn't work. (Cohere has no vision models, so its
absence is correct, not a gap.)

Two independent breaks, both fixed here:
1. **The provider dropped the image.** `agent.Message` carries image attachments
   (`Images []string`, RFC 2397 data: URLs — M93/M241), but the Ollama encoder
   built `{role, content}` and never forwarded them. Ollama's chat API takes raw
   base64 in an `images` array.
2. **The gate blocked the image.** The M91 vision gate only sends attachments to
   a model whose catalog entry reports image input, but Ollama auto-discovery
   marked *every* model text-only — so a discovered llava was refused upstream,
   before the provider was even reached.

## What
- **`plugins/providers/ollama/ollama.go`**: `ollamaMessage` gained an
  `Images []string` field; the user-role encoder extracts the raw base64 payload
  from each data: URL (new `ollamaImageData` helper — strips the
  `data:<media-type>;base64,` envelope Ollama doesn't want) and forwards them.
  Entries Ollama can't use (an http URL it won't fetch, a bare filename) are
  skipped. `omitempty` keeps text-only requests byte-for-byte unchanged.
- **`kernel/catalog/discovery.go`**: `DiscoverOllama` now detects vision models —
  a vision-projector family in `details.families` (`clip` for llava/bakllava,
  `mllama` for llama3.2-vision) or a recognisable model id (new
  `ollamaModelHasVision` helper) — and marks them `input: [text, image]` +
  `Attachment: true`, so the M91 gate lets attachments through to the now
  image-forwarding provider. Text models stay text-only.

## Verification
- **`plugins/providers/ollama/vision_test.go`**: a user message with a data: URL
  plus an http URL and a bare filename → only the data: URL survives, as raw
  base64 (prefix stripped), in the `images` array; a text-only message omits the
  field entirely.
- **`kernel/catalog/catalog_test.go`**: `TestDiscoverOllama_DetectsVisionModels`
  — llava (clip family), llama3.2-vision (mllama), moondream (id marker) all come
  back `SupportsVision() == true` + `Attachment == true`; a plain llama3.2 stays
  text-only.
- Full suite **1970** passing, `go test ./...` exit 0; `gofmt -l` clean on every
  changed file; `go vet` clean; `go.mod` / `go.sum` unchanged. Network-free
  (httptest / direct encode) — the established proof for a provider-wire feature.

## Scope notes
- Additive: non-vision Ollama runs are unchanged (the field is omitted; the
  discovery flag only flips for recognised vision models).
- The vision detection is heuristic — Ollama's `/api/tags` exposes no explicit
  modality field — but covers the common local vision families; an operator can
  always pin capabilities in `custom.json` for anything missed.
- Closes provider vision parity: every multimodal-capable provider family now
  forwards images (cloud + local). Cohere remains correctly text-only.

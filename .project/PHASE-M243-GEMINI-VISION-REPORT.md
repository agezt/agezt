# M243 — Vision delivery for the Gemini provider (completes the mainstream set)

## Why
M241 (Anthropic) and M242 (OpenAI) made `agt run --image` actually reach the
model via a provider-agnostic `data:` URL carrier. The third mainstream
provider, **Gemini** (`plugins/providers/google`), still dropped the image:
`canonicalToGemini` emitted only a text part. This closes that gap, so vision
works across all three first-party providers.

## What
Gemini's `generateContent` already uses a `parts` array per content, so adding
images is natural — its image part is `inlineData: {mimeType, data}` (base64).

- **`plugins/providers/google/google.go`**
  - `geminiPart` gains an `InlineData *geminiInlineData` field; new
    `geminiInlineData{MimeType, Data}` type.
  - `canonicalToGemini` RoleUser parses each `data:` URL (new local
    `parseImageDataURL` helper) into an `inlineData` part, placed before the
    text part. A non-data-URL entry (e.g. a legacy bare filename) is skipped.
  - The streaming path shares `encodeRequest` → `canonicalToGemini`, so both
    request paths benefit.

## Files
- `plugins/providers/google/google.go` — `geminiInlineData` + `geminiPart`
  field; `parseImageDataURL`; `canonicalToGemini` RoleUser inlineData parts
  (edited).
- `plugins/providers/google/vision_test.go` — 3 tests: inlineData image part,
  non-data-URL skipped, `parseImageDataURL` good + malformed cases (new).

## Verification
- `go test ./plugins/providers/google/` — green; full suite **1796 → 1799**
  (+3), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/providers/google/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- **Mainstream vision set complete**: Anthropic (M241), OpenAI (M242), Gemini
  (M243). The OpenAI-compatible `compat` family (Groq, xAI, Cerebras, DeepSeek,
  Moonshot, …) wraps `openai.New`, so it inherits M242's image emission for
  free; Anthropic-via-compat inherits M241.
- Remaining providers without image emission are Bedrock and Vertex (each wraps
  a vendor-specific wire format); those are lower-traffic follow-ups if needed.

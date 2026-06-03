# M242 — Vision delivery for the OpenAI provider

## Why
M241 made `agt run --image` carry the real image bytes to the daemon as a
self-describing `data:` URL and taught the **Anthropic** provider to emit it as
an image content block. The CLI delivery is provider-agnostic, so the same
attachment reaching the **OpenAI** provider was still dropped: `canonicalToOA`
emitted a text-only string, so an OpenAI (or OpenAI-compatible) vision model
never saw the picture. This extends the fix to OpenAI.

## What
OpenAI's Chat Completions `content` field is polymorphic: a plain string for
text-only messages, or an array of typed parts
(`{type:"text",...}` / `{type:"image_url",image_url:{url}}`) for multimodal
input. OpenAI accepts a `data:` URL directly as the `image_url.url`, so the M241
carrier needs no re-encoding.

- **`plugins/providers/openai/openai.go`**
  - `oaMessage.Content` retyped `string` → `any` so one field marshals to either
    the string or the parts-array form. Added `oaContentPart` + `oaImageURL`
    types.
  - Helpers `oaTextOrNil` (returns nil for empty text, preserving the pre-M242
    `omitempty` wire shape — a tool-call-only assistant message must still omit
    `content`) and `oaContentText` (extracts the string form when decoding a
    response, which is always a string). `isImageURL` accepts `data:` / `http(s)`
    and rejects bare filenames.
  - `canonicalToOA` RoleUser: when the message carries deliverable image URLs,
    build the content-parts array (text part first, then one `image_url` part
    per URL); otherwise keep the plain-string form. Assistant/tool content now
    go through `oaTextOrNil`; the response decode reads `oaContentText`.
  - The streaming encoder shares `canonicalToOA`, so streaming and non-streaming
    both benefit.

## Files
- `plugins/providers/openai/openai.go` — polymorphic content + parts types +
  helpers + RoleUser image parts (edited).
- `plugins/providers/openai/vision_test.go` — 5 tests: image content parts,
  text-only stays a string, non-URL image skipped, tool-call-only assistant
  still omits content (omitempty regression guard), `isImageURL` (new).

## Verification
- `go test ./plugins/providers/openai/` — green; full suite **1791 → 1796**
  (+5), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/providers/openai/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- No wire drift on the text path: empty content is still omitted; non-empty is
  still a JSON string. The only new output is the parts array for a user message
  that carries images — which previously dropped them.
- Anthropic (M241) + OpenAI (M242) now both deliver vision. Gemini is the
  remaining mainstream provider; the OpenAI-compatible `compat` family inherits
  this if it routes through this encoder (to confirm in a follow-up).

# M244 — Vision delivery for Claude-on-Bedrock

## Why
M241–M243 made `agt run --image` reach the model on the direct Anthropic,
OpenAI, and Gemini providers. AWS Bedrock has its **own** copy of the Anthropic
Messages-API encoder (`encodeAnthropicOnBedrockRequest` + a package-local
`anthMessage`/`anthBlock`/`canonicalToAnth`), separate from the `anthropic`
package — and it had the same defect: the user block was text-only, so an image
attached to a Claude-on-Bedrock run never reached the model. Claude on Bedrock
is, per the code's own comment, the largest Bedrock use case, so this is a real
production gap.

## What
The identical M241 transformation, applied to the Bedrock copy:

- **`plugins/providers/bedrock/bedrock.go`**
  - `anthBlock` gains a `Source *anthImageSource` field; new `anthImageSource`
    type (`type=base64`, `media_type`, `data`).
  - `canonicalToAnth` RoleUser parses each `data:` URL (new local
    `parseImageDataURL` helper) into a `type=image` block placed before the text
    block. Non-data-URL entries are skipped.
  - The streaming path (`CompleteStream`) shares
    `encodeAnthropicOnBedrockRequest` → `canonicalToAnth`, so both request paths
    benefit.

## Files
- `plugins/providers/bedrock/bedrock.go` — `anthImageSource` + `anthBlock.Source`;
  `parseImageDataURL`; `canonicalToAnth` RoleUser image blocks (edited).
- `plugins/providers/bedrock/vision_test.go` — 3 tests: image block, non-data-URL
  skipped, `parseImageDataURL` good + malformed cases (new, `package bedrock`
  internal so it can reach the unexported encoder).

## Verification
- `go test ./plugins/providers/bedrock/` — green; full suite **1799 → 1802**
  (+3), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/providers/bedrock/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- Only the Anthropic-on-Bedrock body carries images; the other Bedrock model
  families (Mistral, Llama, Cohere, AI21) use text-only chat shapes without a
  standard image part and are not vision models in this catalog, so they are
  untouched (and the M91 vision gate would reject an image against them anyway).
- Remaining: Vertex (Anthropic-on-Vertex + Gemini-on-Vertex) — the last
  first-party surface, a follow-up milestone.

# M302 — Cache the Bedrock + Vertex Claude system prompt

## Why
M301 added system-prompt caching to the direct Anthropic provider (a second
breakpoint so tools AND system are cached). Claude-on-Bedrock and Claude-on-Vertex
still sent `system` as a bare string (tools-only caching from M300). This extends
the optimal tools+system caching to the rest of the Claude family, completing the
cache request side everywhere Agezt runs Claude.

## What
The same pattern as M301, per provider:

- **`plugins/providers/bedrock/bedrock.go`**: `anthBedrockRequest.System`
  `string → any`; new `anthSystemBlock` + `buildBedrockSystem(system)` (nil when
  empty, else a one-element cache-marked block array); the encoder uses it.
- **`plugins/providers/vertex/anthropic.go`**: `anthVertexRequest.System`
  `string → any`; new `anthVxSystemBlock` + `buildVxSystem(system)`; the encoder
  uses it.

Each provider's single encoder serves both streaming and non-streaming, so one
change per provider covers both. Result: two cache breakpoints (tools + system) on
every Claude path.

## Files
- `plugins/providers/bedrock/bedrock.go`,
  `plugins/providers/vertex/anthropic.go` (edited).
- `plugins/providers/bedrock/bedrock_test.go`,
  `plugins/providers/vertex/anthropic_test.go`: the existing Anthropic-on-Bedrock /
  on-Vertex Complete tests now assert `system` is a one-element block array
  carrying `cache_control: ephemeral` (was a bare-string assertion).

## Verification
- Full suite **1913**, 68 packages, `go test ./...` exit 0; `go vet` clean;
  `gofmt -l` clean on the touched files; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged.
- **Network-free proof**: the encode tests assert the system prompt is emitted as
  a cache-marked block array on both the Bedrock and Vertex Claude paths. (The
  actual cache hit needs a live backend; the response-side accounting records it —
  M296 / M290 — and surfaces via `agt cache` / the Cache panel.)

## Scope notes
- Completes the request-side Anthropic prompt caching arc: tools (M299 direct /
  M300 bedrock+vertex) + system (M301 direct / M302 bedrock+vertex). Every Claude
  path now caches its whole stable request prefix (tools AND system).
- The full cache/cost arc is now end-to-end and optimal on every supported
  caching provider: request the cache (M299-M302) → provider caches → response
  accounting (M289 openai/compat, M290 anthropic, M295 gemini, M296 bedrock) →
  cache-aware billing (M289-M291) → visibility (`agt cache` M294, Cache panel M293,
  Budget panel M292).

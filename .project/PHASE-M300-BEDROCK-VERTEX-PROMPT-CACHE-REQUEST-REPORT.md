# M300 — Request prompt caching on Bedrock + Vertex Claude

## Why
M299 made the direct Anthropic provider request prompt caching (mark the last
tool with `cache_control`). Claude-on-Bedrock and Claude-on-Vertex had the **same
gap** — their encoders never set a cache breakpoint either, so the M296/M290
response-side accounting always saw zero on those paths. This extends the M299 fix
to the rest of the Claude family, so the cache savings are real everywhere Agezt
runs Claude.

## What
The same pattern as M299, per provider (each has its own tool struct + single
encoder used by streaming and non-streaming):

- **`plugins/providers/bedrock/bedrock.go`**: `anthTool` gains `cache_control`;
  `anthCacheControl` + `buildBedrockTools(tools)` mark the last tool;
  `encodeAnthropicOnBedrockRequest` uses it.
- **`plugins/providers/vertex/anthropic.go`**: `anthVxTool` gains `cache_control`;
  `anthVxCacheControl` + `buildVxTools(tools)`;
  `encodeAnthropicOnVertexRequest` uses it.

Both `encodeAnthropicOn{Bedrock,Vertex}Request` are the single encoder for both
the streaming and non-streaming paths, so one change covers both per provider.

## Files
- `plugins/providers/bedrock/bedrock.go`,
  `plugins/providers/vertex/anthropic.go` (edited).
- `plugins/providers/bedrock/bedrock_test.go`: **new**
  `TestEncode_PromptCacheMarksLastTool`.
- `plugins/providers/vertex/anthropic_test.go`: the Anthropic-on-Vertex Complete
  test now sends two tools and asserts the last carries
  `cache_control: ephemeral`.

## Verification
- Full suite **1912**, 68 packages, `go test ./...` exit 0; `go vet` clean on the
  touched packages; `gofmt -l` clean; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged.
- **Network-free proof**: the httptest tests drive `Complete` with two tools and
  assert only the LAST tool carries `cache_control: {type: ephemeral}` in the
  captured request body, for both Bedrock and Vertex. (The actual cache hit +
  savings need a real backend; the response-side accounting that records them
  exists — M296/M290 — and surfaces via `agt cache` / the Cache panel.)

## Scope notes
- Completes request-side Anthropic prompt caching across direct (M299) + Bedrock +
  Vertex (M300). The cache arc is now end-to-end on every Claude path: request the
  cache (M299/M300) → provider caches → response accounting (M290/M296) →
  cache-aware billing (M289-291) → visibility (`agt cache` M294 / Cache panel M293).
- Still caches only the tools prefix (the always-present, large, stable part).
  Caching the system prompt too (a second breakpoint) would need the `system`
  field changed from a string to a block array — deferred.
- Gemini (direct + Vertex) uses explicit context caching (a separate API), not
  request markers; M295 already reads its `cachedContentTokenCount` when present.

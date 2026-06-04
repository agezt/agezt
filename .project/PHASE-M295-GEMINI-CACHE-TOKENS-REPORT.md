# M295 — Gemini cache-token accounting

## Why
The cache cost model (M289–M291) covered OpenAI/compat and Anthropic. Google is
the third major provider family; Gemini context caching reports a
`cachedContentTokenCount` in `usageMetadata`. Wiring it extends cache-aware
billing to Google (direct + Vertex).

## What
Gemini's `promptTokenCount` already *includes* the cached subset (like OpenAI),
so the canonical mapping is `InputTokens = promptTokenCount`,
`CachedInputTokens = cachedContentTokenCount` — fresh = input − cached bills at
the input rate, cached at the cache-read rate (M289).

- **`plugins/providers/google/google.go`**: `geminiUsageMetadata` gains
  `cachedContentTokenCount`; the non-streaming decode sets
  `Usage.CachedInputTokens`.
- **`plugins/providers/google/streaming.go`**: `streamState.cachedTokens`,
  captured from the terminal chunk's `usageMetadata`, threaded into the assembled
  `Usage`.
- **`plugins/providers/vertex/vertex.go` + `streaming.go`** (Gemini-on-Vertex):
  the same, on `vxUsageMetadata`.

## Files
- `plugins/providers/google/google.go`, `plugins/providers/google/streaming.go`,
  `plugins/providers/vertex/vertex.go`, `plugins/providers/vertex/streaming.go`
  (edited).
- `plugins/providers/google/google_test.go`: **new** `TestComplete_CacheUsage`
  (promptTokenCount 1000 incl. cachedContentTokenCount 800 → Input 1000, Cached
  800).
- `plugins/providers/vertex/vertex_test.go`: `TestComplete_HappyPathWithCachedToken`
  extended to carry `cachedContentTokenCount` and assert `CachedInputTokens`.

## Verification
- Full suite **1906**, 68 packages, `go test ./...` exit 0; `go vet` clean on the
  touched packages; `gofmt -l` clean on the touched files (the `vertex/auth.go`
  flag is a pre-existing CRLF artifact, untouched); `GOOS=linux` build clean;
  `go.mod` / `go.sum` unchanged.
- **Network-free proof**: the httptest decode tests above assert
  `cachedContentTokenCount` maps to `Usage.CachedInputTokens` for both Google
  code paths (direct + Vertex).
- **Billing effect proven in M289**: the governor bills `CachedInputTokens` at the
  cache-read rate; M295 makes Gemini emit it.

## Scope notes
- Reuses the M289 cost model unchanged — no kernel/governor edits.
- Gemini exposes no separate cache-*write* count in `usageMetadata`, so
  `CacheWriteInputTokens` stays 0 for Gemini (cache creation, if any, bills at the
  input rate — conservative).
- Cache-token parsing now covers OpenAI/compat (M289), Anthropic direct/vertex
  (M290), and Gemini direct/vertex (M295). Remaining: Bedrock, Cohere, Mistral
  (lower traffic / weaker caching) — each threads its own field into the same
  `agent.Usage` cache fields when wanted.

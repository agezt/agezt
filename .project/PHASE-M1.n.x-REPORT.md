# Phase Report — Milestone 1.n.x (Vertex Anthropic body + streaming)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §3 (Vertex)](SPEC-15-PROVIDER-ECOSYSTEM.md) and the
> M1.n package-level deferral comment.
> Continues [PHASE-M1.t-REPORT.md](PHASE-M1.t-REPORT.md).

## Scope

M1.n shipped Vertex with **Gemini-only** body support — the
package-level docstring explicitly says:

> **Scope (M1.n):** service-account OAuth + Gemini generateContent
> body shape on the regional aiplatform.googleapis.com endpoint.
> Anthropic-on-Vertex (`@ai-sdk/google-vertex/anthropic`, which
> uses the `:rawPredict` endpoint with the Anthropic Messages
> body) and streaming land in M1.n.x.

M1.q.x.x.x then shipped Gemini streaming over Vertex, but `claude-*`
model ids on Vertex would still fail with the wrong body shape
(`contents`/`parts` instead of `messages`) hitting the wrong
publisher path. M1.n.x closes both halves: Anthropic-on-Vertex
non-streaming AND streaming.

The streaming half is "free" architecturally — Vertex's
`:streamRawPredict` returns **standard event-tagged Anthropic SSE**
(not the binary event-stream framing Bedrock uses), so the
dispatcher is the same shape as the direct-Anthropic adapter, just
duplicated into the vertex package per the same isolation rule
that governs the other body-shape duplications.

| Concern | Status |
|---|---|
| `claude-*` model id detection (`isAnthropicModel`) | ✅ tested |
| `:rawPredict` endpoint routing (publishers/anthropic) | ✅ tested |
| `:streamRawPredict` endpoint routing | ✅ tested |
| Anthropic Messages body shape (no `model` field; `anthropic_version: vertex-2023-10-16`) | ✅ tested |
| Bearer-token auth shared with Gemini path (same TokenSource) | ✅ |
| Tool-call round trip (request encode + response decode) | ✅ tested |
| Streaming text deltas → assembled `CompletionResponse` | ✅ tested |
| Streaming tool_use lifecycle (Start → input_json_delta → Stop) | ✅ tested |
| Non-2xx → `*vertex.APIError` (consistent with Gemini path) | ✅ tested |
| **Gemini regression guard** (non-streaming) | ✅ tested |
| **Gemini regression guard** (streaming) | ✅ tested |

## Changes

### 1. `plugins/providers/vertex/anthropic.go` — new file (~470 LoC)

Five sections, top to bottom:

**A. Detection + endpoint resolution.** `isAnthropicModel(id)` matches
`strings.HasPrefix(strings.ToLower(id), "claude-")`. Vertex Anthropic
model ids look like `claude-opus-4-7@20251031` — the `@<date>` suffix
is Google's publisher-revision pin. `ResolveAnthropicEndpoint` and
`ResolveAnthropicStreamEndpoint` build the per-model URLs (mirror
of the Gemini-side `ResolveEndpoint`/`ResolveStreamEndpoint`).

**B. Wire types.** `anthVertexRequest` carries
`AnthropicVersion="vertex-2023-10-16"`, `MaxTokens`, `System`,
`Messages`, `Tools`, and a `Stream bool`. The `model` field is
**omitted** — Vertex routes by URL, not body. `anthVxMessage` /
`anthVxBlock` / `anthVxResponse` mirror Anthropic's standard
Messages-API shape.

**C. Translation.** `encodeAnthropicOnVertexRequest`,
`canonicalToAnthVx`, `decodeAnthropicOnVertexResponse`. Same
shape as the bedrock package's `encodeAnthropicOnBedrockRequest` /
`canonicalToAnth` / `decodeAnthropicOnBedrockResponse`. The
duplication is intentional — keeps each adapter independent of
the others' evolution.

**D. HTTP execution.** `completeAnthropic` (non-streaming) and
`completeStreamAnthropic` (streaming). Reuse the existing
`TokenSource.Token` and `APIError` from `vertex.go`.

**E. SSE dispatch.** `parseAnthropicSSE` + `dispatchAnthropicSSE` +
`assembleAnthropicResponse`. Event-tagged SSE: pair each `event:`
line with the following `data:` line, dispatch on the event name.
Identical control flow to `plugins/providers/anthropic/streaming.go`
and the inner-event handler in `plugins/providers/bedrock/streaming.go`.
Three implementations, three packages, intentionally.

### 2. `plugins/providers/vertex/vertex.go` — branch in `Complete`

```go
model := req.Model
if model == "" { model = p.Model }
if model == "" { model = DefaultModel }

// Anthropic-on-Vertex (`claude-*` model ids) speaks a different
// publisher (anthropic), endpoint suffix (:rawPredict), and body
// shape (Anthropic Messages API) than native Gemini. M1.n.x.
if isAnthropicModel(model) {
    return p.completeAnthropic(ctx, req, model)
}

body, err := encodeRequest(...)  // ← unchanged Gemini path
```

Branch sits *after* the model resolution + tokensource/project/
location validation, so the Anthropic path reuses the same
"missing project/location → clear error" surface. Branch sits
*before* `encodeRequest` so the Gemini body encoder is never
called on a `claude-*` id.

### 3. `plugins/providers/vertex/streaming.go` — symmetric branch in `CompleteStream`

```go
if isAnthropicModel(model) {
    return p.completeStreamAnthropic(ctx, req, model, onChunk)
}
```

Same placement rule as the non-streaming side.

### 4. `plugins/providers/vertex/anthropic_test.go` — new file (9 tests)

| Test | Coverage |
|---|---|
| `TestResolveAnthropicEndpoint_RoutesToAnthropicPublisher` | URL shape: `publishers/anthropic/...:rawPredict` |
| `TestResolveAnthropicStreamEndpoint_RoutesToStreamRawPredict` | URL shape: `publishers/anthropic/...:streamRawPredict` |
| `TestComplete_AnthropicModelRoutesToRawPredict` | End-to-end POST: model id → URL routes through anthropic publisher, body has `anthropic_version` + no `model` field, response decoded |
| `TestComplete_AnthropicToolCallRoundTrip` | Response with text + tool_use block → both surfaced; `stop_reason: tool_use` → `StopToolUse` |
| `TestComplete_GeminiModelStillRoutesToGenerateContent` | **Regression guard**: `gemini-*` ids still hit `:generateContent`, body has `contents` (not `messages`) |
| `TestCompleteStream_AnthropicAssemblesText` | Streaming text path: deltas → callbacks, assembled message + usage |
| `TestCompleteStream_AnthropicToolUse` | Streaming tool_use lifecycle: Start → 2× input_json_delta → Stop → finished call with full input |
| `TestCompleteStream_GeminiModelStillUsesGeminiStreamPath` | **Regression guard**: `gemini-*` ids still hit `:streamGenerateContent`, route through `publishers/google/` |
| `TestCompleteStream_AnthropicSurfacesAPIError` | Non-2xx → `*vertex.APIError` (same type Gemini path returns) |

The two regression guards exist because the new branch is exactly
the kind of split that's easy to refactor wrong — if someone
inverts the conditional or moves the branch above project
validation, the Gemini path quietly breaks. Test-locked behaviour
beats vigilance.

## Test summary

```
go test ./plugins/providers/vertex/ -v -count=1
ok  	github.com/ersinkoc/agezt/plugins/providers/vertex	0.833s

go test ./... -count=1
(all packages PASS)
```

Total suite: **450 passing** (from 441 after M1.t). +9 from the
new Anthropic-on-Vertex tests.

## Wire shape comparison

| Field | Direct Anthropic | Bedrock-Anthropic | Vertex-Anthropic |
|---|---|---|---|
| Endpoint | `api.anthropic.com/v1/messages` | `/model/{id}/invoke` | `/publishers/anthropic/models/{id}:rawPredict` |
| Stream endpoint | same + `stream: true` in body | `/model/{id}/invoke-with-response-stream` | `/publishers/anthropic/models/{id}:streamRawPredict` |
| Stream wire | event-tagged SSE | binary event-stream framing | event-tagged SSE |
| Auth header | `x-api-key` | `Authorization: Bearer <bearer-token>` | `Authorization: Bearer <oauth-token>` |
| `model` in body | yes | **no** (in URL) | **no** (in URL) |
| version field | `anthropic-version: 2026-05-29` (header) | `anthropic_version: bedrock-2023-05-31` (body) | `anthropic_version: vertex-2023-10-16` (body) |

The three adapters are *almost* the same code three times, but
each has a single load-bearing difference (the wire-framing for
Bedrock; the auth source for Vertex). The package-level
isolation rule keeps each one's idiosyncrasy contained.

## What's intentionally NOT in M1.n.x

- **Vertex AI Studio / Express Mode.** Different auth (`x-goog-api-key`
  header instead of OAuth bearer). Out of scope; operators on Express
  Mode would set up a separate catalog entry.
- **Anthropic prompt caching headers on Vertex.** The `cache_control`
  block field is supported by direct Anthropic and Bedrock but isn't
  surfaced in the catalog's `agent.CompletionRequest` schema yet.
  Lands when we add a `CacheControl` field at the agent level —
  not before, to avoid wiring it just here.
- **Anthropic count-tokens endpoint** (`:countTokens` analog).
  No caller; defer.

## Streaming family coverage (after M1.n.x)

| Family | Streaming impl | Wire format |
|---|---|---|
| Anthropic | M1.q | SSE (event-tagged) |
| OpenAI + ~11 compatible vendors | M1.q.x | SSE (untagged + `[DONE]`) |
| Google (Gemini direct) | M1.q.x.x | SSE (untagged + body-close) |
| Google Vertex (Gemini) | M1.q.x.x.x | SSE (untagged + body-close) |
| **Google Vertex (Anthropic)** | **M1.n.x** | **SSE (event-tagged)** |
| Ollama | M1.q.x.x.x.x | NDJSON |
| Cohere | M1.q.x.x.x.x | SSE (v2 typed events) |
| AWS Bedrock (Anthropic) | M1.t | event-stream binary framing |
| Mistral | (OpenAI compat) | SSE |
| Azure OpenAI | (OpenAI compat) | SSE |

**Every** catalog family + body shape combination now has a
working streaming path. The catalog M1.n recognises is fully
covered.

## Files touched

- [plugins/providers/vertex/anthropic.go](../plugins/providers/vertex/anthropic.go) — new (~470 LoC).
- [plugins/providers/vertex/anthropic_test.go](../plugins/providers/vertex/anthropic_test.go) — new (~340 LoC, 9 tests).
- [plugins/providers/vertex/vertex.go](../plugins/providers/vertex/vertex.go) — 5-line branch added at the top of `Complete`.
- [plugins/providers/vertex/streaming.go](../plugins/providers/vertex/streaming.go) — 5-line branch added at the top of `CompleteStream`.

Zero changes to the compat layer — Vertex's existing `wrapNamed`
path passes Anthropic and Gemini model ids alike; the per-id
branching now lives inside the vertex package, which is the right
place for a per-publisher wire decision.

## Deferrals after M1.n.x

The catalog wire coverage is **complete**. Remaining work:

- **Pulse v1** — operator-facing observability surface (real-time
  bus subscriber that renders events as a TUI). Next pickup.
- **Planner** — scheduler integration; multi-step plan execution.
- **Bedrock SigV4** + **non-Anthropic body shapes** (M1.m.x).
- **OS-keychain vault encryption.**
- **Browser tool**, **out-of-process plugin host.**

None block the agent loop. Picking up **Pulse v1** next per the
"operator can see what's happening" being the highest-leverage
remaining work.

# Phase Report — Milestone 1.q.x.x.x (Vertex Gemini streaming)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §5 (Streaming)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.q.x.x-REPORT.md](PHASE-M1.q.x.x-REPORT.md).

## Scope

Fourth streaming adapter, lightest phase yet. Vertex Gemini's
`:streamGenerateContent` endpoint shares wire format with the
Generative Language API (M1.q.x.x) — only the URL and auth differ.
The whole adapter is ~210 LoC including parser duplication; the
parser logic itself is mechanically translated from
`plugins/providers/google/streaming.go`.

This was telegraphed in the M1.q.x.x phase report: "Vertex AI
streaming is now ~80% done. The `parseStream` and
`assembleResponse` helpers in this phase apply unchanged; the
Vertex streaming adapter is just a `CompleteStream` that builds the
regional URL + service-account-OAuth-bearer-token request and
feeds the body into the same parser. ~50 LoC follow-up." It turned
out closer to 100 LoC for the adapter and 100 LoC for the
duplicated parser, because the project's existing convention (set
at vertex.go's package comment) is to keep Vertex's types
duplicated rather than share google's unexported helpers.

| Concern | M1.q.x.x.x status |
|---|---|
| URL `:generateContent` → `:streamGenerateContent?alt=sse` | ✅ via `ResolveStreamEndpoint` |
| OAuth Bearer token reused from TokenSource (same as Complete) | ✅ |
| Custom BaseURL override honored (VPC service-control aliases) | ✅ URL-builder test sub-case |
| Regional URL builder works for non-US regions | ✅ tested with europe-west4 |
| Same Chunk lifecycle as M1.q.x.x (synthesized for whole-arg tools) | ✅ |
| Garbage-frame tolerance + onChunk-abort propagation | ✅ inherited from same pattern |
| compat contract guard extended to cover google-vertex | ✅ via dedicated test |
| Compile-time guard: `var _ agent.StreamingProvider = (*vertex.Provider)(nil)` | ✅ |
| Test coverage: 6 streaming tests + 4 URL-builder sub-cases | ✅ |

## Changes

### 1. `plugins/providers/vertex/streaming.go` (NEW, ~210 LoC)

`(*Provider).CompleteStream` reuses the Complete path's body
encoder, OAuth TokenSource, and HTTP client. The only differences:

- URL: `:streamGenerateContent?alt=sse` via the new
  `ResolveStreamEndpoint(model)` (exported, mirrors `ResolveEndpoint`).
- Header: `Accept: text/event-stream` added.
- Body reader: piped to `parseStream` instead of `io.ReadAll` +
  `decodeResponse`.

`parseStream` is a near-exact mechanical translation of
`plugins/providers/google/streaming.go`'s parser, operating on
`vxResponse` types instead of `geminiResponse` types. The
duplication follows the package-level decision documented on
`vertex.go:21`:

> Body shape is identical to plugins/providers/google (Generative
> Language API). We duplicate the encoder/decoder rather than reuse
> google's unexported helpers — Vertex evolves independently and
> the duplication is contained.

That decision predates streaming; this phase honors it. The
alternative — introducing a shared `internal/gemini` package — was
considered and rejected for the same reason the original
author rejected it for the non-streaming path: Vertex and the
public API are likely to diverge (Vertex has historically added
features like `groundingMetadata`, content-filter feedback in
candidates, etc. that the public API doesn't have), and an
internal sharing layer would either become a least-common-
denominator that forces sync-divergence pain or a thin
re-export that adds no real abstraction. Two copies of ~80 LoC
each is the contained cost.

### 2. `ResolveStreamEndpoint` is exported

Same convention as the non-streaming `ResolveEndpoint` —
operators (and tests) can predict the exact URL the adapter will
hit without standing up a server. Useful especially for VPC
service-control or private endpoint debugging where "did my custom
BaseURL get folded in correctly?" is a real question.

### 3. `plugins/providers/vertex/streaming_test.go` (NEW)

Six end-to-end tests via httptest (matching the existing
`vertex_test.go` convention of testing through the public API only,
since the package is tested as `vertex_test`) plus four URL-builder
sub-cases:

- `TestCompleteStream_Vertex_TextEndToEnd` — happy path; verifies
  OAuth Bearer header from the TokenSource flows through, `Accept`
  is set, URL path contains `:streamGenerateContent`, response
  reassembles correctly.
- `TestCompleteStream_Vertex_ToolCallLifecycle` — synthesized
  start→delta→stop for the whole-arg tool case; id consistency
  between start and stop; StopReason override when tools present.
- `TestCompleteStream_Vertex_HTTPError` — 403 surfaces as
  `*vertex.APIError` (same type as non-streaming).
- `TestCompleteStream_Vertex_NoTokenSource` — explicit
  `ErrNoTokenSource` returned when constructed without one.
- `TestCompleteStream_Vertex_NilOnChunkRejected` — contract guard.
- `TestResolveStreamEndpoint_Vertex` — 4 sub-cases:
  default regional host, europe region, VPC service-control alias
  via custom BaseURL, explicit Endpoint wins.

A small helper `streamingTokenSrv(t)` mints a static OAuth access
token, keeping each test focused on streaming-specific behavior.
(The OAuth path is exhaustively covered by vertex_test.go's
existing `TestTokenSource_*` tests.)

Plus compile-time:
`var _ agent.StreamingProvider = (*vertex.Provider)(nil)`.

### 4. `plugins/providers/compat/compat_test.go` — Vertex guard

`TestBuild_PreservesStreamingCapability` is a clean table-driven
test for the simple-auth providers. Vertex doesn't fit the table
because building it requires a real RSA-signed service-account JSON
on disk (the JWT-bearer flow validates the key at TokenSource
construction time, before any HTTP call). A dedicated single-case
test `TestBuild_VertexPreservesStreamingCapability` was added,
reusing the existing `genTestVertexSA` + `writeTempFile` helpers
the other Vertex compat tests use.

## Architectural consequences

1. **The Chunk abstraction held up cleanly across all four wire
   formats.** Anthropic, OpenAI, Google, Vertex — four phases, four
   genuinely different streaming shapes (event-tagged SSE,
   untagged-SSE-with-DONE, untagged-SSE-no-terminator,
   regional-OAuth-untagged-SSE-no-terminator), zero changes to the
   `agent.Chunk` type or `agent.StreamingProvider` interface. The
   abstraction wasn't over-fit to Anthropic.

2. **The cumulative streaming wedge is now genuinely enterprise-
   ready.** With Vertex in, the catalog covers the four large
   enterprise commitments: Anthropic (Claude API), OpenAI (api +
   Azure), Google (consumer Gemini), and GCP (Vertex
   Gemini + the upcoming Vertex Anthropic via M1.n.x). The remaining
   gaps — Cohere, Bedrock, Ollama — are real but smaller (single-
   vendor or local-only).

3. **The "set the package convention, honor it across phases"
   pattern works.** vertex.go's package comment explicitly chose
   duplication over sharing for the non-streaming path. Five
   phases later, the streaming path follows the same choice
   without re-litigation, and the resulting code is easier to
   read because it's locally complete. Setting durable
   architectural defaults in package comments is paying off.

## Streaming coverage so far

| Family | Streaming? | Phase |
|---|---|---|
| anthropic | ✅ | M1.q |
| openai | ✅ | M1.q.x |
| openai-compatible (Groq, Cerebras, SambaNova, Together, DeepInfra, Perplexity, Fireworks, xai, OpenRouter) | ✅ | M1.q.x |
| azure | ✅ | M1.q.x |
| mistral | ✅ | M1.q.x |
| google (gemini) | ✅ | M1.q.x.x |
| **google-vertex (gemini)** | ✅ | **M1.q.x.x.x** |
| cohere | ❌ | next candidate |
| google-vertex (anthropic) | ❌ | depends on M1.n.x |
| aws-bedrock (anthropic) | ❌ | needs AWS event-stream framing (binary) |
| ollama | ❌ | small (JSON-lines) |

Seven of eleven catalog families. The remaining four split into
easy (cohere, ollama) and harder (bedrock binary event-stream,
vertex-anthropic depends on M1.n.x).

## Demo (synthetic; mock OAuth + mock Vertex API)

```
$ GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json \
  GOOGLE_VERTEX_PROJECT=my-project \
  GOOGLE_VERTEX_LOCATION=us-central1 \
  agt provider check --stream google-vertex
streaming provider=google-vertex model=gemini-1.5-flash family=google-vertex …

hello from vertex

OK
  total latency   : 320ms (wall-clock for the full stream)
  stop_reason     : end_turn
  tokens in / out : 4 / 3
```

With a tool call:

```
$ ... agt provider check --stream google-vertex
streaming provider=google-vertex model=gemini-1.5-pro family=google-vertex …

I'll check that file system.
→ tool_use_start: shell (id=call-0)
{"command":"ls"}
← tool_use_stop: call-0

OK
  total latency   : 580ms
  stop_reason     : tool_use
  tokens in / out : 20 / 12
```

## Deferrals → next phases

**Remaining streaming adapters** (in rough order of ease):

1. **Ollama** — `/api/chat` JSON-lines streaming. Very small
   (~100 LoC). Local-only, no auth, no SSE.
2. **Cohere** — `v2/chat` streaming. Cohere ships its own
   JSON-lines event format (`event_type: "text-generation"` etc.).
3. **Bedrock Anthropic** — `InvokeModelWithResponseStream`. Uses
   AWS event-stream framing (binary, length-prefixed, not SSE).
   Needs a small event-stream decoder. ~300 LoC.
4. **Vertex Anthropic** — depends on M1.n.x (Anthropic-on-Vertex
   non-streaming via `:rawPredict`) landing first.

**Unchanged longstanding deferrals:**
- M1.q.y agent loop integration (the "journal every token?"
  decision phase).
- Hot reload of catalog + vault.
- Subscription-first routing.
- OS-keychain encryption for the vault.
- Bedrock SigV4 + non-Anthropic vendor body shapes.
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.

## Files touched

```
plugins/providers/vertex/streaming.go        NEW (~210 LoC)
plugins/providers/vertex/streaming_test.go   NEW (6 e2e tests + 4 URL-builder sub-cases)
plugins/providers/compat/compat_test.go      + TestBuild_VertexPreservesStreamingCapability
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 392 pass, 0 fail (up from 381 in M1.q.x.x)
```

The cumulative operator UX trajectory:

| Milestone | New capability |
|---|---|
| M1.f | `agt catalog sync/list/discover` |
| M1.o | `agt provider creds set/list/rm` |
| M1.p | `agt provider check [id]` |
| M1.p.x | `agt provider check --all` |
| M1.p.y | `--json` + `--bench N` |
| M1.q | `agent.StreamingProvider` + Anthropic SSE + `--stream` |
| M1.q.x | OpenAI streaming → 4 families × ~11 vendors |
| M1.q.x.x | Google Gemini streaming → 1 family; preps Vertex |
| **M1.q.x.x.x** | **Vertex Gemini streaming → enterprise GCP coverage** |

Streaming coverage: 7 of 11 catalog families wired.

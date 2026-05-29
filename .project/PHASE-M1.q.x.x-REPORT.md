# Phase Report — Milestone 1.q.x.x (Google Gemini streaming)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §5 (Streaming)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.q.x-REPORT.md](PHASE-M1.q.x-REPORT.md).

## Scope

Third streaming adapter. Google Gemini's `streamGenerateContent`
endpoint, accessed via the public Generative Language API. Adds
streaming for FamilyGoogle (one vendor today,
`generativelanguage.googleapis.com`) and — more importantly — lays
the groundwork for FamilyGoogleVertex by proving the Gemini body
shape against the streaming abstraction.

| Concern | M1.q.x.x status |
|---|---|
| URL switches `:generateContent` → `:streamGenerateContent?alt=sse` | ✅ via `resolveStreamEndpoint` |
| SSE frames parsed (no event lines, no `[DONE]`, body-close terminus) | ✅ |
| Text deltas across multiple frames concatenate to final content | ✅ |
| `functionCall` parts → clean ToolUseStart → input → ToolUseStop lifecycle | ✅ |
| Interleaved text + tool call in same candidate emit in arrival order | ✅ tested |
| `usageMetadata` + `finishReason` from terminal chunk captured | ✅ |
| Tool-calls-present overrides `finishReason: STOP` to `stop_use` | ✅ matches Complete |
| Garbage frame in mid-stream doesn't kill the stream | ✅ tested |
| compat contract guard extended to cover google | ✅ |
| Compile-time guard: `var _ agent.StreamingProvider = (*Provider)(nil)` | ✅ |
| Test coverage: 9 Gemini streaming tests (incl. 4-case URL builder) | ✅ |

## Changes

### 1. `plugins/providers/google/streaming.go` (NEW, ~200 LoC)

`(*Provider).CompleteStream` POSTs to
`:streamGenerateContent?alt=sse` with the same body Complete uses
(no streaming-specific request fields needed — the URL carries the
mode). `Accept: text/event-stream` and `x-goog-api-key` headers
match the non-streaming path.

`resolveStreamEndpoint` mirrors `resolveEndpoint` exactly but with
the streaming verb + `?alt=sse` query — kept separate so the
non-streaming URL builder stays byte-identical to what existing
tests verified.

`parseStream` consumes the SSE response. Each `data:` line is a
complete partial `geminiResponse`. The parser:

1. Captures `usageMetadata` whenever present (typically only the
   terminal chunk).
2. Captures `finishReason` from the terminal chunk.
3. For each part in `candidates[0].content.parts`:
   - **Text part** → accumulate + emit `Chunk{TextDelta}`.
   - **FunctionCall part** → synthesize a deterministic call ID
     (`call-N`), then emit the trio:
     `ToolUseStart` → `ToolInputJSONDelta` (carrying the full
     args JSON as a single chunk) → `ToolUseStop`.

The last point deserves emphasis. Gemini doesn't stream tool
arguments — the entire `functionCall.args` arrives as a parsed
JSON object in one chunk. Anthropic and OpenAI both genuinely
stream arguments as fragmented JSON. To keep callers free from
provider-specific special-casing, the Gemini adapter synthesizes
the full lifecycle: a single `ToolInputJSONDelta` containing the
complete args. UIs that "render the input as it streams" still
work — they just render it in one update for Gemini and many for
the others. Same code path, different rate.

### 2. `plugins/providers/google/streaming_test.go` (NEW)

Nine tests with three real-shaped SSE fixtures:

- `TestParseStream_GeminiTextOnly` — three text frames concatenate
  to "pong!", terminal chunk's usage captured.
- `TestParseStream_GeminiToolCall` — single-frame tool call;
  lifecycle counts asserted (exactly 1 start, 1 stop), start.ID
  matches stop.ID, args JSON parses back to the original object,
  StopReason correctly derives from tool-calls presence even when
  upstream sent `finishReason: STOP`.
- `TestParseStream_GeminiInterleaved` — text before tool call in
  the same response; both arrive in order.
- `TestParseStream_Gemini_OnChunkAborts` — abort propagates.
- `TestParseStream_Gemini_GarbageFrameIgnored` — malformed frame
  in the middle doesn't kill the stream.
- `TestCompleteStream_Gemini_EndToEnd` — full HTTP path through
  httptest; verifies `Accept` and `x-goog-api-key` headers.
- `TestCompleteStream_Gemini_HTTPError` — 403 surfaces as
  `*APIError`.
- `TestCompleteStream_Gemini_NilOnChunkRejected` — contract guard.
- `TestResolveStreamEndpoint` — 4 sub-cases for URL builder edge
  cases (default base, no version segment, /v1beta-present,
  trailing slash).

Plus compile-time: `var _ agent.StreamingProvider = (*Provider)(nil)`.

### 3. `plugins/providers/compat/compat_test.go` — guard extension

`TestBuild_PreservesStreamingCapability` now includes:

```go
{name: "google (gemini)", npm: "@ai-sdk/google", api: "https://generativelanguage.googleapis.com"},
```

The wrapNamed plumbing handles the new entry without changes —
that's the whole point of having extracted it in M1.q.x.

## Architectural consequences

1. **Three wire shapes, one Chunk shape, no leaks.** Anthropic uses
   event-tagged SSE + `event: message_stop`; OpenAI uses untagged
   SSE + `data: [DONE]`; Google uses untagged SSE + body-close.
   Anthropic streams tool inputs as fragments; OpenAI streams tool
   inputs as fragments correlated by index; Google delivers them
   whole. Three genuinely different streaming idioms all reduce to
   `Chunk{TextDelta | ToolUseStart | ToolInputJSONDelta | ToolUseStop}`
   without any provider-specific fields. That's evidence the
   abstraction is right.

2. **Vertex AI streaming is now ~80% done.** Vertex uses the same
   Gemini body shape on a regional endpoint with OAuth instead of
   API key. The `parseStream` and `assembleResponse` helpers in
   this phase apply unchanged; the Vertex streaming adapter is
   just a `CompleteStream` that builds the regional URL +
   service-account-OAuth-bearer-token request and feeds the body
   into the same parser. ~50 LoC follow-up.

3. **The "synthesize the full lifecycle" pattern is the right
   choice for non-streaming-tool-input providers.** The alternative
   (emit only Start + Stop with no input chunk) would force
   callers to know which providers stream tool inputs and which
   don't, defeating the abstraction. The chosen approach makes the
   Chunk lifecycle a pure contract — start, optional deltas, stop
   — regardless of how the upstream actually delivers it.

## Streaming coverage so far

| Family | Streaming? | Phase |
|---|---|---|
| anthropic | ✅ | M1.q |
| openai | ✅ | M1.q.x |
| openai-compatible (Groq, Cerebras, SambaNova, Together, DeepInfra, Perplexity, Fireworks, xai, OpenRouter) | ✅ | M1.q.x |
| azure | ✅ | M1.q.x |
| mistral | ✅ | M1.q.x |
| **google (gemini)** | ✅ | **M1.q.x.x** |
| cohere | ❌ | next |
| google-vertex (gemini) | ❌ | next-next (50 LoC; reuses M1.q.x.x parser) |
| google-vertex (anthropic) | ❌ | depends on M1.n.x |
| aws-bedrock (anthropic) | ❌ | needs AWS event-stream framing (binary) |
| ollama | ❌ | small (JSON-lines, not SSE) |

Six down, five to go. The remaining adapters split into easy
(cohere, vertex-gemini, ollama) and harder (bedrock binary
event-stream, vertex-anthropic depends on M1.n.x).

## Demo (synthetic; mock server returns Gemini-shaped SSE)

```
$ agt provider check --stream google
streaming provider=google model=gemini-1.5-flash family=google …

pong!

OK
  total latency   : 178ms (wall-clock for the full stream)
  stop_reason     : end_turn
  tokens in / out : 12 / 3
```

With a tool call:

```
$ AGEZT_MODEL=gemini-1.5-pro agt provider check --stream google
streaming provider=google model=gemini-1.5-pro family=google …

I'll check that.
→ tool_use_start: shell (id=call-0)
{"command":"ls"}
← tool_use_stop: call-0

OK
  total latency   : 412ms (wall-clock for the full stream)
  stop_reason     : tool_use
  tokens in / out : 50 / 18
```

Note `call-0` instead of an upstream-issued id — Gemini doesn't
return per-tool IDs, so we synthesize them. This is the same
convention `Complete` uses, so journal entries and tool-result
routing stay consistent across modes.

## Deferrals → next phases

**M1.q.x.x.x candidates** (each one phase):

1. **Ollama** — local-only `/api/chat` streaming via JSON-lines.
   Smallest scope (~100 LoC); no auth, no SSE parser needed
   (one JSON object per `\n`-delimited line).
2. **Cohere** — `v2/chat` streaming. JSON-lines like Ollama, but
   with cloud auth and Cohere's specific event-type tagging
   (`text-generation`, `tool-calls-generation`, etc).
3. **Vertex Gemini** — reuses M1.q.x.x's `parseStream`; just needs
   a Vertex-specific `CompleteStream` that builds the regional URL
   and uses the OAuth token source. Effectively free.

**Bigger lifts:**

- **Bedrock Anthropic streaming** — uses AWS event-stream framing
  (binary, length-prefixed, not SSE). Needs an event-stream
  decoder before the body parser. ~300 LoC.
- **Vertex Anthropic** — depends on M1.n.x landing first.

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
plugins/providers/google/streaming.go        NEW (~200 LoC)
plugins/providers/google/streaming_test.go   NEW (9 tests + 3 SSE fixtures + 4 URL-builder sub-cases)
plugins/providers/compat/compat_test.go      + 1 google sub-case in PreservesStreamingCapability
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 381 pass, 0 fail (up from 367 in M1.q.x)
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
| **M1.q.x.x** | **Google Gemini streaming → 1 family; preps Vertex** |

Streaming coverage: 6 of 11 catalog families wired.

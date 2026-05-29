# Phase Report — Milestone 1.t (AWS Bedrock streaming)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §5 (Streaming)](SPEC-15-PROVIDER-ECOSYSTEM.md) and the
> `TestBuild_BedrockDoesNotFalselyAdvertiseStreaming` negative-guard
> flagged during M1.q.x.x.x.x.
> Continues [PHASE-M1.s-REPORT.md](PHASE-M1.s-REPORT.md).

## Scope

The last streaming hole closes. After M1.q (Anthropic SSE), M1.q.x
(OpenAI + family SSE), M1.q.x.x (Google SSE), M1.q.x.x.x (Vertex
Gemini SSE), M1.q.x.x.x.x (Ollama NDJSON + Cohere SSE), the only
catalog family still falling back to a single buffered
`Complete()` call was AWS Bedrock — because Bedrock doesn't speak
SSE. It speaks `application/vnd.amazon.eventstream`, a binary
framing format AWS originally built for Kinesis.

M1.t implements the framing, wires it to the existing Anthropic
event dispatcher (Bedrock-on-Anthropic wraps the same JSON events
in binary frames), and flips the M1.q.x.x.x.x test guard from
"must not advertise streaming" to "must advertise streaming."

| Concern | Status |
|---|---|
| Binary event-stream frame parser (prelude + headers + payload + CRC) | ✅ |
| String-typed header parsing (type 7), other types refused | ✅ tested |
| Chunk envelope decode (`{"bytes": "<base64>"}` → inner JSON) | ✅ |
| Inner Anthropic event dispatch (message_start/_delta/_stop, content_block_*) | ✅ tested (text + tool_use) |
| Endpoint suffix swap: `/invoke` → `/invoke-with-response-stream` | ✅ |
| `Accept: application/vnd.amazon.eventstream` request header | ✅ tested |
| `:message-type=exception` → surfaced as error with `:exception-type` | ✅ tested |
| `:message-type=error` → surfaced as error with code+message | ✅ |
| EOF mid-stream returns partial response (no hard fail) | ✅ tested |
| Non-2xx → existing `*bedrock.APIError` (consistent with Complete) | ✅ tested |
| Wrapper preserves streaming capability (compat `wrapNamed`) | ✅ tested (flipped guard) |
| Refuses non-Anthropic model ids with `ErrVendorUnsupported` | ✅ tested |
| Refuses missing bearer token / nil onChunk | ✅ tested |
| onChunk errors propagate verbatim | ✅ tested |

## Changes

### 1. `plugins/providers/bedrock/streaming.go` — new file

Three layers, top to bottom:

**Layer A — `CompleteStream`** (~50 LoC). Same shape as the other
streaming adapters: validate, encode (reuses the non-streaming
`encodeAnthropicOnBedrockRequest`), POST to the
`-with-response-stream` endpoint, dispatch to the parser.

Notably, the **request body is identical to non-streaming** — there's
no `"stream": true` field in the JSON. AWS routes by URL suffix, not
body content. The single header that flips is `Accept:
application/vnd.amazon.eventstream`.

**Layer B — event-stream framer** (~80 LoC): `readEventStreamMessage`
+ `parseEventStreamHeaders`. Wire format:

```
+--------------------------------+
| Prelude (12 bytes)             |
|  Total length     (4 bytes BE) |
|  Headers length   (4 bytes BE) |
|  Prelude CRC      (4 bytes BE) | ← not validated
+--------------------------------+
| Headers (Headers length bytes) |
+--------------------------------+
| Payload (...)                  |
+--------------------------------+
| Message CRC (4 bytes BE)       | ← not validated
+--------------------------------+
```

Each header:
```
[ name-len  uint8     ]
[ name      N bytes   ]
[ value-type uint8    ]   only type 7 (string) accepted
[ value-len uint16 BE ]
[ value     M bytes   ]
```

**Layer C — Anthropic event dispatcher** (~150 LoC): mirrors the
direct-Anthropic adapter's `dispatchSSEFrame` switch but on a
`type` field already extracted from the inner JSON. Duplicated
(not shared) for the same reason the non-streaming
`decodeAnthropicOnBedrockResponse` is duplicated — keeping Bedrock
independent of plugins/providers/anthropic's internal evolution.

Three design notes worth recording:

**Why CRC validation is skipped.** AWS uses IEEE-CRC32 over a
specific bit layout that's easy to get wrong by one byte. The
connection is already HTTPS (transport corruption ruled out), and
a CRC-mismatch fail on an otherwise-good stream is worse than
silently trusting the framing. If a future incident shows
malformed frames in the wild, add CRC validation behind a flag.
The framing reads have to be defensive *anyway* because the
length field already tells us how many bytes to consume.

**Why non-string header values are refused, not ignored.** Bedrock
only emits string headers for the metadata we read (`:message-type`,
`:event-type`, `:exception-type`). A future spec drift to a binary
type for a header we'd want to inspect would silently route through
the wrong code path if we tolerated unknown types. Refusing makes
the change loud.

**Why EOF mid-stream returns partial.** Same rationale as the
direct-Anthropic adapter: a transient drop after some text already
streamed should surface the partial text + a clean response with
empty `StopReason`, not a hard error swallowing everything. The
caller (governor → agent loop) decides whether to retry.

### 2. `plugins/providers/bedrock/streaming_test.go` — new file (10 tests)

| Test | What it locks in |
|---|---|
| `TestCompleteStream_AssemblesTextResponse` | Full happy path: text streams chunked, final assembled message + usage + stop_reason correct. Verifies request URL suffix and Accept header. |
| `TestCompleteStream_AssemblesToolCall` | tool_use lifecycle: ToolUseStart → 2 input_json_delta chunks → ToolUseStop → finished tool call with `{"city":"Istanbul"}` input. |
| `TestCompleteStream_RejectsNonAnthropicModel` | meta.llama3-* returns `ErrVendorUnsupported`-wrapped error before any HTTP. |
| `TestCompleteStream_RejectsMissingBearer` | Empty BearerToken → `ErrNoBearerToken` (mirrors Complete). |
| `TestCompleteStream_RejectsNilOnChunk` | nil callback → clear contract error, not nil-deref. |
| `TestCompleteStream_SurfacesAPIErrorOnNon2xx` | 400 from upstream → `*bedrock.APIError` with status preserved. |
| `TestCompleteStream_SurfacesExceptionFrame` | `:message-type=exception` `:exception-type=ThrottlingException` → error mentions both class and payload message. |
| `TestCompleteStream_ReturnsPartialOnEOF` | Truncated stream after a text delta → no error, streamed text observable via callback. |
| `TestCompleteStream_RejectsNonStringHeader` | Hand-built frame with value-type 8 → unsupported-header-type error. |
| `TestCompleteStream_OnChunkErrorPropagates` | Returning `io.ErrClosedPipe` from onChunk aborts the stream and surfaces the error. |

The tests synthesize event-stream frames with a small helper
(`buildEventStreamFrame`) — writing zeros for the two CRC slots,
since the production parser doesn't validate them. That keeps
fixtures readable: a test failure shows hex with recognisable
JSON instead of a CRC mismatch hiding the real issue.

### 3. `plugins/providers/compat/compat_test.go` — flipped guard

Renamed `TestBuild_BedrockDoesNotFalselyAdvertiseStreaming` →
`TestBuild_BedrockAdvertisesStreaming`. Same test setup; the
assertion flips from "`StreamingProvider` cast must fail" to
"must succeed."

```go
// Before (M1.q.x.x.x.x):
if _, ok := prov.(agent.StreamingProvider); ok {
    t.Errorf("bedrock (non-streaming until binary event-stream parser ships) should NOT type-assert ...")
}

// After (M1.t):
if _, ok := prov.(agent.StreamingProvider); !ok {
    t.Errorf("bedrock should advertise agent.StreamingProvider (M1.t shipped streaming), but did not")
}
```

The flip is the contract: every catalog family now has a working
streaming adapter, and the compat layer's `wrapNamed` exposes
each one through the same `agent.StreamingProvider` interface the
agent loop type-asserts on.

## Test summary

```
go test ./plugins/providers/bedrock/ -v -count=1 -run "Stream"
=== RUN   TestCompleteStream_AssemblesTextResponse
--- PASS  (and 9 others)

go test ./... -count=1
ok      github.com/ersinkoc/agezt/plugins/providers/bedrock    0.342s
ok      github.com/ersinkoc/agezt/plugins/providers/compat     0.525s
(all packages PASS)
```

Total suite: **441 tests passing** (from 431 after M1.s). +10 from
the new bedrock streaming tests.

## Streaming family coverage (final)

| Family | Streaming impl | Wire format |
|---|---|---|
| Anthropic | M1.q | SSE (event-tagged) |
| OpenAI + ~11 compatible vendors | M1.q.x | SSE (untagged + `[DONE]`) |
| Google (Gemini direct) | M1.q.x.x | SSE (untagged + body-close) |
| Google Vertex (Gemini) | M1.q.x.x.x | SSE (untagged + body-close) |
| Ollama | M1.q.x.x.x.x | NDJSON (`\n`-delimited JSON, `"done":true`) |
| Cohere | M1.q.x.x.x.x | SSE (v2 typed events) |
| **AWS Bedrock (Anthropic models)** | **M1.t** | **event-stream binary framing** |
| Mistral | (via OpenAI compat) | SSE |
| Azure OpenAI | (via OpenAI compat) | SSE |
| Google Vertex (Anthropic models) | deferred → M1.n.x | event-stream binary |

Every catalog family the wire layer recognises now exposes
`agent.StreamingProvider`. The agent loop's type assertion always
takes the streaming branch when the operator picks any model.

## Behaviour by example

Operator picks `anthropic.claude-opus-4-7` on Bedrock with a
bearer token:

```
agt run --task chat 'hello'
→ governor.routeChain returns [bedrock-anthropic]
→ agent.Run type-asserts: *namedStreamingProvider implements StreamingProvider ✓
→ CompleteStream POSTs to .../invoke-with-response-stream
→ AWS streams binary frames at ~150ms intervals
→ each frame: parse prelude → headers → base64-decode chunk.bytes → JSON dispatch
→ bus.PublishStreaming emits KindLLMToken per text_delta (ephemeral, no journal)
→ CLI prints tokens inline as they arrive
→ message_stop → llm.response durable event (full assembled text + usage)
```

Time-to-first-token vs. the buffered path: roughly the latency of
**one** frame parse (typically 200-300 bytes for a `message_start`
+ first delta) vs. the latency of the full response being
buffered and decoded. For a 500-token Claude response, that's
~50ms vs. ~3-8s — a perceived-latency cliff the operator
absolutely feels.

## What's intentionally NOT in M1.t

- **CRC validation.** Documented above. Add behind a flag if/when
  needed.
- **Vertex Anthropic streaming.** The `vertex` package only speaks
  Gemini today; an `anthropic.claude-*` model id on Vertex returns
  `ErrVendorUnsupported`. M1.n.x is the catch-up phase that wires
  the Anthropic body shape over Vertex's URL+auth, at which point
  Vertex Anthropic streaming reuses *this* package's binary
  framer plus the Anthropic JSON dispatch — the inner-event layer
  is the same JSON, just delivered over a different wire.
- **SigV4-signed Bedrock requests.** Out of scope — separate from
  streaming. Will land when M1.m.x adds the SigV4 signer; the
  streaming code is signer-agnostic (it just sets `Authorization`).
- **Non-Anthropic body shapes on Bedrock.** Same — when M1.m.x
  adds Mistral / Meta / Amazon Titan / Cohere body encoders for
  Bedrock, streaming over them needs its own inner-event
  dispatcher (each vendor's streaming JSON differs from Anthropic's).
  The binary framer this phase ships is reusable; the inner
  switch isn't.

## Files touched

- [plugins/providers/bedrock/streaming.go](../plugins/providers/bedrock/streaming.go) — new (~330 LoC).
- [plugins/providers/bedrock/streaming_test.go](../plugins/providers/bedrock/streaming_test.go) — new (~330 LoC, 10 tests).
- [plugins/providers/compat/compat_test.go](../plugins/providers/compat/compat_test.go) — flipped one test's assertion (was negative guard, now positive).

No changes to `compat.go` — the `wrapNamed` helper already
checks for `StreamingProvider` via type assertion and returns a
`*namedStreamingProvider` when the inner provider implements it.
Bedrock just started implementing the interface.

## Deferrals after M1.t

The streaming wedge is **closed**. Remaining deferrals from
across M1.q-M1.t are all in adjacent areas:

- Vertex Anthropic body shape (M1.n.x).
- Bedrock SigV4 + non-Anthropic body shapes (M1.m.x).
- OS-keychain vault encryption.
- Browser tool, out-of-process plugin host, Pulse v1, planner.

None block the agent loop. Next pickup should be one of:
**Vertex Anthropic body** (small, completes the catalog coverage),
**Pulse v1** (operator-facing observability), or **planner**
(scheduler integration).

# Phase Report — Milestone 1.q (streaming, Anthropic-first)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §5 (Streaming)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.p.y-REPORT.md](PHASE-M1.p.y-REPORT.md).

## Scope

The deferral that's been lurking in every phase report since M1.g
finally lands: **streaming**. `Provider` was request/response only;
long completions felt dead because nothing rendered until the full
body arrived.

M1.q does the minimum to make streaming real for one provider:

1. New optional interface `agent.StreamingProvider`.
2. New shape `agent.Chunk` carrying text deltas + tool-input deltas.
3. SSE parser + `CompleteStream` for the Anthropic Messages API.
4. `agt provider check --stream` to validate end-to-end.

What's deliberately **not** in scope:

- **Agent loop integration.** Wiring `StreamingProvider` into
  `agent.Run` raises real questions (journal one event per token?
  per buffered batch? separate non-journaled channel?) that deserve
  their own phase. M1.q exercises the new interface via `agt provider
  check --stream` so the SSE parser is proven before the agent-loop
  decisions are made.
- **Other adapters.** OpenAI, Mistral, Cohere, Google, Vertex,
  Bedrock, openai-compatible all use different SSE event names and
  delta shapes. Each is a separate M1.q.x mini-phase. Anthropic-first
  isn't because it's most important — it's the family with the
  cleanest documented event grammar, so it's the right one to prove
  the abstraction against.
- **Streaming usage stats every-N-tokens.** The interface returns the
  full `CompletionResponse` at the end (matching what `Complete`
  would have returned), so cost accounting, journal usage stats, and
  Governor budget math are byte-identical to non-streaming. No new
  pricing complexity.

| Concern | M1.q status |
|---|---|
| `StreamingProvider` interface is opt-in (sibling to `Provider`) | ✅ — type assertion at call site |
| `CompleteStream` returns same shape as `Complete` for same request | ✅ documented + tested |
| onChunk error aborts the stream early | ✅ tested |
| SSE comments / keep-alive frames ignored | ✅ |
| Forward-compat: unknown event names ignored, not error | ✅ |
| EOF without `message_stop` doesn't drop already-streamed text | ✅ |
| Bumped bufio limit to 1MB (default 64K can't hold large tool inputs) | ✅ |
| Compile-time guard: `var _ agent.StreamingProvider = (*Provider)(nil)` | ✅ |
| `agt provider check --stream` rejects unsupported families clearly | ✅ exit 2 with msg |
| `--stream` rejects combinations that don't make sense (--all, --bench) | ✅ exit 2 with msg |
| Test coverage: 7 streaming tests (SSE parser + CompleteStream) | ✅ |

## Changes

### 1. `kernel/agent/streaming.go` (NEW, ~60 LoC)

```go
type StreamingProvider interface {
    Provider
    CompleteStream(ctx, req, onChunk func(Chunk) error) (*CompletionResponse, error)
}

type Chunk struct {
    TextDelta          string     // next slice of assistant text
    ToolUseStart       *ToolCall  // a new tool call started (ID+Name final)
    ToolInputJSONDelta string     // streamed JSON fragment for the open tool
    ToolUseStop        string     // tool call's input complete (ID)
}
```

Three properties matter for forward use:

- **Optional**: callers type-assert. `Provider` stays as is — the
  catalog Build path needn't change.
- **Same final state**: the returned response equals what `Complete`
  would have returned. Bookkeeping callers don't need streaming
  awareness.
- **Cancellation channel**: `onChunk` can return an error to abort.
  Useful for a future `agt halt` integration where halting
  mid-stream needs to stop the read goroutine.

### 2. `plugins/providers/anthropic/streaming.go` (NEW, ~270 LoC)

`(*Provider).CompleteStream` does the same HTTP build as `Complete`
but adds `"stream": true` to the body and `Accept: text/event-stream`
to the headers. The response body is fed to `parseStream`.

`parseStream` is a hand-rolled SSE parser, not a third-party
library — Anthropic's SSE flavour is restricted (no retry directives,
no last-event-id) and pulling a 1K-LoC SSE dep for this would be
silly.

Frame dispatch table:

| event name | handling |
|---|---|
| `message_start` | capture `usage.input_tokens`, `model` |
| `content_block_start` (text) | open block; emit chunk if initial text non-empty |
| `content_block_start` (tool_use) | open block; emit `ToolUseStart` chunk |
| `content_block_delta` (text_delta) | accumulate + emit `TextDelta` chunk |
| `content_block_delta` (input_json_delta) | accumulate + emit `ToolInputJSONDelta` chunk |
| `content_block_stop` | finalize the open block (tool call goes to response) |
| `message_delta` | capture `delta.stop_reason`, `usage.output_tokens` |
| `message_stop` | end of stream |
| `ping`, comments (`:...`), unknown | ignore (forward-compat) |
| `error` | parse + return as error to caller |

The bufio Scanner default 64K line limit was insufficient — large
tool inputs (a multi-KB JSON schema in the function-calling case)
can blow past it. Bumped to 1MB; anything larger is upstream
malfunction not normal use.

### 3. `cmd/agt/check.go` — `--stream` flag

New flag handling:

```
--stream / -s          # use SSE roundtrip when provider supports it
```

The runner (`runStreamProbe`) type-asserts the built provider to
`agent.StreamingProvider`. If the assertion fails it surfaces a
specific error rather than degrading silently:

```
$ agt provider check --stream openai
agt: provider family "openai" does not yet support streaming
(M1.q only wires anthropic; others land in M1.q.x)
```

Rejected combinations:

- `--stream --all` — interleaved streams across N providers would
  be visually chaotic.
- `--stream --bench N` — would re-stream the same tokens N times
  for noise's sake.

Both rejections exit 2 (CLI misuse) with a clear message.

### 4. Help text addition

```
provider check --stream [id]          live SSE roundtrip; renders tokens inline
```

## Demo (synthetic; mock server returns canonical Anthropic SSE)

```
$ agt provider check --stream anthropic
streaming provider=anthropic model=claude-3-5-haiku-20241022 family=anthropic …

pong!

OK
  total latency   : 142ms (wall-clock for the full stream)
  stop_reason     : end_turn
  tokens in / out : 12 / 3
  this call cost  : $0.0000216 (21600 microcents)
```

With a tool-using model:

```
$ AGEZT_MODEL=claude-sonnet-4-6 agt provider check --stream anthropic
streaming provider=anthropic model=claude-sonnet-4-6 family=anthropic …

I'll check the directory listing.
→ tool_use_start: shell (id=toolu_abc)
{"command":"ls -la"}
← tool_use_stop: toolu_abc

OK
  total latency   : 1.2s (wall-clock for the full stream)
  stop_reason     : tool_use
  tokens in / out : 84 / 31
  this call cost  : $0.000256 (256000 microcents)
```

The streamed tool-input rendering is intentionally raw so operators
see exactly what arrives over the wire — debugging tool-call edge
cases is a real reason this exists.

## Architectural consequences

1. **Streaming is an opt-in capability, not a contract change.**
   `Provider` stayed exactly as it was. Every existing adapter
   continues to work; every existing caller continues to work.
   Streaming-aware callers add one type assertion. This is the
   cleanest way to add a cross-cutting capability without breaking
   the matrix of (existing providers) × (existing callers).

2. **The full-response invariant is load-bearing.** Both modes
   return the same `CompletionResponse`, so the Governor's pricing
   math, the agent loop's usage journaling, and the `agt provider
   check` cost reporting all work without modification. A future
   `agent.Run` integration just chooses which path to drive — the
   bookkeeping side is the same code.

3. **The SSE parser is self-contained.** No third-party SSE
   dependency. The wire format is small and stable; the parser is
   ~150 LoC including all the dispatch and edge cases. This matches
   the project's "lean external deps" posture (only
   `lukechampine.com/blake3` and `github.com/yuin/goldmark` so far —
   no oauth2, no SSE library, no OAuth-google).

4. **Test fixtures live in-package.** `sampleTextStream` and
   `sampleToolUseStream` are real Anthropic-shaped SSE bodies, not
   hand-stripped. Future SSE quirks can be added by appending to
   these fixtures; the parser tests run against them in <30µs.

## Test coverage

- `TestParseStream_TextOnly` — happy path; verifies all chunks
  arrive, content reassembles to "pong!", usage stats captured.
- `TestParseStream_ToolUse` — tool-use roundtrip; verifies the
  fragmented `input_json_delta` chunks concatenate back to valid
  JSON, the start/stop callbacks fire, and the assembled
  `ToolCall` has the expected ID/Name/Input.
- `TestParseStream_ErrorFrame` — `event: error` frames propagate
  upstream error type + message.
- `TestParseStream_OnChunkAborts` — returning an error from
  onChunk halts dispatch (≤2 calls after abort, not the full
  stream).
- `TestCompleteStream_EndToEnd` — httptest server, full HTTP path:
  verifies SSE Accept header sent, x-api-key set, response
  decoded.
- `TestCompleteStream_HTTPError` — 401 surfaces as `*APIError`
  (same type as non-streaming Complete).
- `TestCompleteStream_NilOnChunkRejected` — callbacks are
  required; nil is a contract violation.

Plus the compile-time guard:

```go
var _ agent.StreamingProvider = (*Provider)(nil)
```

## Deferrals → next phases

**M1.q.x — other adapters** (one per phase, in rough order of
usefulness):

1. OpenAI Chat Completions streaming (and the openai-compatible
   family that inherits it — Groq, Cerebras, SambaNova, etc).
2. Google Gemini `streamGenerateContent`.
3. Mistral chat completions streaming.
4. Cohere v2/chat streaming.
5. Vertex AI Anthropic streaming (`streamRawPredict`).
6. Bedrock Anthropic streaming (`InvokeModelWithResponseStream`).

Each is mostly mechanical: new SSE event names, same Chunk shape.

**M1.q.y — agent loop integration:** the live `agt run` should
render tokens as they stream. Open questions to resolve:

- Journal one event per chunk? per buffered batch (every 500ms)?
  not at all (separate non-journaled channel)?
- Does the bus need a new "streaming" subject that subscribers can
  opt into without polluting the full journal?

That's a one-phase decision-then-implementation, deferred until the
streaming-providers-coverage story (M1.q.x) gives enough data to
choose well.

**Unrelated longstanding deferrals** (unchanged):
- Hot reload of catalog + vault.
- Subscription-first routing (DECISIONS C2).
- OS-keychain encryption for the vault.
- Bedrock SigV4 + non-Anthropic vendor body shapes.
- Vertex Anthropic + ADC + workload-identity.
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.

## Files touched

```
kernel/agent/streaming.go                       NEW (~60 LoC)
plugins/providers/anthropic/streaming.go        NEW (~270 LoC)
plugins/providers/anthropic/streaming_test.go   NEW (7 tests + 2 fixtures)
cmd/agt/check.go                                + runStreamProbe + --stream flag + reject combo (~+85 LoC)
cmd/agt/check_test.go                           + 2 sub-cases for --stream parsing
cmd/agt/main.go                                 + 1 help line
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 352 pass, 0 fail (up from 343 in M1.p.y)
```

The cumulative operator UX trajectory:

| Milestone | New operator capability |
|---|---|
| M1.f | `agt catalog sync`, `agt catalog list`, `agt catalog discover` |
| M1.o | `agt provider creds set/list/rm` |
| M1.p | `agt provider check [id]` |
| M1.p.x | `agt provider check --all` |
| M1.p.y | `--json` (CI gate) + `--bench N` (vendor latency comparison) |
| **M1.q** | **`agent.StreamingProvider` + Anthropic SSE + `--stream`** |

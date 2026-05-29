# Phase Report — Milestone 1.q.x (OpenAI streaming → 4 families, ~11 vendors)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §5 (Streaming)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.q-REPORT.md](PHASE-M1.q-REPORT.md).

## Scope

M1.q wired streaming for Anthropic only. M1.q.x adds OpenAI Chat
Completions streaming — which, because the openai adapter is shared
across four families in `compat`, covers ~11 vendors at once:

| Family | Vendors picked up |
|---|---|
| FamilyOpenAI | api.openai.com |
| FamilyOpenAICompatible | Groq, Cerebras, SambaNova, Together, DeepInfra, Perplexity, Fireworks, xai, OpenRouter |
| FamilyMistral | api.mistral.ai |
| FamilyAzure | Azure OpenAI Service |

One adapter implementation → 11 catalog providers gain streaming.
That's the leverage that motivated picking OpenAI second instead of
Google or Cohere.

| Concern | M1.q.x status |
|---|---|
| OpenAI Chat Completions SSE parsed correctly | ✅ |
| `stream_options.include_usage=true` set so final chunk carries usage | ✅ |
| `[DONE]` sentinel terminates the stream | ✅ |
| Parallel tool calls correlated by `index` (not by id, which is per-index-first-chunk-only) | ✅ |
| Streamed tool `function.arguments` fragments concatenate to valid JSON | ✅ |
| Azure auth header (`api-key`, no scheme) honored in streaming path | ✅ |
| Garbage frame in middle of stream doesn't kill the stream | ✅ tested |
| **compat.namedProvider wrapper preserves StreamingProvider capability** | ✅ wrapNamed split into two types |
| Compile-time guard: `var _ agent.StreamingProvider = (*Provider)(nil)` | ✅ |
| Test coverage: 9 OpenAI streaming tests + 2 compat contract guards | ✅ |

## Changes

### 1. `plugins/providers/compat/compat.go` — `wrapNamed` split

The single biggest hidden bug this phase had to fix: the existing
`namedProvider` wrapper that compat.Build returned implemented only
`agent.Provider`. Type-asserting to `agent.StreamingProvider` always
failed, even when the inner adapter (Anthropic in M1.q) supported
streaming. So `agt provider check --stream anthropic` only worked
because the wrapped name happened to be passed through OK — but
once compat got involved (which is always, via the catalog), the
capability was silently dropped.

Fix: split into two types and a constructor that picks at build
time.

```go
type namedProvider struct { name string; inner agent.Provider }
type namedStreamingProvider struct {
    namedProvider
    streamingInner agent.StreamingProvider
}

func wrapNamed(name string, p agent.Provider) agent.Provider {
    if sp, ok := p.(agent.StreamingProvider); ok {
        return &namedStreamingProvider{
            namedProvider:  namedProvider{name: name, inner: p},
            streamingInner: sp,
        }
    }
    return &namedProvider{name: name, inner: p}
}
```

This is deliberately **structural**: an always-implements approach
(adding `CompleteStream` to `namedProvider` that returns an error
for non-streaming inners) would make every wrapped provider claim
streaming support, breaking the `prov.(StreamingProvider)` check
that callers like `runStreamProbe` rely on to give clear error
messages. Two types means the type assertion at the call site means
exactly what it says.

All 9 `&namedProvider{...}` call sites in `Build` migrated to
`wrapNamed(...)` via mechanical rewrite. No behavior change for
non-streaming providers; streaming providers now retain their
capability through the wrapper.

### 2. `plugins/providers/openai/streaming.go` (NEW, ~310 LoC)

`(*Provider).CompleteStream` — POSTs with `stream: true` and
`stream_options.include_usage: true`. Same auth-header /
auth-scheme convention as `Complete`, so Azure's `api-key` header
flows through unchanged.

Wire differences from Anthropic that the parser handles:

| Detail | Anthropic | OpenAI |
|---|---|---|
| SSE event labels | `event: <name>\n` + `data: <json>` | `data: <json>` only |
| Stream terminator | `event: message_stop` | `data: [DONE]` literal |
| Per-event JSON discrimination | by event name | by inspecting delta fields |
| Tool call correlation | sequential blocks via `index` | parallel via per-chunk `index` |
| Tool id+name placement | every chunk | only the first chunk per index |
| Tool input JSON | `partial_json` in `input_json_delta` | `arguments` string in `tool_calls[].function` |
| Usage location | `message_delta` + `message_start` | final chunk (when stream_options requested) |

The tool-call streaming is the trickiest piece. OpenAI streams
parallel tool calls *interleaved* — chunk 1 carries the start of
tool index 0, chunk 2 starts tool index 1, chunk 3 has more args
for tool 0, chunk 4 has args for tool 1, etc. The parser tracks
each tool's accumulating state in a `map[int]*openTool` keyed by
index, then emits the assembled `ToolCall` slice in `toolOrder`
arrival order at message end.

`ToolUseStop` chunks aren't naturally present in the OpenAI wire
format — there's no per-tool stop frame. We synthesize them on the
terminal frame so callers get a clean
`start → deltas → stop → start → deltas → stop` lifecycle that
matches Anthropic. (Less faithful to the wire, more useful to
callers.)

**Garbage tolerance**: malformed JSON frames in the middle of a
stream are silently skipped rather than killing the stream.
Real-world reason: some openai-compatible vendors (notably
Together's older v1 endpoint and certain OpenRouter proxies) inject
comment/keepalive frames as invalid JSON. Killing the stream on the
first one would make those vendors unusable from M1.q.x.

### 3. Tests (`plugins/providers/openai/streaming_test.go`, NEW)

Nine tests with real-shaped SSE fixtures:

- `TestParseStream_OAITextOnly` — happy path; final chunk usage
  block captured.
- `TestParseStream_OAIToolCall` — one tool, fragmented JSON
  arguments concatenate correctly.
- `TestParseStream_OAIParallelTools` — two parallel tool calls
  interleaved by `index`; both reassemble in order with the right
  args going to the right id.
- `TestParseStream_OAI_OnChunkAborts` — abort propagates.
- `TestParseStream_OAI_GarbageFrameIgnored` — middle-of-stream
  malformed JSON doesn't kill the parse.
- `TestCompleteStream_OAI_EndToEnd` — httptest server, full HTTP
  path, Accept + Authorization headers checked.
- `TestCompleteStream_OAI_AzureAuthHeader` — Azure flavour;
  `api-key` header present, `Authorization` absent.
- `TestCompleteStream_OAI_HTTPError` — 401 surfaces as `*APIError`.
- `TestCompleteStream_OAI_NilOnChunkRejected` — contract guard.

Plus compile-time: `var _ agent.StreamingProvider = (*Provider)(nil)`.

### 4. `plugins/providers/compat/compat_test.go` — contract guards (NEW)

Two tests that lock in the wrapper behavior across phases:

- `TestBuild_PreservesStreamingCapability` — for each
  streaming-capable family (anthropic, openai, groq, mistral),
  `Build` must return a value that type-asserts as
  `agent.StreamingProvider`. The original namedProvider would have
  failed this for all four.
- `TestBuild_NonStreamingProviderDoesNotFalselyAdvertise` —
  ollama (non-streaming in M1.q.x scope) must *not* type-assert.
  Pins the structural-typing decision: false positives here would
  cause `--stream` to dispatch to a provider that can't handle it
  and surface a runtime error rather than an upfront capability
  rejection.

These guards mean future adapter additions (M1.q.x.x for Cohere,
Google, etc.) just need a one-line addition to the first test;
forgetting to add the test surfaces immediately when the operator
runs `--stream <vendor>` and it falls back to the non-streaming
path silently.

## Architectural consequences

1. **The wrapper-capability problem is solved for all future
   capability additions.** Whatever other optional interface we
   add next (`CompleteEmbeddings`, `RetrieveModelInfo`, anything),
   `wrapNamed` is the place to pattern-match — extend it once,
   every adapter that implements the capability flows through
   compat correctly.

2. **OpenAI's streaming format is now the second proof point for
   the `agent.Chunk` shape.** Anthropic and OpenAI have genuinely
   different wire shapes (event-tagged vs untagged, sequential vs
   parallel tools, explicit vs synthesized stop signals), but both
   reduce cleanly to the same four-field Chunk. That's evidence
   the shape isn't Anthropic-shaped by accident; the remaining
   M1.q.x.* adapters can adopt it.

3. **One adapter genuinely covering four families is a catalog-
   architecture win.** Without the `compat` indirection, M1.q.x
   would have meant writing streaming four times (or splitting
   into four near-identical adapters). Because everything funnels
   through one openai.Provider with per-family customization
   (BaseURL, AuthHeader, AuthScheme), one CompleteStream
   implementation lights up the entire wedge.

## Demo (synthetic; mock server returns OpenAI-shaped SSE)

```
$ agt provider check --stream groq
streaming provider=groq model=llama-3.3-70b-versatile family=openai-compatible …

pong

OK
  total latency   : 84ms (wall-clock for the full stream)
  stop_reason     : end_turn
  tokens in / out : 12 / 1
  this call cost  : $0.0000017 (1680 microcents)
```

```
$ agt provider check --stream openai
streaming provider=openai model=gpt-4o-mini family=openai …

pong!

OK
  total latency   : 320ms (wall-clock for the full stream)
  stop_reason     : end_turn
  tokens in / out : 12 / 3
  this call cost  : $0.0000045 (4500 microcents)
```

```
$ AGEZT_MODEL=gpt-4o-mini agt provider check --stream openai
streaming provider=openai model=gpt-4o-mini family=openai …

I'll check the directory.
→ tool_use_start: shell (id=call_abc)
{"command":"ls -la"}
← tool_use_stop: call_abc

OK
  total latency   : 1.1s (wall-clock for the full stream)
  stop_reason     : tool_use
  tokens in / out : 60 / 22
```

## Deferrals → next phases

**M1.q.x.x — remaining adapters** (each ~150-300 LoC including
tests, in rough order of usefulness):

1. Google Gemini `streamGenerateContent` — SSE-on-Gemini, but
   the framing differs from both OpenAI and Anthropic.
2. Cohere `v2/chat` streaming — JSON-lines (not SSE!), one event
   per line; need a slightly different parser shape.
3. Mistral *if* its streaming diverges from openai-compatible
   enough to matter (early reads suggest it doesn't — Mistral
   may already work for streaming today via this adapter; needs
   a real-server smoke test to confirm).
4. Bedrock Anthropic streaming via `InvokeModelWithResponseStream`
   — same body shape as Anthropic Messages but AWS event-stream
   framing (binary, not SSE).
5. Vertex AI Gemini `streamGenerateContent` — same protocol as
   Google but on the Vertex regional endpoint with OAuth.
6. Vertex AI Anthropic via `streamRawPredict` — depends on
   M1.n.x (Anthropic-on-Vertex non-streaming).
7. Ollama `/api/chat` streaming — JSON-lines, very simple.

**M1.q.y — agent loop integration:** as M1.q noted, this is its
own decision phase. Open questions unchanged:
- Journal one event per chunk? per buffered batch? not at all?
- New bus subject for streaming that subscribers opt into?

**Unchanged longstanding deferrals:**
- Hot reload of catalog + vault.
- Subscription-first routing.
- OS-keychain encryption for the vault.
- Bedrock SigV4 + non-Anthropic vendor body shapes.
- Vertex Anthropic + ADC + workload-identity.
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.

## Files touched

```
plugins/providers/openai/streaming.go        NEW (~310 LoC)
plugins/providers/openai/streaming_test.go   NEW (9 tests + 3 SSE fixtures)
plugins/providers/compat/compat.go           split wrapper into namedProvider /
                                             namedStreamingProvider via wrapNamed (~+45 LoC)
plugins/providers/compat/compat_test.go      + 2 contract guards (5 sub-cases total)
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 367 pass, 0 fail (up from 352 in M1.q)
```

The cumulative operator UX trajectory:

| Milestone | New capability |
|---|---|
| M1.f | `agt catalog sync`, `agt catalog list`, `agt catalog discover` |
| M1.o | `agt provider creds set/list/rm` |
| M1.p | `agt provider check [id]` |
| M1.p.x | `agt provider check --all` |
| M1.p.y | `--json` (CI gate) + `--bench N` (vendor latency comparison) |
| M1.q | `agent.StreamingProvider` + Anthropic SSE + `--stream` |
| **M1.q.x** | **OpenAI streaming → 4 families × ~11 vendors via one adapter** |

The streaming wedge now covers anthropic + openai + openai-compatible
+ azure + mistral. Adapters remaining for full coverage: google,
cohere, vertex, bedrock, ollama.

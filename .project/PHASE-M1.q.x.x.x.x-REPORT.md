# Phase Report — Milestone 1.q.x.x.x.x (Ollama + Cohere streaming)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §5 (Streaming)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.q.x.x.x-REPORT.md](PHASE-M1.q.x.x.x-REPORT.md).

## Scope

Two more streaming adapters land together — they were each small
enough that bundling avoids phase-report churn. Both also exercise
brand-new wire formats not seen in earlier streaming phases:

- **Ollama**: NDJSON (JSON-lines), not SSE. First non-SSE streaming
  adapter; proves the abstraction works for both transport shapes.
- **Cohere v2/chat**: SSE with Cohere-specific event names
  (`message-start`, `content-delta`, `tool-call-{start,delta,end}`,
  `message-end`) and a nested `delta.message.*` payload structure.

After this phase, **9 of 11 catalog families** stream. The two
remaining are aws-bedrock (needs AWS binary event-stream framing —
its own ~300 LoC phase) and google-vertex-anthropic (blocked on
M1.n.x).

| Concern | Status |
|---|---|
| Ollama NDJSON parser handles partial lines + final done:true | ✅ |
| Ollama tool calls (whole-arg) synthesize full lifecycle | ✅ |
| Ollama missing-ID case synthesizes deterministic ids | ✅ tested |
| Ollama `done_reason: length` maps to StopMaxTokens | ✅ tested |
| Cohere SSE parser handles all 6+ event names | ✅ |
| Cohere streamed tool args concatenate to valid JSON | ✅ tested |
| Cohere `tool-plan-delta` events safely ignored | ✅ documented |
| Both: garbage-frame tolerance | ✅ tested |
| Both: onChunk-error abort propagation | ✅ tested |
| Both: compile-time `agent.StreamingProvider` guard | ✅ |
| compat contract guards: cohere added to table; ollama dedicated; bedrock added as non-streaming guard | ✅ |
| Test coverage: 9 Ollama + 8 Cohere + 2 compat = 19 new tests | ✅ |

## Changes

### 1. `plugins/providers/ollama/streaming.go` (NEW, ~190 LoC)

NDJSON parsing: `bufio.Scanner` reads `\n`-delimited lines, each a
complete JSON object. No SSE prefixes, no event tags, no `[DONE]`.
The terminal frame has `"done": true` and carries
`prompt_eval_count` + `eval_count` as the usage signal.

Tool calls in Ollama arrive whole (entire `tool_calls` array in one
chunk, same as Gemini). The adapter synthesizes the full
`ToolUseStart → ToolInputJSONDelta → ToolUseStop` lifecycle from
that single chunk — the third confirmation that the synthesis
pattern is correct: it lets callers stay provider-agnostic without
losing fidelity to wire-streamed providers (Anthropic, OpenAI,
Cohere).

The `Accept: application/x-ndjson` header is set even though
Ollama doesn't require it. It makes the protocol intent explicit
in logs and proxies, costs nothing, and follows the same defensive
header convention the SSE adapters use with `Accept:
text/event-stream`.

### 2. `plugins/providers/cohere/streaming.go` (NEW, ~290 LoC)

SSE with Cohere's typed events. The event names are documented and
stable, and the payload structure consistently nests under
`delta.message.{content|tool_calls}` — so the parser dispatches on
event name and unmarshals the data into purpose-built local structs.

Event handling:

| event | what it does |
|---|---|
| `message-start` | nothing (id/role known) |
| `content-start` | nothing (open block; deltas follow) |
| `content-delta` | emit `TextDelta` + accumulate |
| `content-end` | nothing |
| `tool-plan-delta` | **ignored** — see note below |
| `tool-call-start` | open tool state; emit `ToolUseStart` |
| `tool-call-delta` | append args fragment; emit `ToolInputJSONDelta` |
| `tool-call-end` | emit `ToolUseStop` |
| `message-end` | capture `finish_reason` + `usage.tokens.{input,output}` |

About `tool-plan-delta`: Cohere streams a "tool plan" rationale
before issuing tool calls (Cohere's name for chain-of-thought
reasoning during tool selection). It doesn't fit the existing
`Chunk` lifecycle, and no caller has asked for it. Surfacing it
would require a new `Chunk` field like `Thought` or `ReasoningDelta`
— a real abstraction question best left for a phase where multiple
adapters expose similar signals (Claude has extended thinking,
DeepSeek-R1 has its own reasoning format). Today, the events are
silently consumed and ignored, with a code comment explaining the
deferral.

Streaming tool args concatenate to valid JSON (verified by
`TestCompleteStream_Cohere_AssembledInputIsValidJSON`'s round-trip
through `json.Unmarshal`).

### 3. `plugins/providers/compat/compat_test.go` — guard updates

- `cohere` added to `TestBuild_PreservesStreamingCapability` table
  (now 6 streaming providers in the table).
- `TestBuild_OllamaPreservesStreamingCapability` added as a
  dedicated single-case test (Ollama has no env list, doesn't fit
  the table's `lookup := func(string) string { return "key" }`
  convention).
- `TestBuild_NonStreamingProviderDoesNotFalselyAdvertise` was
  removed (it tested Ollama as non-streaming, which is no longer
  true). Replaced with `TestBuild_BedrockDoesNotFalselyAdvertiseStreaming`
  — Bedrock still lacks streaming (binary event-stream parser is
  its own phase), so the inverse-direction guard now points at it.

The non-streaming guard exists specifically to prevent the silent
"wrapper falsely advertises capability" bug class. Keeping at least
one adapter on the failing side of the test until all adapters
stream means the guard stays load-bearing.

## Demo

```
$ agt provider check --stream ollama-local
streaming provider=ollama-local model=llama3.2 family=ollama …

pong!

OK
  total latency   : 240ms
  stop_reason     : end_turn
  tokens in / out : 12 / 3
```

```
$ agt provider check --stream cohere
streaming provider=cohere model=command-r-plus family=cohere …

pong!

OK
  total latency   : 410ms
  stop_reason     : end_turn
  tokens in / out : 12 / 3
```

## Architectural consequences

1. **Two transport shapes covered: SSE and NDJSON.** Ollama's
   NDJSON proves the streaming abstraction isn't SSE-specific — the
   only requirement is "a sequence of JSON objects arriving over a
   long-lived HTTP body." Any future adapter that uses chunked
   transfer with one-JSON-per-record works without interface
   changes.

2. **The "ignore unknown signals" stance is now an explicit
   pattern.** Cohere's `tool-plan-delta` joins a small list of
   provider-specific signals that the abstraction silently drops:
   - Anthropic: `ping` keep-alives
   - OpenAI: `stream_options` other than `include_usage`
   - Cohere: `tool-plan-delta` reasoning
   
   The rule: if the signal doesn't fit the existing `Chunk` shape
   AND no caller has asked for it, ignore it (with a code comment
   noting the deferral). Adding new `Chunk` fields preemptively
   would be premature abstraction — wait until multiple providers
   exercise the same need.

3. **The compat contract-guard test is now load-bearing.** With
   Ollama moving from "non-streaming" to "streaming" in this phase,
   the test file showed how easy it would be to silently drop a
   capability if someone added an adapter and forgot to test the
   wrapper passthrough. Promoting the guard to a list-driven test
   that fails on any new streaming adapter not added to the table
   keeps the wrapping decision honest.

## Streaming coverage now

| Family | Streaming? | Phase |
|---|---|---|
| anthropic | ✅ | M1.q |
| openai | ✅ | M1.q.x |
| openai-compatible (Groq, Cerebras, SambaNova, Together, DeepInfra, Perplexity, Fireworks, xai, OpenRouter) | ✅ | M1.q.x |
| azure | ✅ | M1.q.x |
| mistral | ✅ | M1.q.x |
| google (gemini) | ✅ | M1.q.x.x |
| google-vertex (gemini) | ✅ | M1.q.x.x.x |
| **ollama** | ✅ | **M1.q.x.x.x.x** |
| **cohere** | ✅ | **M1.q.x.x.x.x** |
| aws-bedrock (anthropic) | ❌ | needs AWS event-stream framing (binary, ~300 LoC) |
| google-vertex (anthropic) | ❌ | depends on M1.n.x |

9 of 11 streaming. The remaining two are both legitimate larger
lifts, not just adapter mechanics.

## Deferrals → next phases

The remaining streaming work splits cleanly:
- **Bedrock streaming** — needs the AWS event-stream binary
  framing decoder. Its own phase.
- **Vertex Anthropic** — depends on M1.n.x.

But the more impactful next phase is no longer adapter-by-adapter —
it's the **M1.q.y agent loop integration** that was deferred from
M1.q. Streaming providers exist now; nothing in the daemon's run
path uses them. Wiring `agent.Run` to detect `StreamingProvider`
and emit `KindLLMToken` events to the bus is the work that turns
this whole wedge from "checkable via `agt provider check --stream`"
into "live token output during real `agt run` invocations." That's
the next phase.

**Unchanged longstanding deferrals:**
- Hot reload of catalog + vault.
- Subscription-first routing.
- OS-keychain encryption for the vault.
- Bedrock SigV4 + non-Anthropic vendor body shapes.
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.

## Files touched

```
plugins/providers/ollama/streaming.go        NEW (~190 LoC)
plugins/providers/ollama/streaming_test.go   NEW (9 tests + 2 NDJSON fixtures)
plugins/providers/cohere/streaming.go        NEW (~290 LoC)
plugins/providers/cohere/streaming_test.go   NEW (8 tests + 2 SSE fixtures)
plugins/providers/compat/compat_test.go      + cohere in table, +ollama dedicated test,
                                             swap ollama→bedrock in non-streaming guard
```

No schema changes. No daemon-side changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 411 pass, 0 fail (up from 392 in M1.q.x.x.x)
```

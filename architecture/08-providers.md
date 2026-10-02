# 08 — Providers, provider boot, plugin SDK, MCP bridge

**Scope:** `plugins/providers/*` (anthropic, bedrock, cohere, compat, embed, google, image, ollama, openai,
openairesponses, rerank, vertex, voice, mock, internal/httpread, internal/provopts, internal/retry,
internal/toolname), `plugins/providerboot`, `plugins/sdk` (+ `example/greet`), `plugins/external/mcpbridge`.

Sibling docs: [00-README.md](00-README.md) · [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md) (who calls
`providerboot.Boot`/`Reload`, AWS credential chain, daemonconfig) · [04-agent-runtime.md](04-agent-runtime.md)
(`kernel/agent` loop, middleware, GenerateObject) · [05-governance-routing-security.md](05-governance-routing-security.md)
(governor routing/chains/budgets, catalog, creds, sigv4, chatgptauth, netguard) ·
[07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md) (kernel/plugin host, kernel/mcp in-kernel MCP,
voicetool/imagetool/reranktool) · [02-cli-cmd-agt.md](02-cli-cmd-agt.md) (`agt check`, `agt plugin new`) ·
[12-sdks-and-contract.md](12-sdks-and-contract.md) (client SDKs — not the plugin SDK).

---

## Responsibilities at a glance

| Area | What it does |
|---|---|
| **LLM wire adapters** (`anthropic`, `openai`, `openairesponses`, `google`, `vertex`, `bedrock`, `cohere`, `ollama`) | Translate the canonical, dialect-free `agent.CompletionRequest` / `agent.CompletionResponse` (defined in `kernel/agent`) to/from one vendor wire format. Each implements `agent.Provider`; all except `openairesponses` also implement `agent.StreamingProvider`. |
| **`compat`** | The *only* factory that turns a `catalog.Provider` entry (models.dev-derived) + model id + credential lookup into a live `agent.Provider`, dispatching on `catalog.Family`. Wraps the result so `Name()` = catalog provider id. |
| **`providerboot`** | Daemon provider bootstrap: primary selection (`AGEZT_PROVIDER`), alternate registration (every other credentialed catalog provider + ChatGPT), middleware stack (M997), governor construction from env, and hot reload — Boot and Reload share one registration path. Also ChatGPT model discovery + catalog seeding. |
| **Shared internals** (`internal/httpread`, `retry`, `provopts`, `toolname`) | Bounded body reads (64 MiB), exponential-backoff retry honouring `Retry-After`, `Params`/`ProviderOptions` application helpers, injective tool-name sanitisation + reversal. |
| **Non-chat modality clients** (`embed`, `image`, `rerank`, `voice`) | OpenAI-compatible (plus native ElevenLabs/Deepgram/Cartesia for voice) clients that satisfy *kernel-side seams structurally* (memory.Embedder, runtime ImageGen/Reranker/Voice) without the kernel importing them. Constructed in `cmd/agezt/main.go` from `AGEZT_EMBED_*`, `AGEZT_IMAGE_*`, `AGEZT_RERANK_*`, `AGEZT_STT_*`/`AGEZT_TTS_*`. |
| **`mock`** | Scripted/responder offline provider. Used by ~155 test files across the repo and by the explicit `AGEZT_DEMO_ECHO=1` demo path. |
| **`plugins/sdk`** | Stdlib-only Go kit for out-of-process *tool* plugins speaking the kernel's line-delimited JSON plugin protocol (initialize / tool/invoke / shutdown / host/invoke / progress). |
| **`plugins/external/mcpbridge`** | Standalone binary: an agezt plugin (plugin protocol on its stdio) that fronts one MCP server (JSON-RPC 2.0 over stdio or legacy HTTP+SSE). Predates and is superseded for most uses by the in-kernel `kernel/mcp`. |

Layering: everything here sits **above** the kernel. `plugins/providers/*` import `kernel/agent` (types),
`kernel/catalog` (compat only), `kernel/netguard` (embed, voice, openairesponses), `kernel/creds/sigv4` (bedrock).
The kernel never imports any of these packages (verified: only `cmd/agezt`, `cmd/agt` and test files import them).
`plugins/sdk` and `plugins/external/mcpbridge` import nothing from the kernel except `mcpbridge → kernel/netguard`.

---

## 1. The provider contract (defined in `kernel/agent`, not here)

All adapters target the contract in `kernel/contract/llm` (aliased in `kernel/agent/agent.go` since W1.1) plus `kernel/agent` (`middleware.go`,
`generate.go`). Details of the agent loop are in 04; the wire-relevant contract:

```go
// kernel/agent/agent.go:194
type Provider interface {
    Name() string
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}
// kernel/contract/llm/llm.go (was kernel/agent/streaming.go)
type StreamingProvider interface {
    Provider
    CompleteStream(ctx, req CompletionRequest, onChunk func(Chunk) error) (*CompletionResponse, error)
}
```

### CompletionRequest (fields an adapter reads vs. Governor-only hints)

| Field | Read by adapters? | Notes |
|---|---|---|
| `Model` | yes | Falls back to the adapter's `Model` field; empty in both → `ErrNoModel` (owner rule: no default model). |
| `System` | yes | Top-level system prompt; each dialect places it (Anthropic `system` block array w/ cache_control, OpenAI first `system` message, Gemini `systemInstruction`, Responses `instructions` appended after Codex/base instructions). |
| `Messages []Message` | yes | Roles `system|user|assistant|tool`. `Message.Images []string` = RFC-2397 `data:` URLs (vision). `ToolCalls []ToolCall{ID,Name,Input json.RawMessage}`; tool results are `Role=tool` + `ToolCallID`. |
| `Tools []ToolDef` | yes | `Name`, `Description`, `InputSchema`. `Effect`/`Capability` are `json:"-"` governance metadata and never reach the wire. |
| `MaxTokens` | yes | 0 → adapter default (Anthropic/Bedrock/Vertex-Anthropic 4096; others omit). |
| `JSONMode` | yes (native-JSON families) | OpenAI `response_format:{type:json_object}`, Gemini/Vertex `responseMimeType`, Ollama `format:"json"`; ignored elsewhere. |
| `Params` | yes | Universal sampling knobs: `Temperature, TopP, TopK, Stop, Seed, FrequencyPenalty, PresencePenalty, ReasoningEffort` ("minimal|low|medium|high"). `Params.IsZero()` ⇒ wire byte-identical. |
| `ProviderOptions map[string]json.RawMessage` | yes | Each adapter reads **its own family key** and shallow-merges the JSON object over the marshalled body (`provopts.Merge`). |
| `TaskType`, `ModelChain`, `Agent`, `AgentDailyCeilingMc`, `CorrelationID` | **no** | Governor-only routing/budget hints. |

### CompletionResponse / Chunk

* `Message` (role assistant; `Content` + `ToolCalls`), `StopReason` (`end_turn|tool_use|max_tokens`),
  `Usage{InputTokens, OutputTokens, CachedInputTokens, CacheWriteInputTokens, Model}`, `ReasoningContent`.
* Usage invariant: `InputTokens` is the **total** prompt size including cached parts (Anthropic-family adapters sum
  `input + cache_read + cache_creation`); governor prices cached/cache-write subsets separately.
* `Chunk` carries exactly one of `TextDelta | ToolUseStart *ToolCall | ToolInputJSONDelta | ToolUseStop (id) |
  ReasoningDelta`. Streaming invariant: `CompleteStream` must return the same assembled response `Complete` would; chunks
  are display-only. Providers that receive whole tool calls (Gemini, Vertex, Ollama) **synthesise** the
  Start→JSONDelta→Stop triple.

### Composition helpers used by providerboot (defined in kernel/agent)

* `agent.Wrap(p, mws...)` (`middleware.go`) — applies `Middleware{TransformRequest, WrapComplete, WrapStream,
  SynthesizeStream}`; returns a `StreamingProvider` only if `p` streams or a middleware synthesises.
* `agent.DefaultParamsMiddleware`, `agent.ExtractReasoningMiddleware("<think>","</think>")`,
  `agent.SimulateStreamingMiddleware` — wired by `providerboot.Middleware`.
* `agent.GenerateObject(ctx, p, req, schema, out)` (`generate.go`) — structured output for *every* provider: sets
  `JSONMode`, appends schema to system prompt, extracts/validates JSON, up to `DefaultObjectRepairs=2` repair turns.
  This is why adapters without native JSON mode still work for structured output.

The governor (`kernel/governor`, see 05) holds providers in `governor.Registry` as
`ProviderInfo{Name, Provider, AuthMode(subscription|api-key|local), IsFallback, Models []string}`.
`Register` rejects duplicates, `Replace` upserts preserving order, `Remove` deletes; both check
`info.Provider.Name() == info.Name`.

---

## 2. Adapter comparison matrix

Legend: ✓ supported, — not supported/ignored. "DoHTTP" = `retry.DoHTTP` (shared), "Do" = hand-rolled loop around
`retry.Do` with identical semantics. All stream setups use `retry.DoHTTPStream` (setup retried, mid-stream never).

| Adapter (`Name()`) | Wire API / endpoint | Auth | Streaming | Tool calling | Structured output (`JSONMode`) | Reasoning / thinking | Images | `Params` honoured | `ProviderOptions` key | Retry (Complete) |
|---|---|---|---|---|---|---|---|---|---|---|
| `anthropic` (`"anthropic"`) | Messages API, `POST {BaseURL}/messages` (default `https://api.anthropic.com/v1/messages`), `anthropic-version: 2023-06-01` | `x-api-key` | SSE (`message_start`, `content_block_*`, `message_delta`, `message_stop`, `ping`, `error`); 1 MiB frame cap | ✓ `tool_use`/`tool_result` blocks; tool results sent as `user` messages; last tool + system block carry `cache_control: ephemeral` (prompt caching M299/M301) | — (prompt fallback) | Extended thinking: `ThinkingBudget` (env `AGEZT_ANTHROPIC_THINKING_BUDGET`) or `Params.ReasoningEffort`→budget (1024/2048/8192/16384); `max_tokens` bumped above budget; `thinking` blocks → `ReasoningContent`, stream `ReasoningDelta` | ✓ base64 `image` blocks before text | temperature, top_p, top_k, stop_sequences (no seed/penalties) | `anthropic` | Do |
| `openai` (`"openai"`) — also serves OpenAI-compatible, Mistral, Azure | Chat Completions, `{BaseURL}[/v1]/chat/completions` (default `https://api.openai.com/v1/chat/completions`) | `Authorization: Bearer` (configurable `AuthHeader`/`AuthScheme`; Azure = `api-key` raw) | SSE `data:` frames, `stream_options.include_usage=true`, tool calls correlated by `index` | ✓ `type:function` tools; empty schema → `{"type":"object","properties":{}}` | ✓ `response_format: json_object` (no json_schema, for compat vendors) | `reasoning_effort` pass-through (normalised); reads `reasoning_content` (DeepSeek-R1) or `reasoning` on response + stream | ✓ content-parts `image_url` (data: or http(s) URL) | temperature, top_p, stop, seed, frequency/presence penalty, reasoning_effort (no top_k) | `openai` | Do |
| `openairesponses` (`ID`, `"chatgpt"` in prod) | OpenAI **Responses** API on ChatGPT backend `https://chatgpt.com/backend-api/codex/responses` (unofficial, Codex CLI wire) | OAuth bearer from `TokenFunc` + `chatgpt-account-id`, `OpenAI-Beta: responses=experimental`, `originator: codex_cli_rs`, random `session_id` | Request is always `stream:true` but the body is **buffered** (16 MiB) and parsed after; **not** a `StreamingProvider` | ✓ `function` tools (`strict:false`, `tool_choice:auto`, `parallel_tool_calls:false`), `function_call`/`function_call_output` items | — | `reasoning.effort` (default `"medium"`, overridden by `Params.ReasoningEffort`); no reasoning text returned | — (images dropped) | temperature, top_p only | `openai` (shared with Chat Completions!) | DoHTTP + one forced token refresh on 401 |
| `google` (`"google"`) | Generative Language API `v1beta/models/{model}:generateContent` | `x-goog-api-key` header | SSE via `:streamGenerateContent?alt=sse`; whole tool calls → synthesized chunk triple | ✓ `functionDeclarations`; IDs synthesised `call-<partIndex>`; tool results as `functionResponse{name: ToolCallID surrogate, response:{"result":…}}` | ✓ `generationConfig.responseMimeType=application/json` | `thinkingConfig{includeThoughts, thinkingBudget}` via `AGEZT_GOOGLE_THINKING_BUDGET` (-1 = dynamic) or ReasoningEffort; `thought:true` parts → reasoning; thoughtsTokenCount folded into output tokens | ✓ `inlineData` | temperature, topP, topK, stopSequences (inside generationConfig) | `google` | DoHTTP |
| `vertex` (`"google-vertex"`) | Gemini: `https://{loc}-aiplatform.googleapis.com/v1/projects/{p}/locations/{loc}/publishers/google/models/{m}:generateContent`; **`claude-*` models** dispatch to `publishers/anthropic/models/{m}:rawPredict` / `:streamRawPredict` with Anthropic body (`anthropic_version: vertex-2023-10-16`) | OAuth2 bearer from `TokenMinter`: service-account JWT-bearer (`TokenSource`) or GCE/GKE metadata server (`MetadataTokenSource`) | ✓ both publishers (`:streamGenerateContent?alt=sse`, `:streamRawPredict` SSE) | ✓ (Gemini shape / Anthropic shape) | ✓ Gemini path; ignored on claude path | Gemini thinkingConfig (`AGEZT_GOOGLE_VERTEX_THINKING_BUDGET`) / Anthropic thinking budget (negative = off), both map ReasoningEffort | ✓ both paths | same as google / anthropic | `vertex` | DoHTTP |
| `bedrock` (`"bedrock"`) | `POST https://bedrock-runtime.{region}.amazonaws.com/model/{id}/invoke`; vendor body chosen by model id: anthropic, mistral, cohere, meta (Llama 3 prompt template), ai21.jamba, amazon.nova, deepseek (R1 template); Titan/AI21-J2 intentionally unwired | `Authorization: Bearer AWS_BEARER_TOKEN_BEDROCK` **or** SigV4 (`service=bedrock`, re-signed per retry attempt) | AWS binary event-stream (`/invoke-with-response-stream`, base64 inner Anthropic events; CRC not validated); **anthropic.* only** — other vendors return `ErrVendorUnsupported` | ✓ Anthropic-on-Bedrock only (Mistral/Cohere/Nova/Llama: no tools) | — | Anthropic: thinking via ReasoningEffort (request only — **thinking blocks not decoded**); DeepSeek-R1: `<think>` → ReasoningContent | ✓ Anthropic path | per-vendor applyParams (temperature/top_p etc.) | `bedrock` (one key for all vendors) | DoHTTP; usage falls back to `X-Amzn-Bedrock-*-Token-Count` headers |
| `cohere` (`"cohere"`) | Cohere v2 `POST {base}/v2/chat` | Bearer | SSE with named events (`streaming_dispatch.go`) | ✓ OpenAI-like tool calls | — | — | — | temperature, `p`, `k`, seed, stop_sequences, frequency/presence penalty | `cohere` | DoHTTP |
| `ollama` (`"ollama"`) | `POST {base}/api/chat` (default `http://localhost:11434/api/chat`) | none | NDJSON (not SSE), `done:true` final line carries counts | ✓ OpenAI-flavoured; IDs synthesised `call-<i>`; **no toolname sanitisation** | ✓ `format:"json"` | — (use `AGEZT_EXTRACT_REASONING` middleware for inline `<think>`) | ✓ raw base64 `images` | `options{num_predict, temperature, top_p, top_k, seed, stop}` | `ollama` | Do |
| `mock` (`"mock"`) | none | none | — | scripted | — | — | — | — | — | — |

Common to every chat adapter: model id required (request or field), `httpread.All` 64 MiB body cap, `toolname`
forward-map on encode + `toolname.RestoreCalls` on decode (except ollama), `APIError{Status, Body}` per package for
non-2xx after retries, `netout.OperatorClient` with 5-min timeout (ollama 10 min): http.DefaultTransport behaviour
(proxy, pooling) on a shared transport whose dialer refuses link-local / cloud metadata (W1.4; previously a plain
client that would dial 169.254.169.254). `openairesponses` uses a strict netguard client; `embed`/`voice` netguard
with loopback+private allowed.

### Non-chat modality clients

| Package | Endpoint | Auth | Seam it satisfies | Env (read in cmd/agezt / daemonconfig) | HTTP client |
|---|---|---|---|---|---|
| `embed.Client` | `POST {base}[/v1]/embeddings` `{model, input[]}` | optional Bearer | `memory.Embedder` (`EmbedBatch` → L2-normalised `[][]float32`, ordered by `index`) | `AGEZT_EMBED_URL/MODEL/KEY` | netguard, loopback+private allowed |
| `image.Client` | `POST {base}[/v1]/images/generations`, `response_format=b64_json` | optional Bearer | runtime `ImageGen` (`GenerateImage(ctx,prompt,size,quality,n) ([][]byte,"image/png",error)`) | `AGEZT_IMAGE_URL/MODEL/KEY` | plain, 2 min |
| `rerank.Client` | `POST {base}/rerank` (or `/v1/rerank`; base ending `/rerank` used as is) `{model, query, documents, top_n}` | optional Bearer | runtime `Reranker` (`Rerank` → indices + scores) | `AGEZT_RERANK_URL/MODEL/KEY` | plain, 1 min |
| `voice.Adapter{STT, TTS}` | OpenAI: `/v1/audio/transcriptions` (multipart), `/v1/audio/speech`; ElevenLabs `/v1/text-to-speech/{voice}`, `/v1/speech-to-text` (`xi-api-key`); Deepgram `/v1/listen`, `/v1/speak` (`Token`); Cartesia `/tts/bytes` (`X-API-Key`, `Cartesia-Version: 2025-04-16`, TTS only) | per provider | runtime Voice seam (`Transcribe`, `Speak`) | `AGEZT_STT_*`, `AGEZT_TTS_*` (provider/url/model/voice/key); `daemonconfig` validates provider names | netguard, loopback+private allowed; 25 MiB audio cap |

---

## 3. Package-by-package

### 3.1 `plugins/providers/internal/httpread`

Purpose: bounded response body reader (M189). `All(body io.Reader, max int64) ([]byte, error)` reads
`max+1` via `io.LimitReader`; over-cap returns first `max` bytes + `ErrResponseTooLarge`. `DefaultMaxResponseBytes`
= 64 MiB (a `var` so tests can lower it). No deps. Used by every adapter + vertex token exchange + embed/image/rerank/voice.

| File | What it does |
|---|---|
| `httpread.go` | `All`, `DefaultMaxResponseBytes`, `ErrResponseTooLarge`. |

Tests: `httpread_test.go` (cap boundaries).

### 3.2 `plugins/providers/internal/retry`

Purpose: the one transient-error retry policy for provider HTTP (LD-4).
* `Config{MaxRetries=3, BaseDelay=500ms, MaxDelay=30s, Multiplier=2, Jitter=0.1}` (`DefaultConfig`).
* `Do(ctx, cfg, fn)` retries while `IsTransient(err)`, which returns true only for a `net.Error` whose `Timeout()` is
  true, or an `*HTTPError` with 429 or 5xx. **`*TransientError` (the wrapper every caller puts around `client.Do`
  failures) is never itself consulted** — it only unwraps. So a dial timeout retries, but connection-refused / reset /
  DNS failures do not, despite the comments ("connection failures … retry") in `http.go` and the adapters. Context cancel/deadline never retried. `Retry-After` (seconds or HTTP-date, parsed by
  `ParseRetryAfter`) is a **floor** on the next wait, capped at 2 minutes.
* `DoHTTP(ctx, client, build, maxBytes) (body, header, error)` — `build` runs **per attempt** (fresh SigV4 signature /
  fresh OAuth token); non-2xx → `*HTTPError` (via `NewHTTPError`, which parses `Retry-After`).
* `DoHTTPStream(ctx, client, build, maxErrBytes) (*http.Response, error)` — retries stream *setup* only; returns the
  first 2xx with body open. Post-2xx failures are never replayed (tokens may already have been surfaced).
* Helpers `RetryAfterOf`, internal `itoa`/`trimSpace`/`atoiNonNeg` (avoid strconv/strings imports).

Concurrency: stateless; `jitterDelay` uses `math/rand` global.

| File | What it does |
|---|---|
| `retry.go` | `Config`, `DefaultConfig`, `TransientError`, `IsTransient`, `HTTPError` (+`Transient()`), `NewHTTPError`, `ParseRetryAfter`, `RetryAfterOf`, `Do`, jitter. |
| `http.go` | `DoHTTP` (buffered, bounded) and `DoHTTPStream` (setup-only retry). |

Tests: `http_test.go`, `retryafter_test.go`, `retry_cover_test.go`.

### 3.3 `plugins/providers/internal/provopts`

Purpose: apply M997 per-request knobs.
* `Merge(body, extra json.RawMessage)` — top-level key overlay; empty `extra` returns the **same bytes** (default-preserving
  contract); otherwise re-marshals (key order changes).
* `ThinkingBudget(effort, maxTokens) (int, bool)` — minimal 1024 / low 2048 / medium 8192 / high 16384; clamps to
  `maxTokens-1`, disables if that drops under 1024. Used by anthropic, google, vertex (both), bedrock-anthropic.
* `NormalizeEffort` — validates the enum for OpenAI-dialect pass-through (`openai`, `openairesponses`).

| File | What it does |
|---|---|
| `provopts.go` | `Merge`, `ThinkingBudget`, `NormalizeEffort`. |

Tests: `provopts_test.go`, `provopts_cover_test.go`.

### 3.4 `plugins/providers/internal/toolname`

Purpose: make tool names pass every vendor's validator (`[a-zA-Z0-9_-]`, first char letter/`_`, ≤64) while staying
**injective**. Agezt tool names like `browser.read` or long `mcp_<server>_<tool>` would otherwise 400 and kill that
provider's arm of a fallback chain.
* `Sanitize(name)` — replace bad runes with `_`, prefix `_` if needed, cap 64; never "".
* `Maps(tools) (fwd, rev)` — collisions get `_2`, `_3`… (base truncated to 60 to leave room); `rev` only holds changed names.
* `Wire(fwd, name)` — mapped name, or `Sanitize` for names not in the current tool set (history replay).
* `RestoreCalls(resp, rev)` / `Reverse(tools)` — rewrite response tool-call names back to originals.

Invariant: live stream chunks carry *wire* names (display only); dispatch uses the assembled, restored response.
Depends on `kernel/agent`.

| File | What it does |
|---|---|
| `toolname.go` | `Sanitize`, `Maps`, `Reverse`, `Wire`, `RestoreCalls`. |

Tests: `toolname_test.go`, `toolname_cover_test.go`; each adapter also has a `toolname_test.go`/`tool_name_test.go`.

### 3.5 `plugins/providers/anthropic`

Key types: `Provider{APIKey, Endpoint, BaseURL, Model, HTTP, ThinkingBudget}`, `New(apiKey)`, consts
`DefaultEndpoint`, `APIVersion="2023-06-01"`, `DefaultMaxTokens=4096`, `DefaultTimeout=5m`, `MinThinkingBudget=1024`;
errors `ErrNoAPIKey`, `ErrNoModel`, `*APIError`. Endpoint resolution: `Endpoint` > `BaseURL+"/messages"` (BaseURL
already contains `/v1`, @ai-sdk/anthropic convention — so third-party Anthropic-shaped vendors like MiniMax work) >
default. Prompt caching: system as one-element block array with `cache_control:ephemeral`; last tool also marked.
Usage mapping (`anthUsageToAgent`): total input = input + cache_read + cache_creation.

| File | What it does |
|---|---|
| `doc.go` | Package doc (partially stale: says non-streaming / OAuth later). |
| `anthropic_wire.go` | `Provider`, consts, `New`, `resolveEndpoint`, `Name`, errors, `Complete` (retry.Do loop, maps `*retry.HTTPError`→`*APIError`, restores tool names). Contains an unused pre-loop `http.NewRequestWithContext` (dead build kept from refactor). |
| `anthropic.go` | `encodeRequest` (thinking budget resolution, params, tools, messages, `provopts.Merge`), `parseImageDataURL`, `canonicalToAnth` (system skipped, images→`image` blocks, empty assistant → placeholder text block, tool results as user `tool_result`), `decodeResponse` (`text`/`thinking`/`tool_use`, stop-reason mapping incl. `stop_sequence`→end_turn). |
| `anthropic_dialect.go` | Wire structs `anthRequest`, `anthThinking`, `anthSystemBlock`, `anthTool`, `anthCacheControl`, `anthMessage`, `anthBlock`, `anthImageSource`, `anthResponse`; `applyParams`, `thinkingConfig`, `buildAnthSystem`, `buildAnthTools`, `anthUsageToAgent`. |
| `streaming.go` | `CompleteStream` (DoHTTPStream, `Accept: text/event-stream`), `encodeStreamRequest` (separate struct so non-stream wire stays byte-identical), `streamState`, `openBlock`, `parseStream` (event/data pairing, 1 MiB scanner, EOF without `message_stop` still assembles partial). |
| `streaming_event.go` | `dispatchSSEFrame` (per-event state machine; `thinking_delta`→`ReasoningDelta`, `input_json_delta`→`ToolInputJSONDelta`), `assembleResponse`. |

Tests: unit, `fuzz_test.go`, `streaming_malformed_test.go`, `limit_test.go` (64 MiB cap), `thinking_test.go`,
`vision_test.go`, `toolname_test.go`, coverage files.

### 3.6 `plugins/providers/openai`

Key types: `Provider{APIKey, Endpoint, BaseURL, Model, HTTP, AuthHeader, AuthScheme}`, `New`, `DefaultEndpoint`,
`ErrNoAPIKey`, `ErrNoModel`, `*APIError`. One adapter for families openai, openai-compatible (Groq, DeepSeek, xAI,
Together, OpenRouter, Fireworks, Cerebras, DeepInfra, Perplexity, Moonshot …), mistral, azure. Endpoint: `Endpoint` >
`BaseURL` (+`/chat/completions` if it already has `/v1`, else `/v1/chat/completions`) > default. Cached tokens =
`max(prompt_tokens_details.cached_tokens, prompt_cache_hit_tokens)` (OpenAI vs DeepSeek spelling, M887).

| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `openai.go` | `Provider`, `New`, `resolveEndpoint`, `Name`, errors, `Complete` (auth header/scheme defaulting; retry.Do loop). |
| `openai_dialect.go` | `oaRequest`, embeddable `oaParams` + `applyParams`, `oaResponseFormat` + `jsonObjectFormat`, `oaMessage` (polymorphic `Content any`, response-only `reasoning_content`/`reasoning`), `oaContentPart`, `oaImageURL`, `oaTextOrNil`, `oaContentText`, `isImageURL`, tool/response structs. |
| `openai_wire.go` | `encodeRequest`, `canonicalToOA`, `decodeResponse`, `cachedInputTokens`. |
| `streaming.go` | `CompleteStream`, `encodeStreamRequest` (`stream_options.include_usage`), `streamState`, `openTool` (indexed). |
| `streaming_sse.go` | `parseStream`, `dispatchSSEFrame` (text, reasoning deltas, indexed tool-call fragments, `[DONE]`), `assembleResponse`. |

Tests: unit, fuzz, `cached_usage_test.go`, `json_mode_test.go`, `limit_test.go`, `params_test.go`, `vision_test.go`,
streaming + `empty_response_test.go`.

### 3.7 `plugins/providers/openairesponses` (ChatGPT subscription — "Sign in with ChatGPT")

Purpose: use a ChatGPT Plus/Pro plan as a provider via the **unofficial** Codex backend
(`DefaultBaseURL = https://chatgpt.com/backend-api/codex`). Tokens come from `kernel/chatgptauth` (05).

* `Provider{ID, Model, BaseURL, Token TokenFunc, ReasoningEffort, Instructions map[string]string}`;
  `New(id, model, token)` sets `ReasoningEffort="medium"`. `TokenFunc func(ctx, force bool) (access, accountID, err)`.
* `Complete`: `buildBody` → `send(force=false)`; on **401** `send(force=true)` once (refresh applies to the first retry
  attempt only, so backoff rounds don't hammer the OAuth endpoint); non-2xx returns `"backend status N: <600 chars>"`;
  `parseSSE` assembles output from `response.output_text.delta`, `response.output_item.done`, `response.completed`
  (usage + fallback to terminal `output`), `response.failed`/`error`.
* Instructions: per-model `base_instructions` discovered from `/models`; fallback to vendored `instructions.md`
  (Codex prompt, Apache-2.0, `//go:embed`). `req.System` is appended after it. System-role messages become
  `developer` input items.
* **Model discovery** (`models.go`): `ListModels(ctx, base, token)` → `GET {base}/models?client_version=0.146.0`
  (`ClientVersion` — mandatory; the backend 400s without it and hides models whose `minimal_client_version` is newer),
  headers `originator`, `version`, 20 s timeout, 401 refresh-once. `ModelInfo{Slug, DisplayName, Visibility, Priority,
  ContextWindow, MaxContextWindow, DefaultReasoning, InputModalities, SupportedInAPI, ParallelTools, BaseInstructions}`;
  `Listed()` excludes `visibility=hide`; `ParseModels` drops slug-less and sorts by priority then slug.
* HTTP client: `httpClientFor` → `netguard.New().HTTPClient(timeout)` (SSRF-guarded; tests override).

| File | What it does |
|---|---|
| `doc.go` | Package doc + ToS/unofficial warning. |
| `openairesponses.go` | `codexInstructions` embed, consts (`betaHeader`, `originator`), `httpClientFor`, `TokenFunc`, `Provider`, `New`, `Name`, `base`, `session` (random hex per request), `Complete`, `send`, `client`, request structs `toolDef`/`reasoningField`/`reqBody`, `instructionsFor`, `buildBody` (merges `ProviderOptions["openai"]`). |
| `openairesponses_helpers.go` | `contentText`, `toInput` (message/function_call/function_call_output items), `toTools`. |
| `openairesponses_wire.go` | `sseEvent`, `respItem`, `respObj`, `parseSSE`, `sseError`. |
| `models.go` | `ClientVersion`, `ModelInfo`, `Listed`, `ListModels`, `ParseModels`, `getModels`. |
| `instructions.md` | Vendored Codex system prompt (~6.6 KB). |

Tests: `openairesponses_test.go`, `models_test.go`, `toolname_test.go`, coverage.

### 3.8 `plugins/providers/google` (Gemini, API key)

`Provider{APIKey, Endpoint, BaseURL, Model, HTTP, ThinkingBudget}`; `DefaultBaseURL`, `APIVersion="v1beta"`.
Endpoint `= base[/v1beta]/models/{model}:generateContent` (version segment not duplicated when base already has
`/v1beta` or `/v1`). Tool result encoding uses `json.Marshal` (not `strconv.Quote`) — control bytes in tool output used
to wedge the loop (M481). `generationConfig` emitted only if any of maxTok/JSON/thinking/params is set.

| File | What it does |
|---|---|
| `doc.go` | Package doc (stale: claims Vertex unsupported). |
| `google.go` | `encodeRequest`, `parseImageDataURL`, `canonicalToGemini` (role `model`, functionResponse surrogate name = `ToolCallID`), `decodeResponse` (first candidate only; `call-<i>` IDs; `thought` parts → reasoning; output = candidates + thoughts tokens). |
| `google_wire.go` | Consts, `Provider`, `New`, `resolveEndpoint`, `Name`, errors, `Complete` (DoHTTP), wire structs `gemini*`, `applyParams`. |
| `streaming.go` | `CompleteStream`, `resolveStreamEndpoint` (`:streamGenerateContent?alt=sse`), `parseStream` (thought parts → `ReasoningDelta`; tool calls → synthesized triple), `assembleResponse`. |

Tests: unit, fuzz, `json_mode_test.go`, `thinking_test.go`, `tool_result_internal_test.go`, `vision_test.go`,
`empty_response_test.go`, streaming, params.

### 3.9 `plugins/providers/vertex` (Vertex AI: Gemini + Anthropic publishers)

* `Provider{TokenSource TokenMinter, Project, Location, Endpoint, BaseURL, Model, HTTP, ThinkingBudget}`,
  `New(ts, project, location)`, `Name()="google-vertex"`, `ResolveEndpoint`, `ResolveStreamEndpoint`,
  `ResolveAnthropicEndpoint` (`:rawPredict`), `ResolveAnthropicStreamEndpoint` (`:streamRawPredict`).
* `isAnthropicModel` = case-insensitive `claude-` prefix → `completeAnthropic` / `completeStreamAnthropic`
  (`AnthropicVertexVersion="vertex-2023-10-16"`, `DefaultAnthropicMaxTokens`, `MinAnthropicThinkingBudget`).
* **Auth** (`TokenMinter{Token(ctx)}`):
  * `TokenSource` (service account): `LoadServiceAccountFile`/`ParseServiceAccountJSON` (type must be
    `service_account`; default `token_uri` `https://oauth2.googleapis.com/token`), `parsePrivateKey` (PKCS#8 then
    PKCS#1 RSA), `signJWT` (RS256, `kid`, `iss=client_email`, `scope=cloud-platform`, `aud=token_uri`, 1 h exp) →
    `exchange` (`grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer`). Cached under `sync.Mutex`, refreshed
    `TokenSkew=60s` before expiry; `expires_in<=0` → 3600.
  * `MetadataTokenSource`: `GET http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token`
    with mandatory `Metadata-Flavor: Google`; `ProjectID(ctx)` reads `/computeMetadata/v1/project/project-id`;
    1 MiB response cap; same cache/skew.
  * External/federated ADC is not implemented.

| File | What it does |
|---|---|
| `doc.go` | Package doc (scope note partly stale — streaming and Anthropic now exist). |
| `auth.go` | Consts (`CloudPlatformScope`, `JWTBearerGrantType`, `TokenSkew`), `ServiceAccountKey`, loaders, `parsePrivateKey`, `TokenMinter`, `TokenSource`, `NewTokenSource`. |
| `auth_jwt.go` | `b64url`, `signJWT`. |
| `auth_token.go` | `TokenSource.Token` (cached), `exchange`. |
| `metadata.go` | `MetadataTokenSource`, `NewMetadataTokenSource`, `Token`, `fetchToken`, `ProjectID`, `get`. |
| `vertex.go` | Gemini `encodeRequest`, `canonicalToVertex` (M483 json.Marshal tool results), `decodeResponse`. |
| `vertex_wire.go` | `Provider`, `New`, `Name`, errors, `ResolveEndpoint`, `Complete` (anthropic branch, bearer token per attempt via DoHTTP). |
| `vertex_wire_types.go` | `vx*` wire structs + `applyParams`. |
| `streaming.go` | Gemini `CompleteStream` (+anthropic branch), `ResolveStreamEndpoint`, `parseStream`, `assembleResponse`. |
| `anthropic.go` | Anthropic-on-Vertex consts, `isAnthropicModel`, endpoint resolvers, `anthVertexRequest` & co., `anthVxThinkingConfig`, `buildVxSystem`, `buildVxTools`, `anthVxUsageToAgent`, `completeAnthropic`. |
| `anthropic_encode.go` | `encodeAnthropicOnVertexRequest` (ReasoningEffort→budget, `stream` flag), `parseImageDataURL`, `canonicalToAnthVx`. |
| `anthropic_decode.go` | Anthropic-on-Vertex response decode (incl. thinking → ReasoningContent). |
| `anthropic_stream.go` | `completeStreamAnthropic`, `parseAnthropicSSE`, `dispatchAnthropicSSE` (thinking → `ReasoningDelta`), `assembleAnthropicResponse`. |

Tests: `vertex_test.go`, `anthropic_test.go`, `anthropic_thinking_test.go`, `metadata_test.go`, json/params/thinking/
vision/tool-result/streaming/empty-response, coverage.

### 3.10 `plugins/providers/bedrock`

* `Provider{BearerToken, sigV4 *SigV4Creds (unexported; SetSigV4Creds), Endpoint, BaseURL, Region, Model, HTTP, Now}`,
  `New(bearer, region)`, `ResolveEndpoint(model)` (exported), errors `ErrNoBearerToken`, `ErrVendorUnsupported`,
  `*APIError`, const `AnthropicBedrockVersion="bedrock-2023-05-31"`.
* Auth: bearer wins if set; else SigV4 via `kernel/creds/sigv4.SignRequest` with service code `"bedrock"`
  (`sigv4.go` shim; `SigV4Creds = sigv4.Creds`). `applyAuth` runs inside the DoHTTP `build` closure → signature
  re-computed each attempt (date-scoped).
* Vendor dispatch by model id (prefix or `.vendor.` regional-profile segment: `us.anthropic.*`, `eu.mistral.*`, …).
* Usage fallback (M327): if decoded usage is zero, read `X-Amzn-Bedrock-Input-Token-Count` /
  `X-Amzn-Bedrock-Output-Token-Count` headers (Mistral/Cohere bodies carry no counts).

| File | What it does |
|---|---|
| `doc.go` | Package doc (stale: says bearer-only/Anthropic-only). |
| `bedrock.go` | Consts, `Provider`, `SetSigV4Creds`, `hasAuth`, `applyAuth`, `New`, errors, `Name`, `ResolveEndpoint`, `Complete` (vendor switch → encode/decode pair, DoHTTP, tool-name restore, header usage), `headerTokenCount`. |
| `bedrock_models.go` | `isAnthropicModel`, `isMistralModel`, `isCohereModel`, `isMetaLlamaModel`. |
| `bedrock_anthropic.go` | Anthropic-on-Bedrock wire types, `applyParams`, `thinkingConfig`, `buildBedrockSystem`, `buildBedrockTools` (cache_control), `encodeAnthropicOnBedrockRequest`, `decodeAnthropicOnBedrockResponse` (text + tool_use only). |
| `bedrock_anthropic_helpers.go` | `anthBedrockUsageToAgent`, `parseImageDataURL`, `canonicalToAnth`. |
| `mistral.go` | Mistral chat body (no tools), encode/decode. |
| `cohere.go` | Cohere Command R `message`/`chat_history` body (no tools). |
| `llama.go` | Meta Llama 3 prompt template (`llama3Template`), chat-only. |
| `ai21.go` | AI21 Jamba body + `isAI21JambaModel`. |
| `nova.go` | Amazon Nova messages/inferenceConfig body + `isAmazonNovaModel` (no tools). |
| `deepseek.go` | DeepSeek-R1 prompt template; splits `<think>` reasoning into `ReasoningContent`; `isDeepSeekModel`. |
| `sigv4.go` | `SigV4Creds` alias, `sigV4Service`, `signRequest`. |
| `streaming_event.go` | `CompleteStream` (anthropic.* only; same body, URL suffix `/invoke-with-response-stream`), `resolveStreamEndpoint`, AWS event-stream frame reader (`readEventStreamMessage`, `parseEventStreamHeaders`, `headerValue`). |
| `streaming.go` | `bedStreamState`, `bedOpenBlock`, `chunkPayload`, `parseEventStream`, `dispatchBedrockInnerEvent` (base64 inner Anthropic event), `assembleBedrockResponse` (cache read/creation tracked, M296). |

Tests: per-vendor tests, `sigv4_test.go` (AWS vectors via `Now`), `params_test.go`, fuzz, streaming, vision, toolname.

### 3.11 `plugins/providers/cohere`

`Provider{APIKey, Endpoint, BaseURL, Model, HTTP}`, `DefaultBaseURL="https://api.cohere.com"`, endpoint
`{base}/v2/chat`. Request close to OpenAI; response `message.content` is a typed-block array; usage nested at
`usage.tokens.{input,output}`; finish reasons `COMPLETE|STOP_SEQUENCE`→end_turn, `MAX_TOKENS`, `TOOL_CALL`.

| File | What it does |
|---|---|
| `cohere.go` | Package doc, `Provider`, `New`, `resolveEndpoint`, `Name`, `APIError`, `Complete` (DoHTTP), wire structs incl. `cohereParams` (`p`, `k`, seed, penalties). |
| `cohere_encode.go` | `encodeRequest`, `canonicalToCohere`. |
| `cohere_decode.go` | `decodeResponse`. |
| `streaming.go` | `CompleteStream`, `encodeStreamRequest`, `streamState`, `openTool`, `parseStream`, `assembleResponse`. |
| `streaming_dispatch.go` | `dispatchSSEFrame` (named Cohere stream events → chunks). |

### 3.12 `plugins/providers/ollama`

Local-model floor (DECISIONS C2). `Provider{Endpoint, BaseURL, Model, HTTP}`, `New()` (default
`http://localhost:11434/api/chat`, 10-min timeout), `ErrNoEndpoint`, `ErrNoModel`, `*APIError`. Sampling knobs live in
`options` (`buildOptions`); no penalties, no reasoning. Images are raw base64 (data-URL prefix stripped by
`ollamaImageData`).

| File | What it does |
|---|---|
| `ollama.go` | Package doc, `Provider`, `New`, `resolveEndpoint`, `Name`, errors, `Complete` (retry.Do loop), wire structs, `buildOptions`. |
| `ollama_encode.go` | `encodeRequest` (`format:"json"` for JSONMode), `ollamaImageData`, `canonicalToOllama`. |
| `ollama_decode.go` | `decodeResponse` (synthesised `call-<i>` IDs, `prompt_eval_count`/`eval_count` usage). |
| `streaming.go` | NDJSON `CompleteStream`, `encodeStreamRequest`, `streamState`, `parseStream` (whole tool_calls → synthesized triple), `assembleResponse`. |

### 3.13 `plugins/providers/compat` — catalog entry → live provider

`Build(p *catalog.Provider, modelID string, lookup CredLookup) (agent.Provider, string, error)`:

1. Reject nil entry / empty model / model not in `p.Models` (`ErrModelUnknown` — strict, no guessing).
2. Credentials: for each `p.Env` name, try `catalog.ProviderCredentialLookupNames(p.ID, name)` =
   `["provider:<id>:<ENV>", "<ENV>"]` (provider-scoped vault key first, then the bare/global name); first non-empty
   wins → `apiKey`. Local families (empty `Env`) skip this. None found → `ErrMissingCredentials`.
3. Base URL: catalog `api` > `defaultBaseURL(family)` (anthropic `…/v1`, openai `…/v1`, google, ollama
   `http://localhost:11434`, mistral, cohere) > for openai-compatible, `compatVendorBaseURL(npm)` (groq, xai, cerebras,
   togetherai, deepinfra, perplexity, fireworks, deepseek, moonshotai, openrouter).
4. Family switch (`p.Family()` = `catalog.FamilyFromNPM(p.NPM)`):

| Family | Adapter | Extra construction |
|---|---|---|
| `anthropic` | `anthropic.New` | `AGEZT_ANTHROPIC_THINKING_BUDGET` (>0) via lookup. |
| `ollama` | `ollama.New` | — |
| `google` | `google.New` | `AGEZT_GOOGLE_THINKING_BUDGET` (≠0; -1 dynamic). |
| `openai`, `openai-compatible` | `openai.New` | openai-compatible with empty base → `ErrFamilyUnsupported` (never silently hit api.openai.com). |
| `mistral` | `openai.New` | default base `https://api.mistral.ai/v1`. |
| `cohere` | `cohere.New` | — |
| `google-vertex` | `vertex.New` | `resolveVertexCreds`: `GOOGLE_APPLICATION_CREDENTIALS` (SA JSON) **or** `GOOGLE_VERTEX_USE_METADATA` truthy (+ optional `GOOGLE_VERTEX_METADATA_URL`), `GOOGLE_VERTEX_LOCATION` required, `GOOGLE_VERTEX_PROJECT` optional (→ SA `project_id` or metadata project-id, fetched with `context.Background()` at build time); `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET`. |
| `aws-bedrock` | `bedrock.New` | `resolveBedrockCreds`: `AWS_REGION`/`AWS_DEFAULT_REGION` required; `AWS_BEARER_TOKEN_BEDROCK` or `AWS_ACCESS_KEY_ID`+`AWS_SECRET_ACCESS_KEY` (+`AWS_SESSION_TOKEN`) → `SetSigV4Creds`. The AWS names can also be answered by the daemon's AWS chain (`~/.aws/*`, IMDSv2 — see 01). |
| `azure` | `openai.New` | `resolveAzureCreds` (`AZURE_RESOURCE_NAME`+`AZURE_API_KEY` or `AZURE_COGNITIVE_SERVICES_*`); URL `https://<resource>.openai.azure.com/openai/deployments/<PathEscape(model)>/chat/completions?api-version=<AGEZT_AZURE_API_VERSION or 2024-10-21>`; `AuthHeader="api-key"`, no scheme. |
| other (`unknown`) | — | `ErrFamilyUnsupported` with hint to set `npm: "openai-compatible"` in `custom.json`. |

5. `wrapNamed(p.ID, inner)` → `*namedProvider` (or `*namedStreamingProvider` iff inner streams, so a type assertion on
   the wrapper means exactly what it says). `Name()` = catalog id (e.g. `groq`, `ollama-local`).

Also: `IsSupportedFamily(f)` (the 10 wired families), `FirstModelID(p)` (alphabetically smallest — used as an
*inert construction placeholder*), `envLookup`, `providerEnvLookup`.

| File | What it does |
|---|---|
| `compat.go` | Package doc, errors, `CredLookup`, `Build` (family switch). |
| `compat_creds.go` | `resolveAzureCreds`, `vertexCreds` + `resolveVertexCreds`, `isTruthy`, `bedrockAuth` + `resolveBedrockCreds`. |
| `compat_helpers.go` | `IsSupportedFamily`, `envLookup`, `providerEnvLookup`. |
| `compat_urls.go` | `compatVendorBaseURL`, `defaultBaseURL`. |
| `compat_wrap.go` | `FirstModelID`, `namedProvider`, `namedStreamingProvider`, `wrapNamed`. |

Consumers: `plugins/providerboot`, `cmd/agt` (`check_all*.go`, `check_helpers.go`, `check_json.go` — `agt check`
builds providers directly to probe them). Tests: `compat_test.go`, `compat_m230/m232/m233_test.go`, coverage.

### 3.14 `plugins/providers/embed`, `image`, `rerank`, `voice`, `mock`

| Package / file | What it does |
|---|---|
| `embed/embed.go` | `Client{BaseURL, Model, APIKey, HTTP}`, `New`, `defaultHTTPClient` (netguard + loopback/private allowed), `endpoint`, `EmbedBatch` (index-ordered, re-L2-normalised since kernel cosine is a bare dot product). |
| `image/image.go` | `Client`, `New`, `HasImage`, `endpoint`, `GenerateImage` (b64_json → bytes, always reports `image/png`). Stdlib-only signature so the kernel seam is satisfied structurally. |
| `rerank/rerank.go` | `Client`, `New`, `HasRerank`, `endpoint` (`/rerank` suffix logic), `Rerank(ctx, query, docs, topN) ([]int, []float64, error)`. |
| `voice/voice.go` | `STTClient`, `TTSClient`, `STTBackend`, `TTSBackend`, `Adapter{STT,TTS}`, provider consts (`openai`, `elevenlabs`, `deepgram`, `cartesia`), `Config`, `NewSTT`, `NewTTS` (per-provider defaults: ElevenLabs `eleven_multilingual_v2`, Deepgram `aura-2-thalia-en`, Cartesia `sonic-3.5`), `HasSTT/HasTTS`, `Transcribe`, `Speak`, `endpoint`, `httpClient`, OpenAI multipart transcription + speech. |
| `voice/elevenlabs.go` | `elevenLabsTTS` (mp3_44100_128), `elevenLabsSTT` (multipart `/v1/speech-to-text`). |
| `voice/deepgram.go` | `audioContentType`, `deepgramSTT` (`/v1/listen?model=`), `deepgramTTS` (`/v1/speak?model=`). |
| `voice/cartesia.go` | `cartesiaTTS` (`/tts/bytes`, pinned `Cartesia-Version`). |
| `mock/mock.go` | `Provider{OnRequest, Responder}` (mutex-guarded scripted replay), `New(responses...)`, `ErrExhausted`, `CallCount`, `FinalText`. |

`voice` is also imported by `cmd/agezt/internal/daemonconfig` (validates `voice.Provider*` names).

---

## 4. `plugins/providerboot` — catalog + keyring → live registry

Imports: `internal/brand`, `kernel/agent`, `kernel/catalog`, `kernel/chatgptauth`, `kernel/governor`,
`plugins/providers/{compat,mock,openairesponses}`. Called only from `cmd/agezt` (`main.go`, `boot_providers.go`).
Lives under `plugins/` so the kernel never grows a kernel→plugins edge.

### Public surface

| Symbol | Purpose |
|---|---|
| `Deps{Catalog, Lookup, BaseDir, Get, Stderr}` | Inputs. `Lookup` = daemon's chained credential resolver (vault provider-scoped → vault legacy → env → `~/.aws` → IMDSv2; built in cmd/agezt `buildAWSCredChain(catalogScopedVaultLookup(...))`). `Get` nil → `os.Getenv`; `Stderr` nil → discard. |
| `Result{Governor, Primary, Model, Desc, AuthMode, Eligible func(id) bool}` | Output of `Boot`. |
| `Boot(d) (*Result, error)` | Build registry + governor. |
| `Reload(gov, d) (model string, error)` | Hot reload (`agt provider reload` / control-plane `provider_reload`). |
| `SelectPrimary(d)` | Primary selection. |
| `BuildFromCatalog(d, entry, modelOverride)` | compat.Build wrapper with model-placeholder logic. |
| `Eligible(entry, lookup)` | THE eligibility predicate: supported compat family **and** `entry.HasCredentials(lookup)`. Shared by registration, vision sidecar picker, keyed-model delegation predicate, council membership (cmd/agezt). |
| `Middleware(get) []agent.Middleware` | M997 opt-in stack. |
| `UnconfiguredName = "unconfigured"` | Sentinel primary name; the daemon's first-run nudge keys off `Result.Primary == UnconfiguredName`. |
| `SeedChatGPTCatalog(store, baseDir)`, `SyncChatGPTCatalog(store, baseDir) ([]string, string)` | Keep the `chatgpt` entry in `custom.json` in step with discovery. `SyncChatGPTCatalog` is injected into the control plane as the `ChatGPTSync` hook (sign-in status poll). |

### Boot flow

```
cmd/agezt main:
  catStore := catalog.NewStore(<home>/catalog)          (05)
  credStore.Load + EncryptInPlace                        (05)
  injectConfig (Config Center → process env)             (01)
  daemonconfig.Load                                      (01)
  providerboot.SeedChatGPTCatalog(catStore, baseDir)  ── may hit chatgpt /models
  cat := catStore.Load()
  credLookup := buildAWSCredChain(catalogScopedVaultLookup(cat, credStore.Lookup))
  providerboot.Boot(Deps{cat, credLookup, baseDir, stderr})

Boot:
 1. reg := governor.NewRegistry(); mw := Middleware(d.Get)
 2. SelectPrimary(d):
      AGEZT_PROVIDER == "chatgpt" → buildChatGPTPrimary (error if not signed in)      AuthSubscription
      AGEZT_PROVIDER == <id>      → cat.Providers[id] (unknown id = hard error) → BuildFromCatalog
      unset && AGEZT_DEMO_ECHO=1  → mock echo provider ("[echo] <last user msg>"), model "mock"   AuthLocal
      unset                       → unconfiguredProvider{} (every Complete fails with actionable msg)
 3. reg.Register(primary wrapped in agent.Wrap(primary, mw...), Models = catalog model ids)
 4. registerAlternates(reg, d, primaryName, mw, replace=false):
      for each catalog entry != primary with Eligible(): BuildFromCatalog(entry, "") → Register
        (AuthAPIKey, or AuthLocal when entry.Env empty); build errors skipped silently
      registerChatGPTAlternate (if signed in and not primary) → Models = discovered ids
      returns eligible set {catalogID: true}
 5. eligibleSet{m} stored in liveEligible (sync.Map keyed by *Registry)
 6. governorConfigFromEnv(get)  — hard error on malformed values (boot only)
 7. altFinder = cat.ToolCapableAlternative, or (cross down-route) cat.ToolCapableAlternativeAmong(model, es.has)
 8. governor.New(Config{Registry, ResponseCacheTTL, DailyCeilingMicrocents=DefaultDailyCeilingMicrocents,
        RateLimitPerMin, TaskRoutes, TaskRouteRequires, TaskModelOverrides, TaskModelChains, FallbackChains,
        DefaultChain, TaskBudgets, StrictModelCapabilities, StrictPricing, DownRouteToolModels,
        ModelToolCapable (cat.FindModel→ToolCall), ToolCapableAlternative, ModelJSONNative
        (catalog.FamilySupportsNativeJSONMode), ModelStrictToolArgsNative})
 9. Result{Desc: "primary=…, daily_ceiling=$…, [strict-capabilities], [tool-downrouting(cross)],
           model-routable_alternates=N, task_routes=N, task_budgets=N"}
```

`BuildFromCatalog` model logic (boot-resilient-config law): run model = `AGEZT_MODEL` (may be ""). compat needs a
catalog-valid id, so with no override it constructs with `compat.FirstModelID` as an **inert placeholder** (never
surfaced as default — governor returns `ErrNoModelConfigured` if no route/chain resolves a model). If `AGEZT_MODEL`
is not in this provider's catalog (it rides a chain on another provider), a warning is printed and the placeholder is
used for construction instead of failing boot.

### Governor env knobs parsed here (`providerboot_config.go`, boot only)

`AGEZT_RATE_PER_MIN`, `AGEZT_TASK_ROUTES`, `AGEZT_TASK_ROUTE_REQUIRES`, `AGEZT_TASK_MODEL_OVERRIDES`,
`AGEZT_TASK_MODEL_CHAINS`, `AGEZT_FALLBACK_CHAINS`, `AGEZT_DEFAULT_CHAIN`, `AGEZT_TASK_BUDGETS`, `AGEZT_MODEL_STRICT=on`,
`AGEZT_PRICING_STRICT=on`, `AGEZT_MODEL_DOWNROUTE=on`, `AGEZT_MODEL_DOWNROUTE_CROSS=on` (implies downroute),
`AGEZT_LLM_CACHE_TTL=<duration>`. Parsing helpers live in `kernel/governor` (`ParseTaskRoutesEnv`, …).
Other env read in this package: `AGEZT_PROVIDER`, `AGEZT_MODEL`, `AGEZT_DEMO_ECHO`, `AGEZT_GEN_TEMPERATURE`,
`AGEZT_GEN_TOP_P`, `AGEZT_GEN_REASONING_EFFORT`, `AGEZT_EXTRACT_REASONING`, `AGEZT_SIMULATE_STREAMING`
(on|1|true|yes). Thinking-budget/Azure/Vertex/Bedrock env vars are read by compat **through `Lookup`** (so vault
entries count).

### Reload flow (ordering is load-bearing)

```
cmd/agezt reload closure: credStore.Load(); redactor.SetSecrets(...); c := catStore.Load()
  model := providerboot.Reload(gov, providerDeps(c, credStore, baseDir, stderr)); k.SetModel(model)

Reload:
 1. mw := Middleware(d.Get)                      (env re-read → middleware follows live config)
 2. prov := SelectPrimary(d)                      (errors surfaced, not swallowed)
 3. if prov != sentinel: reg.Remove("unconfigured")   (M816: Replace of a different name would APPEND behind the sentinel)
 4. registerAlternates(reg, d, prov.Name(), mw, replace=true)
      Replace semantics + stale-drop sweep: remove non-fallback entries no longer eligible
 5. liveEligible[reg].set(eligible)              (cross-provider down-route sees the new set)
 6. gov.Replace(primary) LAST — rebuilds governor's cached routing chains over the reconciled registry
 7. return model → caller k.SetModel(model)      (M816: else runs keep the old model id)
```

Deliberate: governor env knobs (rate limit, task routes, strict flags, cache TTL…) are **not** re-read on reload
(a malformed live edit must not break reload); task-model chains have their own live path (control plane).

### ChatGPT subscription path

```
Sign-in (control plane / Setup, kernel/chatgptauth — 05): tokens JSON stored in vault key AGEZT_CHATGPT_OAUTH
  (or imported from ~/.codex/auth.json / $CODEX_HOME/auth.json)
     │
SyncChatGPTCatalog (every sign-in status poll) / SeedChatGPTCatalog (boot)
     │  resolveChatGPTModels(mgr):
     │    memo (authoritative only, TTL 6h) ─hit→ return
     │    1. discoverChatGPTModels → openairesponses.ListModels(DefaultBaseURL, chatgptTokenFn(mgr))   source "backend"
     │    2. chatgptModelsFromCLICache(dir(DefaultCodexAuthPath)/models_cache.json)                     source "codex-cli-cache"
     │    3. builtinChatGPTModelSet (chatgptFallbackModels, default gpt-5.6-sol; never memoised)        source "builtin"
     │  chatgptModelSetFrom: Listed() ids in backend priority order; Instructions kept even for hidden models
     ▼
writeChatGPTEntry → catalog.Store.UpsertCustomProvider(chatgptCatalogEntry(set))
   entry: ID "chatgpt", Env [AGEZT_CHATGPT_OAUTH] (so HasCredentials == signed in), API = codex base,
   NPM "" ⇒ FamilyUnknown ⇒ compat/registerAlternates' Eligible() skip it; models ToolCall+Reasoning=true,
   context from max_context_window, vision from input_modalities.
   Builtin set never overwrites an existing entry; identical id set ⇒ no write.
     ▼
Boot/Reload: AGEZT_PROVIDER=chatgpt → buildChatGPTPrimary; otherwise registerChatGPTAlternate
   provider = openairesponses.New("chatgpt", set.Default|AGEZT_MODEL, tokenFn) + Instructions = set.Instructions
   TokenFunc(force) → mgr.ForceRefresh(ctx) | mgr.Token(ctx); AuthMode = subscription
```

`SyncChatGPTCatalog` clears a non-backend memo first so a fresh sign-in upgrades from the CLI-cache source.
Hard-coding model ids is what broke this path historically; the builtin list is cosmetic.

| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `providerboot.go` | `Deps` (+`get`, `stderr`), `Result`, `Eligible`, `Middleware`, `registerAlternates` doc comment. |
| `providerboot_register.go` | `registerAlternates`, `SelectPrimary`. |
| `providerboot_catalog.go` | `catalogModelIDs`, `demoEchoProvider`, `BuildFromCatalog`. |
| `providerboot_runtime.go` | `Boot`, `Reload`. |
| `providerboot_config.go` | `govEnvConfig`, `governorConfigFromEnv`. |
| `providerboot_set.go` | `eligibleSet` (RWMutex map) + `liveEligible sync.Map`. |
| `providerboot_stub.go` | `UnconfiguredName`, `unconfiguredProvider`. |
| `chatgpt.go` | Fallback model snapshot, source labels, `chatgptCacheTTL`, `chatgptModelSet` (+`Authoritative`), memo cache, `resolveChatGPTModels`, `discoverChatGPTModels`, `chatgptCLICachePath`, `chatgptModelsFromCLICache`, `chatgptModelSetFrom`, `builtinChatGPTModelSet`, `chatgptCatalogEntry`. |
| `chatgpt_build.go` | `chatgptTokenFn`, `newChatGPTProvider`, `buildChatGPTPrimary`, `registerChatGPTAlternate`. |
| `chatgpt_seed.go` | `SeedChatGPTCatalog`, `SyncChatGPTCatalog`, `writeChatGPTEntry`, `sameModelIDs`. |

Concurrency: `eligibleSet` RWMutex (read by governor alt-finder on request goroutines, written by Reload);
`chatgptModelCache` mutex; registry has its own RWMutex. Persistence: writes only via `catalog.Store`
(`<AGEZT_HOME>/catalog/custom.json`, through `UpsertCustomProvider`) and, indirectly, the vault via chatgptauth token
refresh. No journal/bus events emitted here (the governor emits budget/rate/provider events — see 05).
Tests: `boot_test.go`, `reconcile_providers_test.go` (boot/reload parity, sentinel demotion, stale-drop),
`chatgpt_test.go`.

---

## 5. `plugins/sdk` — writing an out-of-process tool plugin

Stdlib-only (deliberately does not import `kernel/plugin` or `kernel/agent`, DECISIONS B0). Its wire structs are
independent copies of `kernel/plugin/protocol.go` (host side; see 07).

### Protocol (newline-delimited JSON over the plugin's stdin/stdout)

| Direction | Frame | Meaning |
|---|---|---|
| host → plugin | `{"id","method":"initialize"}` | Plugin replies `{"id","result":{"tools":[{name,description,input_schema,capability?}]}}` (`protocol_version` omitted ⇒ host assumes 1; host `ProtocolVersion = 1`). |
| host → plugin | `{"id","method":"tool/invoke","params":{"name","input"}}` | Plugin replies `{"id","result":{"output","is_error"?}}` or `{"id","error":"…"}` (protocol error: bad params / unknown tool). |
| plugin → host | `{"id":<same>,"progress":"…"}` | Zero or more progress lines before the terminal response (M1.ss). |
| plugin → host | `{"id":"cb-N","method":"host/invoke","params":{"name","input"}}` | Callback into an operator-allow-listed host tool; host answers with a method-less frame `{"id":"cb-N","result"|"error"}`. |
| host → plugin | `{"method":"shutdown"}` | Clean exit. |

`Capability` (M900) lets a plugin tool *join* an existing Edict axis (e.g. `http.post`); unknown values are ignored by
the host — a plugin cannot invent axes.

### SDK mechanics

* `Serve(tools...)` = `ServeRW(context.Background(), os.Stdin, os.Stdout, tools...)`. Validation: name + handler
  required, no duplicates.
* Read loop: `readFrame` caps one frame at `maxFrameBytes = 16 MiB` (mirrors host `DefaultMaxFrameBytes`; over-cap ⇒
  clean exit since the stream is desynced). Blank lines skipped. Method-less frames → `routeCallback`.
* `tool/invoke` runs each handler on its own goroutine (`sync.WaitGroup.Go`), so the loop keeps serving concurrent
  invokes and callback replies; `ServeRW` waits for in-flight handlers before returning.
* `runHandler` recovers panics → `IsError` result (`tool "x" panicked: …`); a returned `error` → `IsError` result.
* `writeFrame` serialises every write under `wmu` and flushes per frame (no interleaved bytes).
* Per-invocation state rides `ctx` (`ctxKey{}` → `*invocation{s, id}`), so `Emit(ctx, msg)` and
  `CallHost(ctx, tool, input)` need no ids. `CallHost` allocates `cb-<atomic seq>`, registers a 1-buffered channel in
  `pending` (mutex), blocks on reply or `ctx.Done()`; host-side `is_error` becomes a Go error.
* Helpers: `Result{Output, IsError}`, `Text(s)`, `Errorf(fmt, …)`, `Handler`, `Tool{Name, Description, InputSchema,
  Capability, Handle}` (empty schema advertised as `{"type":"object"}`).

| File | What it does |
|---|---|
| `doc.go` | Package doc with minimal example and design constraints. |
| `sdk.go` | `maxFrameBytes`, `readFrame`, `Result`, `Text`, `Errorf`, `Handler`, `Tool`, wire types (`frame`, `toolDef`, `initResult`, `invokeParams`, `invokeResult`), method consts, `session`, `callResp`, `ctxKey`, `invocation`, `fromContext`, `Serve`, `ServeRW`. |
| `sdk_session.go` | `initPayload`, `dispatchInvoke`, `runHandler`, `Emit`, `CallHost`, `routeCallback`, `writeFrame`. |
| `example/greet/main.go` | Runnable plugin with three tools: `greet` (plain), `slow` (progress via `Emit`), `shout` (composes via `CallHost(ctx,"upper",…)`). Run as `AGEZT_PLUGINS=greet=/path/to/greet agezt`. |

Host-side configuration (07): `AGEZT_PLUGINS="<prefix>=<path> [args],…"` (tools surface under the prefix namespace),
`AGEZT_PLUGIN_PINS="<prefix>=<hash>"` (binary pinning), `AGEZT_PLUGIN_TOOLS="<prefix>=<tool>+<tool>"` (advertise
allow-list). `agt plugin new` (`cmd/agt/plugin_new_render.go`, see 02) scaffolds a plugin against this SDK.
Tests: `sdk_test.go`, `integration_test.go` (ServeRW over pipes).

**Note:** this SDK is for *tool* plugins. There is no out-of-process *provider* plugin protocol — new LLM providers are
in-process Go adapters (see §7).

---

## 6. `plugins/external/mcpbridge` — MCP server as an agezt plugin

A standalone `package main` binary (not built by the Makefile; a stray `mcpbridge.exe` sits at the repo root). It
speaks the agezt plugin protocol to the daemon and JSON-RPC 2.0 MCP to one server.

* Config (env only): exactly one of `MCPBRIDGE_SERVER_CMD` (stdio; whitespace-split argv, no quoting) or
  `MCPBRIDGE_SERVER_URL` (legacy HTTP+SSE transport); `MCPBRIDGE_CLIENT_NAME` (default `agezt-mcpbridge`);
  `MCPBRIDGE_PROTOCOL_VERSION` (default `2024-11-05`); SSE SSRF opt-ins `MCPBRIDGE_ALLOW_LOOPBACK=1`,
  `MCPBRIDGE_ALLOW_PRIVATE=1`.
* Startup: `newMCPClient(factory)` → `handshake` (`initialize` with empty capabilities, then
  `notifications/initialized`; 10 s).
* `serve` loop (agezt side) is **lockstep/serial**: one request in, one response out; `initialize` →
  `tools/list` (+ `resources/list`; if any resources, a synthetic `read_resource{uri}` tool whose description
  enumerates URIs; 15 s timeout), `tool/invoke` → `tools/call` or `resources/read` (110 s, just under the host's 2 min),
  content flattened to text (`flattenContent`, `flattenResourceContents`), `shutdown` → flush + exit.
  The bridge does not emit `capability` or progress frames; MCP `notifications/progress` and `notifications/message`
  are only logged to stderr.
* Transports (`transport` interface `send/close`; `transportDeliver` callbacks `onResponse/onNotification/onTransportDead`):
  * stdio: `sandbox.Command` with `Env = os.Environ()` — deliberately the bridge's own env, which is already the plugin host's scrubbed base + the operator's `AGEZT_PLUGIN_ENV` grants the fronted server needs; stderr
    passthrough, read loop with `readBoundedLine` (16 MiB frame cap, M185).
  * SSE: GET event stream; first `endpoint` event announces the POST URL, validated by `sse_guard.go`
    (`buildSSEEndpointPolicy`, `resolveEndpoint`, `classifyHost`, `ipPolicyReason`) — same-origin as the operator's
    URL and netguard IP classification (blocks metadata/link-local, loopback/private unless opted in) — closes
    VULN mcp-sse-ssrf-pivot; POST per request, replies arrive on the stream; event data also capped. Streamable-HTTP
    (2025-03 spec) is **not** implemented here (it is in `kernel/mcp/http.go`).
* `mcpClient` (`mcp_wire.go`): `pending map[int64]chan`, `dead atomic.Bool` + `markDead` (unblocks every pending
  caller once), `deathError`.

| File | What it does |
|---|---|
| `main.go` | Package doc, env consts, timeouts, agezt wire types, `main`, `run`, `serve`, `readResourceToolName`. |
| `handlers.go` | `handleInitialize`, `handleInvoke`. |
| `helpers.go` | `flattenContent`, `flattenResourceContents`, `writeAgezt`, `getenvDefault`. |
| `limits.go` | `maxMCPFrameBytes`, `errMCPFrameTooLarge`, `readBoundedLine`. |
| `mcp.go` | `listResources`, `readResource`, `newMCPClient`, `startMCP`, `startSSEMCP`, `handshake`, `listTools`, `callTool`, `call`, `notify`, `handleNotification`, `markDead`, `deathError`, `close`. |
| `mcp_wire.go` | `mcpClient` + transportDeliver impl, JSON-RPC and MCP wire structs. |
| `transport.go` | `transport`, `transportDeliver`, `transportFactory`. |
| `stdio_transport.go` | `stdioTransport` (`newStdioTransport`, `send` under mutex, `readLoop`, `close`). |
| `sse_transport.go` | `sseTransport` (`newSSETransport` blocks for endpoint event, `send`, `close`, `readLoop`, `dispatchEvent`, `signalEndpoint`). |
| `sse_guard.go` | SSRF policy for the announced POST URL, delegated to `kernel/netguard`. |
| `testdata/mockmcp/main.go` | Test-only fake MCP server binary. |

Tests: `main_test.go`, `mcp_m428_test.go` (panic/wedge regression), `limits_test.go`, `sse_guard_test.go`,
`sse_limit_test.go`, `sse_transport_test.go`.

---

## 7. Extension points

### How to add a new LLM provider

1. **Does it speak an existing dialect?** If it is OpenAI Chat-Completions-compatible, no code is needed: give its
   catalog entry `npm: "openai-compatible"` and an `api` URL in `custom.json` (or add it to `catalog.FamilyFromNPM` +
   `compat.compatVendorBaseURL` if it has a first-party `@ai-sdk/<vendor>` package). Anthropic-shaped vendors
   (MiniMax etc.) work via `npm: "@ai-sdk/anthropic"` + `api` ending in `/v1`.
2. **New dialect:** create `plugins/providers/<name>/` with a `Provider` struct (`APIKey`/token source, `Endpoint`,
   `BaseURL`, `Model`, `HTTP`), `New(...)`, `Name()`, `ErrNoModel`, `APIError{Status, Body}`.
3. Implement `Complete`: resolve model (request > field > `ErrNoModel`), encode with `toolname.Maps`/`Wire`,
   apply `Params` (only fields the vendor supports; `IsZero()` ⇒ untouched wire), honour `JSONMode` if native,
   `provopts.ThinkingBudget`/`NormalizeEffort` for `ReasoningEffort`, `provopts.Merge(body, req.ProviderOptions["<key>"])`;
   send via `retry.DoHTTP` with a per-attempt `build` closure; map `*retry.HTTPError` → `*APIError`; decode to canonical
   (stop reason, usage with **total** input tokens, cached/cache-write subsets, `ReasoningContent`, images from
   `data:` URLs); `toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))`.
4. Optionally implement `CompleteStream` with `retry.DoHTTPStream`; emit chunks per the `agent.Chunk` contract and
   return the same assembled response; synthesise Start/Delta/Stop if tool calls arrive whole.
5. Add a `catalog.Family` constant + `FamilyFromNPM` case (`kernel/catalog`, 05), and
   `FamilySupportsNativeJSONMode` if applicable.
6. Wire it in `compat.Build` (new `case`, credentials via `providerEnvLookup` for multi-credential vendors, always
   `return wrapNamed(p.ID, x)`) and add the family to `compat.IsSupportedFamily`; add a `defaultBaseURL` if it has one
   canonical host.
7. Tests: copy the per-adapter suite shape (unit with `httptest`, `fuzz_test.go`, `params_test.go`,
   `toolname_test.go`, `vision_test.go`, streaming + malformed stream, `limit_test.go` for the 64 MiB cap) and a compat
   test. Run `agt check` against a real key (it builds via `compat.Build` directly).
8. Provider-specific auth that is not a static key (OAuth, subscription) follows the ChatGPT pattern instead: a
   dedicated builder in `providerboot` that registers via `registerAlternates`-style code, and a catalog entry with an
   empty `npm` so the compat loop ignores it.

### Other extension points

* New sampling knob → `agent.Params` + every adapter's `applyParams` (zero must stay byte-identical).
* New middleware → `agent.Middleware` in `kernel/agent`, opt-in env in `providerboot.Middleware`
  (new `AGEZT_*` env read in cmd/agezt must also be listed in controlplane `configEnvVars` — see 01/03 guard).
* New modality client → mirror `embed`/`image`: stdlib-typed method satisfying a kernel seam structurally,
  constructed in `cmd/agezt/main.go`, netguard client with explicit loopback/private allowance if local servers are
  first-class.
* New voice vendor → `voice.NewSTT`/`NewTTS` case + `daemonconfig` provider validation.
* New tool plugin → `plugins/sdk` (`agt plugin new`).
* New MCP transport for the bridge → implement `transport` + a `transportFactory` (Streamable HTTP is the noted gap).

---

## 8. Gotchas / invariants

1. **Default-preserving wire.** Every M997 addition (`Params`, `ProviderOptions`, `JSONMode`, thinking) must leave
   the request byte-identical when unset; tests pin this (`params_test.go`, `Merge` returns the same bytes).
2. **`ProviderOptions` is keyed by dialect, not registry name.** Adapters read only `anthropic`, `openai`, `google`,
   `vertex`, `bedrock`, `cohere`, `ollama`. The `agent.CompletionRequest` doc says "family or registry name" but a
   `groq`/`deepseek` key is ignored. Mistral, Azure, every openai-compatible vendor **and** the ChatGPT Responses
   adapter all read `"openai"` — an option meant for Chat Completions is also merged into the Responses body.
3. **No default provider/model.** Empty `AGEZT_PROVIDER` boots the `unconfigured` sentinel; `FirstModelID` is only a
   construction placeholder. Never add an auto-pick or mock fallback (owner law; mock only via `AGEZT_DEMO_ECHO=1`).
4. **Boot/Reload parity.** All registration goes through `registerAlternates`; middleware is applied on both paths; the
   live eligibility set is refreshed on reload; `gov.Replace(primary)` must be last; the sentinel must be removed
   before installing a differently-named primary (`reconcile_providers_test.go`).
5. **Governor knobs are boot-only** (`governorConfigFromEnv` not called by `Reload`), while middleware env *is*
   re-read on reload.
6. **Tool-name round trip.** Requests carry wire names; responses must be restored. Gemini/Vertex tool *results* use
   `ToolCallID` (`call-<i>`) as the `functionResponse.name` surrogate (documented SPEC-15 gap); Ollama does no
   sanitisation at all.
7. **Synthetic tool-call IDs** (`call-<partIndex>` for Gemini/Vertex, `call-<i>` for Ollama) restart at 0 every
   response, so IDs are unique only within one assistant turn.
8. **Streaming capability is per-adapter type, not per-model.** `bedrock.Provider` always implements
   `StreamingProvider` but `CompleteStream` returns `ErrVendorUnsupported` for non-`anthropic.*` models.
   `openairesponses` is never a `StreamingProvider` (unless `AGEZT_SIMULATE_STREAMING` synthesises one).
   `compat.wrapNamed` preserves the inner posture via two wrapper types.
9. **Retry semantics differ only in plumbing:** anthropic/openai/ollama hand-roll `retry.Do`, the rest use `DoHTTP`;
   behaviour is the same. Stream setup is retried, partial streams never. `Retry-After` is honoured up to 2 min.
   Only timeouts and 429/5xx are transient — `&retry.TransientError{}` wrapping is decorative because
   `IsTransient` has no `errors.As(err, *TransientError)` branch (non-timeout connection errors fail immediately).
10. **Reasoning coverage gaps (observed in code):** Bedrock-Anthropic sends a `thinking` block but its decoder and
    stream dispatcher drop `thinking` content (billed, not surfaced); `openairesponses` sets `reasoning.effort` but
    returns no reasoning text; the canonical `Message` has no field for Anthropic thinking blocks/signatures, so they
    are not replayed into later assistant turns (Anthropic documents that thinking blocks should be passed back in
    tool-use loops — worth verifying if thinking + tools misbehaves).
11. **JSON mode on Vertex-Claude:** `FamilyGoogleVertex` is declared JSON-native for the whole family, but the
    `claude-*` branch ignores `JSONMode` (structured callers still have `GenerateObject`'s prompt+repair path).
12. ✅ **Fixed (W1.4): SSRF posture differed by package.** Chat, image and rerank adapters used plain clients, so a
    configured or catalog-synced base URL could point at the metadata service. All now use `netout.OperatorClient`
    (loopback/private allowed, link-local/metadata refused); Vertex's GCE metadata token source uses the named
    exception `netout.MetadataClient`. `openairesponses` strict, `embed`/`voice` loopback+private netguard; archcheck
    keeps the http-client allowlist empty.
13. **Body caps everywhere:** 64 MiB (`httpread`), 16 MiB (Responses buffer, SDK frames, mcpbridge frames),
    8 MiB (`/models`), 1 MiB SSE scanner frames (anthropic), 1 MiB metadata responses, 25 MiB audio.
14. **Vertex metadata project lookup** happens inside `compat.Build` with `context.Background()` — on a non-GCP host
    with `GOOGLE_VERTEX_USE_METADATA=1` and no project, boot/registration pays a network timeout (the failure is
    non-fatal for alternates).
15. **ChatGPT backend is unofficial.** `ClientVersion` must be bumped to see newer models; hidden models still
    contribute `base_instructions`; the builtin model list must never overwrite a discovered catalog entry.
16. **Stale doc comments** (verify against code, not prose): `anthropic/doc.go` (non-streaming), `google/doc.go`
    (Vertex unsupported), `bedrock/doc.go` and `compat.go` package doc (bearer/Anthropic-only Bedrock), `vertex/doc.go`
    (Anthropic/streaming "land later"), `mcpbridge/main.go` ("why not in-kernel MCP" — `kernel/mcp` now exists).
17. **mcpbridge serialises calls** (one in flight) and its stdio child inherits the bridge's full environment; prefer
    `kernel/mcp` (governed `mcp.install`/`mcp.call`, scrubbed env, Streamable HTTP) for new integrations.

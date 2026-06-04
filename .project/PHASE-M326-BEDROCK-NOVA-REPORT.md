# M326 — Amazon Nova on Bedrock

## Why
Agezt's Bedrock provider already speaks five vendor body shapes (Anthropic,
Mistral, Cohere, Meta-Llama, AI21 Jamba). The notable gap was **Amazon Nova** —
Amazon's flagship current model family (Nova Micro / Lite / Pro / Premier) — which
fell through to `ErrVendorUnsupported`. Nova is a distinct body shape from the
legacy `amazon.titan-*` text models (intentionally unwired), so it needs its own
adapter. This closes the highest-value remaining Bedrock vendor gap.

## What
- **`plugins/providers/bedrock/nova.go`** (new): the Nova `messages-v1` InvokeModel
  body shape, verified against AWS docs:
  - Request: `{schemaVersion:"messages-v1", system:[{text}], messages:[{role,
    content:[{text}]}], inferenceConfig:{maxTokens}}`. The system prompt is the
    top-level `system` array (Nova has no system message role); user/assistant
    roles map through; a per-message system role folds into `system`; tool/other
    roles surface as user content; empty turns are dropped (Nova 400s on empty
    content blocks).
  - Response: `output.message.content[].text` (joined), `stopReason`
    (`max_tokens`→`StopMaxTokens`, else `StopEndTurn`), and inline
    `usage.inputTokens/outputTokens` — so the governor sees real spend (unlike the
    Mistral adapter, whose counts arrive only in response headers).
  - `isAmazonNovaModel`: matches `amazon.nova*` and regional profiles
    (`.amazon.nova*`), deliberately NOT `amazon.titan-*`.
- **`plugins/providers/bedrock/bedrock.go`**: a `case isAmazonNovaModel(model)` in
  the `Complete` dispatch; the unsupported-vendor message lists `amazon.nova.*`.
- Chat-only (no tool round-trip), matching the Mistral/Llama/Cohere/Jamba adapters.
  No catalog change needed — the provider dispatches on the model id, so any
  operator-configured `amazon.nova-*` model under the Bedrock family just works.

## Verification
- **`plugins/providers/bedrock/nova_test.go`** (5 tests): happy path (asserts the
  exact `messages-v1` wire — schemaVersion, system array, content-block messages,
  inferenceConfig.maxTokens — plus decoded answer and inline usage); `max_tokens`
  stop mapping; regional `us.amazon.nova-*` accepted; **legacy `amazon.titan-*`
  stays `ErrVendorUnsupported`** (Nova detection must not swallow Titan); empty-
  output guard.
- **Live (offline httptest)**: the real `bedrock.Provider` pointed at a mock
  InvokeModel endpoint — the request wire was the exact Nova `messages-v1` body
  (system block + content-block user message + `inferenceConfig.maxTokens`), and
  the response decoded to the answer + inline token usage. Network-free.
- Full suite **2016** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Tool use is not wired (chat-only), consistent with every non-Anthropic Bedrock
  adapter — Nova's `toolConfig` needs the content-block tool shape the agent loop
  doesn't emit on this path yet. Operators needing tool use on Bedrock should use
  the `anthropic.*` models.
- Legacy `amazon.titan-*` (text body) and AI21 J2 remain intentionally unwired.
- Remaining Bedrock vendor candidate: **DeepSeek-R1** (`deepseek.r1-*`) — a
  reasoning model whose `reasoning_content` would tie into the M317–M325 pipeline;
  a clean follow-up once its exact Bedrock body shape is confirmed.

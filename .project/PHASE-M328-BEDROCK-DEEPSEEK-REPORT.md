# M328 — DeepSeek-R1 on Bedrock (with reasoning)

## Why
DeepSeek-R1 (`deepseek.r1-*`) is a popular reasoning model available on Bedrock,
and it was the most coherent remaining Bedrock vendor gap — it ties the whole
session together: a reasoning model whose chain of thought flows into the
M317–M325 pipeline, billed via the M327 header overlay. The wire format was
verified against the authoritative AWS docs before implementing (the earlier
deferral was because the format was unconfirmed; the AWS Bedrock DeepSeek
parameters page documents it exactly).

## What
- **`plugins/providers/bedrock/deepseek.go`** (new): the InvokeModel text-
  completion shape.
  - Request: `{prompt, max_tokens}`. `deepseekR1Template` renders the
    conversation into DeepSeek's chat template —
    `<｜begin▁of▁sentence｜>{system}<｜User｜>…<｜Assistant｜>…<｜end▁of▁sentence｜>…<｜Assistant｜><think>\n`.
    The special tokens use full-width vertical bars (U+FF5C) and the SentencePiece
    underline (U+2581), pinned as named constants and codepoint-verified. The
    trailing `<think>` opens R1's reasoning block (AWS's documented prompt form).
  - Response: `{choices:[{text, stop_reason}]}`. Because the prompt opened
    `<think>`, the returned text is `reasoning</think>answer`; the decoder splits
    on `</think>` → `ReasoningContent` + answer. No closing tag (truncated mid-
    thought, or no thinking) → the whole text becomes the answer, so the caller
    never gets an empty response. `stop_reason` `length` → `StopMaxTokens`.
  - Usage: the body carries no token counts; Complete's response-header overlay
    (M327) supplies them — so the reasoning AND the billing both work for free
    off prior milestones.
  - `isDeepSeekModel`: `deepseek.*` + regional `.deepseek.*`.
- **`plugins/providers/bedrock/bedrock.go`**: a `case isDeepSeekModel(model)` in
  the dispatch; the unsupported-vendor message lists `deepseek.*`.

The captured reasoning flows through the entire M317–M325 surfacing pipeline
(ephemeral `llm.reasoning` events → `agt pulse`, ACP `agent_thought_chunk`, OpenAI
API `reasoning_content` / Responses `reasoning` item) with no further work.

## Verification
- **`plugins/providers/bedrock/deepseek_test.go`** (4 tests): happy path (asserts
  the chat-template wire — BOS, system prefix, user turn, trailing
  `<｜Assistant｜><think>\n` — plus the reasoning/answer split and header usage);
  regional `us.deepseek.r1-v1:0` accepted; truncated-thinking (no `</think>`) →
  text surfaces as the answer, `StopMaxTokens`; empty-choices guard.
- Special-token codepoints verified: `｜`=U+FF5C, `▁`=U+2581 (must match the
  model tokenizer).
- **Live (offline httptest)**: real `bedrock.Provider` vs a mock InvokeModel
  endpoint — request wire was the exact DeepSeek prompt template, the answer
  ("42") split from the reasoning ("Let me compute… = 42."), and usage came from
  the `X-Amzn-Bedrock-*-Token-Count` headers (18/57). Network-free.
- Full suite **2022** passing, `go test ./...` exit 0 (two clean runs); `gofmt -l`
  clean; `go vet` clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Wire verified against AWS's documented InvokeModel text-completion schema. The
  Converse API path (which separates `reasoningContent.reasoningText` natively)
  is a different surface; Agezt's Bedrock provider uses InvokeModel, where the
  reasoning arrives inline in the `<think>` block — hence the split.
- Chat-only (no tool round-trip), like every non-Anthropic Bedrock adapter.
- Bedrock vendor coverage is now Anthropic, Mistral, Cohere, Meta-Llama, AI21
  Jamba, Amazon Nova, and DeepSeek-R1. Legacy Titan / AI21 J2 stay unwired.
- Non-Anthropic Bedrock streaming (`InvokeModelWithResponseStream`) remains a
  separate, larger effort (per-vendor event-stream chunk shapes).

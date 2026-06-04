# M317 — Capture reasoning-model chain-of-thought (DeepSeek-R1 et al.)

## Why
Reasoning models — DeepSeek-R1 and its distills, QwQ, and other openai-compatible
gateways — return their chain of thought in a `reasoning_content` field separate
from the answer. Agezt discarded it entirely: only the final answer was kept. For
a system whose pitch is "auditable action," the model's reasoning is exactly the
kind of thing worth surfacing — and it was on the floor.

(Scope note: OpenAI o1/o3 don't return reasoning content over the API — only a
token count — and Anthropic extended thinking needs a request param Agezt doesn't
send, so they have nothing to capture here. The realistic, common target is the
`reasoning_content` field, which the openai provider already covers for every
openai-compatible vendor.)

## What
- **`kernel/agent`**: `CompletionResponse.ReasoningContent` and
  `Chunk.ReasoningDelta` carry the reasoning. The loop streams each reasoning
  delta as an **ephemeral** `llm.reasoning` event (new `event.KindLLMReasoning`) —
  visible live in `agt pulse`, **not** durably journaled (reasoning can be huge;
  the answer is what the audit chain needs). The durable `llm.response` event
  gains a `reasoning_chars` count so a run's reasoning size is recorded without
  the bulk.
- **`plugins/providers/openai`**: the non-streaming decoder reads
  `message.reasoning_content` (and the `reasoning` variant some gateways use); the
  streaming parser accumulates `delta.reasoning_content` into the response and
  emits it as `Chunk.ReasoningDelta`, separate from the answer text. Response-only
  fields with `omitempty` — the request wire is unchanged.

## Verification
- **openai**: `TestComplete_CapturesReasoningContent` (non-streaming →
  `ReasoningContent`, answer unpolluted) + `TestParseStream_Reasoning` (streaming
  → accumulated reasoning + `ReasoningDelta` chunks, answer separate).
- **agent loop**: `TestRun_PublishesReasoningEvents` — reasoning deltas surface as
  **ephemeral** `llm.reasoning` events distinct from the answer's `llm.token`
  events.
- Full suite **1982** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Additive: ordinary models return no `reasoning_content`, so
  `ReasoningContent`/`ReasoningDelta` stay empty and nothing changes.
- The ephemeral-not-journaled choice keeps the journal bounded while still making
  reasoning observable live. A future slice could optionally persist a truncated
  reasoning summary, or render it in `agt pulse`/the web UI as a distinct stream.
- Other providers (Anthropic extended thinking via a request param; Gemini
  thinking) are clean follow-ups behind their own request flags.

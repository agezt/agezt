# M318 — Anthropic extended thinking (Claude reasoning)

## Why
M317 captured reasoning content for DeepSeek-R1-style models (openai-compatible
`reasoning_content`). This brings the other major reasoning family — **Claude
extended thinking** — into the same pipeline. Claude returns its chain of thought
as `thinking` content blocks, but only when the request enables thinking; Agezt
never sent that, so Claude's reasoning was unavailable. This adds the request
enable (operator opt-in) and captures the thinking via M317's `ReasoningContent`.

## What
- **`plugins/providers/anthropic`**:
  - `Provider.ThinkingBudget` (0 = off). `thinkingConfig` builds the
    `thinking: {type: enabled, budget_tokens}` block, clamps the budget up to
    Anthropic's 1024 minimum, and bumps `max_tokens` above the budget (Anthropic
    requires `max_tokens > budget_tokens` — thinking counts toward the output
    allowance). Threaded through `encodeRequest` and `encodeStreamRequest`. Budget
    0 omits the block — the request wire is byte-identical for non-thinking runs.
  - Non-streaming decode: a `thinking` content block → `ReasoningContent` (M317),
    separate from the answer text.
  - Streaming: `content_block_start{type:thinking}` + `thinking_delta` frames →
    `Chunk.ReasoningDelta` and accumulate into the assembled `ReasoningContent`.
- **`plugins/providers/compat`**: the Anthropic build reads
  `AGEZT_ANTHROPIC_THINKING_BUDGET` (via the env-aware lookup) and sets the budget.
  Off unless the operator opts in (thinking costs extra tokens).

Reuses M317 entirely on the consumer side: the loop streams the thinking as
ephemeral `llm.reasoning` events and records `reasoning_chars` on `llm.response`.

## Verification
- **`plugins/providers/anthropic/thinking_test.go`** (5 tests): thinking enabled
  (block + `max_tokens > budget`), disabled-by-default (block omitted), sub-1024
  budget clamped, non-streaming decode captures thinking, and the streaming path
  (thinking_delta → `ReasoningDelta` + assembled `ReasoningContent`, answer
  unpolluted).
- Full suite **1987** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged. Network-free.

## Scope notes
- Off by default; existing Claude runs are byte-for-byte unchanged.
- With M317 + M318 the two major reasoning families (DeepSeek-R1 et al., Claude)
  are captured through one pipeline. Gemini "thinking" (when it returns thought
  summaries) is a clean follow-up behind its own request flag.
- Anthropic requires `temperature` unset (or 1) with thinking — Agezt never sends
  temperature, so that constraint is satisfied with no extra code.

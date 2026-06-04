# M321 — Claude extended thinking on Vertex AI

## Why
M318 added Claude extended thinking on the **direct** Anthropic API. But
Claude-on-Vertex (`claude-*` models served through Vertex AI's `:rawPredict` /
`:streamRawPredict` publisher path) is a **separate code path**
(`vertex/anthropic.go`, `completeAnthropic` / `completeStreamAnthropic`) that
never plumbed thinking — the exact asymmetry M320 fixed for Gemini, but for
Claude. This closes it, so extended thinking works on both of Claude's surfaces.

## What
- **`plugins/providers/vertex/anthropic.go`** (the Anthropic-on-Vertex path):
  - Request: `anthVertexRequest.Thinking *anthVxThinking`;
    `anthVxThinkingConfig(budget, maxTok)` mirrors the direct adapter (M318) —
    clamps the budget up to Anthropic's 1024 floor and bumps `max_tokens` above
    it (Anthropic requires `max_tokens > budget_tokens`). `budget <= 0` omits the
    block (wire byte-identical); a negative "dynamic" value — valid only for
    Gemini — means off here (Anthropic has no dynamic mode).
  - Non-streaming decode: a `thinking` content block → `ReasoningContent`.
  - Streaming SSE: `content_block_start{type:thinking}`, `thinking_delta`, and
    `content_block_stop` route thinking to `Chunk.ReasoningDelta` and the
    assembled `ReasoningContent`, separate from the answer's `text_delta`.
- **`plugins/providers/vertex/vertex.go`**: `Provider.ThinkingBudget` (added in
  M320 for Gemini) is now shared by both publishers — each applies its own
  semantics at encode time (the `Complete`/`CompleteStream` dispatch on
  `isAnthropicModel` already routes to the right encoder). A provider instance is
  bound to one model, so there's no ambiguity. Doc comment updated.
- **No new env var / no compat change**: `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET`
  (M320) already feeds `Provider.ThinkingBudget`; it now drives whichever
  publisher the bound model uses.

Reuses the M317 consumer side (ephemeral `llm.reasoning` events +
`reasoning_chars`).

## Verification
- **`plugins/providers/vertex/anthropic_thinking_test.go`** (5 tests): thinking
  enabled (block + `max_tokens > budget`), disabled for budget 0 **and** the
  Gemini-only `-1`, sub-1024 clamp, non-streaming decode captures thinking, and
  the streaming path (`thinking_delta` → `ReasoningDelta` + assembled
  `ReasoningContent`, answer unpolluted).
- **Live (offline httptest)**: real `vertex.Provider` (fake OAuth) vs a mock
  `:rawPredict` returning a `thinking` block + answer, with `ThinkingBudget = 800`
  and `MaxTokens = 1024`. Request routed to the Anthropic body shape, carried
  `thinking:{type:enabled,budget_tokens:1024}` (clamped from 800) and
  `max_tokens:5120` (bumped above the budget); response captured
  `ReasoningContent="6 sevens: …"`, answer `"42"`. Network-free.
- Full suite **2002** passing, `go test ./...` exit 0; `gofmt -l` clean (the
  pre-existing `vertex/auth.go` CRLF artifact is untouched); `go vet` clean;
  `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Off by default; existing Claude-on-Vertex runs are byte-for-byte unchanged.
- Reasoning capture is now uniform across **every** reasoning-capable provider
  Agezt speaks: direct Anthropic (M318), direct Gemini (M319), Vertex Gemini
  (M320), Vertex Claude (M321), and openai-compatible DeepSeek-R1 (M317) — one
  pipeline, one `ReasoningContent` field, one `llm.reasoning` event stream.
- Anthropic-on-Vertex also returns `signature`/`redacted_thinking` blocks on
  some configs; those aren't needed for capture (the human-readable summary is in
  the `thinking` text) and are ignored, same as the direct adapter.

# M320 — Gemini thinking on Vertex AI

## Why
M319 added Gemini "thinking" on the Generative Language API
(`generativelanguage.googleapis.com`). Gemini is also served through **Vertex
AI** (`{region}-aiplatform.googleapis.com`, service-account / metadata OAuth) —
a separate adapter (`plugins/providers/vertex`) with its own copy of the
generateContent wire. Without this, Vertex-routed Gemini runs couldn't request
or capture thinking, so the capability was asymmetric across the two Gemini
surfaces. This closes that gap with the exact M319 shape.

## What
- **`plugins/providers/vertex`** (native-Gemini path only — Anthropic-on-Vertex
  `claude-*` models branch earlier into `completeAnthropic` and are unaffected):
  - `Provider.ThinkingBudget` (0 = off). When non-zero, `encodeRequest` adds
    `generationConfig.thinkingConfig: {includeThoughts: true, thinkingBudget: N}`;
    `-1` = dynamic budget (sent, not treated as off); 0 omits the block (wire
    byte-identical). Threaded through `Complete` and `CompleteStream`.
  - Non-streaming decode: a `thought:true` part → `ReasoningContent`, separate
    from the answer.
  - Streaming: `thought`-flagged deltas → `Chunk.ReasoningDelta` and accumulate
    into the assembled `ReasoningContent`.
  - **Usage**: `usageMetadata.thoughtsTokenCount` (reported separately, billed as
    output) is folded into `Usage.OutputTokens` in both paths — added once, no
    double-count.
- **`plugins/providers/compat`**: the Vertex build reads
  `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET` (distinct from M319's
  `AGEZT_GOOGLE_THINKING_BUDGET` — Vertex is a separate billing/credential
  surface). Off unless the operator opts in.

Reuses the M317 consumer side entirely (ephemeral `llm.reasoning` events +
`reasoning_chars` on `llm.response`).

## Verification
- **`plugins/providers/vertex/thinking_test.go`** (5 tests): thinking enabled
  (`thinkingConfig` + `includeThoughts:true` + budget), dynamic `-1` sent,
  disabled-by-default (omitted), non-streaming decode (thought capture + token
  fold), streaming (`thought` deltas → `ReasoningDelta` + assembled
  `ReasoningContent` + token fold, answer unpolluted).
- **Live (offline httptest)**: real `vertex.Provider` (fake OAuth token source)
  vs a mock `:generateContent` returning a `thought:true` part +
  `thoughtsTokenCount:40`, with `ThinkingBudget = -1`. Request carried
  `thinkingConfig:{includeThoughts:true,thinkingBudget:-1}` and a `Bearer` header;
  response captured `ReasoningContent`, answer `"42"`, `OutputTokens=42` (2+40).
  Network-free.
- Full suite **1997** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Off by default; existing Vertex Gemini runs are byte-for-byte unchanged.
- Thinking now spans both Gemini surfaces (Generative Language API M319 + Vertex
  AI M320) and all three major reasoning families (DeepSeek-R1, Claude, Gemini)
  through one pipeline.
- Anthropic-on-Vertex (Claude via `:rawPredict`) extended thinking is a separate
  follow-up if needed — it uses the Anthropic Messages body, distinct from the
  generateContent path touched here.

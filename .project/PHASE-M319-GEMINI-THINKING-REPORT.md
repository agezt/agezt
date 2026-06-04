# M319 — Gemini thinking (Google reasoning)

## Why
M317 captured reasoning for DeepSeek-R1 (openai-compatible `reasoning_content`);
M318 added Claude extended thinking. This completes the trio with the third major
reasoning family — **Gemini "thinking"** (2.5-series models). Gemini can return
its chain of thought as thought-summary parts, but only when the request asks for
them (`thinkingConfig.includeThoughts`). Agezt never sent that, so Gemini's
reasoning was unavailable. This adds the request enable (operator opt-in) and
captures the summaries through M317's `ReasoningContent` pipeline.

## What
- **`plugins/providers/google`**:
  - `Provider.ThinkingBudget` (0 = off). When non-zero, `encodeRequest` adds
    `generationConfig.thinkingConfig: {includeThoughts: true, thinkingBudget: N}`.
    A positive budget caps the thinking tokens; `-1` asks Gemini for a dynamic
    budget (a distinct, legitimate opt-in — sent, not treated as off). Budget 0
    omits `thinkingConfig` entirely — the request wire is byte-identical for
    non-thinking runs. Threaded through both `Complete` and `CompleteStream`.
  - Non-streaming decode: a part flagged `thought: true` → `ReasoningContent`
    (M317), separate from the answer text.
  - Streaming: `thought`-flagged text deltas → `Chunk.ReasoningDelta` and
    accumulate into the assembled `ReasoningContent`.
  - **Usage accuracy**: Gemini reports `usageMetadata.thoughtsTokenCount`
    *separately* from `candidatesTokenCount` (total = prompt + candidates +
    thoughts) but bills it at the output rate. Both decode paths fold it into
    `Usage.OutputTokens`, so the governor sees the true billable output — no
    double-count (it's a distinct field, added once).
- **`plugins/providers/compat`**: the Google build reads
  `AGEZT_GOOGLE_THINKING_BUDGET` (via the env-aware lookup) and sets the budget.
  Off unless the operator opts in (thinking costs extra tokens).

Reuses M317 entirely on the consumer side: the loop streams the thinking as
ephemeral `llm.reasoning` events and records `reasoning_chars` on `llm.response`.

## Verification
- **`plugins/providers/google/thinking_test.go`** (5 tests): thinking enabled
  (`thinkingConfig` with `includeThoughts:true` + budget), dynamic budget `-1`
  sent (not dropped), disabled-by-default (block omitted), non-streaming decode
  captures the thought + folds `thoughtsTokenCount` into `OutputTokens`, and the
  streaming path (`thought` deltas → `ReasoningDelta` + assembled
  `ReasoningContent` + token fold, answer unpolluted).
- **Live (offline httptest)**: real `google.Provider` pointed at a mock
  `:generateContent` returning a `thought:true` part + answer with
  `thoughtsTokenCount:40`, `candidatesTokenCount:2`. Request wire carried
  `thinkingConfig:{includeThoughts:true,thinkingBudget:2048}`; response captured
  `ReasoningContent="Step 1: 6*7. Step 2: =42."`, answer `"42"`,
  `OutputTokens=42` (2+40). Network-free.
- Full suite **1992** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Off by default; existing Gemini runs are byte-for-byte unchanged.
- With M317 + M318 + M319 the three major reasoning families (DeepSeek-R1 et al.,
  Claude, Gemini) are captured through one pipeline.
- Vertex AI (FamilyGoogleVertex) is a separate adapter; thinking there is a clean
  follow-up if/when its Gemini-on-Vertex path needs it (same wire shape).

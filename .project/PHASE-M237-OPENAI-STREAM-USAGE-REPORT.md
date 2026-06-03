# M237 — OpenAI streaming `stream_options.include_usage`

## Why
The OpenAI-compatible API (`kernel/openaiapi`) lets any OpenAI client/IDE drive
agezt — a high-traffic, user-facing surface. Auditing it found the chat
completions path solid (OpenAI-shaped error envelope, `[DONE]` terminator, a
`usage` object on non-streaming responses, lenient request decoding, a
`defer sub.Cancel()` so streams don't leak subscriptions) **except** for one
real, increasingly-relied-on feature: `stream_options.include_usage`.

OpenAI's streaming default omits token usage. When a client opts in with
`stream_options: {"include_usage": true}`, OpenAI sends a final usage-only chunk
(`choices: []` plus a `usage` object) just before `[DONE]`. The OpenAI SDK and
cost-tracking middlewares request this to account for streamed runs. agezt
ignored the field, so those clients got no usage from streaming and under-counted.

## What
- **`kernel/openaiapi/openaiapi.go`**:
  - `chatRequest` gains `StreamOptions *streamOptions` (`include_usage bool`).
  - `streamChat` takes an `includeUsage` flag. It now accumulates the streamed
    answer text and, on stream close, emits a usage-only chunk
    (`{… "choices": [], "usage": {...}}`) before `[DONE]` when the flag is set.
    The terminal-finish/usage/`[DONE]` sequence was factored into one `endStream`
    closure so both stream-exit paths (subscription close and run completion,
    including the error case) behave identically.

Usage figures reuse the existing `estimateUsage(prompt, completion)` the
non-streaming path already returns, so streaming and non-streaming report the
same way.

## Files
- `kernel/openaiapi/openaiapi.go` — `streamOptions` type, request field, threaded
  flag, `endStream` + usage chunk, content accumulation (edited).
- `kernel/openaiapi/openaiapi_usage_test.go` — 2 tests (new):
  `TestChatStreaming_IncludeUsage` (a usage chunk with empty `choices` appears
  before `[DONE]`) and `TestChatStreaming_NoUsageByDefault` (no usage chunk
  without the option). The pre-existing `TestChatCompletionStreaming` still passes
  (no regression to the default path).

## Verification
- `go test ./kernel/openaiapi/` — green; full suite **1777 → 1779** (+2),
  66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/openaiapi/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Proof:** the tests drive the real handler (`s.Handler().ServeHTTP`) over an
  httptest recorder with a token-streaming fake engine and assert the SSE body
  contains (or omits) the usage chunk in the OpenAI shape and ordering.

## Scope notes
- The Responses API (`responses.go`) has its own streaming format and isn't
  affected; its usage surfacing could be a separate follow-up if a client needs it.
- Token counts are `estimateUsage`'s word-count approximation (as on the
  non-streaming path) — consistent across both, not a billing-grade tokenizer.

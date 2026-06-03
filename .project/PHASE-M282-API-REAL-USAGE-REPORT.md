# M282 — OpenAI-compatible API reports real provider token usage

## Why (found against the real gateway)
Driving Agezt's own OpenAI-compatible API (`AGEZT_API_ADDR`) with a real backend
(gpt-5.5) for the first time exposed a second bug: the `usage` block was a
whitespace word-count estimate, not the provider's real token usage.

```
external client → POST /v1/chat/completions → Agezt → gpt-5.5
returned usage: {prompt_tokens: 8, completion_tokens: 1}     ← whitespace estimate
journal budget.consumed: in=1411 out=5                       ← real provider usage
```

A cost/quota-tracking client reading the standard `usage` field — the normal way
to meter an OpenAI endpoint — would undercount by ~175×. The real numbers were in
the journal all along; the API just didn't surface them.

## What
- **`kernel/openaiapi/openaiapi.go`** — new optional `UsageReporter` engine
  capability: `UsageFor(corr) (promptTokens, completionTokens int, ok bool)`. A
  `chatUsage(eng, corr, intent, answer)` helper returns the real usage when the
  engine reports it (ok), else the existing `estimateUsage`. Used by `handleChat`
  (non-streaming) and `streamChat`'s `stream_options.include_usage` chunk.
- **`kernel/openaiapi/responses.go`** — parallel `responsesUsageFor` (Responses
  field names `input_tokens`/`output_tokens`); `responseObject` now takes the
  engine and uses it for both the non-streaming and streaming-final response.
- **`cmd/agezt/main.go`** — `kernelAPIEngine.UsageFor` implements the reporter by
  folding the journal's `budget.consumed` events for the correlation (summing
  `input_tokens`/`output_tokens` across the run's LLM calls); returns ok=false
  when nothing was consumed (free/local/mock) so the estimate still applies.

The change is additive and backward-compatible: an engine that doesn't implement
`UsageReporter` (every test fake) keeps the estimate behaviour exactly.

## Files
- `kernel/openaiapi/openaiapi.go`, `responses.go` — `UsageReporter`, helpers,
  call sites (edited).
- `cmd/agezt/main.go` — `kernelAPIEngine.UsageFor` (edited).
- `kernel/openaiapi/usage_test.go` — 2 tests (new): a reporter engine surfaces
  real `1406/11/1417`; `ok=false` and a non-reporter engine both fall back to the
  whitespace estimate (never 0/0).

## Verification
- `go test ./kernel/openaiapi/` — green; full suite **1890 → 1892** (+2), 68
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet` clean;
  `GOOS=linux build` clean; `go.mod` / `go.sum` unchanged.
- **Live-proven against the real gateway** (external OpenAI client → Agezt API →
  gpt-5.5): before, `usage {prompt_tokens: 8, completion_tokens: 1}`; after,
  `usage {prompt_tokens: 1406, completion_tokens: 11, total_tokens: 1417}` —
  matching the journal's `budget.consumed` for the run.

## Scope notes
- Second real bug surfaced by the user's live gateway (after the M279
  dotted-tool-name 400). Both were invisible while testing leaned on the mock.
- Usage is summed across all LLM calls in a run, so a multi-turn tool-using
  request reports the full token cost, matching `agt runs show`'s spend.

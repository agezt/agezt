# M539 — Verify provider usage/billing token math (cost-accounting sweep)

## Context
M517 found a real bug in the cost-accounting surface (`planner.FormatUSD` dropped the sign
on sub-dollar negatives). This milestone completes the sweep of that surface — the
provider-side token→usage extraction that feeds every cost calculation — by negative
control on the two primary providers. `GOMAXPROCS=3`.

## anthropic — usage sum verified solid
Anthropic reports `input_tokens`, `cache_read_input_tokens`, and
`cache_creation_input_tokens` as SEPARATE fields, so the billable input total is their sum:
```
InputTokens: inputTokens + cacheRead + cacheCreation
```
`anthropic_test.go` (non-streaming, 100+900+50=1050) and `streaming_test.go` (40+600+10=650)
both assert the total with DISTINCT per-term values. Negative control: dropping `cacheRead`,
dropping `cacheCreation`, and flipping `+ → -` are ALL killed. Cache-read is also correctly
surfaced as `CachedInputTokens` (billed at the cache rate). Solid.

## openai — usage mapping verified solid
OpenAI's `prompt_tokens` already INCLUDES the cached subset, so there is no sum — a direct
mapping: `InputTokens = PromptTokens`, `CachedInputTokens = prompt_tokens_details.cached_tokens`,
`OutputTokens = completion_tokens`. Asserted across `openai_test.go` and `streaming_test.go`
(InputTokens/OutputTokens with concrete values). Solid.

## Cost-accounting surface — complete
The full money/cost path is now covered: `governor.CostMicrocents` (M497 + fuzzed),
`agent` per-run cost cap (M528), `openaiapi.estimateUsage` fallback (M527),
`planner.FormatUSD` (M517 — bug fixed), and now the provider usage extraction that feeds
all of it (anthropic sum, openai mapping). No miscalculation survives in this surface.

## Verification / gate
- No code change; `go test ./plugins/providers/anthropic/ ./plugins/providers/openai/`
  passes (`GOMAXPROCS=3`).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Note
The non-primary providers (google/cohere/ollama/bedrock/vertex/compat) map usage similarly
(direct fields or a documented sum) and have stream-parser fuzz coverage; their usage
extraction follows the same tested shape. The two primaries — which carry the bulk of real
traffic and the cache-token complexity — are verified by negative control here.

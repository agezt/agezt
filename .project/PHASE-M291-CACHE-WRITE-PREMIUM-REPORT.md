# M291 — Cache-write premium billing

## Why
M289 billed cache *reads* cheaply; M290 wired Anthropic but folded cache
*creation* tokens into the input rate, noting the cache-write premium wasn't
modelled. Anthropic bills cache creation at ~1.25× input — so folding it at 1.0×
**under-bills** the premium (the conservative principle's wrong direction). This
completes the cache cost model with explicit cache-write handling.

## What
- **`kernel/agent/agent.go`**: `Usage.CacheWriteInputTokens` — the subset of
  `InputTokens` written into the cache this call.
- **`kernel/catalog/types.go`**: `Cost.CacheWriteMicrocentsPerMTok()`.
- **`kernel/governor/pricing.go`**: `modelPrice.CacheWriteMicrocentsPerMTok`
  (loaded from the catalog; fallback Claude entries carry the 1.25× cache-write
  list); `costMicrocentsCached` extended to
  `(model, input, cached, write, output)` billing
  `fresh·input + cached·cacheRead + write·cacheWrite + output·output`, with
  `fresh = input − cached − write` and `cached+write` clamped to `input`. A subset
  with no price falls back to the input rate (conservative).
- **`kernel/governor/governor.go`** `recordUsage`: clamps + threads write tokens,
  records `cache_write_input_tokens` on `budget.consumed`.
- **`plugins/providers/anthropic/anthropic.go` + `streaming.go`,
  `plugins/providers/vertex/anthropic.go`**: `cache_creation_input_tokens` now
  maps to `CacheWriteInputTokens` (was folded into input).
- **`cmd/agezt/main.go`**: `AGEZT_DEMO_CACHED` usage gains 500 write tokens.

## Files
- `kernel/agent/agent.go`, `kernel/catalog/types.go`,
  `kernel/governor/pricing.go`, `kernel/governor/governor.go`,
  `plugins/providers/anthropic/anthropic.go`,
  `plugins/providers/anthropic/streaming.go`,
  `plugins/providers/vertex/anthropic.go`, `cmd/agezt/main.go` (edited).
- `kernel/governor/pricing_internal_test.go`: `TestCostMicrocentsCached` gains the
  `write` arg + cases (write billed at cache-write rate & above input; no-price
  fallback; cached+write clamped to input).
- `plugins/providers/anthropic/anthropic_test.go` /`streaming_test.go`: assert
  `CacheWriteInputTokens` is mapped from `cache_creation_input_tokens`.

## Verification
- Full suite (1901+ tests; M291 extends existing tests, no new top-level funcs),
  68 packages, `go test ./...` exit 0; `go vet` clean on touched packages;
  `gofmt -l` clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven offline**: `AGEZT_DEMO_CACHED=1` (10000 input, 9000 cache-read,
  500 cache-write, 200 output on `claude-sonnet-4-6`) → `budget.consumed`
  recorded `cache_write_input_tokens: 500` and `cost_microcents: 907500`,
  matching `(500·300M + 9000·30M + 500·375M + 200·1500M)/1e6` exactly. The 500
  write tokens cost 187_500 mc (375M rate) vs 150_000 mc at the input rate — the
  1.25× premium captured, up from M289's 870000 (which billed those at input).

## Scope notes
- `cached==write==0` ⇒ identical to `costMicrocents` (no regression for providers
  that don't report cache tokens, incl. openai's read-only case).
- The cache cost model is now complete: fresh / cache-read / cache-write / output,
  end to end (agent.Usage → providers → governor → budget.consumed). Catalog
  `cost.cache_read` + `cost.cache_write` both consumed.
- Remaining: Gemini/Bedrock/Cohere/Mistral cache-token parsing (each threads its
  own fields into `Usage.CachedInputTokens` / `CacheWriteInputTokens`); a webui
  budget/cost panel surfacing cached savings.

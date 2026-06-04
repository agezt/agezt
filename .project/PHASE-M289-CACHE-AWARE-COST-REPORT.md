# M289 — Cache-aware cost accounting

## Why
Flagged at the end of M288. Real reasoning models (and the gpt-5.5 gateway used
earlier this arc) serve a large fraction of the prompt from a **prompt cache**,
billed at a much lower cache-read rate (Anthropic: 0.1× input). Agezt's catalog
already carried `cost.cache_read` (`kernel/catalog/types.go`), but nothing used
it: the providers didn't parse cached-token counts, `agent.Usage` had no field
for them, and the governor billed every input token at the full input rate. So
spend was **over-estimated** for any cache-heavy run — invisible with the mock,
real on a live provider. (The session also audited the usage-*reporting* path
clean in M282; this was the remaining cost-*math* gap.)

## What
- **`kernel/catalog/types.go`**: `Cost.CacheReadMicrocentsPerMTok()` (mirrors the
  input/output converters).
- **`kernel/agent/agent.go`**: `Usage.CachedInputTokens` — the subset of
  `InputTokens` served from the provider's prompt cache.
- **`plugins/providers/openai/openai.go`**: parse
  `usage.prompt_tokens_details.cached_tokens` → `Usage.CachedInputTokens` (the
  compat vendors inherit this decoder).
- **`kernel/governor/pricing.go`**: `modelPrice.CacheReadMicrocentsPerMTok`
  (populated from the catalog in `priceForOk`; fallback Claude entries carry the
  0.1× cache-read list); new `costMicrocentsCached(model, input, cached, output)`
  billing `(input−cached)·input + cached·cacheRead + output·output` with the same
  saturating integer math. A model with no cache price bills cached at the input
  rate (conservative); `cached==0` is identical to `costMicrocents`.
- **`kernel/governor/governor.go`** `recordUsage`: clamps cached to `[0, input]`,
  bills via `costMicrocentsCached`, and records `cached_input_tokens` on
  `budget.consumed`.
- **`cmd/agezt/main.go`**: `AGEZT_DEMO_CACHED=1` demo hook (scripts one priced,
  mostly-cached answer) + `kernel/controlplane/config.go` inventory entry.

## Files
- `kernel/catalog/types.go`, `kernel/agent/agent.go`,
  `plugins/providers/openai/openai.go`, `kernel/governor/pricing.go`,
  `kernel/governor/governor.go`, `cmd/agezt/main.go`,
  `kernel/controlplane/config.go` (edited).
- `kernel/governor/pricing_internal_test.go`: **new** `TestCostMicrocentsCached`
  (cached=0 ≡ non-cached; cached billed at cache-read rate & strictly cheaper; no
  cache price ⇒ input rate; cached clamped to input). The existing white-box test
  updated to keyed `modelPrice` literals.

## Verification
- Full suite **1899**, 68 packages, `go test ./...` exit 0; `go vet ./...` clean;
  `gofmt -l` clean on all touched files; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged.
- **Live-proven offline**: `AGEZT_DEMO_CACHED=1` (synthetic usage: 10000 input,
  9000 cached, 200 output on `claude-sonnet-4-6`) → `budget.consumed` recorded
  `cached_input_tokens: 9000` and `cost_microcents: 870000` ($0.00087). Billing
  every input token at the full rate would have been `3300000` ($0.0033) — a
  2_430_000-microcent (~74%) reduction, exactly the cache-read math
  `((10000−9000)·300M + 9000·30M + 200·1500M)/1e6`.

## Scope notes
- `agent.Usage.CachedInputTokens` is additive (omitempty); providers that don't
  report cached tokens leave it 0 → behaviour unchanged. Only the openai/compat
  decoder parses it so far — Anthropic/Gemini/others can thread their own
  cache-token fields into the same `Usage` field in follow-ups.
- The exported `governor.CostMicrocents(model,in,out)` and the agent loop's
  per-run `CostFn` are unchanged (they stay cache-agnostic — a conservative
  run-budget *estimate*); only the authoritative `recordUsage` →
  `budget.consumed` path is cache-aware, which is what cost-tracking clients read.
- Fallback Claude cache-read prices are list-accurate (0.1× input); the live
  catalog overrides per-model on `agt catalog sync`.

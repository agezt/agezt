# M290 — Anthropic cache-token accounting

## Why
M289 made the governor cache-aware but only the openai/compat decoder reported
cached tokens. Anthropic is the default provider and has the strongest caching
story — and its accounting was actually **wrong** before this: Anthropic reports
`usage.input_tokens` *excluding* cached prompt tokens, with
`cache_read_input_tokens` and `cache_creation_input_tokens` separate. The
providers parsed only `input_tokens`, so cached prompt tokens (reads AND
creations) were **dropped — billed at zero** whenever prompt caching was active.
That's an under-count (the dangerous direction). This wires Anthropic into the
M289 model.

## What
- **`plugins/providers/anthropic/anthropic.go`**: usage struct gains
  `cache_read_input_tokens` / `cache_creation_input_tokens`; a shared
  `anthUsageToAgent(input, cacheRead, cacheCreation, output, model)` maps them to
  `agent.Usage` as `InputTokens = input+read+creation`, `CachedInputTokens = read`.
- **`plugins/providers/anthropic/streaming.go`**: `message_start` parses the two
  cache fields into `streamState`; `assembleResponse` uses `anthUsageToAgent`.
- **`plugins/providers/vertex/anthropic.go`** (Anthropic-on-Vertex): the same, via
  a local `anthVxUsageToAgent` (separate package); non-streaming + streaming.

Semantics: cache reads bill at the model's cache-read rate (M289, cheap);
cache-creation tokens fold into the input total (billed at the input rate — the
cache-*write* premium is not modelled yet, a documented follow-up). Either way
this is strictly more accurate than billing them at zero.

## Files
- `plugins/providers/anthropic/anthropic.go`,
  `plugins/providers/anthropic/streaming.go`,
  `plugins/providers/vertex/anthropic.go` (edited).
- `plugins/providers/anthropic/anthropic_test.go`: **new**
  `TestDecodeResponse_CacheUsage` (input 100 + read 900 + creation 50 → Input
  1050, Cached 900).
- `plugins/providers/anthropic/streaming_test.go`: **new**
  `TestParseStream_CacheUsage` (message_start read 600 + creation 10 + input 40 →
  Input 650, Cached 600).

## Verification
- Full suite **1901**, 68 packages, `go test ./...` exit 0; `go vet` clean on the
  touched packages; `gofmt -l` clean; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged.
- **Network-free proof**: the decode + stream tests above assert the split-token
  wire shape maps to the canonical `agent.Usage` for both Anthropic code paths.
- **Billing effect proven in M289**: the governor bills `CachedInputTokens` at the
  cache-read rate (M289's `AGEZT_DEMO_CACHED` live demo: 9000 cached → ~74% lower
  cost). M290 makes Anthropic emit that field, so Anthropic now gets the same
  treatment instead of dropping cache tokens.

## Scope notes
- Reuses the M289 cost model unchanged — no kernel/governor/catalog edits, no new
  `agent.Usage` field. Low risk.
- Cache-*write* premium (Anthropic bills cache-creation at ~1.25× input) is not
  modelled — creation tokens bill at the input rate. A follow-up could add
  `CacheWriteInputTokens` + a cache-write rate (catalog already has
  `cost.cache_write`) across `agent.Usage` / `governor` / the providers.
- Remaining providers without cache-token parsing: Gemini, Bedrock, Cohere,
  Mistral (lower-traffic or weaker caching); each threads its own fields into the
  same `agent.Usage.CachedInputTokens` when wanted.

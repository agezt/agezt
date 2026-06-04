# M296 — Claude-on-Bedrock cache-token accounting

## Why
M290 fixed an under-count for direct Anthropic (cached prompt tokens billed at
zero). Claude-on-Bedrock has the *same* wire shape — `input_tokens` excludes
cached, with `cache_read_input_tokens` / `cache_creation_input_tokens` separate —
and the same bug: the Bedrock provider parsed only `input_tokens`. AWS Bedrock is
a production path for Claude with strong prompt caching, so this is a real
cost-accuracy fix, not just completeness.

## What
Mirrors M290 (Anthropic) and M291 (cache-write) via a local
`anthBedrockUsageToAgent(input, cacheRead, cacheCreation, output, model)`:
`InputTokens = input+read+creation`, `CachedInputTokens = read`,
`CacheWriteInputTokens = creation` — reads bill at the cache-read rate, creations
at the cache-write premium (M289/M291).

- **`plugins/providers/bedrock/bedrock.go`**: usage struct gains the two cache
  fields; the non-streaming decode uses the helper.
- **`plugins/providers/bedrock/streaming.go`**: `bedStreamState.cacheRead` /
  `cacheCreation`, captured from `message_start`; `assembleBedrockResponse` uses
  the helper.

## Files
- `plugins/providers/bedrock/bedrock.go`,
  `plugins/providers/bedrock/streaming.go` (edited).
- `plugins/providers/bedrock/bedrock_test.go`: **new**
  `TestComplete_BedrockCacheUsage` (input 100 + read 900 + creation 50 → Input
  1050, Cached 900, Write 50).

## Verification
- Full suite **1907**, 68 packages, `go test ./...` exit 0; `go vet` clean on the
  package; `gofmt -l` clean on the touched files; `GOOS=linux` build clean;
  `go.mod` / `go.sum` unchanged.
- **Network-free proof**: the httptest decode test asserts the split-token wire
  maps to the canonical `agent.Usage`.
- **Billing effect proven in M289/M291**: the governor bills `CachedInputTokens`
  at the cache-read rate and `CacheWriteInputTokens` at the cache-write rate.

## Scope notes
- Reuses the M289–M291 cost model unchanged — no kernel/governor edits.
- Cache-token parsing now covers OpenAI/compat (M289), Anthropic direct/vertex
  (M290) + Bedrock (M296), and Gemini direct/vertex (M295). Remaining: Cohere and
  Mistral (weaker/less-common caching) — each threads its own fields into the same
  `agent.Usage` cache fields if/when wanted; the cache-aware billing core needs no
  further change.

# M232 — Make DeepSeek work (compat classification)

## Why
The README lists DeepSeek in the compat provider's supported vendors ("Groq,
DeepSeek, xAI, OpenRouter, Together, …"). But it didn't actually work. DeepSeek's
official Vercel AI SDK package is `@ai-sdk/deepseek`, and `catalog.FamilyFromNPM`
only enumerates a fixed set of openai-compatible vendor packages — `deepseek`
wasn't among them, so it fell through to `FamilyUnknown`. `compat.Build` then
refused it at the family switch:

```
compat: provider family not yet supported: family="unknown" provider="deepseek"
(… This branch should be unreachable for any models.dev catalog entry …)
```

The error even asserts that branch is unreachable for catalog entries — but it
*is* reachable for DeepSeek, which models.dev carries. So a README-claimed vendor
was dead on arrival. Found by probing the compat path after M230/M231.

DeepSeek speaks the OpenAI Chat Completions dialect (Bearer auth,
`/chat/completions`), so it belongs in `FamilyOpenAICompatible` exactly like the
vendors M230 already handled.

## What
- **`kernel/catalog/types.go`** — added `"deepseek"` to the
  `FamilyOpenAICompatible` case in `FamilyFromNPM`, so `@ai-sdk/deepseek`
  classifies correctly and `compat.Build` routes it to the openai adapter.
- **`plugins/providers/compat/compat.go`** — added DeepSeek's base URL
  (`https://api.deepseek.com/v1`) to `compatVendorBaseURL` (M230's table), so it
  needs only `DEEPSEEK_API_KEY` — no `custom.json` URL. (DeepSeek keys are
  `sk-…`, already redacted, so no redaction change was needed.)

## Files
- `kernel/catalog/types.go` — classify `deepseek` as openai-compatible (edited).
- `plugins/providers/compat/compat.go` — deepseek base URL (edited).
- `kernel/catalog/catalog_test.go` — `@ai-sdk/deepseek` → `FamilyOpenAICompatible`
  added to the `TestFamilyFromNPM` table (edited).
- `plugins/providers/compat/compat_m232_test.go` — `TestDeepSeek_NowFirstClass`
  (new): asserts the family classification, the base URL, and that `Build` with
  an empty `api` now succeeds (was `ErrFamilyUnsupported`).

## Verification
- `go test ./kernel/catalog/ ./plugins/providers/compat/` — green; full suite
  **1760 → 1761** (+1), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet` clean on both packages.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof:** a probe builds a DeepSeek provider via `compat.Build` (the
  daemon's exact provider-construction path) with an empty `api` — it now returns
  a provider named `"deepseek"` where before M232 it returned
  `ErrFamilyUnsupported`.

## Scope notes
- Base URL `https://api.deepseek.com/v1` is DeepSeek's documented OpenAI-compatible
  root; an explicit catalog `api` (custom.json) still overrides it.
- This is the last README-named compat vendor that wasn't wired; the others were
  covered in M230.

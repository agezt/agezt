# M230 — Built-in base URLs for OpenAI-compatible vendors

## Why
agezt's README advertises the `compat` provider as supporting "OpenAI-compatible
vendors: Groq, DeepSeek, xAI, OpenRouter, Together, …", and `catalog.FamilyFromNPM`
already enumerates several of them (`groq`, `xai`, `cerebras`, `togetherai`,
`deepinfra`, `perplexity`, `fireworks`, plus OpenRouter) as
`FamilyOpenAICompatible`. But when such a provider's catalog `api` URL was empty,
`compat.Build` **refused** it:

```
provider "groq" is openai-compatible but has no `api` URL in the catalog — add it via custom.json
```

That refusal is correct in spirit (an empty URL would otherwise inherit the
openai adapter's `api.openai.com` default and silently misroute to the wrong
vendor). But it's friction the design intended to avoid — `defaultBaseURL`'s own
doc says compat "carries those defaults so operators don't have to add custom.json
entries just to get a working setup." That was only true for the single-host
families (Mistral, Cohere), not the openai-compatible vendors. So agezt *knew*
"groq" was a vendor yet wouldn't fill in its (stable, well-documented) URL.

## What
- **`plugins/providers/compat/compat.go`** — a new `compatVendorBaseURL(npm)`
  table maps the well-known OpenAI-compatible vendors to their documented base
  URLs, keyed on the npm package the **same way** `catalog.FamilyFromNPM`
  classifies them (lowercase, the `@openrouter/…` special case, then strip
  `@ai-sdk/`), so the URL table and the family table agree on what's a known
  vendor. `Build` consults it for `FamilyOpenAICompatible` providers when the
  catalog `api` is empty, *after* the explicit `api` (which still wins) and
  *before* the empty-api guard (which still fires for unrecognised vendors).

  | vendor | base URL |
  |---|---|
  | groq | `https://api.groq.com/openai/v1` |
  | xai | `https://api.x.ai/v1` |
  | cerebras | `https://api.cerebras.ai/v1` |
  | togetherai | `https://api.together.xyz/v1` |
  | deepinfra | `https://api.deepinfra.com/v1/openai` |
  | perplexity | `https://api.perplexity.ai` |
  | fireworks | `https://api.fireworks.ai/inference/v1` |
  | openrouter | `https://openrouter.ai/api/v1` |

  Comments on the guard and `defaultBaseURL` were updated to match.

## Files
- `plugins/providers/compat/compat.go` — `compatVendorBaseURL` + the Build
  fallback + comment fixes (edited).
- `plugins/providers/compat/compat_m230_test.go` — 4 tests (new, white-box):
  the URL table (incl. case-insensitivity and the `""` cases that keep the guard
  active), every known vendor classifies as `FamilyOpenAICompatible` and has a
  default, `Build` for a known vendor with empty `api` now succeeds, and an
  explicit `api` still builds (override path).
- `plugins/providers/compat/compat_test.go` — the existing
  `TestBuild_OpenAICompatibleEmptyAPIRefused` now uses an *unrecognised* vendor
  (`@ai-sdk/openai-compatible`) so it still asserts the guard for the unknown
  case (edited).

## Verification
- `go test ./plugins/providers/compat/` — green; full suite **1753 → 1757**
  (+4), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/providers/compat/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- URLs cross-checked against vendor docs (Cerebras confirmed via search; the
  rest are the documented OpenAI-compatible roots).
- **Proof:** the tests exercise the daemon's exact provider-construction entry
  point — `compat.Build` with a realistic `catalog.Provider` — and show the
  before→after change (a known vendor with empty `api`: refused → builds) plus
  the two invariants that bound the risk (explicit `api` wins; unknown vendor
  still refused). A real network call isn't meaningful offline since the URLs
  target the live vendors by design.

## Scope / risk
- The defaults are a *fallback*: an explicit catalog `api` (custom.json) always
  takes precedence, so a vendor relocating its endpoint is a one-line operator
  fix, never a rebuild.
- DeepSeek is intentionally omitted: models.dev classifies it via the generic
  `openai-compatible` npm rather than a vendor-named package, so there's no
  stable per-vendor key to attach a default to here; it continues to use its
  catalog `api` (which models.dev populates).

# Phase Report — Milestone 1.h (OpenAI Chat Completions adapter; openai-compatible fleet)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-06](TASKS.md), continuing the user's catalog-first
> directive from M1.f/M1.g.
> Continues [PHASE-M1.g-REPORT.md](PHASE-M1.g-REPORT.md).

## Scope

M1.h is the biggest single-step unlock in the catalog pivot. One new
wire adapter — [`plugins/providers/openai`](../plugins/providers/openai/openai.go)
— covers two compatibility families: `FamilyOpenAI` (the real
api.openai.com) and `FamilyOpenAICompatible`. Plus a one-line
extension to `FamilyFromNPM` that recognises the first-party Vercel
AI SDK packages whose wire dialect is *also* OpenAI Chat Completions
(`@ai-sdk/groq`, `@ai-sdk/xai`, `@ai-sdk/cerebras`, …).

Counted against the synced [models.dev](https://models.dev/api.json)
catalog:

| | M1.g | M1.h | Δ |
|---|---:|---:|---:|
| Supported catalog providers | ~7 | **121** | **+114** |
| Families wired | 2 (anthropic, ollama) | 4 (+ openai, openai-compatible) | +2 |

| Concern | M1.h status |
|---|---|
| OpenAI Chat Completions adapter (Bearer auth, /v1/chat/completions) | ✅ `plugins/providers/openai` |
| Real OpenAI provider | ✅ via `FamilyOpenAI` |
| Groq / xAI / Cerebras / Together / DeepInfra / Perplexity / Fireworks / OpenRouter / DeepSeek | ✅ via `FamilyOpenAICompatible` |
| Empty `api` field guard (prevents Groq calls landing at api.openai.com) | ✅ `ErrFamilyUnsupported` with hint at `custom.json` |
| Tool calls (assistant `tool_calls[]` with JSON-string `arguments`; `role:"tool"` results) | ✅ canonical ↔ OpenAI translation |
| Synthetic tool-call IDs for openai-compat servers that omit `id` | ✅ `call-<i>` fallback |
| Per-vendor base URL discovered from catalog `api` field | ✅ `resolveEndpoint` handles `/v1`, `/v1/`, and bare hosts |
| Google / Mistral / Cohere / Bedrock / Azure | ⏳ M1.i (separate adapters) |

## Changes

### 1. New `plugins/providers/openai` package

In-process OpenAI Chat Completions adapter. Same `Provider` shape as
`anthropic`/`ollama`: `APIKey`, `Endpoint`, `BaseURL`, `Model`, `HTTP`.
`Name()` returns `"openai"` (the wire-family default; the
catalog-driven `namedProvider` wrapper overrides this to the catalog
id at construction time, so the Governor's registry stays
catalog-aligned).

`resolveEndpoint` precedence:

```
1. explicit Endpoint
2. BaseURL — append /chat/completions if already ends with /v1 or
            contains /v1/; otherwise append /v1/chat/completions
3. DefaultEndpoint (https://api.openai.com/v1/chat/completions)
```

The `/v1`-aware suffixing matches how models.dev publishes
openai-compatible URLs: most carry the full `/v1` root already
(`https://api.groq.com/openai/v1`, `https://api.deepseek.com`, …).

**Dialect translation** (canonical ↔ OpenAI):

- Assistant `tool_calls[]` carry `arguments` as a JSON-encoded
  string (OpenAI's spec); the adapter casts the canonical
  `json.RawMessage` to/from that string form.
- `role:"tool"` results require `tool_call_id`; round-trip verified
  in `TestComplete_RoundtripWithToolResult`.
- `finish_reason` mapping: `stop` → `StopEndTurn`, `tool_calls` /
  `function_call` → `StopToolUse`, `length` → `StopMaxTokens`.
- Defensive: if `tool_calls` are present but `finish_reason` is
  missing (some openai-compat servers do this), force `StopToolUse`.
- Defensive: synthesize `call-<i>` IDs when the server returns
  empty `id` fields (Ollama-style behaviour leaking through).

7 tests in `plugins/providers/openai/openai_test.go`:
text response with system prompt; tool-call emission;
tool-result round-trip; 4-case endpoint resolution;
no-key + API-error errors.

### 2. `compat.Build` routes openai families to the new adapter

Added one `case` to the switch:

```go
case catalog.FamilyOpenAI, catalog.FamilyOpenAICompatible:
    if p.Family() == catalog.FamilyOpenAICompatible && strings.TrimSpace(base) == "" {
        return nil, "", fmt.Errorf("%w: provider %q is openai-compatible but has no `api` URL in the catalog — add it via custom.json",
            ErrFamilyUnsupported, p.ID)
    }
    op := openai.New(apiKey)
    op.BaseURL = base
    op.Endpoint = ""
    op.Model = modelID
    return &namedProvider{name: p.ID, inner: op}, modelID, nil
```

The empty-`api` guard is critical: an openai-compatible entry with no
base URL would fall through to the adapter's `DefaultEndpoint` and
silently send Groq/xAI/DeepSeek traffic to `api.openai.com` —
wrong-vendor leakage with the user's bearer token. Real OpenAI is
exempt because its default endpoint *is* the right destination.

`IsSupportedFamily` extended to include the two new families.

### 3. `FamilyFromNPM` recognises per-vendor AI SDK packages

models.dev tags ~7 providers with first-party Vercel AI SDK npm
packages (`@ai-sdk/groq`, `@ai-sdk/xai`, …) rather than the generic
`@ai-sdk/openai-compatible`. Wire-dialect-wise they're all OpenAI
Chat Completions. The mapping is a literal switch case:

```go
case "groq", "xai", "cerebras", "togetherai",
     "deepinfra", "perplexity", "fireworks":
    return FamilyOpenAICompatible

// non-Vercel namespaces handled before TrimPrefix:
case "@openrouter/ai-sdk-provider":
    return FamilyOpenAICompatible
```

This is the per-provider knowledge layer — if a new openai-compat
vendor ships a first-party AI SDK package with a unique npm tag,
add it here. No other code changes.

Extended `TestFamilyFromNPM` to cover all 8 new mappings (groq, xai,
cerebras, togetherai, deepinfra, perplexity, fireworks, openrouter).

### 4. `compat.IsSupportedFamily` enumeration

```go
case catalog.FamilyAnthropic,
     catalog.FamilyOllama,
     catalog.FamilyOpenAI,
     catalog.FamilyOpenAICompatible:
    return true
```

Test `TestIsSupportedFamily` now enumerates **all 10** family values
explicitly (was 6) so additions or removals in either direction
break the test loudly.

## Demo transcript

Reuses the M1.g demo home; catalog already synced.

### Step 1 — catalog now recognises every major openai-compat vendor

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo agt catalog list \
    | grep -E "^  (groq|cerebras|xai|deepseek|fireworks-ai|openai|openrouter|perplexity|togetherai)\b"
  cerebras      (Cerebras,        family=openai-compatible)  [no creds]
  deepinfra     (Deep Infra,      family=openai-compatible)  [no creds]
  deepseek      (DeepSeek,        family=openai-compatible)  [no creds]
  fireworks-ai  (Fireworks AI,    family=openai-compatible)  [no creds]
  groq          (Groq,            family=openai-compatible)  [no creds]
  openai        (OpenAI,          family=openai)             [no creds]
  openrouter    (OpenRouter,      family=openai-compatible)  [no creds]
  perplexity    (Perplexity,      family=openai-compatible)  [no creds]
  togetherai    (Together AI,     family=openai-compatible)  [no creds]
  xai           (xAI,             family=openai-compatible)  [no creds]
```

Pre-M1.h, all of these except `deepseek` and `openai` reported
`family=unknown` and were unselectable.

### Step 2 — openai routes through the new adapter

```
$ AGEZT_PROVIDER=openai AGEZT_MODEL=gpt-4o-mini \
  OPENAI_API_KEY=sk-fake agezt
  governor : primary=openai(catalog; family=openai, model=gpt-4o-mini)
             → fallback=mock(offline), daily_ceiling=$20.00
```

### Step 3 — empty `api` field is refused (no silent wrong-vendor leak)

models.dev's `groq` entry has no `api` field (the AI SDK has it
baked in). Without `custom.json`, M1.h refuses to construct it:

```
$ AGEZT_PROVIDER=groq GROQ_API_KEY=gsk-fake agezt
agezt: compat: provider family not yet supported:
  provider "groq" is openai-compatible but has no `api` URL in the catalog
  — add it via custom.json
```

### Step 4 — operator adds the URL via `custom.json`

```
$ cat /tmp/agezt-m1g-demo/catalog/custom.json
{
  "groq": {
    "id": "groq", "name": "Groq",
    "npm": "@ai-sdk/groq",
    "api": "https://api.groq.com/openai/v1",
    "env": ["GROQ_API_KEY"],
    "models": { "llama-3.3-70b-versatile":
                { "id": "llama-3.3-70b-versatile", "name": "Llama 3.3 70B Versatile" } }
  }
}

$ AGEZT_PROVIDER=groq AGEZT_MODEL=llama-3.3-70b-versatile \
  GROQ_API_KEY=gsk-fake agezt
  governor : primary=groq(catalog; family=openai-compatible,
             model=llama-3.3-70b-versatile) → fallback=mock(offline),
             daily_ceiling=$20.00
```

Same pattern works for `xai` (`https://api.x.ai/v1`), `cerebras`
(`https://api.cerebras.ai/v1`), `togetherai`
(`https://api.together.xyz/v1`), `perplexity`
(`https://api.perplexity.ai`), and any future openai-compat vendor.

## Architectural consequences

1. **One adapter, two families.** Operations folk often think of
   "OpenAI" and "OpenAI-compatible" as two distinct vendors. At the
   wire level they're the *same* — Bearer + `/v1/chat/completions` —
   so they share one `*openai.Provider` constructor. The difference
   is purely catalog metadata (base URL, env var). This is the
   strongest argument for the catalog-pivot architecture: the
   difference between "talking to OpenAI" and "talking to Groq" is
   one line in `custom.json`.

2. **Per-vendor SDK packages don't fragment the wire layer.** When
   Vercel publishes `@ai-sdk/groq` distinct from `@ai-sdk/openai-compatible`,
   that's a Vercel ergonomics choice; the underlying HTTP shape is
   identical. M1.h folds that distinction into `FamilyFromNPM` so the
   compat adapter never has to know about per-vendor variation.

3. **Wrong-vendor leakage is now structurally impossible.** The empty-
   `api` guard means a misconfigured openai-compat entry fails *at
   startup* with a clear error, instead of routing a bearer token to
   the wrong host. The default endpoint of an HTTP wire adapter must
   only apply when *that adapter is the primary use case* — i.e.
   only the real `FamilyOpenAI` keeps the api.openai.com default.

4. **Catalog refresh ≠ wire compatibility.** A daily `agt catalog
   sync` will pull new models for existing providers automatically.
   But adding a *new wire family* (Google's `generateContent`,
   Bedrock's signed-request shape, Mistral's chat) still requires a
   new adapter package. M1.i ships those.

## Deferrals → M1.i and beyond

Still returning `ErrFamilyUnsupported` after M1.h:

- **google** (`@ai-sdk/google`, `@ai-sdk/google-generative-ai`,
  `@ai-sdk/google-vertex`) — Gemini's `generateContent` API has a
  meaningfully different request shape (`contents` instead of
  `messages`, function calling under `functionDeclarations`).
- **mistral** (`@ai-sdk/mistral`) — close to OpenAI's shape but with
  its own auth+headers quirks.
- **cohere** (`@ai-sdk/cohere`) — different request format.
- **aws-bedrock** (`@ai-sdk/amazon-bedrock`) — needs SigV4 signing.
- **azure** (`@ai-sdk/azure`) — resource-specific base URLs with
  api-version query params.

Plus the long-deferred items unchanged from M1.g:
subscription-first routing, `agt provider creds`, browser tool,
plugin host, Pulse v1, planner.

## Files touched

```
plugins/providers/openai/openai.go         NEW
plugins/providers/openai/openai_test.go    NEW (7 tests, 5 sub-tests)
plugins/providers/compat/compat.go         (+ openai case, + empty-api guard,
                                              + openai import, doc update,
                                              error message: M1.h hint → M1.i)
plugins/providers/compat/compat_test.go    (repurposed unused helper into
                                              TestBuild_OpenAIFamilyRoutesToOpenAIWire,
                                              + TestBuild_OpenAICompatibleFamilyRoutesToOpenAIWire,
                                              + TestBuild_OpenAICompatibleEmptyAPIRefused,
                                              expanded TestIsSupportedFamily)
kernel/catalog/types.go                    (FamilyFromNPM: + 7 vendor SDK
                                              names + @openrouter/ai-sdk-provider)
kernel/catalog/catalog_test.go             (TestFamilyFromNPM: + 8 mappings)
```

No schema changes. No runtime changes. No daemon command changes.
Same `AGEZT_PROVIDER=<catalog-id>` + `AGEZT_MODEL=<id>` UX as M1.g.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 231 pass, 0 fail (up from 217 in M1.g)
```

Per-package growth in this phase: `plugins/providers/openai` (new,
7 tests), `plugins/providers/compat` (+4 tests), `kernel/catalog`
(+8 cases in TestFamilyFromNPM).

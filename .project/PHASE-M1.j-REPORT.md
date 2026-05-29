# Phase Report — Milestone 1.j (Mistral via OpenAI adapter; per-family default URLs)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-08](TASKS.md). Continues
> [PHASE-M1.i-REPORT.md](PHASE-M1.i-REPORT.md).

## Scope

M1.j is a deliberately small phase that ships two things:

1. **Mistral support** — `api.mistral.ai/v1` is wire-identical to
   OpenAI Chat Completions (Bearer auth, same body, same tool
   shape). M1.j adds a `FamilyMistral` case to `compat.Build` that
   reuses the existing `plugins/providers/openai` adapter. No new
   wire package; ~10 lines of routing.

2. **Per-family default base URLs** — models.dev's `api` field is
   empty for vendors whose URL is well-known to their first-party
   AI SDK package (anthropic, openai, mistral, google, ollama). M1.j
   introduces `defaultBaseURL(catalog.Family)` so operators don't
   need a `custom.json` entry just to get those vendors working.
   `FamilyOpenAICompatible` deliberately returns `""` — the
   empty-`api` guard from M1.h still fires there.

| | M1.i | M1.j | Δ |
|---|---:|---:|---:|
| Supported catalog providers | 122 | **123** | +1 (mistral) |
| Families wired | 5 | **6** | +1 (mistral) |
| Per-family default URLs encoded | (constructor defaults only) | **5** | — |

| Concern | M1.j status |
|---|---|
| Mistral routing through openai adapter | ✅ |
| Mistral default base URL when catalog `api` empty | ✅ `api.mistral.ai/v1` via `defaultBaseURL` |
| openai-compatible empty-`api` guard still fires | ✅ `defaultBaseURL` returns `""` for that family |
| `IsSupportedFamily` reports mistral | ✅ |
| Refusal message lists current support + next-up deferrals | ✅ |
| Vertex / Cohere / Bedrock / Azure | ⏳ M1.k+ |

## Changes

### 1. `defaultBaseURL` centralises catalog defaults

```go
func defaultBaseURL(f catalog.Family) string {
    switch f {
    case catalog.FamilyAnthropic: return "https://api.anthropic.com"
    case catalog.FamilyOpenAI:    return "https://api.openai.com/v1"
    case catalog.FamilyGoogle:    return "https://generativelanguage.googleapis.com"
    case catalog.FamilyOllama:    return "http://localhost:11434"
    case catalog.FamilyMistral:   return "https://api.mistral.ai/v1"
    }
    return ""
}
```

In `Build`:

```go
base := strings.TrimSpace(p.API)
if base == "" {
    base = defaultBaseURL(p.Family())
}
```

Why this lives in `compat`, not in each wire package's
`resolveEndpoint`: the wire packages already have their own
constructor defaults (used when neither `Endpoint` nor `BaseURL` is
set), but those are *wire-family* defaults — there's only one
"OpenAI" endpoint. `compat` operates at the *catalog-family* layer,
where Mistral reuses the OpenAI adapter but needs a *different*
default URL. Centralising here means the OpenAI adapter doesn't have
to know about every vendor that reuses its wire shape.

`FamilyOpenAICompatible` returning `""` is load-bearing:
post-M1.h that family has an explicit empty-`api` refusal so a
misconfigured Groq entry can never silently route to api.openai.com.
The new `defaultBaseURL` preserves that behaviour by design.

### 2. Mistral case in `compat.Build`

```go
case catalog.FamilyMistral:
    // api.mistral.ai/v1 is wire-identical to OpenAI Chat
    // Completions — reuse the openai adapter; default base URL
    // comes from defaultBaseURL.
    mp := openai.New(apiKey)
    mp.BaseURL = base
    mp.Endpoint = ""
    mp.Model = modelID
    return &namedProvider{name: p.ID, inner: mp}, modelID, nil
```

`IsSupportedFamily` extended; `TestIsSupportedFamily` table updated
(`FamilyMistral: true`).

### 3. Tests

Two new tests in `plugins/providers/compat/compat_test.go`:

- `TestBuild_MistralRoutesThroughOpenAIWire` — full Bearer-auth +
  `/v1/chat/completions` roundtrip through an httptest server using
  a Mistral catalog entry; verifies `prov.Name()=="mistral"` (not
  `"openai"`, despite reusing the openai adapter — the
  `namedProvider` wrapper substitutes the catalog id) and that the
  request body carries `model: mistral-small-latest`.
- `TestBuild_MistralDefaultBaseURLWhenAPIEmpty` — verifies
  `Build` succeeds when the catalog entry has no `api` field. This
  is the regression guard: if `defaultBaseURL` ever returns `""`
  for `FamilyMistral`, the openai adapter would fall through to
  api.openai.com and send Mistral keys to OpenAI — same wrong-
  vendor leak the openai-compatible guard exists to prevent.

Plus the existing `TestIsSupportedFamily` updated for the new
mistral mapping.

## Demo transcript

Reuses the demo home from M1.g–M1.i with its synced catalog.

### Step 1 — Mistral is now classified and selectable

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo agt catalog list | grep "^  mistral"
  mistral  (Mistral, family=mistral)  [no creds]
```

### Step 2 — daemon routes mistral through the openai wire

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=mistral \
  MISTRAL_API_KEY=ms-fake AGEZT_MODEL=mistral-small-latest agezt
  governor : primary=mistral(catalog; family=mistral,
             model=mistral-small-latest) → fallback=mock(offline),
             daily_ceiling=$20.00
```

Note `family=mistral` in the banner even though the wire goes
through the openai adapter — the catalog-family identity stays
distinct from the wire-family choice.

### Step 3 — refusal message lists the new M1.j frontier

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=cohere \
  COHERE_API_KEY=fake agezt
agezt: compat: provider family not yet supported:
  family="cohere" provider="cohere"
  (M1.j supports anthropic + ollama + openai + openai-compatible + google + mistral;
   vertex/cohere/bedrock/azure land in M1.k+)
```

## Architectural consequences

1. **Family ≠ adapter, finally honest.** Pre-M1.j every supported
   family had its own wire adapter package. Mistral is the first
   "category" entry that shares an adapter with another category.
   This is the right architecture: the *adapter* knows the wire
   protocol; the *catalog family* names a vendor. When `FamilyMistral`
   becomes its own adapter later (because Mistral diverges from
   OpenAI), the catalog identity doesn't change — only the case
   branch's first line does.

2. **Per-family defaults belong in the routing layer.** A wire
   package can have at most one default (it knows one protocol).
   `compat` knows the full *family taxonomy* and is the right place
   to encode "Mistral's catalog `api` defaults to `api.mistral.ai/v1`".
   This pattern scales — Cohere's default lives here when it ships,
   even though its wire package will be new.

3. **The empty-`api` discipline is now formalised.** Three behaviours,
   each correct for its family:
   - **Anthropic / OpenAI / Google / Ollama / Mistral** — default URL exists, fall through.
   - **openai-compatible** — *no* default; refuse with hint at custom.json.
   - **Unsupported families** — refuse with M1.k+ deferral hint.

   The same `defaultBaseURL("") == ""` check would now correctly
   reject a misconfigured Cohere entry if Cohere followed this
   pattern (it won't — Cohere's wire is different enough to deserve
   its own adapter).

## Deferrals → M1.k and beyond

Still returning `ErrFamilyUnsupported` after M1.j:

- **cohere** (`@ai-sdk/cohere`) — distinct `chat` API
  (`message` + `chat_history`, no `messages[]` array).
- **azure** (`@ai-sdk/azure`) — openai-shaped body but
  resource-specific URL with `?api-version=...` and deployment name
  in the path.
- **aws-bedrock** (`@ai-sdk/amazon-bedrock`) — SigV4 signing,
  region-aware URL, model id in path.
- **google-vertex** (`@ai-sdk/google-vertex`,
  `@ai-sdk/google-vertex/anthropic`) — service-account OAuth,
  regional URL builder.

Unchanged deferrals from prior milestones: subscription-first
routing, `agt provider creds`, browser tool, plugin host, Pulse v1,
planner.

## Files touched

```
plugins/providers/compat/compat.go         (+ FamilyMistral case,
                                              + defaultBaseURL helper,
                                              + per-family base URL fallback,
                                              + IsSupportedFamily entry,
                                              doc update + error-message update)
plugins/providers/compat/compat_test.go    (+ TestBuild_MistralRoutesThroughOpenAIWire,
                                              + TestBuild_MistralDefaultBaseURLWhenAPIEmpty,
                                              expanded TestIsSupportedFamily)
```

No new wire packages. No schema changes. No daemon-command changes.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 245 pass, 0 fail (up from 243 in M1.i)
```

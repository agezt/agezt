# Phase Report — Milestone 1.l (Azure OpenAI; openai adapter learns auth customisation)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-10](TASKS.md). Continues
> [PHASE-M1.k-REPORT.md](PHASE-M1.k-REPORT.md).

## Scope

M1.l adds Azure OpenAI Service via a small extension to the existing
openai adapter rather than a new wire package. Azure's wire is
openai-shaped at the body level — same `/chat/completions` request
and response — but two things differ from real OpenAI:

1. **URL structure:** `{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version=2024-10-21`. Resource subdomain, deployment-in-path (NOT model name in body), required `api-version` query param.
2. **Auth header:** `api-key: <key>` instead of `Authorization: Bearer <key>`.

M1.l unblocks the auth difference by adding two optional fields to
`openai.Provider` (`AuthHeader`, `AuthScheme`) — additive, backward-
compatible. The URL difference lives in `compat`, which composes the
full URL from the catalog entry's env vars (resource name) and the
selected model id (deployment name).

Both Azure catalog entries (`azure` and `azure-cognitive-services`,
which differ only in env-var prefix) are now supported.

| | M1.k | M1.l | Δ |
|---|---:|---:|---:|
| Supported catalog providers | 124 | **126** | +2 (azure, azure-cognitive-services) |
| Families wired | 7 | **8** | +1 (azure) |
| Wire adapter packages | 5 | **5** | 0 (azure reuses openai) |

| Concern | M1.l status |
|---|---|
| Azure URL builder (resource + deployment + api-version) | ✅ in `compat.Build` |
| Dual-credential resolution (resource + api-key from env) | ✅ via `resolveAzureCreds` |
| `api-key` header instead of Bearer | ✅ via `openai.Provider.AuthHeader/AuthScheme` |
| Operator escape hatch: `api` field in custom.json overrides URL builder | ✅ |
| Default api-version with `AGEZT_AZURE_API_VERSION` override | ✅ `2024-10-21` |
| Clear refusal when resource missing AND no `api` override | ✅ "or an `api` URL in custom.json" hint |
| Existing openai callers (Bearer) unchanged | ✅ verified by `TestComplete_DefaultAuthIsBearer` |
| Vertex / Bedrock | ⏳ M1.m+ |

## Changes

### 1. `openai.Provider` gains `AuthHeader` + `AuthScheme`

```go
type Provider struct {
    APIKey   string
    Endpoint string
    BaseURL  string
    Model    string
    HTTP     *http.Client

    // NEW (both optional, both zero-valued by default):
    AuthHeader string // defaults to "Authorization"
    AuthScheme string // defaults to "Bearer "
}
```

Implementation in `Complete`:

```go
authHeader := p.AuthHeader
if authHeader == "" { authHeader = "Authorization" }
authScheme := p.AuthScheme
if authScheme == "" && p.AuthHeader == "" {
    // Only default the scheme when the header is also defaulted, so
    // an explicit empty-scheme caller (Azure) isn't silently
    // promoted back to Bearer.
    authScheme = "Bearer "
}
httpReq.Header.Set(authHeader, authScheme+p.APIKey)
```

That tiny conditional is the load-bearing bit: Azure sets
`AuthHeader: "api-key"` with `AuthScheme: ""` and gets a raw-value
header. A caller who sets *only* `AuthScheme: ""` (intending to
override the prefix) won't accidentally lose Bearer auth.

Backward-compat verified by `TestComplete_DefaultAuthIsBearer`.

### 2. Azure case in `compat.Build`

```go
case catalog.FamilyAzure:
    resource, azKey, err := resolveAzureCreds(p, lookup)
    if err != nil { return nil, "", err }
    urlBase := strings.TrimSpace(p.API)
    if urlBase == "" {
        urlBase = "https://" + resource + ".openai.azure.com"
    }
    urlBase = strings.TrimRight(urlBase, "/")
    apiVersion := strings.TrimSpace(envLookup(lookup, "AGEZT_AZURE_API_VERSION"))
    if apiVersion == "" {
        apiVersion = "2024-10-21"
    }
    fullURL := urlBase + "/openai/deployments/" + modelID +
               "/chat/completions?api-version=" + apiVersion
    op := openai.New(azKey)
    op.Endpoint = fullURL          // pinned: deployment + api-version baked in
    op.AuthHeader = "api-key"
    op.AuthScheme = ""             // raw value, no Bearer
    op.Model = modelID
    return &namedProvider{name: p.ID, inner: op}, modelID, nil
```

### 3. `resolveAzureCreds` — name-suffix-driven dual lookup

Azure providers carry **two** credentials in the catalog `env` list
(resource name + API key), not the usual one. The default
"first non-empty wins" loop in `Build` would pick whichever env var
the operator set first, which is wrong for Azure. `resolveAzureCreds`
inspects env-var *names* via suffix matching:

```go
case strings.HasSuffix(name, "_RESOURCE_NAME"): resource = v
case strings.HasSuffix(name, "_API_KEY"):       key      = v
```

This handles both flavours models.dev publishes:
- `azure`: `AZURE_RESOURCE_NAME` + `AZURE_API_KEY`
- `azure-cognitive-services`: `AZURE_COGNITIVE_SERVICES_RESOURCE_NAME` + `AZURE_COGNITIVE_SERVICES_API_KEY`

Resource is **optional when `api` is set** (custom.json escape
hatch); api-key is always required.

### 4. New helper: `envLookup(lookup, name) string`

Nil-safe wrapper around `CredLookup`. Local-family providers (Ollama)
pass `nil` lookup; Azure needs to read `AGEZT_AZURE_API_VERSION`
optionally without panicking when the daemon path doesn't supply a
lookup (only Azure currently calls it; centralising the nil check
keeps future readers honest).

### 5. Refusal message updated

```
M1.l supports anthropic + ollama + openai + openai-compatible
              + google + mistral + cohere + azure;
vertex/bedrock land in M1.m+
```

## Demo transcript

Reuses the demo home from prior phases.

### Step 1 — both Azure entries now classified

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo agt catalog list | grep "^  azure"
  azure                      (Azure,                       family=azure)        [no creds]
  azure-cognitive-services   (Azure Cognitive Services,    family=azure)        [no creds]
```

### Step 2 — daemon banner with resource + key from env

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=azure \
  AZURE_RESOURCE_NAME=my-resource AZURE_API_KEY=az-fake \
  AGEZT_MODEL=gpt-3.5-turbo-1106 agezt
  governor : primary=azure(catalog; family=azure,
             model=gpt-3.5-turbo-1106) → fallback=mock(offline),
             daily_ceiling=$20.00
```

The model id (`gpt-3.5-turbo-1106`) is interpolated into the URL as
the deployment name. Operators whose Azure deployment names differ
from model ids set `AGEZT_MODEL=<their-deployment-name>` (since
the catalog model-id IS the deployment from the URL's perspective).

### Step 3 — missing resource refused with custom.json hint

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=azure \
  AZURE_API_KEY=az-fake AGEZT_MODEL=gpt-3.5-turbo-1106 agezt
agezt: compat: no credentials available:
  azure provider "azure" needs a *_RESOURCE_NAME env var
  (one of [AZURE_RESOURCE_NAME AZURE_API_KEY])
  or an `api` URL in custom.json
```

### Step 4 — Bedrock still refused with M1.m hint

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=amazon-bedrock \
  AWS_ACCESS_KEY_ID=fake AWS_SECRET_ACCESS_KEY=fake \
  AWS_REGION=us-east-1 agezt
agezt: compat: provider family not yet supported:
  family="aws-bedrock" provider="amazon-bedrock"
  (M1.l supports anthropic + ollama + openai + openai-compatible
   + google + mistral + cohere + azure;
   vertex/bedrock land in M1.m+)
```

## Architectural consequences

1. **First adapter customisation via flags, not wrappers.** The
   `AuthHeader/AuthScheme` fields are the openai adapter's first
   tunable knobs. Until M1.l, "different auth header" would have
   meant a new adapter package. The cost is two trivial fields with
   well-defined defaults; the benefit is that Azure (and any future
   openai-shaped vendor with non-Bearer auth) doesn't need its own
   ~150-LoC duplicate of the encode/decode logic.

2. **Per-family credential resolvers.** Azure is the first family
   that needs *two* credentials from `env`. The default "first non-
   empty wins" rule wasn't general enough; `resolveAzureCreds` shows
   the pattern for vendors with structured credential lists.
   Bedrock will reuse this pattern (access key + secret + session
   token + region) when it lands.

3. **Operator escape hatch is now consistently messaged.** Every
   path that refuses a provider points the operator at exactly the
   knob that fixes it:
   - openai-compatible empty `api` → `custom.json`
   - Azure missing resource → `*_RESOURCE_NAME env var or custom.json`
   - Unsupported family → "lands in M1.X"
   - Unknown model → `AGEZT_MODEL`
   - Missing model in catalog → `agt catalog sync`

   Operators never see a generic "configuration error" — they always
   see the exact next step.

4. **Centralising URL composition in compat is correct.** Tempting
   to put Azure's URL builder inside a new `plugins/providers/azure`
   package, but that would force the package to know about the
   catalog (to read env). Keeping URL composition in compat means
   wire packages stay catalog-agnostic and the catalog → URL mapping
   lives in one place.

## Deferrals → M1.m and beyond

Two families left:

- **aws-bedrock** (`@ai-sdk/amazon-bedrock`) — SigV4 signing,
  regional URL (`https://bedrock-runtime.{region}.amazonaws.com`),
  model-id-in-path (`/model/{id}/invoke`). The shortcut path is
  `AWS_BEARER_TOKEN_BEDROCK` (if set, Bedrock accepts Bearer auth
  with that token — no SigV4 needed). Worth checking which AWS SDK
  helper for Go signs requests minimally.
- **google-vertex** / **google-vertex/anthropic**
  (`@ai-sdk/google-vertex`, `…/anthropic`) — service-account OAuth
  token refresh via `google.golang.org/api/option` or
  `golang.org/x/oauth2/google`, regional URL builder.

Once both ship, **every family in the catalog will be wired** — the
provider-pivot work that started in M1.f reaches its terminal state.

Unchanged deferrals from prior milestones: subscription-first
routing, `agt provider creds`, browser tool, plugin host, Pulse v1,
planner.

## Files touched

```
plugins/providers/openai/openai.go         (+ AuthHeader, AuthScheme fields;
                                              + auth-header build logic)
plugins/providers/openai/openai_test.go    (+ TestComplete_CustomAuthHeader
                                              + TestComplete_DefaultAuthIsBearer
                                                  regression guard)
plugins/providers/compat/compat.go         (+ FamilyAzure case
                                              + resolveAzureCreds helper
                                              + envLookup nil-safe wrapper
                                              + IsSupportedFamily entry
                                              + doc + error-message update)
plugins/providers/compat/compat_test.go    (+ TestBuild_AzureFamilyRoutesToOpenAIWireWithAzureURL
                                              + TestBuild_AzureURLBuildsFromResourceWhenAPIEmpty
                                              + TestBuild_AzureMissingApiKeyRefused
                                              + TestBuild_AzureMissingResourceAndNoAPIOverrideRefused
                                              + FamilyAzure: true in enumeration)
```

No new wire packages. No schema changes. No daemon-command changes.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 262 pass, 0 fail (up from 256 in M1.k)
```

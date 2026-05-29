# Phase Report — Milestone 1.i (Google Gemini adapter; Vertex split)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-07](TASKS.md). Continues the catalog-pivot work
> from [PHASE-M1.h-REPORT.md](PHASE-M1.h-REPORT.md).

## Scope

M1.i adds the third major LLM ecosystem to the catalog-driven wire
layer: Google Gemini via the Generative Language API
(`generateContent`). One new adapter + one family split + one wired
case in `compat.Build`.

The split: previously `@ai-sdk/google-vertex` mapped to `FamilyGoogle`.
That was wrong — Vertex uses Google service-account OAuth, a
fundamentally different auth and URL shape from the API-key path. M1.i
introduces `FamilyGoogleVertex` so the api-key adapter can land
without misclaiming support for Vertex.

| | M1.h | M1.i | Δ |
|---|---:|---:|---:|
| Supported catalog providers | 121 | **122** | +1 (google) |
| Refused-with-clear-error providers | (rest) | +2 (google-vertex, google-vertex-anthropic) | +2 |
| Families wired | 4 | **5** | +1 (google) |
| Families enumerated (split-aware) | 9 | **10** | +1 (google-vertex) |

| Concern | M1.i status |
|---|---|
| Gemini `generateContent` adapter (contents/parts) | ✅ `plugins/providers/google` |
| `x-goog-api-key` header auth (not query param — keys stay out of logs) | ✅ |
| `model` interpolated into URL path (Gemini quirk, not body field) | ✅ |
| `systemInstruction` at top-level (folded from canonical `system`) | ✅ |
| `tools[0].functionDeclarations` schema mapping | ✅ |
| Tool calls (`functionCall` parts) with synthesized stable IDs | ✅ |
| Tool results (`functionResponse` parts in user message) | ✅ (with name-binding caveat) |
| URL versioning: `/v1`, `/v1beta`, bare base — all handled | ✅ |
| Vertex OAuth refused explicitly with M1.j hint | ✅ |
| Mistral / Cohere / Bedrock / Azure / Vertex | ⏳ M1.j |

## Changes

### 1. Family split: `FamilyGoogleVertex` separates from `FamilyGoogle`

`kernel/catalog/types.go`:

```go
FamilyGoogle       Family = "google"        // Generative Language API (API key)
FamilyGoogleVertex Family = "google-vertex" // Vertex AI (service-account OAuth)
```

`FamilyFromNPM`:

```go
case "google", "google-generative-ai":
    return FamilyGoogle
case "google-vertex", "google-vertex/anthropic":
    return FamilyGoogleVertex
```

The `google-vertex/anthropic` case (Anthropic models hosted on
Vertex) catches the unusual nested npm name models.dev uses for that
catalog entry. Before M1.i, that fell to `FamilyUnknown` and was
unselectable; now it correctly maps to Vertex and refuses with the
deferral message.

Test `TestFamilyFromNPM` extended to cover both new mappings.

### 2. New `plugins/providers/google` package

In-process Gemini Provider. Same `Provider` struct shape as the
other wire packages: `APIKey`, `Endpoint`, `BaseURL`, `Model`,
`HTTP`. `Name()` returns `"google"` (the `namedProvider` wrapper
substitutes the catalog id at construction).

**resolveEndpoint** precedence:

```
1. explicit Endpoint
2. BaseURL — append /<APIVersion>/models/<model>:generateContent
            unless BaseURL already carries /v1 or /v1beta
3. DefaultBaseURL + /v1beta/models/<model>:generateContent
```

This matches Gemini's URL convention: model id is in the path, not
the body — which is structurally different from every other vendor
in the catalog and verified by `TestBuild_GoogleFamilyRoutesToGeminiWire`
which asserts `body["model"] == nil`.

**Dialect translation**:

- Canonical `system` → top-level `systemInstruction` (NOT in
  `contents`). Per-message system roles are dropped (folded already).
- `RoleUser` → `{role:"user", parts:[{text}]}`
- `RoleAssistant` → `{role:"model", parts:[{text} | {functionCall}]}`
- `RoleTool` → `{role:"user", parts:[{functionResponse}]}` —
  Gemini doesn't have a tool role; results piggyback on a user turn.
- Empty content slot gets a `{text:""}` placeholder so Gemini accepts it.
- `finishReason`: `STOP`/empty → `StopEndTurn`; `MAX_TOKENS` →
  `StopMaxTokens`; tool-call presence → `StopToolUse` regardless of
  finishReason.
- Per-call IDs synthesized as `call-<i>` (Gemini doesn't return any
  identifier per functionCall — SPEC-15 canonical requires non-empty).

**Known caveat — tool-result name binding**: Gemini's
`functionResponse` requires the function `name`, but canonical
`RoleTool` messages carry only `ToolCallID`, not the originating
function name. The adapter currently passes the `ToolCallID` as a
surrogate. This works for the common case (a single round of
tool calls), but produces semantically odd `functionResponse.name`
values that don't match the originating `functionCall.name`. Tracked
in SPEC-15; will be revisited when canonical `Message` grows a tool
name field (independent of this milestone).

7 tests in `plugins/providers/google/google_test.go`: text response
with system folding; tool-call decode with stable IDs; tool-def
encoding as `functionDeclarations` (verifies the single `tools[0]`
wrapper); 3-leg tool-result round-trip; 4-case endpoint resolution;
no-key + API-error errors.

### 3. `compat.Build` routes FamilyGoogle to the new adapter

```go
case catalog.FamilyGoogle:
    gp := google.New(apiKey)
    gp.BaseURL = base
    gp.Endpoint = ""
    gp.Model = modelID
    return &namedProvider{name: p.ID, inner: gp}, modelID, nil
```

`FamilyGoogleVertex` deliberately *not* listed → falls through to
`default` → `ErrFamilyUnsupported` with the M1.j deferral message.

`IsSupportedFamily` extended; `TestIsSupportedFamily` table now
enumerates 11 family values (split-aware) so a future Vertex
adapter has a clear test to flip.

## Demo transcript

Reuses the demo home from M1.g/M1.h with its synced catalog.

### Step 1 — catalog list shows the family split

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo agt catalog list | grep -E "^  google"
  google                   (Google,             family=google)         [no creds]
  google-vertex            (Vertex,             family=google-vertex)  [no creds]
  google-vertex-anthropic  (Vertex (Anthropic), family=google-vertex)  [no creds]
```

Pre-M1.i: all three were `family=google` and would route through the
same (non-existent) adapter. Post-M1.i: only `google` is
api-key-driven; the two Vertex variants are explicitly other.

### Step 2 — `google` routes through the Gemini adapter

models.dev's `google` entry has no `api` field — for Google that's
fine: the adapter's `DefaultBaseURL` (`generativelanguage.googleapis.com`)
is the right destination.

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=google \
  GEMINI_API_KEY=fake AGEZT_MODEL=gemini-2.0-flash-lite agezt
  governor : primary=google(catalog; family=google,
             model=gemini-2.0-flash-lite) → fallback=mock(offline),
             daily_ceiling=$20.00
```

`GEMINI_API_KEY` is one of three env vars the catalog lists
(`GOOGLE_API_KEY`, `GOOGLE_GENERATIVE_AI_API_KEY`, `GEMINI_API_KEY`);
`compat.Build` takes the first non-empty.

### Step 3 — `google-vertex` refused with clear deferral

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=google-vertex \
  GOOGLE_APPLICATION_CREDENTIALS=/tmp/fake.json agezt
agezt: compat: provider family not yet supported:
  family="google-vertex" provider="google-vertex"
  (M1.i supports anthropic + ollama + openai + openai-compatible + google;
   vertex/mistral/cohere/bedrock/azure land in M1.j)
```

Operator gets the exact missing-family name and the milestone where
it lands — no guessing.

## Architectural consequences

1. **Auth surface ≠ wire surface.** Gemini and Vertex share the
   *wire format* (both speak `generateContent`) but differ in auth
   (API key vs service-account OAuth) and URL shape (public host vs
   regional `aiplatform.googleapis.com`). M1.i treats these as
   distinct families precisely because **wire-only sharing would
   force the API-key path to learn OAuth**. Vertex gets its own
   adapter when it ships — and might internally reuse the dialect
   translation from `plugins/providers/google`.

2. **Model-in-URL is a thing.** Gemini is the first family where the
   model identifier is in the URL path, not a body field. The
   `resolveEndpoint(model string)` signature in
   `plugins/providers/google` is the first hint that future
   adapters (Bedrock comes to mind — model id is in the path there
   too) won't all follow the same "BaseURL + static suffix" shape
   the other three families use.

3. **Tool-result name binding is now a known gap.** Canonical
   `Message{Role:RoleTool}` carries only `ToolCallID`. Gemini and
   any other family that needs the originating function *name* in
   tool results will surface this. Tracked but not blocking — the
   common single-round case works, and the agent loop owns the
   call/result pairing.

4. **The `default` arm of `compat.Build` is now load-bearing
   documentation.** Every catalog provider that operators might
   want to use *but can't yet* hits that branch with a precise
   message naming both the missing family and the milestone where
   it ships. That's preferable to a silent fall-through to mock.

## Deferrals → M1.j

Still returning `ErrFamilyUnsupported` after M1.i:

- **google-vertex** (`@ai-sdk/google-vertex`,
  `@ai-sdk/google-vertex/anthropic`) — needs Google service-account
  OAuth via `google.golang.org/api/option`, regional URL builder,
  and Anthropic-on-Vertex fork of the request shape.
- **mistral** (`@ai-sdk/mistral`) — close to OpenAI shape with
  Mistral-specific auth+headers.
- **cohere** (`@ai-sdk/cohere`) — distinct `chat` API.
- **aws-bedrock** (`@ai-sdk/amazon-bedrock`) — needs SigV4 signing,
  region-aware URL building, model-id-in-path.
- **azure** (`@ai-sdk/azure`) — resource-specific URLs with
  `?api-version=...` query params.

Unchanged deferrals from prior milestones: subscription-first
routing, `agt provider creds`, browser tool, plugin host, Pulse v1,
planner.

## Files touched

```
kernel/catalog/types.go                    (+ FamilyGoogleVertex;
                                              FamilyFromNPM: google-vertex split)
kernel/catalog/catalog_test.go             (TestFamilyFromNPM: + 2 mappings)
plugins/providers/google/google.go         NEW (~290 LoC)
plugins/providers/google/google_test.go    NEW (7 tests, 4 sub-tests)
plugins/providers/compat/compat.go         (+ google import + FamilyGoogle case
                                              + IsSupportedFamily entry
                                              + doc + error-message update)
plugins/providers/compat/compat_test.go    (+ TestBuild_GoogleFamilyRoutesToGeminiWire
                                              + FamilyGoogleVertex enumeration
                                              + reworked unsupported-family test)
```

No schema changes. No runtime changes. No daemon-command changes.
Same `AGEZT_PROVIDER=<catalog-id>` UX.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 243 pass, 0 fail (up from 231 in M1.h)
```

Per-package growth: `plugins/providers/google` (new, 7 tests),
`plugins/providers/compat` (+1 route test, expanded enumeration),
`kernel/catalog` (+2 cases in TestFamilyFromNPM).

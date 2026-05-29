# Phase Report — Milestone 1.n (Google Vertex; the catalog pivot lands)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-12](TASKS.md). Continues
> [PHASE-M1.m-REPORT.md](PHASE-M1.m-REPORT.md).

## Scope

M1.n is the **closing milestone of the catalog pivot** that started in
[M1.f](PHASE-M1.f-REPORT.md). Every family in models.dev/api.json is
now wired to a working wire adapter.

This phase ships:

1. **`plugins/providers/vertex` package** — Vertex AI provider
   (Gemini-on-Vertex body shape, regional URL builder, service-
   account OAuth via JWT-bearer flow).
2. **In-package OAuth implementation** — no `golang.org/x/oauth2`
   dependency. ~290 LoC of RSA/JWT/HTTP that keeps the project's
   lean-deps ethos intact (only `blake3` remains as an external dep).
3. **`compat.Build` FamilyGoogleVertex case** with `resolveVertexCreds`
   (path + project + location resolution).

After M1.n, every catalog family with a known wire shape has an
adapter. The default branch of `compat.Build` is now a safety net for
*future* npm tags rather than a deferral placeholder.

| | M1.m | M1.n | Δ |
|---|---:|---:|---:|
| Supported catalog providers | 127 | **129** | +2 (google-vertex, google-vertex-anthropic) |
| Families wired | 9 | **10** | +1 (google-vertex) |
| Wire adapter packages | 6 | **7** | +1 (vertex) |
| Families still refused | 1 (vertex) | **0** | **−1 (catalog pivot closed)** |

### Final family tally — 129/137 catalog providers (94%) supported

| Family | Count | Phase landed |
|---|---:|---|
| openai-compatible (Groq, DeepSeek, Together, …) | 111 | M1.h |
| unknown (third-party AI SDK packages) | 8 | — (add per-vendor npm name) |
| anthropic | 6 | M1.g |
| openai | 3 | M1.h |
| google-vertex | 2 | **M1.n** |
| azure | 2 | M1.l |
| ollama | 1 | M1.g |
| mistral | 1 | M1.j |
| google | 1 | M1.i |
| cohere | 1 | M1.k |
| aws-bedrock | 1 | M1.m |

The 8 "unknown" entries (`@aihubmix/ai-sdk-provider`,
`gitlab-ai-provider`, `venice-ai-sdk-provider`,
`@jerome-benoit/sap-ai-provider-v2`, `ai-gateway-provider`, etc.) are
third-party AI SDK packages whose npm tags `FamilyFromNPM` doesn't
recognise yet. Each is a one-line addition when operators ask — most
are openai-compatible at the wire level.

| Concern | M1.n status |
|---|---|
| Vertex regional URL builder (`{loc}-aiplatform.googleapis.com`) | ✅ |
| Service-account JWT-bearer OAuth (RS256, no external deps) | ✅ in-package |
| Token caching with skew window (cache miss within 60s of expiry) | ✅ verified by `TestTokenSource_StalenessWindow` |
| `GOOGLE_APPLICATION_CREDENTIALS` env path | ✅ |
| `GOOGLE_VERTEX_PROJECT` env override (falls back to JSON's project_id) | ✅ |
| `GOOGLE_VERTEX_LOCATION` env (required) | ✅ |
| `custom.json` `api` field override (VPC service-control endpoints) | ✅ |
| Anthropic-on-Vertex (`:rawPredict`) | ⏳ M1.n.x |
| Application Default Credentials, workload-identity, GCE metadata server | ⏳ M1.n.x |
| Streaming (SSE) | ⏳ separate phase across all providers |

## Changes

### 1. New `plugins/providers/vertex` package

Two files, ~580 LoC total:

**`auth.go`** — RFC 7523 JWT-bearer flow implementation:
- `ServiceAccountKey` — subset of Google's service-account JSON
- `LoadServiceAccountFile(path)` / `ParseServiceAccountJSON(raw)`
- `parsePrivateKey(pem)` — PKCS#8 (Google's format) with PKCS#1 fallback
- `signJWT(sa, key, scope, aud, now)` — base64url + RS256 HMAC
- `TokenSource{ ... }` — minted-and-cached access token, mutex-guarded
- `TokenSkew = 60s` — re-exchange tokens that expire within this window

**`vertex.go`** — Provider with `ResolveEndpoint` URL builder + Gemini body translation:

```go
type Provider struct {
    TokenSource *TokenSource
    Project, Location string
    Endpoint, BaseURL, Model string
    HTTP *http.Client
}
```

URL precedence (same pattern as Bedrock's `ResolveEndpoint`):

```
1. explicit Endpoint
2. BaseURL  + "/v1/projects/{project}/locations/{location}/publishers/google/models/{model}:generateContent"
3. derived: https://{location}-aiplatform.googleapis.com + (2)'s suffix
```

Body translation is duplicated from `plugins/providers/google`
(same Gemini shape: `contents`/`parts`/`functionCall`/`functionResponse`/
`systemInstruction`/`tools[0].functionDeclarations`). The
duplication is intentional and contained — Vertex's shape can evolve
independently of the public Gemini endpoint's.

11 tests in `plugins/providers/vertex/vertex_test.go`:
- service-account JSON parse (rejects non-`service_account` type
  with M1.n.x hint; defaults `token_uri`)
- token source: mints + caches + survives a 401 + respects staleness window
- bad-PEM rejection
- `ResolveEndpoint` 3-case table (derive / override / explicit)
- happy-path Complete with two test servers (token + API)
- missing TokenSource / Project / Location errors
- API error propagation

### 2. `compat.Build` FamilyGoogleVertex case

```go
case catalog.FamilyGoogleVertex:
    credsPath, project, location, err := resolveVertexCreds(p, lookup)
    if err != nil { return nil, "", err }
    sa, err := vertex.LoadServiceAccountFile(credsPath)
    if err != nil { return nil, "", fmt.Errorf("%w: %v", ErrMissingCredentials, err) }
    if project == "" { project = sa.ProjectID }
    if project == "" { return nil, "", fmt.Errorf("%w: vertex provider %q needs a project (...)", ErrMissingCredentials, p.ID) }
    ts, err := vertex.NewTokenSource(sa, vertex.CloudPlatformScope, nil)
    if err != nil { return nil, "", fmt.Errorf("%w: %v", ErrMissingCredentials, err) }
    vp := vertex.New(ts, project, location)
    vp.BaseURL = strings.TrimSpace(p.API)
    vp.Model = modelID
    return &namedProvider{name: p.ID, inner: vp}, modelID, nil
```

### 3. `resolveVertexCreds`

Fourth structured cred resolver (after Azure, Bedrock, openai-style).
Reads `GOOGLE_APPLICATION_CREDENTIALS` + `GOOGLE_VERTEX_LOCATION`;
project is optional (falls back to `project_id` in the JSON file).
Error messages name the M1.n.x deferral for ADC / workload-identity.

### 4. `IsSupportedFamily` + doc + error message

`IsSupportedFamily` now lists all 10 families true. Package doc
updated to enumerate all wired adapters with a closing note:
**"Every family in the catalog is now wired."**

The unsupported-family error message is now load-bearing only as a
safety net:

```
M1.n wired every catalog family — anthropic + ollama + openai +
openai-compatible + google + mistral + cohere + azure + aws-bedrock
+ google-vertex. This branch should be unreachable for any models.dev
catalog entry; if you see it, the catalog has a new family the wire
layer doesn't recognise.
```

### 5. `TestBuild_UnsupportedFamilyReturnsErr` retargeted

The test previously targeted whichever family was still deferred.
With nothing left, it now synthesises a `FamilyUnknown` entry via a
made-up npm tag (`@future-vendor/some-sdk`) to keep the safety-net
branch under test.

## Demo transcript

Reuses the demo home from prior phases.

### Step 1 — all three Google entries now supported

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo agt catalog list | grep "^  google"
  google                   (Google,             family=google)         [no creds]
  google-vertex            (Vertex,             family=google-vertex)  [no creds]
  google-vertex-anthropic  (Vertex (Anthropic), family=google-vertex)  [no creds]
```

(`google-vertex-anthropic` will route correctly at the wire layer but
fail downstream because the body shape — Anthropic-on-Vertex via
`:rawPredict` — defers to M1.n.x.)

### Step 2 — Vertex daemon banner with real service-account creds

```
$ # service-account JSON generated via Python's cryptography for the demo
$ AGEZT_HOME=/tmp/agezt-m1g-demo \
  AGEZT_PROVIDER=google-vertex \
  GOOGLE_APPLICATION_CREDENTIALS=/tmp/sa.json \
  GOOGLE_VERTEX_PROJECT=demo-project \
  GOOGLE_VERTEX_LOCATION=us-central1 \
  AGEZT_MODEL=gemini-2.0-flash \
  agezt
Agezt 0.0.0-m0 — daemon ready (protocol v1)
  governor : primary=google-vertex(catalog; family=google-vertex,
             model=gemini-2.0-flash) → fallback=mock(offline),
             daily_ceiling=$20.00
```

### Step 3 — missing creds path names the M1.n.x deferral

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=google-vertex \
  GOOGLE_VERTEX_LOCATION=us-central1 agezt
agezt: compat: no credentials available:
  vertex provider "google-vertex" needs GOOGLE_APPLICATION_CREDENTIALS
  (path to service-account JSON; M1.n doesn't yet support
   ADC/workload-identity — those land in M1.n.x)
```

### Step 4 — missing location

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=google-vertex \
  GOOGLE_APPLICATION_CREDENTIALS=/tmp/sa.json agezt
agezt: compat: no credentials available:
  vertex provider "google-vertex" needs GOOGLE_VERTEX_LOCATION (e.g. us-central1)
```

## Architectural consequences

1. **The catalog pivot is complete.** Every adapter the daemon
   could ever need to talk to a models.dev-listed vendor is now in
   the repo. Adding a new vendor whose wire fits an existing family
   is a `custom.json` entry (zero code). Adding a new wire shape is
   a new package + one case in `compat.Build` (proven 7 times in
   M1.g through M1.n).

2. **In-package OAuth was the right call.** Adding
   `golang.org/x/oauth2/google` would have brought in
   `cloud.google.com/go/compute/metadata` and (transitively) several
   sub-packages just for `JWTConfigFromJSON`. The ~290 LoC we wrote
   instead uses only `crypto/rsa`, `crypto/x509`, `crypto/sha256`,
   and `net/http`. The project's external-dep count remains: 1
   (`blake3`).

3. **The structured-cred-resolver pattern stabilised at 4
   instances.** Single-cred (openai/anthropic/cohere/mistral),
   azure dual-cred, bedrock bearer+region, vertex creds+project+
   location. Future vendors needing structured creds will follow
   the same per-family resolver shape.

4. **`ResolveEndpoint` is now the standard URL-axis test surface.**
   Bedrock introduced the exported `ResolveEndpoint` in M1.m;
   Vertex adopted it in M1.n. Future adapters with non-trivial URL
   building should follow.

5. **Deferral hints became a feature, not a sad-path.** Reading
   through M1.g–M1.n error messages: every refused path tells the
   operator the *exact* milestone where the gap closes. The
   refused→fixed pipeline is fully transparent.

## Deferrals → M1.n.x and beyond

**M1.n.x — Vertex completeness:**
- Anthropic-on-Vertex (`@ai-sdk/google-vertex/anthropic`,
  `publishers/anthropic/models/{model}:rawPredict`). Body is
  Anthropic Messages shape; auth is the same Vertex OAuth.
- Application Default Credentials: `gcloud auth application-default
  login`-style discovery via `~/.config/gcloud/application_default_credentials.json`.
- Workload Identity Federation: external account types.
- GCE metadata server token source for in-cluster workloads.

**M1.m.x — Bedrock completeness** (deferred from M1.m, still valid):
- SigV4 signing (alternative to `AWS_BEARER_TOKEN_BEDROCK`).
- Per-vendor body builders for `mistral.*`, `meta.*`, `amazon.*`,
  `cohere.*`, `ai21.*`, `deepseek.*`, `qwen.*`.

**Per-vendor npm tag expansion** (lightweight, one-line each in
`FamilyFromNPM`):
- `@aihubmix/ai-sdk-provider`, `gitlab-ai-provider`,
  `venice-ai-sdk-provider`, etc. → likely `FamilyOpenAICompatible`.

**Cross-cutting (next major phase, post-catalog-pivot):**
- Streaming (SSE) uniformly across all 7 wire adapters.
- Subscription-first routing (DECISIONS C2) — Governor picks
  subscription-mode providers over pay-per-token.
- `agt provider creds` CLI for vault-backed credential storage.
- Browser tool, plugin host, Pulse v1, planner.

## Files touched

```
plugins/providers/vertex/auth.go         NEW (~280 LoC)
plugins/providers/vertex/vertex.go       NEW (~330 LoC)
plugins/providers/vertex/vertex_test.go  NEW (11 tests, 3 sub-tests)
plugins/providers/compat/compat.go       (+ vertex import + FamilyGoogleVertex case
                                            + resolveVertexCreds helper
                                            + IsSupportedFamily entry
                                            + doc + error-message update)
plugins/providers/compat/compat_test.go  (+ TestBuild_VertexFamilyRoutesToVertexWire
                                            + TestBuild_VertexMissingCredsRefused
                                            + genTestVertexSA / writeTempFile helpers
                                            + FamilyGoogleVertex: true in enumeration
                                            + retargeted unsupported-family test
                                                to synthetic FamilyUnknown)
```

No schema changes. No daemon-command changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 293 pass, 0 fail (up from 277 in M1.m)
```

## The catalog pivot in numbers

From M1.f's first hardcoded-table provider through M1.n's full
catalog coverage:

| Milestone | Catalog providers supported | Families wired | Tests |
|---|---:|---:|---:|
| Pre-pivot (M1.e) | ~2 (hardcoded) | 2 | ~150 |
| M1.f — catalog v1 data layer | (still 2) | 2 | 198 |
| M1.g — wire layer pivot | 7 | 2 | 209 |
| M1.h — openai-compatible | 121 | 4 | 231 |
| M1.i — google Gemini | 122 | 5 | 243 |
| M1.j — mistral + per-family defaults | 123 | 6 | 245 |
| M1.k — cohere v2 | 124 | 7 | 256 |
| M1.l — azure | 126 | 8 | 262 |
| M1.m — bedrock (bearer + anthropic) | 127 | 9 | 277 |
| **M1.n — vertex** | **129** | **10** | **293** |

The user directive that started this — *"hardcoded provider veya model
işlerini bırak, her şey esnek olacak"* — is structurally satisfied.
Every provider and model selectable by the daemon comes from a
catalog refresh (`agt catalog sync`) or an operator override
(`custom.json`). Nothing is hardcoded except the per-family default
URLs and well-known npm-tag mappings, both of which are encoded as
small, auditable lookup functions in `compat.go` and `catalog/types.go`.

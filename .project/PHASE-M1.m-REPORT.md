# Phase Report — Milestone 1.m (AWS Bedrock; Anthropic-on-Bedrock with bearer-token auth)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-11](TASKS.md). Continues
> [PHASE-M1.l-REPORT.md](PHASE-M1.l-REPORT.md).

## Scope

M1.m adds AWS Bedrock — but **deliberately partial**, to land in one
phase rather than fold a SigV4 signing implementation and multi-
vendor body shapes into a single big-bang milestone:

| Slice | M1.m | M1.m.x (deferred) |
|---|---|---|
| **Auth** | `AWS_BEARER_TOKEN_BEDROCK` (long-lived) | SigV4 signing (`AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` + session token) |
| **Body shape** | Anthropic Messages API only | Mistral, Meta Llama, Amazon Titan, Cohere, AI21, DeepSeek, Qwen on Bedrock |

This slice covers the largest real-world Bedrock surface — Anthropic
models are the bulk of Bedrock usage, and the bearer-token path was
introduced specifically so workloads don't need SigV4 machinery. The
remaining slices have clean errors that name the deferral.

Cross-region inference profiles (`us.anthropic.*`, `eu.anthropic.*`,
`global.anthropic.*`, etc.) are recognised — the Anthropic-vendor
detector looks for `.anthropic.` anywhere in the model id.

| | M1.l | M1.m | Δ |
|---|---:|---:|---:|
| Supported catalog providers | 126 | **127** | +1 (amazon-bedrock) |
| Families wired | 8 | **9** | +1 (aws-bedrock) |
| Wire adapter packages | 5 | **6** | +1 (bedrock) |
| Families still refused | 1 (vertex) + 1 (bedrock) | **1 (vertex only)** | −1 |

After M1.m, **only Google Vertex (`@ai-sdk/google-vertex`,
`@ai-sdk/google-vertex/anthropic`) is still refused**. M1.n closes
the catalog pivot.

| Concern | M1.m status |
|---|---|
| Bedrock URL builder (`bedrock-runtime.{region}.amazonaws.com/model/{id}/invoke`) | ✅ in `plugins/providers/bedrock` |
| Bearer-token auth via `AWS_BEARER_TOKEN_BEDROCK` | ✅ |
| Anthropic body shape with `anthropic_version: "bedrock-2023-05-31"` (no `model` field) | ✅ |
| Cross-region inference profiles accepted (`us.anthropic.*` etc.) | ✅ via substring detector |
| Non-Anthropic vendors rejected with M1.m.x hint | ✅ `ErrVendorUnsupported` |
| Missing bearer token rejected with SigV4/M1.m.x hint | ✅ via `resolveBedrockCreds` |
| Missing region rejected (unless `custom.json` `api` override) | ✅ |
| `ResolveEndpoint` exported for direct URL verification in tests | ✅ |
| SigV4 signing | ⏳ M1.m.x |
| Non-Anthropic bodies (Mistral / Meta / Amazon / Cohere / AI21 / DeepSeek) | ⏳ M1.m.x |
| Google Vertex (OAuth) | ⏳ M1.n |

## Changes

### 1. New `plugins/providers/bedrock` package

In-process Bedrock Provider. ~330 LoC including dialect translation
duplicated from the direct-Anthropic adapter (intentional — Bedrock
needs the same Messages-API shape minus `model` plus `anthropic_version`,
and keeping the duplication means Bedrock can evolve without entangling
with the direct-Anthropic adapter).

```go
type Provider struct {
    BearerToken string
    Endpoint    string
    BaseURL     string
    Region      string
    Model       string
    HTTP        *http.Client
}
```

**`isAnthropicModel(id)` heuristic**:

```go
if strings.HasPrefix(id, "anthropic.") { return true }
if strings.Contains(id, ".anthropic.") { return true }  // us.anthropic.*, eu.anthropic.*, ...
return false
```

This is wire-axis vendor detection, not body-builder dispatch. When
M1.m.x lands, it'll grow into a switch returning a `bodyBuilder`
function per vendor segment.

**Body shape** (anthropic-on-bedrock):

```json
{
  "anthropic_version": "bedrock-2023-05-31",
  "max_tokens": 4096,
  "system": "...",
  "messages": [...],
  "tools": [...]
}
```

No `model` field — the model id is in the URL path. The
`anthropic_version` value is pinned by AWS; updates require
coordination with their release notes.

**`ResolveEndpoint(model)`** is exported (uppercase) so tests can
verify URL routing without an HTTP round-trip. This is the first
adapter to expose the URL builder directly; the test `TestResolveEndpoint`
covers four cases:

- default — derived from `Region`
- `BaseURL` override (custom.json VPCE endpoint, etc.)
- explicit `Endpoint` wins
- trailing-slash on `BaseURL` is trimmed

8 tests in `plugins/providers/bedrock/bedrock_test.go`: text response
(verifies `anthropic_version` present and `model` absent in body),
tool-use round-trip, tool_use stop reason, non-Anthropic vendor
refused, regional-profile accepted, no-bearer-token error, endpoint
resolution (4 sub-cases), API error.

### 2. `compat.Build` Bedrock case

```go
case catalog.FamilyAWSBedrock:
    bearer, region, err := resolveBedrockCreds(p, lookup)
    if err != nil { return nil, "", err }
    bp := bedrock.New(bearer, region)
    bp.BaseURL = strings.TrimSpace(p.API) // optional override
    bp.Model = modelID
    return &namedProvider{name: p.ID, inner: bp}, modelID, nil
```

### 3. `resolveBedrockCreds` — bearer + region resolver with SigV4 hint

```go
func resolveBedrockCreds(p, lookup) (bearer, region string, err error) {
    // bearer: from any *_BEARER_TOKEN_BEDROCK in catalog env, or
    //         bare AWS_BEARER_TOKEN_BEDROCK fallback
    // region: prefer AWS_REGION, fall back to AWS_DEFAULT_REGION
    if bearer == "" {
        return "", "", fmt.Errorf(
            "%w: bedrock provider %q needs AWS_BEARER_TOKEN_BEDROCK "+
            "(M1.m doesn't yet sign with AWS_ACCESS_KEY_ID/SECRET — "+
            "SigV4 lands in M1.m.x)", ErrMissingCredentials, p.ID)
    }
    if region == "" && p.API == "" {
        return "", "", fmt.Errorf(
            "%w: bedrock provider %q needs AWS_REGION (or "+
            "AWS_DEFAULT_REGION), or an `api` URL in custom.json",
            ErrMissingCredentials, p.ID)
    }
    return bearer, region, nil
}
```

This is the **third** structured cred resolver after Azure's
`resolveAzureCreds` and OpenAI/Cohere/Anthropic's single-value default.
The pattern is now stable: a family that needs structured credentials
gets its own resolver in compat. Bedrock SigV4 (M1.m.x) will extend
this one rather than introducing a parallel mechanism.

### 4. `IsSupportedFamily` + doc + error message

`IsSupportedFamily` lists `FamilyAWSBedrock` true. Doc comment
updated to enumerate all 9 wired families. Unsupported-family error
now reports:

```
M1.m supports anthropic + ollama + openai + openai-compatible
              + google + mistral + cohere + azure + aws-bedrock;
only google-vertex lands in M1.n
```

The list-of-supported in this error is now long enough to be
load-bearing on its own — operators reading it see what *works*
without needing to consult the catalog list.

### 5. Tests realigned around the new frontier

`TestBuild_UnsupportedFamilyReturnsErr` now points at Vertex (the
only remaining unsupported family). The `IsSupportedFamily`
enumeration test flipped Bedrock from `false` to `true`.

3 new Bedrock-route tests in `plugins/providers/compat/compat_test.go`:
- full round-trip with httptest verifying URL path, Bearer header,
  body shape (`anthropic_version` present, `model` absent)
- missing bearer token → `ErrMissingCredentials` with SigV4 hint
- missing region with no `api` override → `ErrMissingCredentials`
  with `custom.json` hint

## Demo transcript

Reuses the demo home from prior phases.

### Step 1 — Bedrock now classified

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo agt catalog list \
    | grep -E "^  (amazon-bedrock|google-vertex)"
  amazon-bedrock           (Amazon Bedrock,     family=aws-bedrock)    [no creds]
  google-vertex            (Vertex,             family=google-vertex)  [no creds]
  google-vertex-anthropic  (Vertex (Anthropic), family=google-vertex)  [no creds]
```

### Step 2 — Bedrock daemon banner with bearer + region

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=amazon-bedrock \
  AWS_BEARER_TOKEN_BEDROCK=br-fake AWS_REGION=us-east-1 \
  AGEZT_MODEL=anthropic.claude-opus-4-7 agezt
  governor : primary=amazon-bedrock(catalog; family=aws-bedrock,
             model=anthropic.claude-opus-4-7) → fallback=mock(offline),
             daily_ceiling=$20.00
```

### Step 3 — bedrock without bearer rejected with SigV4 hint

```
$ AWS_ACCESS_KEY_ID=k AWS_SECRET_ACCESS_KEY=s AWS_REGION=us-east-1 \
  AGEZT_PROVIDER=amazon-bedrock AGEZT_MODEL=anthropic.claude-opus-4-7 agezt
agezt: compat: no credentials available:
  bedrock provider "amazon-bedrock" needs AWS_BEARER_TOKEN_BEDROCK
  (M1.m doesn't yet sign with AWS_ACCESS_KEY_ID/SECRET — SigV4 lands in M1.m.x)
```

### Step 4 — Vertex is now the only remaining refused family

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo AGEZT_PROVIDER=google-vertex \
  GOOGLE_APPLICATION_CREDENTIALS=/tmp/fake.json agezt
agezt: compat: provider family not yet supported:
  family="google-vertex" provider="google-vertex"
  (M1.m supports anthropic + ollama + openai + openai-compatible
   + google + mistral + cohere + azure + aws-bedrock;
   only google-vertex lands in M1.n)
```

## Architectural consequences

1. **Deliberately partial milestones are OK.** Bedrock's SigV4 +
   per-vendor-body matrix would have been a multi-week phase. M1.m
   ships the **largest single slice** (anthropic + bearer) and
   defers the rest with clean errors. This pattern — "ship the 80%
   case, refuse the 20% with a specific deferral name" — has worked
   throughout the catalog pivot and continues to scale.

2. **Vendor detection from model id is its own axis.** Bedrock is
   the first family where the *model id encodes the vendor body
   shape*. The `isAnthropicModel` substring check is small for now,
   but it's the seed of a real `detectBedrockVendor(id) → bodyBuilder`
   function in M1.m.x. The wire layer (URL + auth) is already
   vendor-agnostic; only the body builder needs the dispatch.

3. **`ResolveEndpoint` is the URL-axis test interface.** Up until
   M1.m we tested URL routing via httptest + path inspection. That
   works but couples URL builder testing to HTTP machinery. Bedrock's
   exported `ResolveEndpoint(model)` lets tests verify the URL with
   no goroutines, no servers, no network. Future adapters (Vertex
   especially, with its regional URL builder) should follow this
   pattern.

4. **The deferral message is now a feature, not a sad-path.** It's
   long, specific, and tells operators *exactly* which knob to
   wait for. With one family left, the next iteration of this
   message says "*all* families supported."

## Deferrals

**M1.m.x — Bedrock completeness**:
- SigV4 signing implementation (auth path via
  `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` + optional
  `AWS_SESSION_TOKEN`). The Go std library doesn't ship a SigV4
  signer; either vendor `aws-sdk-go-v2/aws/signer/v4` or write a
  minimal HMAC-SHA256 implementation (~150 LoC).
- Per-vendor body builders: `mistral.*`, `meta.*`, `amazon.*`
  (Titan), `cohere.*`, `ai21.*`, `deepseek.*`, `qwen.*`.

**M1.n — Vertex (the last family)**:
- Service-account OAuth via `golang.org/x/oauth2/google` or a
  minimal JWT-grant exchange.
- Regional URL builder
  (`{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/google/models/{model}:generateContent`).
- Anthropic-on-Vertex fork
  (`@ai-sdk/google-vertex/anthropic`) — different endpoint path
  but same auth.

Unchanged deferrals from prior milestones: subscription-first
routing, `agt provider creds`, browser tool, plugin host, Pulse v1,
planner.

## Files touched

```
plugins/providers/bedrock/bedrock.go         NEW (~330 LoC)
plugins/providers/bedrock/bedrock_test.go    NEW (8 tests, 4 sub-tests)
plugins/providers/compat/compat.go           (+ bedrock import + FamilyAWSBedrock case
                                                + resolveBedrockCreds helper
                                                + IsSupportedFamily entry
                                                + doc + error-message update)
plugins/providers/compat/compat_test.go      (+ TestBuild_BedrockFamilyRoutesToBedrockWire
                                                + TestBuild_BedrockMissingBearerTokenRefused
                                                + TestBuild_BedrockMissingRegionRefusedUnlessAPIOverride
                                                + FamilyAWSBedrock: true in enumeration
                                                + retargeted unsupported-family test to Vertex)
```

No schema changes. No daemon-command changes.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 277 pass, 0 fail (up from 262 in M1.l)
```

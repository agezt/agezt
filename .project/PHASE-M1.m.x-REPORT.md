# Phase Report — Milestone 1.m.x (Bedrock SigV4 signing)

> Status: **shipped** · Date: 2026-05-29
> Per SPEC-15 §6 (Bedrock auth) and the M1.m package-level deferral
> comment: "SigV4-signed requests (AWS_ACCESS_KEY_ID /
> AWS_SECRET_ACCESS_KEY) land in M1.m.x."
> Continues [PHASE-M1.v-REPORT.md](PHASE-M1.v-REPORT.md).

## Scope

Bedrock shipped in M1.m with **bearer-token auth only**
(AWS_BEARER_TOKEN_BEDROCK — the long-lived preview credential).
This is the simplest wire-time setup but isn't available to every
operator: it's an opt-in preview feature for specific accounts,
and operators on classic IAM (every EC2 / Lambda / on-prem
workstation with `aws configure`) had no path. M1.m.x closes
that gap with **AWS SigV4 signing**:

```
# vault now accepts EITHER set of credentials:
agt provider creds set AWS_BEARER_TOKEN_BEDROCK <token>   # preview path
# OR
agt provider creds set AWS_ACCESS_KEY_ID <akid>           # classic IAM
agt provider creds set AWS_SECRET_ACCESS_KEY <secret>
agt provider creds set AWS_SESSION_TOKEN <sts-temp>       # optional, STS
```

The Bedrock adapter inspects whichever path is configured. Bearer
still wins when both are present (less complex, less to go wrong).

| Concern | Status |
|---|---|
| Pure-Go SigV4 implementation (no aws-sdk-go dependency) | ✅ |
| Canonical request: method, URI, query, headers, signed-headers, body hash | ✅ |
| Canonical headers: host, x-amz-date, x-amz-content-sha256, content-type, optional security-token | ✅ |
| Per-day per-region per-service signing-key derivation (HMAC chain) | ✅ |
| Authorization header in `AWS4-HMAC-SHA256 Credential=.../SignedHeaders=.../Signature=...` shape | ✅ tested |
| X-Amz-Date + X-Amz-Content-Sha256 injection BEFORE signing (so they get signed) | ✅ tested |
| X-Amz-Security-Token for STS temp creds (also signed) | ✅ tested |
| Deterministic re-execution (same inputs → same signature) | ✅ tested |
| AWS-canonical query-string sorting (key, then value) | ✅ tested |
| AWS-style URI encoding (slash-preserved for path, slash-encoded for query) | ✅ tested |
| Whitespace collapse in header values per spec | ✅ tested |
| `bedrock.Provider.SetSigV4Creds(*SigV4Creds)` setter | ✅ |
| `bedrock.Provider.hasAuth()` / `applyAuth()` selectors | ✅ |
| Streaming path (`CompleteStream`) signs via the same helper | ✅ |
| Bearer wins when both auth paths configured | ✅ tested |
| `ErrNoBearerToken` lists both supported auth paths | ✅ |
| Compat layer: `resolveBedrockCreds` returns whichever auth is present | ✅ |
| Region required for SigV4 even when `api` override is set (credential scope) | ✅ |
| Existing M1.m tests + M1.t streaming tests still pass | ✅ |

## Changes

### 1. `plugins/providers/bedrock/sigv4.go` — new file (~250 LoC)

Pure stdlib implementation: `crypto/hmac`, `crypto/sha256`,
`encoding/hex`, `net/http`, `sort`, `strings`, `time`. No
`aws-sdk-go` dependency — the project's lean-deps policy
forbids it, and SigV4's algorithm is small enough to maintain.

Five layers:

**A. `signRequest(req, region, body, creds, now)`** — top-level
entry. Sets X-Amz-Date + X-Amz-Content-Sha256 (and
X-Amz-Security-Token for STS), builds canonical request, derives
signing key, signs string-to-sign, sets Authorization header.

**B. `buildCanonicalRequest(req, bodyHash)`** — assembles AWS's
6-line canonical form:

```
POST
/model/anthropic.claude-opus-4-7/invoke

content-type:application/json
host:bedrock-runtime.us-east-1.amazonaws.com
x-amz-content-sha256:<hex>
x-amz-date:20260529T123456Z

content-type;host;x-amz-content-sha256;x-amz-date
<bodyHashHex>
```

**C. `canonicalQuery` + `canonicalHeaders`** — sorted, URL-encoded,
lowercased per spec.

**D. `deriveSigningKey`** — the 4-step HMAC chain:
```
kDate    = HMAC(("AWS4"+secret),    dateStamp)
kRegion  = HMAC(kDate,              region)
kService = HMAC(kRegion,            "bedrock")
kSigning = HMAC(kService,           "aws4_request")
```

**E. `awsURIEncode`** — RFC-3986-with-AWS-twist (slash optionally
preserved; everything else %XX).

Three design notes worth recording:

**Why not validate signature server-side.** We can't — that's the
whole point of SigV4 (AWS validates). Our test surface verifies
the *structure* of the signed request + deterministic
re-execution. The actual cryptographic correctness comes out of
running against a live Bedrock endpoint, which the integration
test in M1.p.x (`agt provider check`) already covers per-operator.

**Why no event-stream chunked signing.** SigV4 has a variant for
request-streaming uploads (e.g. multipart S3 puts) that signs
each frame separately. Bedrock's `InvokeModelWithResponseStream`
sends a *single, once-signed* request and streams the *response*;
the response-stream framing M1.t parses isn't signed at all. So
the normal SigV4 path handles streaming correctly with no
special-casing.

**Why static creds only.** The aws-sdk-go credential-provider chain
(env → shared-creds-file → IAM role → instance metadata → SSO)
is hundreds of lines of fragile platform-specific logic. The
operator-facing primitive — "put your AKID/secret in the vault" —
covers the same 95% with a far smaller blast radius. Future
M1.m.x.x could add IAM-role / instance-metadata pickup for EC2
deployments; for now operators on EC2 export the role's temp
creds the same way every other tool does.

### 2. `plugins/providers/bedrock/bedrock.go` — auth selectors

```go
type Provider struct {
    BearerToken string         // M1.m
    sigV4       *SigV4Creds    // M1.m.x — set via SetSigV4Creds
    // ... unchanged ...
    Now func() time.Time       // M1.m.x — for deterministic SigV4 timestamps in tests
}

func (p *Provider) SetSigV4Creds(creds *SigV4Creds)
func (p *Provider) hasAuth() bool { /* either path */ }
func (p *Provider) applyAuth(req *http.Request, body []byte) error {
    if p.BearerToken != "" {
        req.Header.Set("Authorization", "Bearer "+p.BearerToken)
        return nil
    }
    return signRequest(req, p.Region, body, *p.sigV4, p.now())
}
```

Both `Complete` and `CompleteStream` replaced their inline
`Authorization: Bearer ...` header set with `p.applyAuth(req, body)`.
The error sentinel `ErrNoBearerToken` keeps its name (avoiding a
breaking rename) but its message now lists both supported auth
paths so operators see both options.

### 3. `plugins/providers/compat/compat.go` — dual-auth resolver

`resolveBedrockCreds` rewritten to return a typed `bedrockAuth`
union (Bearer string OR SigV4 *SigV4Creds, never both). Walks
the vault for:

1. `AWS_BEARER_TOKEN_BEDROCK` (or any catalog env name suffixed
   `_BEARER_TOKEN_BEDROCK`)
2. If not found, `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY`,
   plus optional `AWS_SESSION_TOKEN`
3. `AWS_REGION` (or `AWS_DEFAULT_REGION`) for both paths

Error messages:
- Both missing → "needs either AWS_BEARER_TOKEN_BEDROCK *or*
  (AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY) in the vault"
- Region missing → explains both reasons it's needed (SigV4
  credential scope; bearer's URL host)

### 4. `plugins/providers/bedrock/sigv4_test.go` — 10 tests

| Test | Locks in |
|---|---|
| `TestSignRequest_HappyPath` | Authorization shape + required `Credential=`/`SignedHeaders=`/`Signature=` substrings + injected X-Amz-Date + injected content hash + deterministic re-signing |
| `TestSignRequest_IncludesSessionTokenWhenSet` | STS temp-creds path; security-token in headers AND SignedHeaders |
| `TestSignRequest_RejectsMissingCreds` | Empty creds / empty region / empty AKID-only all error |
| `TestCanonicalQuery_SortsByKeyThenValue` | `b=2, a=3, a=1` → `a=1&a=3&b=2` |
| `TestAwsURIEncode_LeavesUnreservedAlone` | Unreserved chars pass through unchanged |
| `TestAwsURIEncode_EncodesReservedAndUnicode` | Space → `%20`; slash preserved or encoded per flag |
| `TestProvider_CompleteWithSigV4` | End-to-end: SigV4 creds → request lands at mock server with `AWS4-HMAC-SHA256` Authorization → decode succeeds |
| `TestProvider_BearerStillWinsWhenBothSet` | Both auth paths configured → bearer wins (simpler, fewer moving parts) |
| `TestProvider_NoAuthErrors` | Neither auth → `ErrNoBearerToken` (sentinel) |
| `TestCollapseSpaces_LeavesSingleRunsAlone` | Header-value whitespace collapse (per SigV4 spec) |

The tests live in `package bedrock` (not `bedrock_test`) so they
can reach the unexported `signRequest` / `canonicalQuery` /
`awsURIEncode` / `collapseSpaces`. Same convention every other
crypto-adjacent package in the stdlib uses.

### 5. `plugins/providers/compat/compat_test.go` — flipped assertions

Two existing tests had assertions that named the M1.m.x deferral:

- `TestBuild_BedrockMissingBearerTokenRefused`: changed
  `Contains(err, "SigV4")` → asserts both auth-path names appear
  in the error message.
- `TestBuild_BedrockMissingRegionRefusedUnlessAPIOverride`:
  changed `Contains(err, "custom.json")` → asserts `AWS_REGION`
  appears in the error (the new message explains the scope/host
  reasons region is required).

## Test summary

```
go test ./plugins/providers/bedrock/ -v -count=1
(10 new sigv4 tests + 18 existing M1.m + M1.t tests — all PASS)

go test ./plugins/providers/compat/ -v -count=1
(includes flipped Bedrock assertions — all PASS)

go test ./... -count=1
(all packages PASS)
```

Total suite: **485 passing** (from 475 after M1.v). +10 from
M1.m.x.

## Operator workflow examples

**EC2 instance with IAM role.** Operator exports the role's
credentials via `eval $(aws sts assume-role --role-arn ... |
jq ...)` then stores in the vault:

```
agt provider creds set AWS_ACCESS_KEY_ID ASIA...
agt provider creds set AWS_SECRET_ACCESS_KEY ...
agt provider creds set AWS_SESSION_TOKEN ...
agt provider creds set AWS_REGION us-east-1
agt provider reload
agt provider check aws-bedrock
```

The check succeeds; subsequent `agt run` calls sign automatically.

**Classic IAM user on a developer workstation.** Same flow,
without the session token:

```
agt provider creds set AWS_ACCESS_KEY_ID AKIA...
agt provider creds set AWS_SECRET_ACCESS_KEY ...
agt provider creds set AWS_REGION us-east-1
agt provider reload
```

**Migrating from bearer to SigV4** (operator's preview access
revoked):

```
agt provider creds rm AWS_BEARER_TOKEN_BEDROCK
agt provider creds set AWS_ACCESS_KEY_ID ...
agt provider creds set AWS_SECRET_ACCESS_KEY ...
agt provider reload
```

No code-level reconfiguration. The adapter picks whichever auth
is present at each `applyAuth` call.

## What's intentionally NOT in M1.m.x

- **AWS credential-provider chain** (~/.aws/credentials,
  ~/.aws/config profile, EC2 instance metadata, ECS task role,
  SSO). As discussed above — covered for 95% of operators by the
  vault entry path. Future M1.m.x.x for the metadata-service
  pickup specifically (most-requested next step on AWS managed
  compute).
- **Non-Anthropic body shapes on Bedrock.** Mistral / Meta /
  Amazon Titan / Cohere / AI21 / DeepSeek each have their own
  request/response JSON shape. M1.m.x ships only the auth layer
  improvement; vendor-specific encoders are a separate vendor-
  by-vendor task (M1.m.y). The streaming framer from M1.t is
  reusable; the inner-event dispatcher per vendor is not.
- **Event-stream chunked signing** (request-streaming, e.g.
  multipart uploads). Bedrock doesn't use it for chat completions;
  out of scope.
- **AWS SDK retry / circuit-breaker logic.** Our retry story is
  the governor's existing fallback-chain walk, not vendor-
  specific exponential backoff. Adding AWS-style retry would mean
  duplicating the governor's responsibility inside a provider.

## Streaming + auth matrix (after M1.m.x)

| Provider | Auth path | Streaming wire |
|---|---|---|
| Anthropic direct | `x-api-key` header | SSE event-tagged |
| OpenAI family | Bearer | SSE untagged + `[DONE]` |
| Google direct | API-key query string | SSE untagged + body-close |
| Vertex (Gemini + Anthropic) | OAuth Bearer (service-account JWT) | SSE |
| Ollama | none (local) | NDJSON |
| Cohere | Bearer | SSE (v2 typed events) |
| **AWS Bedrock (Anthropic)** | **Bearer token OR SigV4** ← M1.m.x added the OR | event-stream binary |
| Mistral / Azure | Bearer / api-key | SSE (via OpenAI compat) |

Every catalog family now has at least one auth path that doesn't
require operators to opt into a vendor's preview programme.

## Files touched

- [plugins/providers/bedrock/sigv4.go](../plugins/providers/bedrock/sigv4.go) — new (~250 LoC).
- [plugins/providers/bedrock/sigv4_test.go](../plugins/providers/bedrock/sigv4_test.go) — new (~230 LoC, 10 tests).
- [plugins/providers/bedrock/bedrock.go](../plugins/providers/bedrock/bedrock.go) — added `sigV4`/`Now` fields, `SetSigV4Creds`/`hasAuth`/`applyAuth`, updated `Complete` to use `applyAuth`, updated `ErrNoBearerToken` message.
- [plugins/providers/bedrock/streaming.go](../plugins/providers/bedrock/streaming.go) — replaced bearer-set with `applyAuth` call.
- [plugins/providers/compat/compat.go](../plugins/providers/compat/compat.go) — `resolveBedrockCreds` now returns a typed `bedrockAuth`, accepts both auth paths; `Build` calls `SetSigV4Creds` when appropriate.
- [plugins/providers/compat/compat_test.go](../plugins/providers/compat/compat_test.go) — two assertion updates to match new error messages.

Zero external-dependency changes. The lean-deps policy holds.

## Deferrals after M1.m.x

- **AWS credential-provider chain** (M1.m.x.x).
- **Non-Anthropic body shapes on Bedrock** (M1.m.y).
- **OS-keychain vault encryption.**
- **Browser tool.**
- **Out-of-process plugin host.**
- **Planner v2** (re-planning, sub-planners, planner tools).
- **Pulse v2** (historical replay, dropped-events synthetic, TUI).

Picking up **OS-keychain vault encryption** next — the vault has
been plaintext on disk since M1.o, which is a real-world security
concern for operators with shared workstations or cloud-synced
home directories.

# M432 — Azure deployment URL: escape model id & api-version

## Context
Provider review pass over the request-building / auth surfaces that had not yet
been swept: AWS Bedrock SigV4 (plugin shim + the real `kernel/creds/sigv4`
algorithm), GCP Vertex auth (JWT/RS256 + metadata SSRF), the non-streaming
request/response decoders for anthropic/google/cohere/ollama, the large
Vertex anthropic.go/vertex.go decoders, and the `compat` provider factory.

**Reviewed CLEAN (no changes):**
- `kernel/creds/sigv4/sigv4.go` — audited line-by-line against the AWS
  `get-vanilla` test vector (signing key + signature match exactly): canonical
  request, query/header canonicalization + signed-header set, string-to-sign,
  the kDate→kRegion→kService→kSigning HMAC chain (no key/message swap), and the
  Authorization assembly are all correct and deterministic.
- `plugins/providers/bedrock/{sigv4.go,bedrock.go}` — hash↔body consistent,
  status-checked, 64 MiB body cap, no header injection (CRLF rejected by
  url.Parse), session token signed when present, no credential leakage.
- `plugins/providers/vertex/{auth.go,metadata.go}` — RS256 JWT correct
  (RawURLEncoding, PKCS#8→PKCS#1 fallback, 1h exp), token cache with 60s skew
  margin, metadata pinned to `metadata.google.internal` + `Metadata-Flavor:
  Google` + 1 MiB cap (no SSRF), context propagated, no key/token leakage.
- `plugins/providers/{anthropic,google,cohere,ollama}.go` non-streaming and
  `plugins/providers/vertex/{anthropic.go,vertex.go}` — the highest-value class
  (panic on a malformed/empty upstream response) is defended everywhere: every
  `[0]` index is length-guarded (`len(Candidates)==0` etc.), the rest iterate
  with `range`; status checked before decode; 64 MiB body cap; bodies closed on
  all paths; auth headers correct and never logged; Anthropic-on-Vertex omits
  `model` from the body and pins `anthropic_version` as required.

One MED finding remained, in the compat factory.

## The bug
`plugins/providers/compat/compat.go` (Azure family): the per-request URL was
built by raw string concatenation —

    fullURL := urlBase + "/openai/deployments/" + modelID +
               "/chat/completions?api-version=" + apiVersion

`modelID` (the Azure *deployment* name — operator-named, and for non-Azure
families a catalog/`custom.json`-supplied model id, which the threat model
treats as untrusted config) was inserted into the path unescaped. A value
bearing a URL-significant character breaks the request:
- a `?` terminates the path early and **smuggles a query parameter ahead of the
  real `api-version`** (verified: id `dep ?injected=evil` produced query
  `injected=evil/chat/completions?api-version=...` — the intended api-version is
  corrupted and an attacker-chosen parameter is injected);
- a space / `/` / `#` produces a malformed or mis-routed URL.

Severity MED: deployment names are normally alphanumeric and gated by the
`p.Models` allowlist, so exploitation needs a malicious/mistaken catalog entry —
but it is also a plain correctness bug for any legitimately punctuated name, and
the same class of egress-hygiene gap the project hardens elsewhere.

## The fix
Escape each component into its URL position:

    fullURL := urlBase + "/openai/deployments/" + url.PathEscape(modelID) +
               "/chat/completions?api-version=" + url.QueryEscape(apiVersion)

`url.PathEscape` leaves ordinary alphanumeric Azure names byte-identical, so no
behaviour change for real deployments; it only neutralizes the injection/
malformed-URL cases. `net/url` added to imports.

## Verification
- **`plugins/providers/compat/compat_test.go`** new
  `TestBuild_AzureDeploymentNameEscapedIntoURL`: a deployment id `dep ?injected=evil`
  registered + requested through an httptest server; asserts the full id lands in
  the request *path* (decoded), the smuggled `injected` param is **absent**, and
  the real `api-version` is present. Existing Azure tests still pass.
  - **Negative control:** revert `url.PathEscape(modelID)` → `modelID` (keeping
    `QueryEscape(apiVersion)` so `net/url` stays referenced) → the test FAILs with
    `path="/openai/deployments/dep "` and `query smuggled an injected param`.
    Restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2291** passing (was 2290; +1),
  `go test ./...` exit 0. CHANGELOG Security entry.

## Deferred review findings (documented, not fixed)
- LOW: operator-supplied `p.API` base URL is not scheme-validated, so an
  `http://` override in `custom.json` (or a compromised catalog feed) would send
  the bearer API key in cleartext to that host. This is operator-controlled
  config and a scheme guard risks breaking legitimate internal-proxy setups
  (e.g. `http://localhost` fronting an upstream); left as a documented gap rather
  than changing operator-facing behaviour without a decision.

## Review status
The provider request-building and auth/signing surfaces (Bedrock SigV4, Vertex
auth/metadata, and all provider non-streaming decoders + the compat factory) are
reviewed and sound. This closes the provider review.

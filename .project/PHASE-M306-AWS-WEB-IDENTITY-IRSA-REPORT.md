# M306 — AWS web identity (IRSA / EKS Pod Identity)

## Why
The symmetric twin of M305 (Vertex GCE/GKE metadata credentials), on the AWS
side. After M305 I audited the AWS credential chain and found it already deep —
env, shared `~/.aws` files with profiles, `credential_process`, SSO, STS
`AssumeRole`, and IMDSv2 are all implemented (the `aws.go` header comment
claiming otherwise was simply stale). The one genuinely-missing production path
was **`AssumeRoleWithWebIdentity`** — the credential flow for **IRSA**
(IAM Roles for Service Accounts) and **EKS Pod Identity**.

On EKS the cluster injects, with no application config, a projected OIDC token
file plus a role ARN (`AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN`); the
workload exchanges them at STS for temporary credentials. Without this, running
Agezt's Bedrock provider on EKS meant either mounting a static access key (the
anti-pattern M305 also eliminates for GCP) or falling through to the **node's**
IMDS instance-profile role — coarse, shared across every pod, and not what IRSA
exists to provide. Critically, web identity is **keyless**: the OIDC token *is*
the proof of identity, so the STS call is **unsigned** (no base credentials, no
SigV4) — that's the whole point of IRSA.

## What
- **`kernel/creds/web_identity.go`** (new):
  - `AssumeRoleWithWebIdentity(ctx, WebIdentityParams)` — reads the OIDC token
    from `TokenFile` (re-read on each refresh, since the kubelet rotates it),
    POSTs an **unsigned** `Action=AssumeRoleWithWebIdentity` form to the regional
    STS endpoint, and parses the `AssumeRoleWithWebIdentityResponse` XML for the
    temporary credentials + expiry. Reuses sts.go's `AssumedCreds`,
    `stsAssumeRoleEndpoint`, `defaultSessionName`, `refreshLeadTime`.
  - `AWSWebIdentityLookup(params)` — a `ChainLookup`-shaped lookup that maps the
    canonical `AWS_*` names to a cached result (refreshes within
    `refreshLeadTime` of expiry); non-credential names and any exchange failure
    return empty so the chain falls through cleanly.
- **`cmd/agezt/awschain.go`**: a new layer that **auto-activates** when the
  standard EKS-injected `AWS_WEB_IDENTITY_TOKEN_FILE` + `AWS_ROLE_ARN` are
  present — no agezt-specific opt-in, the SDK-native ambient experience. Placed
  **before** the default chain (env/file/IMDS) so a pod assumes its OWN role
  rather than falling through to the node's IMDS role. The boot banner reports
  `web_identity=<role>`.
- **`kernel/creds/aws.go`**: corrected the stale "not implemented" header — SSO,
  `credential_process`, assume-role, and web identity are all wired now; only
  SAML federation remains out of scope.

## Verification
- **`web_identity_test.go`** (6 tests): happy path asserting the temporary
  creds, expiry, the forwarded token-file contents as `WebIdentityToken`, the
  right `Action`, **and that the request carries no `Authorization` header**
  (the keyless/unsigned property — the core invariant); missing- and
  empty-token-file errors; STS API error surfaced; `AWSWebIdentityLookup`
  caches across probes (one STS call for many lookups) with region passthrough;
  and an STS-failure-falls-through case.
- **`cmd/agezt/awschain_test.go`** (2 tests, new file): the EKS env vars
  auto-activate the layer (banner reports `web_identity=EksPodRole`), and a
  token file without `AWS_ROLE_ARN` does **not** activate it. Construction is
  network-free (lazy lookup), so no STS call fires.
- Full suite **1965** passing (`grep "PASS:"`), `go test ./...` exit 0; `go vet`
  clean on `kernel/creds` + `cmd/agezt`; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged; `gofmt -l` clean on every new/changed file.

## Scope notes
- New capability, additive: zero behaviour change when the EKS env vars aren't
  present. No new dependency (stdlib `net/http` + `encoding/xml`, plus the
  existing `sigv4` types for the result shape — not for signing). No
  protocol/format change.
- Deliberately out of scope (clean follow-ups): SAML federation
  (`AssumeRoleWithSAML`); regional STS endpoint selection beyond the
  `sts.{region}.amazonaws.com` default; an `agt doctor` line surfacing which AWS
  credential layer engaged (the boot banner already prints it).
- This closes the keyless-ambient-credentials gap on both major clouds: GCP via
  the metadata server (M305), AWS via IRSA/web identity (M306).

# Phase Report — Milestone 1.dd (AWS credential-provider chain)

> Status: **shipped** · Date: 2026-05-29
> Closes the M1.m.x deferral: "Future M1.m.x.x could add AWS_PROFILE / metadata-service auto-discovery."
> Continues [PHASE-M1.cc-REPORT.md](PHASE-M1.cc-REPORT.md).

## Scope

M1.m.x shipped pure-stdlib AWS SigV4 for Bedrock, but credential
resolution stopped at "look up AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY
in env or vault." Operators using:

- the canonical `~/.aws/credentials` file (everyone with `aws configure` run once),
- a non-default profile via `AWS_PROFILE=work`,
- EC2 instance-role credentials (anyone running agezt on EC2 with an IAM role attached),

had to manually pluck the values out and either put them in env or
vault, defeating the entire "AWS SDKs Just Work" experience the
operator expected.

M1.dd ships the standard AWS credential-provider chain in pure
stdlib (no aws-sdk-go transitive deps, in line with the lean-deps
policy). Three new sources, all `func(string) string` so they
compose with the existing `creds.ChainLookup`:

```go
creds.ChainLookup(
    credStore.Lookup,         // 1. agezt vault (M1.w, AES-GCM-at-rest)
    os.Getenv,                // 2. process env
    creds.AWSDefaultChain(),  // 3+4. AWS shared file + IMDSv2
)
```

Stages 3 and 4 answer only AWS_* names, so non-AWS lookups still
fall through cleanly to the next chain entry.

| Concern | Status |
|---|---|
| `~/.aws/credentials` (default + named profiles) | ✅ tested |
| `~/.aws/config` (region; `[profile X]` quirk for non-default) | ✅ tested |
| `AWS_PROFILE` env var selects profile when arg is empty | ✅ tested |
| `AWS_SHARED_CREDENTIALS_FILE` / `AWS_CONFIG_FILE` overrides | ✅ tested |
| Missing file → empty (chain falls through) | ✅ tested |
| Tiny INI parser (handles comments, whitespace, no extra deps) | ✅ |
| IMDSv2 happy-path: token PUT + role list + creds JSON + region | ✅ tested (httptest server) |
| IMDS response caching (cred fetch once across many lookups) | ✅ tested |
| IMDS expiration tracking (refresh 60s before Expiration) | ✅ |
| IMDS failure neg-caching (30s suppression — no boot-time slowness) | ✅ tested |
| Non-AWS names → empty (no IMDS call) | ✅ tested |
| `AWSDefaultChain` composes env > file > IMDS in correct precedence | ✅ tested |
| Daemon wires `AWSDefaultChain` into both initial + hot-reload paths | ✅ |
| Zero new deps in go.mod | ✅ (verified) |

## Why pure stdlib (again)

The AWS Go SDK is a 30+ transitive-dep dependency tree. agezt's
lean-deps policy excludes it (current go.mod still: only
`lukechampine.com/blake3` + transitive `klauspost/cpuid/v2`). The
chain we implement covers the three sources that account for ~95%
of operator setups; SSO, web-identity, process credentials, and
assume-role flows are NOT implemented and should round-trip via
env vars (which the operator already gets from `aws sso login`'s
shell exports or from a wrapper script around `aws sts
assume-role`).

This phase is the third "we don't need the AWS SDK" demonstration:

| Phase  | Built without aws-sdk-go            |
|--------|--------------------------------------|
| M1.m.x | SigV4 signing                       |
| M1.t   | Bedrock binary event-stream framer  |
| M1.dd  | Credential-provider chain           |

The Bedrock surface agezt uses is now fully stdlib.

## Files

### `kernel/creds/aws.go` — new (~390 LoC)

Three exported functions that all return `func(string) string`:

**`AWSSharedCredentialsLookup(profile string)`** — Reads
`~/.aws/credentials` (selected profile) plus `~/.aws/config` for
region. Lazy on first call, cached for the process. Handles the
AWS-CLI quirk where the config file uses `[profile NAME]` for
non-default profiles but bare `[default]` for the default profile —
get that wrong and "I'm using the default profile, why isn't my
region picked up?" silently fails.

**`AWSIMDSLookup(client *http.Client)`** — IMDSv2 flow:
PUT `/latest/api/token` for a session token, GET
`/latest/meta-data/iam/security-credentials/` for role name,
GET `/latest/meta-data/iam/security-credentials/<role>` for the
credentials JSON, then GET `/latest/meta-data/placement/region`.
Caches the result, tracks Expiration, refreshes 60s before. On
failure, neg-caches for 30s so the next lookup doesn't pay the
slow timeout again.

**`AWSDefaultChain()`** — Convenience that composes env first,
then shared credentials file, then IMDS. Operators wire it via
`creds.ChainLookup(vault.Lookup, creds.AWSDefaultChain())` — a
single canonical chain rather than ad-hoc composition that drifts
between callers.

Implementation notes:

- **Tiny INI parser** (`readINISection`). AWS INI is a strict subset
  of full INI — no nested sections, no quoted strings, no multiline
  values. ~30 LoC handles every real-world AWS config file. If we
  ever hit one that needs more, we'll pull in a real INI lib then.
- **No retries.** A failed IMDS call returns empty; the chain
  falls through. No exponential backoff, no jitter. Operators
  whose IMDS is unreachable have a config problem the daemon
  should surface clearly when nothing in the chain answers, not
  paper over with retries.
- **Neg-cache** (`negCacheTTL = 30s`). The first lookup on a
  non-EC2 daemon hits the IMDS timeout (~1s); the second through
  N-th lookups within 30s short-circuit immediately. The 30s
  window is well under the daemon's typical startup → first-call
  latency, so a real EC2 daemon's first lookup correctly populates
  the cache without the neg-cache ever firing.
- **Closed-set name filter** (`awsRecognisedNames`). Only AWS_*
  names trigger any I/O. `lookup("OPENAI_API_KEY")` on an AWS
  source is a constant-time map miss returning "". This is what
  lets the AWS sources sit safely at the end of `ChainLookup`
  without affecting non-AWS lookups' performance.

### `kernel/creds/aws_test.go` — new (~280 LoC, 12 tests)

| Test | Locks in |
|---|---|
| `TestAWSSharedCredentialsLookup_ReadsCredentialsFile` | Happy-path read of `[default]` section: key/secret/token/region |
| `TestAWSSharedCredentialsLookup_HonoursProfile` | Both explicit profile arg AND `AWS_PROFILE` env var route to the right section |
| `TestAWSSharedCredentialsLookup_ConfigFileSuppliesRegion` | Region in `~/.aws/config` (not credentials) is picked up — the most common operator setup |
| `TestAWSSharedCredentialsLookup_DefaultProfileNoPrefix` | The `[default]` vs `[profile X]` quirk in `~/.aws/config` |
| `TestAWSSharedCredentialsLookup_MissingFile` | Missing files → empty, no panics |
| `TestAWSSharedCredentialsLookup_RecognisedNamesOnly` | Non-AWS lookups return empty so the chain falls through |
| `TestAWSIMDSLookup_HappyPath` | Token PUT + role list + creds JSON + region all round-trip correctly via httptest server |
| `TestAWSIMDSLookup_CachesAcrossCalls` | Four lookups → exactly one IMDS handshake (no per-call hits) |
| `TestAWSIMDSLookup_FailureFallsThroughAndNegCaches` | 500 from IMDS → empty + neg-cache suppresses second-lookup retry |
| `TestAWSIMDSLookup_NonAWSNameReturnsEmpty` | Non-AWS lookup → zero IMDS hits (constant-time short-circuit) |
| `TestAWSDefaultChain_EnvWins` | SDK precedence: env beats file when both are set |
| `TestAWSDefaultChain_FallsThroughToFile` | When env is cleared, file values surface |

The IMDS tests use `httptest.NewServer` with a small mux that
mimics the IMDSv2 verb/header semantics — including the 401 on
missing token header that real IMDSv2 returns — so a regression
in the bridge's request shape (e.g. forgetting the token header)
fails the test immediately.

### `cmd/agezt/main.go` — two edits (initial + hot-reload paths)

Both the daemon startup chain and the `OnReload` chain now end with
`creds.AWSDefaultChain()` after the env source:

```go
credLookup := creds.ChainLookup(
    credStore.Lookup,        // 1. agezt vault
    os.Getenv,               // 2. process env
    creds.AWSDefaultChain(), // 3+4. AWS shared file + IMDS
)
```

Banner description updated so operators can see the chain is
active: `vault entries=N at <path> (env vars + AWS chain fall through)`.

## Operator workflow examples

**EC2 with attached IAM role (no env, no vault):**

```bash
# On the EC2 instance:
export AGEZT_PROVIDER=bedrock
agezt
# Bedrock provider transparently uses instance-role credentials —
# refreshes on expiry without restart.
```

**Local dev with `aws configure` setup:**

```bash
# Previously: had to copy AWS_ACCESS_KEY_ID/SECRET into vault or env.
# Now:
aws configure  # if you haven't already — writes ~/.aws/credentials
export AWS_PROFILE=work
export AGEZT_PROVIDER=bedrock
agezt
# Bedrock picks up work-profile creds + region.
```

**Mixed: vault for OpenAI, AWS chain for Bedrock:**

```bash
agt vault set OPENAI_API_KEY sk-...
# AWS creds left in ~/.aws/credentials as usual.
agezt
# Vault answers OpenAI; AWS chain answers Bedrock — both work.
```

## Test summary

```
go test ./kernel/creds/ -v -count=1 -run TestAWS
(12 tests — all PASS)

go test ./... -count=1
(35 packages — all PASS, no regressions)
```

+12 from M1.dd.

## What's intentionally NOT in M1.dd

- **AWS SSO (`aws sso login`).** Token cache file is JSON, would
  require parsing + refresh against the SSO API. Operators using
  SSO already get an env-var export from `aws sso login` →
  `eval "$(...)"`; the env source covers it.
- **`assume-role` profile chaining.** When a profile in
  `~/.aws/config` says `role_arn = ...`, the AWS SDK calls STS
  with the source profile to swap in temporary credentials. That's
  an STS dependency + signature loop; not in scope. Operators
  wanting it should pre-exchange via `aws sts assume-role` and
  pass the temporary creds in env.
- **Web identity tokens.** Used by EKS pod-identity / IRSA. Same
  argument as assume-role — a focused future phase if the demand
  is real, otherwise the operator's `aws-iam-authenticator` or
  similar already produces env vars.
- **Process credentials (`credential_process = ...`).** Spawns a
  binary that prints JSON to stdout. Easy to add but adds an
  exec-side-channel risk (the binary runs with the daemon's
  privileges), so deserves its own phase + threat model.
- **IMDSv1 fallback.** v1 is deprecated and many AMIs disable it.
  v2-only keeps the code simple and avoids accidentally letting
  an SSRF chain steal creds from an unauthenticated metadata
  endpoint. Operators stuck on v1-only AMIs need to upgrade.
- **Region discovery from EC2 IMDS when no role attached.** We
  fetch region from IMDS only as part of the credential path. A
  pure "I want region without creds" lookup isn't a real agezt
  scenario — every code path that needs region also needs creds.

## Files touched

- [kernel/creds/aws.go](../kernel/creds/aws.go) — new, AWS chain sources + composer.
- [kernel/creds/aws_test.go](../kernel/creds/aws_test.go) — new, 12 tests.
- [cmd/agezt/main.go](../cmd/agezt/main.go) — wires `AWSDefaultChain()` into both initial + reload cred lookups.

Zero changes to bedrock provider, governor, bus, journal, agent
loop, or any other package. The chain composes via the existing
`ChainLookup` extension point.

## Deferrals after M1.dd

Unchanged from M1.cc, minus the AWS chain just shipped:

- Pulse v3+ (TUI dashboard, until/last flags, replay rate limit,
  subject indexing).
- Planner v2 (re-planning, sub-planners, planner-side tools).
- Plugin sandboxing, signing, hot-reload, streaming, callbacks.
- Browser: JS rendering, screenshots, search, cookies.
- **Non-Anthropic body shapes on Bedrock** (M1.m.y) — next pickup,
  rounds out the Bedrock surface.
- Vault: OS-keychain integration, passphrase rotation, argon2.
- MCP bridge v2 (resources/sampling/progress/cancellation/SSE/image).
- Routing extensions (`TaskRouteRequire`, per-task-type budgets,
  per-task-type model overrides) — wait for demand.
- AWS extensions noted above (SSO, assume-role, web identity,
  process creds, IMDSv1) — wait for demand.

# M308 — `agt doctor` credential-chain preflight check

## Why
Health-pane parity for M307. `agt status` now shows the resolved AWS credential
chain, but `agt doctor` — the production-readiness preflight an operator runs
*before* trusting a deployment — didn't. Several daemon facts (channels,
exposure) already appear in both surfaces precisely because the preflight is
where an operator wants confirmation; the credential chain belongs there too. An
operator deploying to EKS wants `agt doctor` to confirm "IRSA engaged" alongside
the sandbox / provider / exposure checks, in one pass.

## What
- **`cmd/agt/doctor.go`**: new `checkCredentials(status)` pure check, wired into
  `runDoctorChecks` next to `checkChannels`. It reads the `cred_chain` field
  (added to `CmdStatus` in M307) and reports an informational `OK` line with the
  chain, **calling out a keyless layer** (`web_identity` / `assume_role` / `sso`)
  with a `[keyless: …]` tag when present. Always OK: the chain always resolves to
  at least vault→env→file/IMDS, and whether those hold live credentials is a
  runtime fact the separate provider check covers — so this is confirmation, not
  a failure gate (same role `checkChannels` plays).

## Verification
- **`cmd/agt/doctor_test.go`**: `TestCheckCredentials` — keyless IRSA layer →
  OK with `keyless: web_identity` highlighted; base chain → OK with no keyless
  tag; absent → OK with the default-chain description (never a FAIL).
- **Live demo**: a mock daemon with the EKS env (`AWS_WEB_IDENTITY_TOKEN_FILE`,
  `AWS_ROLE_ARN`) → `agt doctor` printed:

      [OK  ] aws creds        : AWS chain: vault → env → web_identity=EksDemoRole → default(file+IMDS)  [keyless: web_identity]

- Full suite **1967** passing, `go test ./...` exit 0; `gofmt -l` clean on the
  changed files; `go vet` clean; `go.mod` / `go.sum` unchanged. Adding a check
  doesn't perturb the doctor JSON shape (each check serialises identically) or
  the exit-code logic (an OK check can't worsen `worst`).

## Scope notes
- Additive, read-only, informational. No new control-plane round-trip (reuses the
  `status` map the doctor already fetches). No new dependency.
- Completes the keyless-credentials observability arc: minted (M305 GCP / M306
  AWS), surfaced in status (M307), confirmed at preflight (M308).
- Clean follow-up still open: web-UI status-panel parity for the cred chain.

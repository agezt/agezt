# M307 — Surface the AWS credential chain in `agt status`

## Why
Completes the operator story for M305/M306 (keyless ambient credentials on GCP
and AWS). The daemon already computes a human-readable description of the
resolved AWS credential chain at boot — `AWS chain: vault → env →
web_identity=<role> → default(file+IMDS)` — but it was printed **only to the
startup banner**. A production operator on EKS who wants to confirm "did IRSA
actually engage?" had to grep pod logs for that one line; the canonical
`agt status` round-trip didn't carry it. Every other piece of boot metadata
(HTTP bindings, channels) is already surfaced through status precisely so
operators don't have to scroll back to the banner — the credential chain was the
gap.

## What
- **`kernel/controlplane/server.go`**: new `credChain string` field on `Server`
  plus `SetCredChain(desc string)` — same lock-free, set-at-construction pattern
  as `SetHTTPBindings` / `SetChannels`.
- **`kernel/controlplane/status.go`**: `handleStatus` includes
  `"cred_chain": <desc>` when non-empty (omitted otherwise, so non-AWS daemons
  add no noise).
- **`cmd/agezt/main.go`**: records the already-computed `awsChainDesc` via
  `srv.SetCredChain(awsChainDesc)`, alongside the existing `SetHTTPBindings` /
  `SetChannels` wiring.
- **`cmd/agt/status.go`**: renders an `aws creds : <chain>` line when present,
  quiet otherwise.

## Verification
- **`kernel/controlplane/status_credchain_test.go`** (new): `CmdStatus` omits
  `cred_chain` when unset, and surfaces it verbatim after `SetCredChain`
  (asserting the `web_identity=<role>` layer is visible).
- **Live demo** (end-to-end, the full wiring): a mock daemon started with the
  EKS-injected env (`AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN`,
  `AWS_REGION`) → `agt status` printed:

      aws creds : AWS chain: vault → env → web_identity=EksDemoRole → default(file+IMDS)

  proving boot env → `buildAWSCredChain` auto-activation → `SetCredChain` →
  `handleStatus` → CLI render. No STS call happens (the lookup is lazy; the
  description is built at construction).
- Full suite **1966** passing, `go test ./...` exit 0; `gofmt -l` clean on every
  changed file; `go vet` clean; `go.mod` / `go.sum` unchanged. Adding a field to
  the status map is backward-compatible (the web UI / JSON consumers ignore
  unknown keys).

## Scope notes
- Additive + read-only: no behaviour change, no new command, no new dependency.
  Quiet when AWS credentials aren't configured.
- The description reflects the **boot-time** chain composition. The hot-reload
  path (`cmd/agezt/main.go`) recomposes the chain but the env-var-driven opt-ins
  don't change at runtime, so the boot description stays representative.
- Clean follow-ups: an `agt doctor` informational check echoing the same chain
  (health-pane parity); surfacing it in the web UI status panel.

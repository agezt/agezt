# M265 — Migrate tooling, milestone 3: `agt vault status` shows KDF policy

## Why
M263 + M264 added vault re-encryption and the `agt vault migrate` command, but
an operator had no way to know *whether a migration was needed* short of running
it. `agt vault status` reported only encrypted-vs-plaintext and an entry count.
This milestone closes that gap: status now surfaces the vault's key-derivation
policy and whether it is up to date, so the operator sees at a glance whether
`agt vault migrate` is worth running — and crucially, **without needing the
passphrase** (the envelope's KDF parameters are plaintext metadata).

## What
- **`cmd/agt/vault.go`** — new `printVaultKDF(stdout, path)` helper, called from
  both encrypted branches of `cmdVaultStatus` (the loaded-and-encrypted case and
  the `ErrPassphraseRequired` case):
  - reads the envelope via `creds.InspectVault` (M263) — no decryption, no
    passphrase;
  - prints `key deriv:   <kdf> (<n> iterations)`;
  - prints `migration:   up to date` when at the current PBKDF2 + iteration
    policy, else `migration:   recommended — run `agt vault migrate` to upgrade
    to pbkdf2-hmac-sha256/200000 iterations`;
  - no-op for a plaintext or unreadable vault (no spurious KDF line).

## Files
- `cmd/agt/vault.go` — `printVaultKDF` + two call sites in `cmdVaultStatus`
  (edited).
- `cmd/agt/vault_status_test.go` — 2 tests (new): an encrypted current-KDF vault
  shows the KDF line + "up to date"; a plaintext vault shows no KDF line.

## Verification
- `go test ./cmd/agt/ -run 'TestVaultStatus|TestVaultMigrate'` — all green; full
  suite **1853 → 1855** (+2), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on both touched files; `go vet ./cmd/agt/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven** against real vaults under `$AGEZT_HOME`:
  - encrypted current PBKDF2 → `key deriv: pbkdf2-hmac-sha256 (200000
    iterations)` + `migration: up to date`;
  - **legacy KDF, no passphrase set** → `key deriv: hmac-sha256-iter (200000
    iterations)` + `migration: recommended — run `agt vault migrate`…` (the key
    win: an operator learns migration is available without the passphrase);
  - plaintext → no KDF line.

## Scope notes
- Completes the operator's migrate loop end-to-end for the credential vault:
  **see** it's stale (`agt vault status`, M265) → **fix** it (`agt vault
  migrate`, M264) → both backed by the same passphrase-free `InspectVault`
  detection (M263).
- Pure read-side, additive: no new event, no schema change, no passphrase
  requirement added to `vault status`.

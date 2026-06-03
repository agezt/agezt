# M264 — Migrate tooling, milestone 2: `agt vault migrate` CLI

## Why
M263 added the kernel-level credential-vault migration (`creds.InspectVault` /
`Store.MigrateEncryption`) but left it unreachable by an operator — the report
explicitly noted "the CLI command wires this up next". This milestone is that
wiring: an operator who upgrades Agezt can now upgrade a vault written with the
legacy (pre-PBKDF2) key-derivation, or one below the current iteration policy,
to the current policy with a single command.

## What
- **`cmd/agt/vault.go`** — new `migrate` subcommand:
  - `cmdVaultMigrate` inspects the on-disk vault (no passphrase needed) and:
    - plaintext / absent → prints "vault is not encrypted — nothing to migrate"
      and exits 0;
    - already at the current KDF + iteration policy → prints "vault already at
      the current key-derivation policy (…)" and exits 0;
    - encrypted but stale, with no `AGEZT_VAULT_PASSPHRASE` set → prints a clear
      "passphrase must be set to re-encrypt" notice and exits 2;
    - encrypted and stale, passphrase present → re-encrypts in place at the
      current PBKDF2 policy, prints the before→after KDF and iteration count, and
      points to `agt provider reload`.
  - Dispatch `case "migrate"`, the subcommand-required / unknown-subcommand
    messages, and `printVaultHelp` all updated to list `migrate`.
- **`cmd/agt/main.go`** — top-level help gains a `vault migrate` line.

## Files
- `cmd/agt/vault.go` — `cmdVaultMigrate`, dispatch, help (edited).
- `cmd/agt/main.go` — top-level help line (edited).
- `cmd/agt/vault_migrate_test.go` — 2 tests (new): a plaintext vault →
  "not encrypted"; a freshly-saved (current-KDF) encrypted vault → "already
  current", neither requiring the passphrase. The legacy→current upgrade
  mechanics are covered by M263's kernel tests; `buildLegacyEnvelope` is
  unexported in package `creds`, so the CLI test covers the no-op/output
  branches and the kernel test covers the actual re-encryption.

## Verification
- `go test ./cmd/agt/ -run TestVaultMigrate` — both green; full suite
  **1851 → 1853** (+2), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on the three touched files; `go vet
  ./cmd/agt/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven** against a real `$AGEZT_HOME`:
  - absent and plaintext vaults → "vault is not encrypted — nothing to migrate";
  - a real current-PBKDF2 encrypted vault (200000 iterations) → "vault already
    at the current key-derivation policy (pbkdf2-hmac-sha256, 200000
    iterations)".

## Scope notes
- This completes the operator-facing path for the migrate arc's first format
  (the credential vault). The detect-then-upgrade pattern established in M263 +
  M264 is now end-to-end and can be replayed for other on-disk formats if they
  ever need it.
- Re-encryption reuses `Save`'s atomic write-temp-then-rename, so an interrupted
  migrate never leaves a half-written vault; the no-op cases never touch the
  file.

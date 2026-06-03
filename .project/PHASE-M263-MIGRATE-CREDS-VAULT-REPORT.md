# M263 — Migrate tooling, milestone 1: credential-vault re-encryption

## Why
With the SDK arc complete, the next big feature (chosen autonomously for being
concrete, offline-testable, and low-risk) is **migrate tooling** — upgrading
out-of-date on-disk formats when an operator updates Agezt. The clearest,
documented-as-deferred need is the credential vault: vaults written before the
PBKDF2 switch (M172) use a legacy keyed-HMAC KDF, and a vault's iteration count
is fixed at write time. Such vaults stay *readable* but are weaker than a
freshly-saved one, with no path to upgrade them short of re-saving by hand. This
milestone adds the detection + in-place re-encryption.

## What
- **`kernel/creds/migrate.go`** — new:
  - `VaultStatus{Encrypted, KDF, Iterations, UpToDate}` — a vault's
    key-derivation parameters and whether they meet current policy.
  - `InspectVault(path)` — reads the envelope WITHOUT decrypting (no passphrase
    needed) and reports its KDF/iterations and whether it is up to date. Plaintext
    or absent → `Encrypted=false`.
  - `Store.MigrateEncryption()` — for an encrypted vault not at the current
    policy, decrypts (with its stored legacy/low-iteration KDF) and re-encrypts
    in place at the current PBKDF2 + iteration policy; no-op for plaintext or
    already-current vaults. The passphrase and secrets are unchanged — only the
    KDF/iteration parameters improve.

## Files
- `kernel/creds/migrate.go` — `VaultStatus`, `InspectVault`, `MigrateEncryption`
  (new).
- `kernel/creds/migrate_test.go` — 3 tests: inspect (legacy / plaintext /
  absent), upgrade a legacy vault (secret preserved, now PBKDF2 at current
  iterations, idempotent on re-run), and the plaintext/absent no-ops (new). Uses
  the existing `buildLegacyEnvelope` helper to craft a pre-M172 vault.

## Verification
- `go test ./kernel/creds/` — green; full suite **1848 → 1851** (+3), 68
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/creds/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- This is the kernel-level migration logic; the next milestone wires it to a CLI
  command (`agt vault migrate` / status) so an operator can run it.
- Migration is deterministic and safe: re-encryption is the same atomic
  write-temp-then-rename `Save` uses, and the no-op cases never touch the file.
- Sets the pattern for the migrate arc — detect-then-upgrade — that further
  on-disk formats (journal, state projections) can follow if they ever need it.

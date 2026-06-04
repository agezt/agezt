# M303 — Credential vault: bad-nonce panic → clean error

## Why
A security/correctness audit of the credential vault crypto (`kernel/creds`,
chosen as an untouched, high-stakes subsystem). The cipher design is sound — fresh
salt + nonce per save from `crypto/rand`, AES-256-GCM (authenticated), PBKDF2 with
a minimum-iteration floor, GCM-open failure mapped to `ErrWrongPassphrase` (no
plaintext leak). But `decryptVault` passed the envelope's decoded `nonce` straight
into `gcm.Open` **without validating its length**.

Go's GCM `Open` *panics* — `panic("crypto/cipher: incorrect nonce length given to
GCM")` — when the nonce isn't `NonceSize()` (12) bytes, rather than returning an
error (verified with a standalone reproducer). So a vault file whose `nonce` field
base64-decodes to the wrong length — disk corruption, a truncated/partial write, or
deliberate tampering — would **crash the daemon or CLI** instead of failing
cleanly. A vault read happens at daemon boot and on `agt vault`/`agt provider`
operations, so this is a real availability bug (a corrupt file shouldn't panic the
process).

## What
- **`kernel/creds/encrypt.go`**: `decryptVault` validates `len(nonce) ==
  NonceBytes` right after base64-decoding it, returning a clear
  "vault corrupt or tampered" error before reaching `gcm.Open`. Ciphertext and
  salt need no check — GCM errors (not panics) on a short ciphertext, and PBKDF2
  accepts any salt length; the derived key is always 32 bytes (no `aes.NewCipher`
  panic). The nonce was the only panic vector.

## Files
- `kernel/creds/encrypt.go` (edited).
- `kernel/creds/encrypt_test.go`: **new**
  `TestDecrypt_BadNonceLengthErrorsNotPanic` — encrypts a real vault, rewrites its
  envelope with a 5-byte nonce, and asserts `decryptVault` returns an error (the
  test harness fails on a panic, so it genuinely guards the regression).

## Verification
- Full suite **1914**, 68 packages, `go test ./...` exit 0; `go vet ./kernel/creds`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.
  (`gofmt -l` flags `encrypt.go` — the pre-existing envelope-struct comment
  re-alignment noted since M267; `gofmt -d` confirms the diff is only those comment
  lines, not the added nonce check.)
- **Reproducer confirmed**: a standalone snippet calling `gcm.Open` with a 5-byte
  nonce panics with "incorrect nonce length given to GCM"; the new guard turns that
  into a clean error.

## Scope notes
- Pure robustness/availability hardening of an existing path; no behaviour change
  for a well-formed vault, no new dependency, no format change.
- Audited and found solid in the same pass (no fix needed): fresh per-save salt +
  nonce (no nonce reuse), authenticated GCM, KDF-iteration downgrade floor
  (`KDFIterMinAccepted`), wrong-passphrase → `ErrWrongPassphrase` with no plaintext
  leak, `InspectVault` reads the envelope without decrypting.

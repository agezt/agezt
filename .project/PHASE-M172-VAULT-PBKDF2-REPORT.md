# M172 — Vault KDF hardened to genuine PBKDF2-SHA256 (crypto review)

## Why
A cryptographer-style review of the at-rest credential vault (`kernel/creds` —
provider API keys encrypted with a passphrase) was commissioned. The vault uses a
*custom* KDF (iterated HMAC-SHA256) because the project is stdlib-only (PBKDF2 lives
in `x/crypto`, excluded by lean-deps), so the KDF construction was the focus.

## Confirmed correct (the disaster properties)
The review verified — and left unchanged — the two properties whose failure would
be catastrophic:
- **No GCM (key, nonce) reuse.** Every save draws a fresh 32-byte salt → derives a
  fresh key AND a fresh 12-byte nonce, both from `crypto/rand` with short-read
  errors checked. Even a chance nonce collision can't matter because the key
  differs per save.
- **No plaintext at rest.** Encrypt-then-write: the atomic temp file is written
  already-encrypted, in `Save` and in `Rotate`; rotation decrypts-in-memory then
  re-encrypts with fresh salt+nonce, updating the in-memory passphrase only after a
  successful rename.
Also confirmed: GCM auth tag verified (wrong passphrase → `ErrWrongPassphrase`, no
plaintext), 0600 perms, no passphrase logging, strict equality checks on
schema/cipher/kdf ids (no algorithm-downgrade path), empty-passphrase guards.

## The finding (Medium) and fix
The legacy KDF keyed HMAC with the passphrase every round (so each guess genuinely
costs `iter` HMACs — better than a naive `H(H(H(x)))`), but it fed only the *final*
round's output forward. Unlike PBKDF2's `U_1 ⊕ U_2 ⊕ … ⊕ U_c` accumulation, an
internal collision between two candidates' chains would converge thereafter — a
slight (sub-exponential) offline-cracking speedup, making the header's
"equivalent … to PBKDF2" claim only approximately true.

Fix — `deriveKeyPBKDF2` is now genuine PBKDF2-HMAC-SHA256 (RFC 8018), stdlib-only.
Since dkLen (32) == the PRF output (SHA-256, 32) there is exactly one block:
`U_1 = HMAC(P, salt‖INT32BE(1))`, `U_j = HMAC(P, U_{j-1})`, `DK = U_1 ⊕ … ⊕ U_iter`.

### Backward compatibility (no broken vaults)
The KDF is **versioned**: a new envelope id `pbkdf2-hmac-sha256` (`KDFPBKDF2`) is
written by all new saves/rotations, while `decryptVault` dispatches on the
envelope's `kdf` field and keeps `deriveKeyLegacyHMAC` for vaults written with the
old `hmac-sha256-iter` id. Existing encrypted vaults continue to decrypt unchanged.

### Iteration-count floor (F3)
The decrypt floor was raised from `1000` → `KDFIterMinAccepted = 100000`. v2 has
always written 200000; a stored count far below that is malformed or an attempt to
make a stolen vault cheap to crack.

## Tests (+4, all passing)
- `TestDeriveKeyPBKDF2_KnownAnswers` — the correctness proof: matches published
  PBKDF2-HMAC-SHA256 vectors for (`password`,`salt`,1), (…,2), and
  (`passwordPASSWORD…`,`saltSALT…`,4096). A wrong block-index or missing XOR would
  fail these.
- `TestDecrypt_LegacyKDFStillReadable` — a hand-built legacy-KDF envelope still
  decrypts (and wrong passphrase still → `ErrWrongPassphrase`).
- `TestEncrypt_UsesPBKDF2` — new envelopes declare `pbkdf2-hmac-sha256` and round-trip.
- `TestDecrypt_RejectsLowIterFloor` — an envelope claiming 1000 iterations is refused.

## Not changed (accepted/inherent)
- Passphrase/derived-key zeroing (F2, Low) — inherent to env-var + Go-string +
  lean-deps design; the passphrase lives in the process env regardless.
- Atomic-write concurrent-temp race / no fsync (F4, Low) — integrity, not
  confidentiality; plaintext is never staged.
- A pre-existing gofmt comment-alignment artifact in `encrypt.go`'s envelope struct
  (unrelated to this change) is left as-is, per the pre-existing-artifact policy.

## Verification
- `go.mod` / `go.sum` unchanged (PBKDF2 hand-implemented on stdlib `crypto/hmac` +
  `crypto/sha256`); no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` clean on my added lines (the flagged lines are the pre-existing
  struct-comment artifact, not in this diff).
- `go test ./... -count=1` — **FAIL 0**, **1553 tests** (was 1549; +4), 61 packages.

## Result
The vault now derives its AES-256 key with a real, test-vector-verified
PBKDF2-HMAC-SHA256, closing the only substantive crypto finding — while every
existing encrypted vault still opens. The vault's catastrophic-failure surfaces
(nonce reuse, plaintext-at-rest) were reviewed and confirmed already sound.

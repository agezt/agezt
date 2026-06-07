# M494 — Mutation testing creds: pin the legacy KDF (and strengthen PBKDF2)

## Context
Sixth and last clearly-security-critical package in the mutation pass:
`kernel/creds` (the credential vault, including at-rest AES-256-GCM encryption and
key derivation). `go-mutesting .` scored 0.469 over 591 mutants — the figure is
dominated by error-message and HTTP-config mutants in the large STS/SSO/web-identity
credential-fetch code, which are low value. The crypto core was triaged directly.

## Crypto-core triage
- **AES-GCM encrypt/decrypt, nonce-length guard, schema/encryption/KDF validation,
  iter-floor**: well covered by the existing `encrypt_test.go` (round-trip,
  wrong-passphrase sentinel, tamper rejection, bad-nonce-length, low-iteration
  rejection). The surviving mutants there are equivalent or negligible: the redundant
  KDF guard is caught again by the later `switch` default; `KDFIter < / <=` and the
  `100000 → 99999` floor constant have no security impact at that magnitude.
- **deriveKeyPBKDF2**: already pinned by `TestDeriveKeyPBKDF2_KnownAnswers`
  (hard-coded PBKDF2-HMAC-SHA256 vectors) — its XOR/block-index internals were
  already covered.
- **deriveKeyLegacyHMAC (the genuine gap)**: NOT pinned. Every test that exercises it
  round-trips with the *same* function on both sides, so a regression survives. The
  mutation run confirmed it: removing `mac.Write(d)` from the keyed-hash chain
  **survived**. The legacy KDF is *frozen* — it exists only to decrypt vaults written
  before M172 — so any change to its output silently makes those vaults undecryptable.

## Fix
`kernel/creds/kdf_known_answer_internal_test.go` (internal `package creds`):
1. **`TestDeriveKeyLegacyHMAC_GoldenAnswer`** (the gap-closer): pins
   `deriveKeyLegacyHMAC` to golden digests computed by an *independent*
   reimplementation of the documented chain (`d := salt; repeat d =
   HMAC-SHA256(passphrase, d); take 32 bytes`). Non-circular — the goldens were
   generated outside the creds package.
2. **`TestDeriveKeyPBKDF2_MatchesStdlib`** (a strengthening, not a gap-closer):
   cross-checks `deriveKeyPBKDF2` against the stdlib `crypto/pbkdf2` (Go 1.24+,
   authoritative, so the pinning can never itself drift) and adds empty-passphrase /
   empty-salt / unicode cases the existing hard-coded vectors omit. `crypto/pbkdf2` is
   stdlib — no module dependency is added (the production KDF stays hand-rolled per the
   lean-deps policy; this only verifies it in a test).

## Negative control (manual)
Applying the confirmed survivor — `mac.Write(d)` → `_, _ = mac.Write, d` in
`deriveKeyLegacyHMAC` — makes `TestDeriveKeyLegacyHMAC_GoldenAnswer` fail; restored
byte-for-byte (`git diff --ignore-all-space` on encrypt.go empty). The PBKDF2 test
was validated live against stdlib (passes; would fail on any divergence).

## Verification / gate
- New tests pass; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- `kernel/creds` suite green; `go.mod`/`go.sum` unchanged; tracked tree otherwise clean.

## Mutation pass — overall (M490–M494)
Six highest-stakes packages assessed: redact (0.575→0.725, gaps fixed M490), journal
(rotation accounting + Tail trim, M491), edict (whitespace-normalizer contract, M492;
authz core verified), netguard (SSRF core verified solid, M493), event (hash-chain
verified solid — `h.Write(prevBytes)` is equivalent because Canonical already carries
prev_hash), creds (legacy KDF pinned + PBKDF2 strengthened, this milestone). Genuine,
security/integrity-relevant gaps were found and closed where they existed (redact,
journal, edict, creds-legacy-KDF); the rest were confirmed already solid with survivors
that are equivalent or error-message mutants. The mutation criterion in
`.project/HARDENING.md` ("no surviving non-equivalent mutant on the highest-stakes
packages") now holds across all six.

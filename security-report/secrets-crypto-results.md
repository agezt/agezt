# Security Hunt — Secrets, Crypto, Vault, JWT, Data Exposure

Scope: hardcoded secrets/credentials, crypto misuse, vault flow, JWT/token, data exposure.
Codebase: AGEZT (Go + React), D:\Codebox\PROJECTS\AGEZT.

## Verdict

No genuinely exploitable issues found in the secrets/crypto lane. The crypto
construction code is correct and the previously-known hardcoded-secret class
(old agentgw `change-me-in-production`) is confirmed removed. Below is what was
checked, the (one) low-severity informational note, and the evidence that the
high-risk classes are clean.

---

## Informational / Low (not exploitable on their own)

### INFO-1: agentgw JWT tokens carry no issuer/audience claim
- Severity: Informational
- CWE: CWE-345 (Insufficient Verification of Data Authenticity) — partial
- File: `kernel/agentgw/token.go:79-131`
- Detail: `ValidateToken` correctly pins `alg=HS256`/`typ=JWT` (closing the
  alg-confusion / `none` hole), verifies the HMAC with `hmac.Equal`
  (constant-time), and enforces `ExpiresAt`. It does NOT validate `iss`/`aud`.
- Why not a finding: these tokens are single-issuer/single-audience by design
  (one per-install HMAC secret mints and validates them; there is no second
  relying party that could be confused). With the secret per-install and
  alg-pinned, cross-service token confusion is not reachable. Worth a one-line
  `aud` claim only if agentgw tokens ever become multi-audience.
- Confidence: high (that it's not currently exploitable).

### INFO-2: NIP-04 (nostr DM) uses unauthenticated AES-256-CBC
- Severity: Informational
- CWE: CWE-353 (Missing Support for Integrity Check)
- File: `plugins/channels/nostr/nip04.go:24-69`
- Detail: AES-CBC with a fresh random IV and PKCS#7 (padding validated on
  unpad). No MAC — so malleable/no integrity, and theoretical padding-oracle
  surface. This is mandated by the NIP-04 protocol spec, not a local design
  choice; the code comments already flag NIP-44/NIP-17 as the auth'd upgrade.
  IV is random per message (correct), key is the ECDH x-coordinate (per spec).
- Why not a finding: protocol-imposed, optional channel, random IV present.
- Confidence: high.

---

## Verified CLEAN (evidence)

### Hardcoded secrets / fallback secrets — CLEAN
- Grepped source (excluding `*_test.go`, `.env`, node_modules) for
  `secret|token|password|passphrase|apikey` `= "..."` literals. Every hit with a
  real value is in a `_test.go` file (test fixtures, not prod paths).
- The old agentgw hardcoded `"change-me-in-production"` is GONE: it now appears
  only in tests that ASSERT it no longer works (`kernel/agentgw/secret_test.go`,
  `security_test.go`) and in comments documenting the replacement. The live
  signing key is per-install: env override → persisted `<baseDir>/agentgw.secret`
  (0600, O_EXCL first-run race handled) → fresh 32-byte CSPRNG. See
  `kernel/agentgw/secret.go:40-136`.
- No `BEGIN PRIVATE KEY` material in source; the two hits are a secret-classifier
  (`kernel/configcenter/classifier.go:122`) and a redaction regex
  (`kernel/redact/redact.go:99`) — both defensive.

### Vault crypto (`kernel/creds/encrypt.go`) — CLEAN
- AES-256-GCM via stdlib. Fresh 32-byte salt AND fresh 12-byte nonce per save,
  both from `crypto/rand` (`encrypt.go:156-173`). No nonce reuse path.
- KDF is genuine PBKDF2-HMAC-SHA256 (XOR accumulation over rounds, verified
  against RFC vectors per comments), 200,000 iterations (`deriveKeyPBKDF2`,
  `encrypt.go:305-321`).
- KDF-downgrade hardened: decrypt refuses `kdf_iter < 100000`
  (`encrypt.go:213-217`) — an attacker cannot present a stolen vault with a low
  iteration count to make cracking cheap. Legacy `hmac-sha256-iter` KDF is still
  accepted for old vaults but is also subject to the same iteration floor.
- Nonce length validated before `gcm.Open` to avoid a panic on tampered input
  (`encrypt.go:232-234`).
- kdf-cache key uses `sha256(passphrase)`, never the raw passphrase, in the map
  key (`encrypt.go:285-294`).
- Machine-bound default key (`kernel/creds/machine.go`) derives from stable
  machine id + OS user via SHA-256; honest threat model documented (protects
  file-leaving-machine, not local same-user). Operator passphrase always wins.
- Vault file persisted 0600 (`kernel/creds/creds.go:227,233`).

### agentgw token HMAC — CLEAN
- HMAC-SHA256, alg+typ pinned, `hmac.Equal` constant-time compare, expiry
  enforced, subprocess tokens enforce capability subset + never outlive parent
  (`kernel/agentgw/token.go`).

### WebUI auth — CLEAN
- Session ids: 32 bytes from `crypto/rand` (`kernel/webui/session.go:62-72`).
- Console password compared with `subtle.ConstantTimeCompare`
  (`session.go:200`); failed-attempt lockout bounds online guessing.
- URL token compared with `subtle.ConstantTimeCompare` (`kernel/webui/webui.go:1076`).
- Session cookie: HttpOnly, SameSite=Strict, Secure when TLS.

### Channel webhook signature verification — CLEAN
- All inbound signature checks use constant-time comparison
  (`subtle.ConstantTimeCompare` or `hmac.Equal`): slack, whatsapp, webhook,
  nextcloudtalk, line, zalo, onebot, dingtalk, sms, imessage, feishu, wecom,
  chatwebhook, whatsappgw. Discord uses Ed25519 with a replay window and
  fail-closed on missing/invalid key (`plugins/channels/discord/discord.go:450-474`).

### Weak hashes — CLEAN (protocol-mandated only)
- `sha1` appears only in: HMAC-SHA1 signature schemes required by external
  protocols (wecom/WeChat, onebot, sms provider) and the AWS SSO cache-filename
  convention (`kernel/creds/sso.go:80`, non-security). No MD5. No SHA1/MD5 used
  as a security primitive of our own design.

### math/rand vs crypto/rand — CLEAN
- `math/rand` used only for backoff jitter (`plugins/providers/internal/retry/retry.go:150`,
  `kernel/governor/governor.go:759`). All security material (vault salt/nonce,
  session ids, ULIDs, token secrets) uses `crypto/rand`. ULID randomness is
  `crypto/rand`, panics on failure (`kernel/ulid/ulid.go:82-86`).

### Data exposure — CLEAN
- Config Center read path returns presence-only for secret-rated fields, never
  the value (`kernel/controlplane/settings.go:53-55`); comment + code agree.
- Backup explicitly excludes creds/tokens (`cmd/agt/backup.go`).
- A redaction layer (`kernel/redact/redact.go`) scrubs PEM keys and configured
  secret literals from journaled output.
- No grep hits for secrets/passphrases/tokens being logged in plaintext in prod
  paths.

---

## Method
Grepped: crypto/rand vs math/rand, aes./cipher.NewGCM/Nonce/GCM, pbkdf2,
hmac.New, sha1/md5, "secret"/"password"/"token"/"apikey", BEGIN PRIVATE KEY,
subtle.ConstantTimeCompare, jwt, log+secret, file-perm constants. Read the full
construction code for: encrypt.go, machine.go, agentgw/secret.go,
agentgw/token.go, webui/session.go, ulid.go, nip04.go, settings.go (config read),
discord verify, sso.go.

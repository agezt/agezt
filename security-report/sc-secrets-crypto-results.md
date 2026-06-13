# Security Hunter Results — Secrets, Cryptography & Data Exposure

Domain: Hardcoded secrets, Cryptography misuse, Sensitive-data exposure
Codebase: D:/Codebox/PROJECTS/AGEZT
Scanner: sc-secrets / sc-crypto / sc-data-exposure

## Summary of counts

| Severity | Count |
|----------|-------|
| Critical | 2 |
| High     | 2 |
| Medium   | 2 |
| Low      | 1 |
| Info / verified-correct (no action) | 4 |

---

## CRITICAL

### SECRET-001 — Hardcoded HMAC token-signing secret used in production (live auth bypass)
- **Severity:** Critical
- **Confidence:** 100
- **CWE:** CWE-798 (Use of Hard-coded Credentials), CWE-321 (Hard-coded Cryptographic Key)
- **File:** `kernel/agentgw/token.go:25`, used at `kernel/agentgw/gateway.go:63`, `kernel/runtime/runtime.go:743`, `cmd/agt/token.go:225`
- **Description:** `const DefaultTokenSecret = "change-me-in-production"` is the HMAC-SHA256 secret used to sign and validate the agent-gateway "JWT-like" bearer tokens. It is NOT a placeholder that must be overridden — it is the live value:
  - `DefaultGatewayConfig()` (gateway.go:63) hardcodes `TokenSecret: []byte("change-me-in-production")`.
  - Production boot calls `agentgw.DefaultGatewayConfig(cfg.BaseDir)` (runtime.go:743) with **no env/vault override** for the secret (only the socket path is overridable via `AGEZT_AGENTGW_SOCKET`).
  - The CLI signs/validates with the same constant (`getTokenSecret()` → `cmd/agt/token.go:225`).
- **Impact:** Anyone who knows this public constant (it is in source, in `security-report/architecture.md`, and reproduced in repo Python helpers) can forge a valid gateway token with arbitrary capability claims, run id, and expiry. The token grants access to the gateway's eventbus / memory write+delete / log read+write / agent query / config get+set endpoints. This is a complete authentication bypass for the agent gateway. Severity is amplified by SECRET-002 and EXPOSE-001 below.
- **Remediation:** Generate a random 32-byte secret at first boot, persist it in the encrypted creds vault (or read `AGEZT_AGENTGW_SECRET` from env/vault), and fail closed (refuse to start, or run gateway disabled) if no secret is configured rather than silently using a known default. The CLI must read the same secret from the vault, not from a compile-time constant. Rotate immediately on any deployment that has run with the default.

### SECRET-002 — Real JWT tokens in working-tree files not covered by .gitignore (git-leak risk)
- **Severity:** Critical (latent — not yet committed, one `git add .` away)
- **Confidence:** 95
- **CWE:** CWE-312 (Cleartext Storage of Sensitive Information), CWE-540 (Information Exposure Through Source Code)
- **File:** `token.txt` (232 bytes), `temp_token.txt` (264 bytes) at repo root
- **Description:** Both files contain real HS256 JWT-style tokens (header decodes to `{"alg":"HS256","typ":"JWT"}`). Confirmed via `git ls-files` (untracked) and `git check-ignore` (NOT ignored). The repo `.gitignore` only covers `.env*`, `*.env`, `creds.json` — not `*.txt` token dumps. Because these tokens are signed with the SECRET-001 hardcoded secret, their leak is doubly damaging: they are both valid now AND forgeable by anyone. (Full contents intentionally not reproduced here.)
- **Impact:** A routine `git add .` / `git commit -am` would commit live bearer tokens to history. Combined with SECRET-001 they authenticate to the gateway. Even without that, committed tokens are valid until expiry.
- **Remediation:** Delete `token.txt` and `temp_token.txt` now. Add a `.gitignore` rule for local token/secret scratch files (e.g. `*token*.txt`, `temp_token.txt`, `token.txt`). Treat any token that touched disk as compromised and let it expire / rotate the signing secret. Also review the untracked Python helper scripts (`verify_sig.py`, `decode_jwt.py`, `test_hash.py`, `check_len.py`, `test_token.py`, `test_gateway.py`, `decode_jwt.py`) — several embed the hardcoded secret as a literal (`verify_sig.py:17`, `decode_jwt.py:40`, `test_hash.py:3`, `check_len.py:1`) and should not be committed.

---

## HIGH

### EXPOSE-001 — Agent-gateway token-create endpoint is unauthenticated
- **Severity:** High
- **Confidence:** 90
- **CWE:** CWE-306 (Missing Authentication for Critical Function), CWE-269 (Improper Privilege Management)
- **File:** `kernel/agentgw/gateway.go:117` and handler `gateway.go:276`
- **Description:** `POST /v1/token/create` is mounted with `g.handleTokenCreate` directly — every other data route uses `g.withAuth(...)`, but this one does not. The handler accepts caller-supplied `caps`, `run_id`, rate, and expiry and returns a freshly minted, validly-signed token (`responseJSON(... {"token": token})`). There is no check that the caller already holds a (parent) token, nor any cap-subset enforcement against a parent.
- **Impact:** Any client that can reach the gateway socket can mint a token with arbitrary capabilities, with no credential at all. Normally the gateway listens on an abstract/Unix socket (local-only, which bounds this to local processes), but `Listen` explicitly supports `tcp://host:port` (gateway.go:95/141) and the socket is overridable via `AGEZT_AGENTGW_SOCKET` — a TCP binding turns this into a remote, unauthenticated token-minting oracle. Together with SECRET-001 the forgeability exists regardless; this endpoint removes even the need to know the secret.
- **Remediation:** Require authentication (a parent token) on `/v1/token/create` and enforce that requested caps are a subset of the parent's (the `CreateSubprocessToken` model already exists for this). If an unauthenticated bootstrap mint is genuinely required, scope it to the loopback/abstract socket only and refuse on TCP listeners.

### EXPOSE-002 — Gateway TCP-listen exposes hardcoded-secret auth surface beyond loopback
- **Severity:** High
- **Confidence:** 70
- **CWE:** CWE-668 (Exposure of Resource to Wrong Sphere)
- **File:** `kernel/agentgw/gateway.go:95,141`; socket source `kernel/runtime/runtime.go:745` (`AGEZT_AGENTGW_SOCKET`)
- **Description:** The gateway's listener supports `tcp://host:port`. The socket path is operator-settable via `AGEZT_AGENTGW_SOCKET`. With the SECRET-001 hardcoded signing secret and EXPOSE-001's unauthenticated mint, a TCP-bound gateway is fully exploitable from the network.
- **Impact:** Remote attackers on a reachable network can forge/mint gateway tokens and drive the eventbus/memory/log/config endpoints.
- **Remediation:** Fix SECRET-001 and EXPOSE-001 first. Additionally, refuse to bind the gateway to a non-loopback TCP address unless a non-default secret is explicitly configured, and warn loudly in the boot banner when TCP-bound.

---

## MEDIUM

### CRYPTO-001 — Machine-bound vault key derivation is single-SHA-256 (no stretching)
- **Severity:** Medium
- **Confidence:** 60
- **CWE:** CWE-916 (Use of Password Hash With Insufficient Computational Effort)
- **File:** `kernel/creds/machine.go:55-72` (`computeMachinePassphrase`)
- **Description:** The default at-rest vault passphrase (M934) is `"machine-v1:" + hex(SHA-256("agezt-vault-machine-v1|" + machineID + "|" + user))`. That passphrase is then fed through the proper 200k-iteration PBKDF2 in `encrypt.go`, so the *vault key* is stretched. The note here is design posture, not a stretching bug per se: the machine passphrase is fully determined by low-entropy, partly-discoverable inputs (Windows MachineGuid / `/etc/machine-id` / IOPlatformUUID + OS username). The code documents this honestly ("protects the FILE leaving the machine, not against local same-user malware"). The residual risk: an attacker who learns the machine identity (e.g. from another file, a logged MachineGuid, or registry read) can reconstruct the passphrase offline and the 200k PBKDF2 is the only barrier.
- **Impact:** A stolen `creds.json` plus knowledge of the host's machine-id and username is decryptable without a real secret. This is the documented, accepted threat model — flagged here as defense-in-depth, not a defect.
- **Remediation:** None required if the documented threat model is accepted. For higher assurance, encourage `AGEZT_VAULT_PASSPHRASE` (operator-managed, already takes precedence) for vaults that may leave the host, and avoid logging the raw machine identity anywhere.

### EXPOSE-003 — Verbose decrypt/envelope error strings on the vault path
- **Severity:** Medium
- **Confidence:** 40
- **CWE:** CWE-209 (Information Exposure Through an Error Message)
- **File:** `kernel/creds/encrypt.go:204-237`
- **Description:** Envelope-parse failures surface internal detail (`unsupported envelope schema %q`, `nonce length %d invalid`, `kdf_iter %d implausibly low`). These are operator-facing CLI/daemon errors, not remote API responses, and they never echo key material. `ErrWrongPassphrase` correctly collapses GCM-open failures to a single generic sentinel (good — no oracle on the tag). Low practical risk.
- **Impact:** Minor internal-structure disclosure to a local operator who already holds the file. Not a remote leak.
- **Remediation:** Acceptable as-is for a local CLI. Ensure these strings are never forwarded into an HTTP response body.

---

## LOW

### SECRET-003 — Default gateway secret duplicated as a string literal in gateway.go
- **Severity:** Low (sub-finding of SECRET-001, separate location)
- **Confidence:** 100
- **CWE:** CWE-798
- **File:** `kernel/agentgw/gateway.go:63`
- **Description:** `DefaultGatewayConfig` re-spells `"change-me-in-production"` as a raw literal instead of referencing `agentgw.DefaultTokenSecret`, so a future maintainer fixing the constant could miss this copy. Tracked separately so the remediation for SECRET-001 covers both sites.
- **Remediation:** When fixing SECRET-001, eliminate this literal (load from the same secret source).

---

## Verified correct / false positives (no action)

### VERIFIED-001 — PBKDF2-HMAC-SHA256 reimplementation is CORRECT
- **File:** `kernel/creds/encrypt.go:305-321` (`deriveKeyPBKDF2`)
- **Finding:** The stdlib-only PBKDF2 reimpl was validated against RFC published SHA-256 test vectors AND cross-checked against a from-scratch textbook multi-block PBKDF2. `U_1 = HMAC(P, salt || INT32BE(1))`, `U_j = HMAC(P, U_{j-1})`, `DK = U_1 ⊕ … ⊕ U_iter`. Results for c=1, c=2, c=4096, and c=200000 all match exactly:
  - c=1 → `120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b` (matches RFC vector)
  - c=2 → `ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43`
  - c=4096 / c=200000 → byte-identical to reference implementation.
  The XOR accumulation and `INT32BE(1)` block index are both correct. dkLen (32) == PRF output (32) so the single-block assumption is valid. This is a genuine PBKDF2-SHA256, not an approximation. AES-256-GCM with fresh 12-byte random nonce + 32-byte random salt per save; nonce length validated before `gcm.Open` to avoid the stdlib panic. The legacy HMAC-chain KDF (`deriveKeyLegacyHMAC`) is correctly retained decrypt-only. kdfCache keys on a SHA-256 of the passphrase (never the raw passphrase) — good. KDFIterMinAccepted floor (100000) blocks downgrade attacks on stolen vaults.

### VERIFIED-002 — Inbound webhook HMAC verification is correct (constant-time + replay guard)
- **File:** `plugins/channels/webhook/webhook.go:260-267`, dedup `:371-403`
- **Finding:** `verify` uses `hmac.Equal` (constant-time), fails closed on empty secret/signature, enforces a 5-minute freshness window on `ts_ms` (with integer-overflow-safe delta math), and de-dupes message ids via a two-generation bounded set so a captured signed body can't replay. Solid.

### VERIFIED-003 — webui session/login crypto is correct
- **File:** `kernel/webui/session.go`
- **Finding:** Session IDs from `crypto/rand` (32 bytes hex), password compared with `subtle.ConstantTimeCompare`, online-guess lockout (8 fails → 5 min), cookies `HttpOnly` + `SameSite=Strict` + `Secure` when TLS, sliding expiry. No issues.

### VERIFIED-004 — SHA-1 / math/rand usages are non-security (false positives)
- **Files:** `kernel/creds/sso.go:80` (AWS SSO cache filename — AWS SDK convention, not a security hash); `plugins/channels/sms/sms.go:293` (Twilio's mandated `HMAC-SHA1` signature scheme — required by the vendor, not a choice); `kernel/governor/governor.go:671` (`math/rand/v2` for retry jitter — non-security); `kernel/memory/vector.go:83` (`fnv` for embedding hashing — non-security). `kernel/ulid/ulid.go` uses `crypto/rand` for the 80-bit random suffix — correct, IDs are not predictable.

### Note — redaction chokepoint is wired and on-by-default
- `kernel/redact/redact.go` is comprehensive (provider key formats, JWT, bearer, PEM, connection-string passwords, AWS secret-key) and is installed on the bus before journaling (`kernel/bus/bus.go:109`, enabled at `cmd/agezt/main.go:802` unless `AGEZT_REDACT=off`). Tenant journals get the same redactor (main.go:1288). The hardcoded-secret-signed JWTs themselves WOULD be caught by the `jwt` pattern if they reached a journal payload — but redaction does not mitigate SECRET-001/SECRET-002, which concern the secret/tokens at rest in source and scratch files, outside the journal path.

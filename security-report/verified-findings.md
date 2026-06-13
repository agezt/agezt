# Verified Findings — AGEZT (security-check Phase 3 / Verification)

Deduped + confidence-scored consolidation of 6 hunter reports. Multi-agent consensus noted where independent hunters converged (raises confidence). Owner-policy false positives (default-allow posture, deliberately high-capability `code_exec`/`shell`) excluded.

Confidence legend: ⭐⭐⭐ code-confirmed by orchestrator / ≥3-agent consensus · ⭐⭐ single-agent, plausible · ⭐ needs runtime confirmation.

## CRITICAL

### C1 — Hardcoded HMAC token-signing secret (gateway) ⭐⭐⭐ (4-agent consensus + code-confirmed)
- **CWE-321 / CWE-798.** `kernel/agentgw/token.go:25` `DefaultTokenSecret="change-me-in-production"`; wired live via `gateway.go:63` (`DefaultGatewayConfig`) → `kernel/runtime/runtime.go:743` with **no env/vault override**; `cmd/agt/token.go:225` returns the same constant.
- **Impact:** the signing key is a public source constant. Anyone can forge a valid bearer token offline with arbitrary `RunID` + all capabilities → complete auth bypass of the agent gateway (memory read/write/delete, eventbus publish, config read). The machine-bound vault (M934) exists but never seeds this secret.
- **Fix:** derive `TokenSecret` from the encrypted vault (or a per-install random secret persisted at first boot); remove the constant; fail closed if unset in non-dev.

### C2 — Unauthenticated capability-minting endpoint ⭐⭐⭐ (4-agent consensus + code-confirmed)
- **CWE-306 / CWE-862 / CWE-269.** `kernel/agentgw/gateway.go:117` registers `POST /v1/token/create` **without `withAuth`** (the only data route that skips it). The handler returns a fully-signed token with caller-chosen `run_id` and `caps`, no caller authentication and no capability subsetting.
- **Impact:** independent of C1 — even with a strong secret, any process reaching the socket mints a max-capability token. Becomes a *remote* token oracle when the gateway is TCP-bound (`AGEZT_AGENTGW_SOCKET=tcp://…`, the documented Windows path). No audit record is produced.
- **Fix:** require an authenticated parent token; enforce that minted `caps` ⊆ parent caps; rate-limit and audit the route.

## HIGH

### H1 — Real JWTs committed-adjacent / hardcoded-secret leak in working tree ⭐⭐⭐
- **CWE-312 / CWE-538.** `token.txt`, `temp_token.txt` hold live HS256 tokens; untracked but **NOT** matched by `.gitignore` (only `.env*`/`creds.json`). Untracked Python helpers (`decode_jwt.py`, `verify_sig.py`, `test_token.py`, `test_hash.py`) embed the hardcoded secret literally. One `git add .` commits them.
- **Fix:** delete the artifacts; broaden `.gitignore` (`*.txt`, `*token*`, `temp_*`); rotate once C1 is fixed.

### H2 — `CreateSubprocessToken` does not enforce capability subset ⭐⭐
- **CWE-269.** `kernel/agentgw/token.go:127` copies requested `caps` verbatim into the child token with no check against `parent.Caps`. A narrow token can mint a superset.
- **Fix:** intersect requested caps with parent caps; reject escalation.

### H3 — Rate limiter self-disables after one window ⭐⭐⭐ (code-confirmed)
- **CWE-770.** `kernel/agentgw/types.go:155-172` `Allow()`: when `now-lastTick >= windowMs` it `return true` **without** updating `lastTick` or resetting the counter. `lastTick` is set once at construction, so ~60s after the first request every subsequent call takes the early-return branch and is allowed unconditionally — the per-token throttle vanishes permanently.
- **Fix:** on window roll-over, set `lastTick=now` and reset the counter (atomically); add a unit test asserting limiting persists across windows.

### H4 — Unbounded per-SubprocessID rate-limit map (DoS) ⭐⭐
- **CWE-400 / CWE-770.** `kernel/agentgw/gateway.go:220 allowRate` creates a `*RateLimit` per distinct `SubprocessID` and never evicts. With forgeable/mintable tokens (C1/C2) an attacker varies `SubprocessID` to exhaust memory.
- **Fix:** bound the map (LRU/TTL eviction).

### H5 — Self-update is its own integrity authority (update-channel RCE) ⭐⭐
- **CWE-494 / CWE-345.** `kernel/update/update.go` `validateSHA256` checks the downloaded binary against a SHA supplied by the *same* endpoint — no signature, no pinned key, no out-of-band anchor; path ends in `os.StartProcess`. Endpoint/DNS/TLS compromise → RCE as the daemon. Compounded by UPD-002 (one hand-followed redirect, no `https://` enforcement → downgrade).
- **Fix:** sign releases with an embedded public key; verify signature (not just hash) before swap; enforce HTTPS; reject cross-scheme redirects.

### H6 — CI self-hosted runners execute untrusted fork-PR code ⭐⭐
- **CWE-draft (CI/CD trust).** `.github/workflows/ci.yml:16-19` + all `runs-on: [self-hosted, Linux, X64]` run `npm ci`/`go test`/`cargo test`/shell on `pull_request` from forks → attacker code on the owner's persistent WSL dev host (cred theft, cache poisoning, WSL→Windows pivot).
- **Fix:** gate fork PRs behind `pull_request_target` review/approval, use ephemeral runners for untrusted PRs, or restrict CI to trusted branches.

### H7 — JWT `alg`/`typ` header never validated (alg-confusion latent) ⭐⭐
- **CWE-347.** `ValidateToken` (`token.go:82`) ignores the decoded header; it trusts the HMAC path unconditionally. Safe today (single verify path), but latent if asymmetric/`none` support is ever added.
- **Fix:** pin `alg=HS256`,`typ=JWT`; reject others.

## MEDIUM

- **M1 — Gateway audit logger is dead code ⭐⭐⭐ (code-confirmed).** CWE-778. `NewAuditLogger(nil)` (`gateway.go:73`); `writeEntries` no-ops on `a.j==nil`; no handler calls `Log`. Zero audit trail for capability access. Fix: wire the kernel journal + log every `withAuth` access.
- **M2 — Config write gated by the read capability ⭐⭐.** CWE-862. `config_handler.go:185` allows config *writes* with `config.access` (read) since no `config.write` cap exists. Fix: add and require `config.write`.
- **M3 — SSRF on outbound fetches that bypass netguard ⭐⭐.** CWE-918. `kernel/mcp/http.go` (remote MCP `DialHTTP`), `kernel/catalog/sync.go` + `discovery.go` (catalog/Ollama discovery) fetch config-supplied URLs without routing through `kernel/netguard`. Fix: funnel all server-side fetches through netguard (the guard itself is sound).
- **M4 — Wildcard CORS on agentgw SSE ⭐⭐.** CWE-942. Lone `Access-Control-Allow-Origin: *` site in the tree. Fix: scope origins; never pair `*` with credentials.
- **M5 — Gateway JSON decoders have no body-size cap ⭐⭐.** CWE-770. `handleTokenCreate` and peers decode unbounded bodies (`MaxHeaderBytes` only caps headers). Fix: `http.MaxBytesReader`.
- **M6 — Governor agent daily-budget TOCTOU over-spend ⭐⭐.** CWE-367. `kernel/governor/governor.go:540-561` reads spend under lock, compares to the ceiling outside any lock; concurrent runs for one agent overshoot. Fix: check-and-reserve under a single lock.
- **M7 — `credential_process` arg-splitter mis-handles escapes ⭐⭐.** CWE-88 (footgun, not injection — argv-form, env-opt-in + `~/.aws` write needed). `kernel/creds/aws.go splitCommandLine`. Fix: vetted shlex.
- **M8 — Self-update redirect allows http downgrade ⭐⭐.** CWE-319. (see H5).
- **M9 — Frontend renders untrusted `bookmarkUrl` href without `safeHref` ⭐⭐.** CWE-79. `javascript:` scheme possible. Fix: route through existing `safeHref` scheme allowlist.

## LOW
- INJ-002: sandbox `confineUnder` is string-prefix, no `EvalSymlinks` (CWE-59; operator-auth'd). 
- CICD-002: `ci.yml` lacks top-level `permissions:` scoping.
- CICD-003: `dtolnay/rust-toolchain@stable` unpinned (not full-SHA), beside a crates.io token.
- UPD-003: GitHub update source never sets SHA256 (feature broken / latent verify-bypass).
- UPD-004: stale update lockfile can wedge updates (availability).
- DEP-001: old transitive `klauspost/cpuid`. DEP-002: dual npm+pnpm lockfiles can diverge.

## VERIFIED-CORRECT (cleared — not findings)
- **PBKDF2 reimplementation** (`creds/encrypt.go`) — validated against RFC vectors (XOR accumulation + INT32BE(1)); genuine PBKDF2-SHA256, not an approximation. AES-256-GCM, per-save random salt+nonce, nonce-length pre-check, KDF-iter floor all sound.
- **Web console auth** (`webui/session.go`) — `crypto/rand` ids, `subtle.ConstantTimeCompare`, 8-attempt lockout, HttpOnly/SameSite=Strict/Secure cookie, sliding 12h session. Solid.
- **Inbound webhook HMAC** — constant-time, freshness window, replay dedup. Sound.
- **Redaction chokepoint** on-by-default; **plugin BLAKE3 hash-pinning** fail-closed before write/spawn; **lean-deps** real (2 Go modules, CI-gated); **no Docker/IaC/WebSocket** surface; **REST/OpenAI APIs** Bearer-only, no cookies/CORS.
- **code_exec / mcp-attach broad capability** — by owner policy (default-allow); not vulns. No sandbox-escape or secret-leak found in them.

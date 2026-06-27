# TypeScript / JavaScript Deep Language Scan — Results

> Scanner: `sc-lang-typescript`. Repo: `D:\Codebox\PROJECTS\AGEZT`.
> Scope: React 19 console (`frontend/src`, 211 tsx + 130 ts), TS SDK (`sdk/typescript`),
> loose JS, plus quick passes of the Python SDK (`sdk/python`) and Rust SDK (`sdk/rust`).
> Method: Discovery (grep sink patterns) → Verify (read hotspots). DOM-XSS coordinated with
> the client-side XSS scanner; this scanner covers language/ecosystem idioms.

## Severity counts

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 0 |
| Medium | 0 |
| Low | 2 |
| Informational | 3 |

**Headline: the TS/JS surface is clean of language-specific weakness classes.** No prototype
pollution, no `eval`/`new Function`/`setTimeout(string)`, no `dangerouslySetInnerHTML`, no
`child_process`/`exec` in JS tooling, no `rejectUnauthorized:false` / `verify=False`, no tokens
in `localStorage`, no committed secrets, no source maps shipped, and the one untrusted-link path
(agent-output markdown) is scheme-allowlisted. The frontend's risk is further bounded by being a
**same-origin, token-gated, strict-CSP, loopback-only** console (see architecture §5/§9).

---

## What was checked and found safe (verification notes)

### Prototype pollution — NONE
- No `__proto__` / `constructor.prototype` writes, no `deepMerge` / `_.merge` / `defaultsDeep`.
- Only `Object.assign` use: `frontend/src/components/FleetNowBar.tsx:224` — merges a locally-built
  `eventPhase(e)` object (own fixed keys) into a row, not untrusted JSON keys. Not pollutable.
- No lodash/deep-extend dependency in `frontend/package.json`.

### Dynamic code execution — NONE
- `eval` / `new Function` / `setTimeout("string")` appear ONLY in built vendor bundles
  (`kernel/webui/dist/assets/*.js`, normal for React/flow vendors), never in source.
- No dynamic `require(userInput)` / `import(userInput)` with attacker-controlled paths.

### React XSS idioms — NONE
- `dangerouslySetInnerHTML`: **zero occurrences** anywhere in `frontend/src`.
- Agent-output markdown renderer (`frontend/src/components/Markdown.tsx`) renders every leaf as
  plain React children (auto-escaped); there is no raw-HTML path.
- `javascript:` URL injection via links is **defended**: `frontend/src/lib/markdown.ts:38-40`
  `safeHref()` allowlists only `^(https?://|mailto:)` and returns "" otherwise; the renderer
  falls back to plain text for unsafe hrefs (`markdown.ts:67-69`). The same `safeHref` guards the
  Bookmarks data view (`frontend/src/views/Data.tsx:614-632`, with an explicit anti-XSS comment).
- All `target="_blank"` links carry `rel`: Markdown uses `rel="noopener noreferrer nofollow"`
  (`Markdown.tsx:150`); `window.open(...)` calls in `Channels.tsx:182` and `Models.tsx:327` pass
  `"noopener,noreferrer"`. Data.tsx:624 uses `rel="noreferrer"` (noreferrer implies noopener).

### Token / secret handling — SAFE
- The daemon's `?token=` is read once and kept **in memory only, never localStorage**
  (`frontend/src/lib/api.ts:1-21`). Sent as `Authorization: Bearer` for fetch; query-param
  fallback only for SSE/EventSource (which cannot set headers) — documented and unavoidable.
- All `localStorage`/`sessionStorage` writes are non-secret UI prefs (theme, accent, console name,
  voice/wake toggles, chat thread store). No tokens, JWTs, or credentials persisted.
- No `console.log`/`console.*` of tokens, secrets, passwords, or `authHeaders` in `frontend/src`.

### Untrusted JSON (SSE / events / API) — SAFE
- SSE handler (`frontend/src/lib/events.tsx:47-58`) wraps `JSON.parse(m.data)` in try/catch and
  drops malformed frames; parsed data flows into React (escaped) and a bounded `MAX_FEED` slice.
- TS SDK SSE parser (`sdk/typescript/src/client.ts:309-327`) and Agent-SDK SSE
  (`sdk/typescript/src/agent.ts:402-411`) both swallow invalid JSON (`{raw}` / skip).
- `agentrepair.ts:231-244` validates each field with `typeof` guards after parse (no blind cast).

### TS SDK (`sdk/typescript`) — SAFE
- `client.ts` / `agent.ts`: Bearer-token auth, `AbortController`/socket timeouts, **every** query
  param `encodeURIComponent`-escaped, no shell, no TLS-disable. Agent SDK talks over an abstract
  Unix socket to the capability-scoped gateway.

### Build / supply chain — SAFE
- `frontend/vite.config.ts`: `sourcemap:false` (no maps shipped to the embedded dist),
  `assetsInlineLimit:0` + no inline scripts (enables strict `script-src 'self'` CSP, no nonce).
  No `define`/`DefinePlugin` leaking env vars into the bundle.
- `frontend/package.json`: no `preinstall`/`postinstall`/`prepare` lifecycle scripts; pinned-ish
  caret ranges with committed `pnpm-lock.yaml`/`package-lock.json`; `overrides.undici ^7.28.0`
  pins a known-CVE transitive dep — good hygiene.

### Python SDK (`sdk/python`) — SAFE (quick pass)
- No `verify=False`, `shell=True`, `subprocess`, `os.system`, `eval`/`exec`, `yaml.load`,
  `pickle.loads`, or `ssl._create_unverified` / `CERT_NONE` in source (only doc-comment text).
- `client.py` uses stdlib `urllib` with default TLS verification.

### Rust SDK (`sdk/rust`) — SAFE (quick pass)
- `http.rs` propagates all network-read errors with `?`; the 4 `.unwrap()`/`.expect()` matches are
  in `#[cfg(test)]` code only. `json.rs` `.unwrap()`s are on internal serializer state, not
  untrusted-network parse results.
- No `unsafe` blocks. Plain-`http://`-only is a documented loopback design choice (see Low-002).

---

## Findings

### Finding: TS-001
- **Title:** Rust/Python/TS SDKs are HTTP-only with no TLS (transport confidentiality relies on a proxy)
- **Severity:** Low
- **Confidence:** 80
- **File:** `sdk/rust/src/http.rs:25-37` (explicitly rejects `https://`); TS `sdk/typescript/src/client.ts` and Python `sdk/python/agezt/client.py` also default to `http://`
- **Vulnerability Type:** CWE-319 (Cleartext Transmission)
- **Description:** All three SDK HTTP clients speak plain `http://`. The Rust client hard-rejects
  `https://` (std has no TLS). For the documented loopback daemon this is fine, but if an operator
  points an SDK at a remote daemon without a TLS-terminating proxy, the bearer token and request
  bodies travel in cleartext.
- **Remediation:** This is an intentional zero-dependency design tied to the loopback deployment
  model — keep, but ensure the SDK READMEs state prominently that any non-loopback use MUST front
  the daemon with an HTTPS reverse proxy. No code change required.
- **References:** CWE-319.

### Finding: TS-002
- **Title:** Broad `as any` / `as unknown as T` casts at a few data boundaries (defense-in-depth, not exploitable)
- **Severity:** Low
- **Confidence:** 55
- **File:** ~50 `as any`/`as unknown`/`@ts-ignore` occurrences across 20 files (e.g. `frontend/src/views/World.tsx`, `views/Dashboard.tsx`, `views/Toolbox.tsx`, `views/Mcp.tsx`); SDK `sdk/typescript/src/agent.ts:375,378` (`this.client as unknown as {...}`), `client.ts:226` (`ev.data as unknown as Mail`)
- **Vulnerability Type:** CWE-704 (Type Confusion) / CWE-20
- **Description:** Type assertions bypass compile-time checks on data crossing the API/SSE boundary.
  In this codebase the casts are UI-internal conveniences (event shaping, private-field access in
  the SDK) and the data sinks are React text (escaped) or already-runtime-guarded (`typeof` checks
  in `agentrepair.ts`), so no concrete unsafe flow was found. Flagged for hygiene only.
- **Remediation:** Prefer runtime narrowing (`typeof`/`in` guards, or a tiny validator) over `as`
  on freshly-parsed API/SSE payloads at the point they first enter typed code. No security fix
  required given current sinks; revisit if any cast ever feeds a privileged action.
- **References:** CWE-704; checklist SC-TS-269, SC-TS-272.

### Informational notes
- **INFO-1 (XSS inventory for the client scanner):** The single point where untrusted (agent/model)
  text becomes a clickable `href` is `frontend/src/components/Markdown.tsx:144-155` + Bookmarks
  `views/Data.tsx:624`; both are gated by `lib/markdown.ts:safeHref` (http/https/mailto allowlist).
  No `dangerouslySetInnerHTML` exists to hand off.
- **INFO-2 (localhost-console mitigation):** Residual frontend risk is bounded by same-origin +
  token/session auth + `default-src 'none'` / `script-src 'self'` CSP + loopback default bind
  (architecture §5/§9), so even a hypothetical injected string cannot load external script.
- **INFO-3 (no Node backend in TS):** There is no Express/Fastify/Next.js/ORM layer in TS — the
  backend is Go. So the Node-server checklist classes (middleware ordering, ReDoS-in-routes, raw
  SQL, CORS-in-Express, SSTI) do not apply to the TS surface; they belong to the Go scanner.

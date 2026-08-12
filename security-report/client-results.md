# Client-Side Security Assessment — AGEZT Console

**Scope:** `frontend/src/` (436 files, ~107k LOC, React 19.2 + TypeScript, Vite 8, go:embed-ed), `sdk/typescript/src/`, and the server-side header/CORS surface in `kernel/webui/`.
**Skills run:** `sc-lang-typescript`, `sc-xss`, `sc-clickjacking`, `sc-cors`
**Commit:** main @ `f815f56e` (supersedes the report from `99d2e426`)
**Date:** 2026-08-12
**Posture:** READ-ONLY. No source file was modified.

---

## Executive summary

**No XSS was found.** The console has no raw-HTML rendering path at all — the audit found zero occurrences of `dangerouslySetInnerHTML`, `innerHTML`, `outerHTML`, `insertAdjacentHTML`, or `document.write` in application source, and zero `eval` / `new Function` / string-`setTimeout`. Agent output and channel messages — the untrusted inputs that reach the console — are rendered through a hand-rolled Markdown AST whose every leaf becomes a React child, and Markdown link hrefs pass a `safeHref()` scheme allowlist. This is a genuinely well-hardened rendering path, not an accident.

**Clickjacking and CORS are both clean.** `X-Frame-Options: DENY` and CSP `frame-ancestors 'none'` are set on *every* response including 401s and public routes, and there are no `Access-Control-Allow-*` headers anywhere in the Go tree.

The console findings below are, in order of importance: one latent **supply-chain** exposure (Monaco loaded from a third-party CDN with no SRI, currently neutralized only by CSP), and four token-hygiene / defense-in-depth items.

**The highest-severity finding in this report is not in the console — it is SDK-001**, a verified abstract-vs-relative unix-socket mismatch between the Go gateway and the TypeScript SDK that allows a planted socket file to capture agent capability tokens. See the SDK section.

### Console findings

| ID | Title | Severity | Confidence |
|----|-------|----------|-----------|
| CLIENT-001 | Monaco editor loaded from third-party CDN without SRI | Medium | 95 |
| CLIENT-002 | Console bearer token persists in URL and browser history | Low | 95 |
| CLIENT-003 | Full-privilege console token still accepted in `/events` query string | Low | 90 |
| CLIENT-004 | "Ephemeral" SSE token is process-lifetime and never rotates | Low | 85 |
| CLIENT-005 | Daemon-supplied URL passed unvalidated to `window.open()` | Low | 70 |
| CLIENT-006 | CSP omits `blob:` — preview relies solely on CSP to contain blob-origin content | Informational | 90 |

---

## CLIENT-001 — Monaco editor loaded from a third-party CDN without Subresource Integrity

- **CWE:** CWE-829 (Inclusion of Functionality from Untrusted Control Sphere); related CWE-494 (Download of Code Without Integrity Check)
- **Severity:** Medium (latent — currently blocked by CSP; becomes Critical the moment CSP is relaxed)
- **Confidence:** 95
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\frontend\src\lib\monaco.ts:13` and `:22`
  - `D:\Codebox\PROJECTS\AGEZT\frontend\src\components\MonacoView.tsx:26-30`
  - Shipped in the embedded bundle: `D:\Codebox\PROJECTS\AGEZT\kernel\webui\dist\assets\Markdown-BBoM78vb.js`, `...\assets\vendor-BuaPXTA3.js`

The code editor used to render **every fenced code block an agent emits** is not bundled. It is fetched at runtime from a public CDN:

```ts
export const MONACO_CDN_BASE = `https://cdn.jsdelivr.net/npm/monaco-editor@${PINNED_MONACO_VERSION}/min/vs`;
...
loader.config({ paths: { vs: MONACO_CDN_BASE } });
```

`@monaco-editor/react`'s AMD loader injects a `<script src="…jsdelivr.net…/loader.js">` into the document. There is no Subresource Integrity hash, and the `loader.config({paths})` API provides no way to attach one.

**Exploit scenario.** A jsDelivr compromise, an npm-side package compromise republished to the CDN, a BGP/DNS hijack, or a TLS-intercepting middlebox on any non-loopback deployment yields arbitrary JavaScript execution **in the console's own origin**. That origin holds the console bearer token in module memory (`lib/api.ts`) and can reach every authenticated route — `/api/run`, the `code_exec` and `shell` tools, the file manager, provider key management. A single poisoned script therefore escalates from "editor asset" to full host compromise via the agent's own tool surface.

**Current mitigating factor — and why this is still worth fixing.** The shipped CSP is `script-src 'self'; connect-src 'self'` with no CDN host, so the browser **blocks this load today**. The editor silently degrades to the plain `<pre>` fallback in `MonacoView` (line 55-59), which means the failure is invisible — no user-facing error. The danger is precisely that: when someone eventually notices "code blocks don't syntax-highlight," the obvious fix is to add `https://cdn.jsdelivr.net` to `script-src` and `connect-src`, which silently converts this from a blocked load into a live third-party-script RCE path into the console origin.

**Secondary observation (version drift).** The app pins `0.52.2` in `lib/monaco.ts:11`, but the vendored `@monaco-editor/react` default in `assets/vendor-BuaPXTA3.js` is `monaco-editor@0.55.1`. If `ensureLoader()` has not run before an `<Editor/>` mounts, the library's own default CDN base is used instead of the pinned one — so the "pinned version is the only knob" comment does not hold in all mount orders.

**Remediation.** Self-host Monaco: vendor `monaco-editor/min/vs` into the Vite build output so it ships under `/assets/` and is covered by `script-src 'self'`. `lib/monaco.ts:5-6` already documents this as the intended path ("To self-host later, point `paths.vs` at the vendored `monaco-editor/min/vs` directory"). Do **not** resolve this by adding the CDN to CSP. If the ~3 MB binary-size cost is unacceptable, the correct alternative is a lighter self-hosted highlighter (e.g. Shiki/Prism), not a CDN allowance.

---

## CLIENT-002 — Console bearer token persists in the URL and browser history

- **CWE:** CWE-598 (Use of GET Request Method With Sensitive Query Strings)
- **Severity:** Low
- **Confidence:** 95
- **File:** `D:\Codebox\PROJECTS\AGEZT\frontend\src\lib\api.ts:10-11`

```ts
const TOKEN =
  typeof location !== "undefined" ? new URLSearchParams(location.search).get("token") || "" : "";
```

The token is read from `?token=` and correctly kept **in memory only** — it is never written to `localStorage` or `sessionStorage` (verified: the only storage keys are UI prefs — theme, accent hue, console name, chat threads, voice prefs, dismissed alerts). Fetches send it as an `Authorization: Bearer` header, not a query parameter. That part is right.

However, the SPA never removes the token from the address bar. There is no `history.replaceState` call anywhere in `App.tsx` or `main.tsx` that scrubs it (the only `replaceState` uses are `Board.tsx:463` and `Workboard.tsx:311,339`, which manage view hashes).

**Exploit scenario.** The full-privilege console token remains visible in the address bar and is written to the browser's on-disk history database. It survives into session restore, bookmarks, and any screenshot or screen-share of the console — a realistic exposure given this is an operator dashboard people demo and screen-record. Anyone with read access to the browser profile (a shared workstation, a backup, a synced-history account) recovers a credential that grants full agent control.

**Correctly handled already:** Referer leakage. `Referrer-Policy: no-referrer` is set on every response (`kernel\webui\webui.go:1315`), and outbound links additionally carry `rel="noreferrer"`, so the token never rides a `Referer` header to a third party.

**Remediation.** Immediately after reading `TOKEN`, strip it from the visible URL:
```ts
if (TOKEN) history.replaceState(null, "", location.pathname + location.hash);
```
This costs nothing — the value is already captured in the module-level constant — and closes the history/address-bar exposure while leaving the daemon-banner deep-link flow intact.

---

## CLIENT-003 — Full-privilege console token still accepted in the `/events` query string

- **CWE:** CWE-598
- **Severity:** Low
- **Confidence:** 90
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:1413-1416`

The codebase carries a documented fix ("VULN query-string-token") that introduced a separate ephemeral SSE token precisely so the main console token would stop appearing in URLs. The fallback branch re-opens it:

```go
// Fallback for programmatic / non-browser SSE clients that pass the
// main token in query (before the SPA was updated). Accept the main
// token in query ONLY for /events and ONLY as a transition aid.
return s.tokenMatch(r.URL.Query().Get("token"))
```

**Exploit scenario.** Any client — or any operator copy-pasting a URL — can still put the admin-tier console token in a `/events` query string, where it lands in proxy logs, shell history, and any intermediary that records request lines. The comment itself marks this as a transition aid, and the SPA has long since been updated (`lib/api.ts:44-46` uses `?st=`), so the compatibility rationale has expired.

**Remediation.** Delete the fallback. Programmatic SSE clients can use the `Authorization` header, which line 1410's `s.tokenPresentedFrom(r, false)` already accepts.

---

## CLIENT-004 — "Ephemeral" SSE token is process-lifetime and never rotates

- **CWE:** CWE-613 (Insufficient Session Expiration)
- **Severity:** Low
- **Confidence:** 85
- **Files:** `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:175-184`, `:1462-1468`; `D:\Codebox\PROJECTS\AGEZT\kernel\webui\session.go:167-177`

The token itself is cryptographically sound: 32 bytes from `crypto/rand`, hex-encoded, compared in constant time via `kernelauth.NewStaticVerifier`. The issue is lifetime and placement — it is minted **once in `New()`** and stored as an immutable struct field, so "ephemeral" is a misnomer: it is valid for the entire daemon process lifetime, has no TTL, and cannot be rotated without a restart. It is also the one credential that deliberately travels in a URL query string (`/events?st=…`), because `EventSource` cannot set headers.

**Exploit scenario.** The `/events` firehose carries agent outputs, tool invocation arguments, and run payloads. An attacker who recovers this token once — from a memory dump, a browser devtools network panel captured in a screen-share, or a debugging proxy — retains a read tap on everything the daemon does until the process is restarted, with no way for the operator to revoke it short of a restart.

**Mitigating factors.** No request logging that records query strings was found in `kernel/httpserver`; `Referrer-Policy: no-referrer` applies; and `EventSource` URLs do not enter browser history. The token is also scoped — `sseTokenMatch` is only consulted when `r.URL.Path == "/events"`, so it cannot be replayed against `/api/*` routes.

**Remediation.** Give the SSE token a TTL (minutes) and re-mint it on each `/api/sse-token` call, keying a small set of live tokens rather than one immortal value. The frontend already re-fetches on failure (`lib/api.ts:33` clears `sseTokenPromise` to allow retry), so a rotating token needs little client change.

---

## CLIENT-005 — Daemon-supplied URL passed unvalidated to `window.open()`

- **CWE:** CWE-601 (Open Redirect) / CWE-79
- **Severity:** Low
- **Confidence:** 70
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\frontend\src\views\Channels.tsx:211`
  - `D:\Codebox\PROJECTS\AGEZT\frontend\src\views\Models.tsx:362`
  - `D:\Codebox\PROJECTS\AGEZT\frontend\src\views\Setup.tsx:223`

```ts
window.open(r.authorize_url, "_blank", "noopener,noreferrer");
```

`authorize_url` comes from the control plane (`kernel\controlplane\channel_oauth.go:172`, `kernel\controlplane\provider_oauth.go:94`) and is opened with no scheme validation.

**Why this is separate from the JSX cases.** React 19 *errors* on `javascript:` URLs in `href`/`src` attributes, which covers the other daemon-data link sites (`ACPAgents.tsx:165`, `Channels.tsx:309,743`, `Research.tsx:223`, `VoiceSetup.tsx:479`, `QuickConnect.tsx:299`). `window.open()` is a raw browser API and receives **no such protection** — in Chromium, `window.open("javascript:…")` executes in the opener's origin.

**Exploit scenario.** Requires an attacker able to influence the control plane's OAuth response (a malicious channel preset, a tampered catalog entry, or a compromised control-plane path). Given that precondition, the payload runs in the console origin and reaches the in-memory bearer token. The preconditions make this defense-in-depth rather than a directly reachable bug — hence Low with 70 confidence — but the fix is free.

**Remediation.** The codebase already has the right primitive and uses it correctly elsewhere (`Data.tsx:662` guards stored bookmark URLs with it). Apply it here:
```ts
const url = safeHref(r.authorize_url);
if (!url) throw new Error("provider returned a non-navigable authorize URL");
window.open(url, "_blank", "noopener,noreferrer");
```
Consider a lint rule banning bare `window.open(` on non-literal arguments.

---

## CLIENT-006 — CSP omits `blob:`; artifact preview relies solely on CSP to contain blob-origin content

- **CWE:** CWE-1021 (defense-in-depth observation)
- **Severity:** Informational
- **Confidence:** 90
- **Files:** `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:1316-1325`; `D:\Codebox\PROJECTS\AGEZT\frontend\src\views\Files.tsx:152,170-171`

`BlobArtifact` fetches artifact bytes and renders them via `URL.createObjectURL(blob)` into an `<iframe src={href}>` (PDF) or `<img src={href}>`. The CSP is `img-src 'self' data:` (no `blob:`) with no `frame-src`/`child-src`, so iframes fall through to `default-src 'none'`. **Both are therefore blocked today**, and artifact preview is silently non-functional.

This is recorded not as a vulnerability but because the containment reasoning matters for whoever fixes it: `blob:` URLs **inherit the creating document's origin**. If a blob-backed `<iframe>` were permitted and the blob's type were ever HTML, previewing an attacker-supplied artifact would be stored XSS in the console origin. Two controls currently prevent that, and both must be preserved: the CSP block above, and `safeContentType()` in `kernel\webui\artifact_route.go:65-76`, which coerces any non-allowlisted stored mime to `application/octet-stream`.

**Remediation guidance.** The correct narrow fix is `img-src 'self' data: blob:` plus an explicit `frame-src 'self' blob:` — *not* widening `script-src`. Keep `safeContentType()`'s allowlist and the SVG `sandbox` CSP (`artifact_route.go:50`) exactly as they are; they are the second layer.

**Related, benign:** the OAuth result page (`kernel\webui\webui.go:914`) contains an inline `<script>setTimeout(...window.close()...)</script>` that `script-src 'self'` also blocks. The page's auto-close does not work; nothing else is affected. Its `msg` interpolation *is* correctly escaped via `htmlEscape` (line 908/918) into element-text context.

---

## Verified clean — with evidence

### XSS (sc-xss) — no findings

- **No raw-HTML sinks exist.** Repo-wide search of `frontend/src` for `dangerouslySetInnerHTML|innerHTML|outerHTML|insertAdjacentHTML|document.write` returns exactly two hits, both test assertions checking a component renders nothing (`Chat.context.test.tsx:63`, `HelpDrawer.test.tsx:15`). There is no unsafe rendering path to exploit.
- **No dynamic code execution.** Zero matches for `eval(`, `new Function`, or string-form `setTimeout`/`setInterval`. Confirmed in the *built* bundle too: `grep "new Function("` across all `kernel/webui/dist/assets/*.js` returns nothing.
- **Agent/channel output rendering is safe by construction.** `frontend\src\lib\markdown.ts` is a dependency-free parser emitting a typed block/inline AST; `frontend\src\components\Markdown.tsx` renders every leaf as a React child (auto-escaped). Fenced code goes to Monaco/`<pre>` as a `value` prop; ` ```json ` fences go to `DataView`, which contains no `href`, `src`, `window.*`, or `document.*` references at all.
- **Markdown link hrefs are allowlisted.** `safeHref` (`lib\markdown.ts:42-44`) admits only `^(https?://|mailto:)` after trimming, anchored — `javascript:`, `data:`, and `vbscript:` all return `""`, and the caller renders the raw token as literal text (`markdown.ts:108`). Covered by tests at `markdown.test.ts:43-45`.
- **`postMessage` is not used.** Zero occurrences of `postMessage` or a `"message"` event listener, so the origin-bypass class does not apply.
- **React Flow / `@xyflow/react` canvas** renders React node components; it introduces no HTML-injection sink (covered by the repo-wide sink search above).
- **`FileMention`** only builds a same-origin blob download with `a.download` set — no navigation sink.
- **Stored-SVG XSS is handled server-side.** `artifact_route.go:43-51` applies a `sandbox` CSP to `image/svg+xml` responses, correctly reasoning that `nosniff` cannot help when the type is genuinely SVG.

### Clickjacking (sc-clickjacking) — no findings

Both layers are present and applied universally:

```go
h.Set("X-Frame-Options", "DENY")
... "frame-ancestors 'none'"
```
`kernel\webui\webui.go:1314`, `:1325`.

Critically, `Handler()` (line 872) returns `s.secure(s.routeRegistry().ServeHTTP)` — the header middleware wraps the **entire** registry, so headers are set before auth and therefore also cover 401 responses, the public asset routes, `/hooks/`, and `/oauth/callback`. This is the correct placement; a common failure mode is wrapping only authenticated routes. Asserted by `webui_test.go:1363-1365`.

### CORS (sc-cors) — no findings

There are **no `Access-Control-Allow-*` headers anywhere in the Go tree.** A case-insensitive repo-wide search across all `*.go` returns a single hit, and it is a comment documenting their deliberate absence (`kernel\agentgw\handlers.go:56`). With no CORS headers, the browser same-origin policy applies unmodified: no wildcard-with-credentials, no origin reflection, no `null` origin, no regex bypass.

Supporting controls, all correct:
- `sameOriginMutation` (`webui.go:1345-1362`) rejects `Sec-Fetch-Site: cross-site` and requires an exact host:port match on the `Origin` header for state-changing methods (empty `Origin` is allowed for non-browser CLI clients, which is appropriate since those must still present a Bearer token).
- `hostAllowed` (`webui.go:1328-1343`) enforces a Host allowlist, blocking DNS-rebinding.
- Session cookie is `HttpOnly` + `SameSite=Strict` + `Secure` (proxy-aware via `X-Forwarded-Proto`) — `session.go:228-236`.

### CSP strength — verified compatible, no `unsafe-inline`/`unsafe-eval` needed for scripts

```
default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline';
connect-src 'self'; img-src 'self' data:; font-src 'self' data:;
base-uri 'none'; form-action 'none'; frame-ancestors 'none'
```

Confirmed against the shipped artifact, not just the source: `kernel\webui\dist\index.html` contains **zero inline scripts** (only external hashed `/assets/*.js` module tags), and no bundle chunk contains `new Function(`. `default-src 'none'` with explicit `base-uri 'none'` and `form-action 'none'` is a notably strict baseline. `style-src 'unsafe-inline'` is genuinely required by React Flow/Radix runtime transforms and enables no code execution.

The only thing in the tree that *would* require loosening `script-src` is CLIENT-001.

### TypeScript-specific patterns (sc-lang-typescript) — no findings in frontend

- **Prototype pollution:** no recursive merge, no `Object.assign` into user-keyed maps, no `__proto__` access. `JSON.parse` results are read field-wise, never spread into a merge.
- **Insecure randomness:** `Math.random()` appears at `lib\chatStore.tsx:46`, `lib\conductorStore.ts:107`, `lib\councilStore.ts:122` — all generating **local UI element keys**, not security tokens. `conductorStore`/`councilStore` already prefer `crypto.randomUUID()` when available. Not a finding.
- **Sensitive client storage:** no tokens or credentials in `localStorage`/`sessionStorage`; only UI preferences (theme, accent, console name, chat threads, voice prefs, dismissed alert ids).
- **Dynamic import:** the only non-literal-adjacent `import()` is the static-specifier lazy load in `MonacoView.tsx:28`; no user-controlled module paths.

---

## TypeScript SDK (`sdk/typescript/src/`)

Files reviewed in full: `client.ts` (327 lines), `agent.ts` (605), `errors.ts` (50), `index.ts` (38), plus tests, examples, `package.json`, and `package-lock.json`.

| ID | Title | Severity | Confidence |
|----|-------|----------|-----------|
| SDK-001 | Default gateway socket path is CWD-relative in Node — capability-token theft | High | 85 |
| SDK-002 | Bearer token sent over plaintext HTTP; no scheme validation | Medium | 80 |
| SDK-003 | API token is an enumerable own property — leaks via object logging | Medium | 70 |
| SDK-004 | Unbounded SSE buffering; request timeout disarmed before body is read | Medium | 75 |
| SDK-005 | Unvalidated `limit` interpolated raw into query string | Low | 55 |
| SDK-006 | Redirects followed by default on authenticated requests | Low | 45 |
| SDK-007 | Response body echoed verbatim into exception messages | Low | 45 |
| SDK-008 | `Content-Length` computed from a separate serialization of the body | Low | 35 |

### SDK-001 — Default gateway socket path is CWD-relative in Node (verified)

- **CWE:** CWE-426 (Untrusted Search Path); CWE-522 (Insufficiently Protected Credentials)
- **Severity:** High · **Confidence:** 85 (independently verified against the Go server)
- **Files:** `D:\Codebox\PROJECTS\AGEZT\sdk\typescript\src\agent.ts:43` and `:198`; server side `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\gateway.go:187`

```ts
export const DEFAULT_SOCKET_PATH = "@agezt/agentgw.sock";
```

There is a **runtime mismatch between the two ends of this socket**, confirmed by reading both:

- The Go gateway explicitly branches on the leading `@` and binds a Linux **abstract-namespace** socket (`gateway.go:187`, with the comment "Go maps the leading @ to the abstract namespace on Linux").
- Node/libuv performs **no such mapping** — it reaches abstract sockets only via a literal `\0` prefix. The string is passed verbatim to `http.request({ socketPath })` at `agent.ts:198`, so libuv copies `"@agezt/agentgw.sock"` into `sun_path` as an ordinary **relative** path, resolved against the agent subprocess's current working directory.

There is no path normalization, no absolute-path requirement, and no `startsWith('/')` validation anywhere.

**Exploit scenario.** An agent subprocess runs with CWD set to a workspace, repo, or temp directory that the agent itself — or untrusted content it processes — can write to. An attacker creates `./@agezt/agentgw.sock` there and listens on it. Every `AgentClient` request then delivers `Authorization: Bearer <scoped JWT capability token>` (`agent.ts:202`) to the attacker's listener, who can (a) replay that token against the real gateway for the full granted capability set, and (b) return forged `memory.search` / `config.get` / `agent.list` responses — a direct prompt-injection channel into the agent's reasoning.

**Accuracy note on reachability.** Because the relative path normally does not exist, the SDK *fails closed* (ENOENT) rather than silently misconnecting. That is what keeps this from being Critical. The attack is precisely that **planting the file turns a non-working path into a working one** — the attacker supplies the endpoint the SDK was never able to reach legitimately, so there is no "correct" connection to compete with.

**Cross-reference.** The same constant appears at `D:\Codebox\PROJECTS\AGEZT\sdk\python\agezt\agent.py:45`; Python's `socket.connect()` also does not map `@`, so the Python SDK very likely shares this defect. Worth confirming in a follow-up (outside this run's scope).

**Remediation.** Require an absolute socket path, or prefix with `\0` for Node's abstract-namespace form; reject relative paths outright with a clear error.

**Contextual note (server-side, outside SDK scope).** Abstract-namespace unix sockets carry **no filesystem permissions** — any process in the same network namespace may connect to `@agezt/agentgw.sock`. The gateway's authentication therefore rests entirely on JWT secrecy, which makes SDK-001 and SDK-003 more consequential than they would otherwise be.

### SDK-002 — Bearer token transmitted over plaintext HTTP; no scheme validation

- **CWE:** CWE-319 · **Severity:** Medium · **Confidence:** 80
- **File:** `D:\Codebox\PROJECTS\AGEZT\sdk\typescript\src\client.ts:93-98`, `:247`

The constructor's only processing of `baseUrl` is trailing-slash trimming (`baseUrl.replace(/\/+$/, "")`). There is no check that the scheme is `https:`, no confinement of plaintext to loopback, and no runtime warning. Every documented example uses `http://` (`index.ts:11`, `client.ts:79`, `README.md`), making plaintext the path of least resistance.

**Exploit scenario.** A user follows the documented pattern against a non-loopback daemon (`new Client("http://agezt.internal:8800", token)`). Any on-path observer — shared LAN, cloud VPC with mirrored traffic, corporate proxy, compromised sidecar — reads the long-lived API token in cleartext and gains full daemon API access (`/api/v1/runs` executes arbitrary agent intents).

**Remediation.** Reject non-`https:` `baseUrl` unless the host is loopback, or require an explicit `allowInsecure: true` opt-in.

### SDK-003 — API token held as an enumerable own property

- **CWE:** CWE-532 · **Severity:** Medium · **Confidence:** 70
- **Files:** `client.ts:89`, `agent.ts:150` (`private readonly token: string;`)

TypeScript's `private` is compile-time only. At runtime both `token` fields are enumerable own properties; neither class defines `toJSON()`, a `[util.inspect.custom]` hook, nor uses an ECMAScript `#private` field. `console.log(client)`, `util.inspect(client)`, or any structured logger that serializes context objects (pino, winston, Sentry breadcrumbs) prints the raw bearer token.

Amplified in `agent.ts`: each handle keeps a back-reference (`constructor(private client: AgentClient)` at lines 271, 336, 447, 471, 505), so logging any handle — `console.log(client.memory)` — walks back to `AgentClient { token: 'eyJ...' }`. The value is a live capability JWT read from `process.env.AGEZT_AGENT_TOKEN` in the documented pattern.

**Remediation.** Convert to a `#token` ECMAScript private field, or add a redacting `[util.inspect.custom]` / `toJSON`.

### SDK-004 — Unbounded SSE buffering; timeout disarmed before body is read

- **CWE:** CWE-400 · **Severity:** Medium · **Confidence:** 75
- **Files:** `client.ts:245-249`, `:278`, `:283`; `agent.ts:389-418`

```ts
const timer = setTimeout(() => ac.abort(), this.timeoutMs);
return fetch(...).finally(() => clearTimeout(timer));
```

`.finally()` runs when the fetch promise settles — i.e. **when response headers arrive** — so the abort timer is cleared before the body is consumed, and nothing bounds body reading afterward. The documented 30 s `timeoutMs` therefore protects only the handshake.

Combined with `client.ts:278` (`let buf = ""`) and `:283` (`buf += decoder.decode(...)`), which grow with no size cap until a `\n\n` frame separator appears, a malicious or compromised daemon gets two attacks: (a) send headers then trickle body bytes forever — `await res.json()` (`client.ts:235`) and the `runStream`/`mailboxWatch` generators hang indefinitely, exhausting sockets; (b) stream megabytes containing no frame separator until `buf` OOMs the process.

`EventbusHandle.subscribe` has the parallel problem: `buffer` (line 394) is unbounded, the `ReadableStream` (line 389) enqueues every event without ever pausing `res`, and **no `timeout` is set on the request options at all** (lines 372-380) — so the `req.on("timeout")` handler at line 424 is dead code that can never fire.

**Remediation.** Cap the accumulation buffer and move `clearTimeout` to after body consumption, or apply a separate body-read deadline.

### SDK-005 — Unvalidated `limit` interpolated raw into the query string

- **CWE:** CWE-20 · **Severity:** Low · **Confidence:** 55
- **File:** `agent.ts:312-315`

```ts
`/v1/memory/search?q=${encodeURIComponent(query)}&limit=${limit}`
```

`query` is correctly encoded; `limit` is interpolated with no encoding, numeric coercion, or range guard. A JavaScript caller (no compile-time checking) or a value arriving from JSON config injects extra query parameters — `search("x", "20&tenant=other&all=true")` — reaching gateway parameters the SDK deliberately does not expose. Node rejects control characters in the path, so this is parameter injection only, not request smuggling. For contrast, `client.ts:180` is incidentally protected by an `if (limit > 0)` numeric guard; `agent.ts:314` has none.

### SDK-006 — Redirects followed by default on authenticated requests

- **CWE:** CWE-601 · **Severity:** Low · **Confidence:** 45
- **File:** `client.ts:247`

No `redirect: "manual"`, so the default `"follow"` applies. A malicious or compromised daemon can 302 any SDK request to an arbitrary host, and the SDK issues it from the caller's network position — useful for probing internal services from inside a trust boundary. Node 18+/undici strips `Authorization` on cross-origin redirects, which is what keeps this Low; that mitigation does not apply to same-origin redirects.

### SDK-007 — Response body echoed verbatim into exception messages

- **CWE:** CWE-209 · **Severity:** Low · **Confidence:** 45
- **Files:** `agent.ts:233`, `:235`, `:561`

The full raw response body becomes the error message and is baked into `super(\`[${code}] ${message}\`)` (`agent.ts:81`), so it appears in every stack trace and log line. `ConfigHandle.get` compounds this at `agent.ts:561` with `String(err)`. Since `ConfigHandle` is the API that retrieves secret-rated values, a gateway error response echoing request context lands in a caller-side exception string that is routinely logged.

### SDK-008 — `Content-Length` from a separate serialization

- **CWE:** CWE-444 (theoretical) · **Severity:** Low · **Confidence:** 35
- **File:** `agent.ts:209` vs `:262`

`Buffer.byteLength(JSON.stringify(body))` sets `Content-Length`; line 262 calls `req.write(JSON.stringify(body))` — a second, independent serialization. A getter or `toJSON()` whose output differs between calls makes declared and written length diverge on a keep-alive connection. Node's `OutgoingMessage` validates and errors on mismatch, hence Low — but the pattern is fragile and serializing once is free.

### SDK note — security-control observability

`ConfigHandle.get` (`agent.ts:555-562`) catches and re-throws `ConfigAccessError`, but `AgentClient.request` only ever rejects with `AgentError` and never constructs a `ConfigAccessError`. Every access denial (403 secret-rated key, denied HITL approval, missing `config.access` capability) therefore reaches the caller as `ConfigAccessError(key, "INTERNAL_ERROR", ...)`. Callers following the documented pattern at `agent.ts:526-528` and `:536-540` branch on `err.code` and cannot distinguish "denied by policy" from "gateway broke" — so a policy denial may be retried as a transient fault. Flagged because it degrades the observability of a security control.

### SDK — verified clean

- **SSRF / URL construction:** `client.ts:247` uses string concatenation, *not* `new URL(path, base)`, so the absolute-path-replaces-host escape does not exist. All caller-supplied path segments pass `encodeURIComponent` (`client.ts:130`, `:170`, `:179`; `agent.ts:550`), neutralizing `../`, `//evil.com`, and absolute-URL override. Query strings use `URLSearchParams` (`client.ts:158`, `:187`, `:213`). The one `new URL()` (`agent.ts:370`) is built on a hardcoded `http://localhost` base with only `pathname + search` extracted.
- **Header injection (CRLF):** no header value is assembled from server-controlled data; header names are all literals. Both undici and Node's `http` validate and throw on invalid header characters.
- **Prototype pollution:** no deep merge, no `Object.assign`, no spread over server JSON, no `obj[key] = value` with a controlled key. `JSON.parse` results are type-asserted and returned, never merged.
- **Insecure randomness:** no `Math.random`; no client-side ID/nonce/token generation at all — all identifiers originate server-side.
- **`eval` / `new Function` / dynamic import:** zero occurrences. Only two static literal ESM specifiers (`node:http`, `./errors.js`).
- **TLS:** no `rejectUnauthorized`, no `NODE_TLS_REJECT_UNAUTHORIZED`, no custom agent or `checkServerIdentity` override — verification is never weakened. (Absence of TLS *enforcement* is SDK-002, a separate matter.)
- **ReDoS:** six regexes, none with nested quantifiers; those applied to server-controlled SSE data are anchored and bounded (`/^(\r?\n){1,2}/`, `/\r$/`, `/^ /`, `/\r?\n/`).
- **Credential handling (mechanics):** the token is a constructor argument, never read from a global, and is transmitted **only** in the `Authorization` header — never in a URL, query string, or body, and never in a `console.log`/`process.stdout` call. It is never interpolated into an exception message. Residual exposure is SDK-002 and SDK-003 only.
- **`package.json`:** **no lifecycle scripts** — no `preinstall`/`install`/`postinstall`/`prepare`/`prepublish`/`prepublishOnly`/`prepack`; only `build` and `test`. The **zero-runtime-dependency claim is confirmed**: no `dependencies`, `peerDependencies`, `optionalDependencies`, or `bundledDependencies`. `package-lock.json` resolves exactly three packages, all `"dev": true` — `typescript@6.0.3`, `@types/node@26.0.1`, transitive `undici-types@8.3.0` — all with integrity hashes.

---

## Recommended priority

**Console (frontend + kernel/webui)**

1. **CLIENT-001** — self-host Monaco. This is the only console finding that can become critical, and it will become critical the day someone "fixes" the editor by editing the CSP. Consider a CI guard asserting `script-src 'self'` carries no host allowances.
2. **CLIENT-002** — one line (`history.replaceState`) removes the token from the address bar and on-disk history.
3. **CLIENT-003** — delete the expired `/events` query-token fallback.
4. **CLIENT-005** — route the three `window.open` sites through the existing `safeHref`.
5. **CLIENT-004** — give the SSE token a TTL when convenient.
6. **CLIENT-006** — when fixing artifact preview, add `blob:` to `img-src`/`frame-src` only; never widen `script-src`.

**SDK**

1. **SDK-001** — highest-severity finding in this report. Require an absolute socket path or the `\0` abstract-namespace form; reject relative paths. Check the Python SDK for the same defect.
2. **SDK-003** — `#token` private field or a redacting inspect hook.
3. **SDK-002** — reject non-`https:` non-loopback `baseUrl` without an explicit opt-in.
4. **SDK-004** — cap the SSE buffer; move `clearTimeout` past body consumption.
5. **SDK-005** — coerce and range-check `limit`.

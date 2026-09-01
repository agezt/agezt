# AGEZT — Client-Side Domain Results (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` (`main`)
**Skills applied:** `sc-xss`, `sc-csrf`, `sc-cors`, `sc-clickjacking`, `sc-websocket`
**Method:** source-code audit only. No daemon started, no browser driven, `.dev-home/` never read.

## Headline

The two sharpest classes for a localhost-bound console — **CSRF and DNS rebinding** — both came
back **defended**, and by three independent layers each. **CORS does not exist anywhere in the
repository** (not a misconfiguration: zero `Access-Control-Allow-*` headers, verified repo-wide).
**No raw-HTML render path exists anywhere in the SPA**, and every server-rendered byte that could
carry agent content is either `application/octet-stream` + `nosniff`, an allowlisted MIME, or
escaped. **No WebSocket server exists.**

The findings below are therefore all **Medium and lower**, and the most consequential one is not
an exploit but a **divergence**: the console ships a strict CSP that the shipped SPA does not
comply with, in four separate places, under a code comment asserting that it does.

---

## Findings

### CLI-001 — CSP declares a policy the shipped SPA violates in four places; the comment asserting compliance is false

- **Severity:** Medium · **Base confidence:** 92 · **CWE-1021 / CWE-693** (Protection Mechanism Failure)
- **File:** `kernel/webui/webui.go:1303-1310` (the claim) vs `frontend/src/lib/monaco.ts:13`, `frontend/src/lib/tts.ts:60-61`, `frontend/src/views/Files.tsx:170-171`, `frontend/src/views/Artifacts.tsx:513`

The policy, set on every console response (`kernel/webui/webui.go:1316-1325`):

```
default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline';
connect-src 'self'; img-src 'self' data:; font-src 'self' data:;
base-uri 'none'; form-action 'none'; frame-ancestors 'none'
```

The comment above it (`webui.go:1303-1305`) states:

> "the SPA loads only external, **same-origin** hashed JS/CSS, so `script-src 'self'` admits the
> genuine bundle and refuses any inline/injected script"

That is not true of the shipped SPA. `frontend/src/lib/monaco.ts:13` builds a cross-origin script
source and `:22` installs it as the loader path:

```ts
export const MONACO_CDN_BASE = `https://cdn.jsdelivr.net/npm/monaco-editor@${PINNED_MONACO_VERSION}/min/vs`;
...
loader.config({ paths: { vs: MONACO_CDN_BASE } });
```

`frontend/src/components/MonacoView.tsx:21-24` documents the intent explicitly ("The Monaco bundle
(~3 MB) is loaded from the pinned jsdelivr CDN… We deliberately do NOT bundle the editor").

Four SPA behaviours have no matching CSP allowance:

| SPA behaviour | Source | Directive that governs it | Allowed? |
|---|---|---|---|
| Monaco loaded from `cdn.jsdelivr.net` (every fenced code block, `Markdown.tsx:123`; the Files editor) | `lib/monaco.ts:13,22` | `script-src 'self'` / `connect-src 'self'` | **no** |
| `<iframe>` for PDF preview and HTML artifacts | `Files.tsx:170`, `Artifacts.tsx:513` | `frame-src` → falls back to `default-src 'none'` | **no** |
| `<img src={blob:…}>` previews from `URL.createObjectURL` | `Files.tsx:171-152` | `img-src 'self' data:` (no `blob:`) | **no** |
| `new Audio(URL.createObjectURL(blob))` — Voice Mode TTS playback | `lib/tts.ts:60-61` | `media-src` → falls back to `default-src 'none'` | **no** |

A fifth, server-side: the inline `<script>setTimeout(function(){window.close()},1500)</script>`
that `oauthResultPage` emits (`kernel/webui/webui.go:914`) is blocked by `script-src 'self'`, so
the OAuth result page never self-closes.

**Exploitation path.** This is not directly exploitable; it is a *pressure* finding. An operator
or maintainer who reports "the code editor never loads / PDF preview is blank / voice playback is
silent" gets a natural fix: add `https://cdn.jsdelivr.net` to `script-src`, `blob:` to
`img-src`/`media-src`, and `frame-src blob:`. That single edit puts a third-party CDN into the
script origin that holds the console bearer token and the `agezt_web_session` cookie, and
simultaneously un-blocks the unsandboxed iframe in CLI-004. The CSP is the SPA's only backstop
against a future raw-HTML path, and it has demonstrably never been exercised against the running
app.

**Why not a false positive.** The divergence between `webui.go:1303-1305` ("same-origin") and
`lib/monaco.ts:13` (`https://cdn.jsdelivr.net`) is verifiable from source alone and is not in
dispute. The header is applied unconditionally to every response, including the SPA document, by
`setSecurityHeaders(w)` at the top of `secure()` (`webui.go:1262`), which wraps the entire route
registry (`webui.go:871-872`) — there is no path that skips it.

**Honest scope limit.** The four "feature is blocked" rows are derived from the CSP specification,
not from running the app. I did not start the daemon or drive a browser (out of scope). The
*divergence* is source-verified; the *runtime symptoms* are predicted.

**Remediation.** Pick one and make code and comment agree: (a) vendor Monaco into the bundle —
`lib/monaco.ts:5-6` already documents this path ("To self-host later, point `paths.vs` at the
vendored `monaco-editor/min/vs` directory") — and add `frame-src blob:`, `img-src … blob:`,
`media-src blob:`, `worker-src blob:` for the legitimate same-origin blob usage; or (b) drop the
features. Do **not** add a CDN host to `script-src`. Add a test that asserts the CSP admits every
resource origin the bundle actually requests.

---

### CLI-002 — `RouteOpts.Mutation` is inert; the origin gate keys off HTTP method, so the one state-changing GET route escapes it

- **Severity:** Low · **Base confidence:** 90 · **CWE-352** (CSRF) / CWE-1123 (inert control)
- **File:** `kernel/httpserver/router.go:126` + `:142-144`, `kernel/webui/webui.go:1345-1349`, `:864-867`

`RouteOpts.Mutation` is recorded into the `Route` snapshot (`router.go:126`) and never read by the
dispatcher — `ServeHTTP` is `rt.mux.ServeHTTP(w, r)` and nothing else (`router.go:142-144`). Outside
tests, the only consumers of `.Mutation` are `restapi_test.go`, `webui_test.go`,
`openaiapi_test.go` and `httpserver_test.go`. It is inspectable metadata, not enforcement.

The actual CSRF gate keys off the **method**, not the flag (`webui.go:1345-1349`):

```go
func sameOriginMutation(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
```

`/oauth/callback` is the single route in the tree where "mutating" and "POST" diverge — it is
registered `publicRead` (GET, `TierPublic`) with `oauthCallback.Mutation = true`
(`webui.go:864-867`) — and it genuinely changes state: `handleOAuthCallback` forwards to
`CmdChannelOAuthCallback`, which exchanges the authorization code and stores the resulting channel
token (`webui.go:892-897`). So the one route the flag marks as needing protection is exactly the
one the gate waves through, unauthenticated.

**Exploitation path.** An attacker page issues `<img src="http://127.0.0.1:8787/oauth/callback?code=ATTACKER_CODE&state=…">`
to bind the operator's AGEZT channel to an attacker-controlled account (authorization-code
injection). This is **bounded to Low** because `state` must match a server-minted unguessable value
and the `hostAllowed` check (CLI verified safe, below) still rejects an attacker DNS name — the
request only survives if the victim navigates by IP literal or `localhost`.

**Why not a false positive.** I confirmed `Mutation` has no non-test reader by grepping the whole
tree, and confirmed the route is both GET and state-changing by reading the handler through to the
control-plane call.

**Remediation.** Make the router enforce what it records: apply the origin check when
`opts.Mutation` is true regardless of method, rather than re-deriving mutation-ness from the verb
in `sameOriginMutation`. Alternatively drop the field so it stops reading as a control.

---

### CLI-003 — The second browser-facing HTML surface (`127.0.0.1:1455`) sets none of the console's security headers and performs no Host validation

- **Severity:** Low · **Base confidence:** 95 · **CWE-1021** (missing frame protection) / CWE-350 (Host header trust)
- **File:** `kernel/controlplane/provider_oauth.go:72-74`, `:237-238`

The provider OAuth listener builds a bare mux and server with no middleware:

```go
mux := http.NewServeMux()
mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) { s.providerCallback(w, r, login) })
login.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
```

and `providerLoginPage` sets exactly one header (`provider_oauth.go:238`):

```go
w.Header().Set("Content-Type", "text/html; charset=utf-8")
```

Everything `kernel/webui` applies through `secure()` (`webui.go:1260-1273`) is absent here: no CSP,
no `X-Frame-Options`, no `X-Content-Type-Options`, no `Referrer-Policy`, and — the one that
matters most — **no `hostAllowed` check**, so this listener has no DNS-rebinding defence at all
while the console does.

**Exploitation path.** During the `providerLoginTTL` window a page at `rebind.attacker.com`
(rebound to `127.0.0.1`) can reach `http://rebind.attacker.com:1455/auth/callback` same-origin and
read the response. The recoverable value is low: `providerCallback` reflects only server-generated,
escaped text (`provider_oauth.go:241` → `htmlEscapeProv`), and the `error` branch at `:102-104`
deliberately does **not** interpolate the attacker-supplied `error` param into the page. Completing
a code injection still requires `login.state`. The page is also framable, but carries no controls.

**Why not a false positive.** The absence is structural — this surface does not use
`kernel/httpserver.Router` or `webui.secure()`, so it inherits nothing. Verified by reading the
server construction and the only handler.

**Remediation.** Route this listener through the same header + Host middleware as the console, or
at minimum set `Content-Security-Policy: default-src 'none'` (note: it would then need
`script-src 'unsafe-inline'` or a refactor, since its inline `<script>` at `:247` is load-bearing
here, unlike its console twin) and `X-Frame-Options: DENY`, and reject unknown `Host` values.

---

### CLI-004 — PDF preview iframe omits `sandbox`, unlike its hardened sibling

- **Severity:** Low · **Base confidence:** 85 · **CWE-1021**
- **File:** `frontend/src/views/Files.tsx:170`

```tsx
if (kind === "pdf") return <iframe src={href} title={title || entry.name || "pdf"} className={className} />;
```

No `sandbox`, no `referrerPolicy`. Its sibling renderer for agent-authored HTML gets both, with an
explicit rationale (`frontend/src/views/Artifacts.tsx:513-516`, comment at `:454-461`):

```tsx
<iframe srcDoc={text} sandbox="" referrerPolicy="no-referrer" …
```

**I could not construct an exploit, and I am recording that plainly.** I tried to reach this
iframe with an active document type and failed on two independent controls:

1. `categoryOf` (`Files.tsx:95-105`) tests `svg` at `:99` and `html` at `:101` **before** `pdf` at
   `:102`, so an entry whose mime or extension is SVG/HTML is routed to `<img>` or to the sandboxed
   `srcDoc` iframe, never here.
2. The blob's type comes from the server's `Content-Type`, which `safeContentType`
   (`kernel/webui/artifact_route.go:65-76`) restricts to a fixed allowlist derived from the *same*
   `e.mime` the category was computed from (`Files.tsx:114` passes it as `?mime=`). The only types
   that can reach `kind="pdf"` are `application/pdf` and `application/octet-stream`.

Additionally, the console CSP has no `frame-src`, so `default-src 'none'` blocks this iframe from
loading at all today (see CLI-001).

**Why it is still worth filing.** It is a defense-in-depth gap that becomes live the moment
CLI-001 is "fixed" by adding `frame-src blob:` — at which point the only thing standing between an
agent-authored artifact and this frame is the ordering of three `if` statements in `categoryOf`.

**Remediation.** `sandbox="allow-same-origin"` is not needed for a blob PDF; add
`sandbox="" referrerPolicy="no-referrer"` to match `Artifacts.tsx:513-516`.

---

### CLI-005 — `sameOriginMutation` treats an absent `Origin` header as same-origin (informational)

- **Severity:** Info · **Base confidence:** 95 · **CWE-352**
- **File:** `kernel/webui/webui.go:1353-1356`

```go
origin := strings.TrimSpace(r.Header.Get("Origin"))
if origin == "" {
	return true
}
```

Recorded for completeness because comments and docs present this function as a boundary. It is not
one against a non-browser client — but a non-browser client must present the bearer token anyway,
and every browser sends `Origin` on a cross-origin POST, so there is **no browser-reachable CSRF
here**. The `Sec-Fetch-Site: cross-site` check at `:1350-1352` and the `SameSite=Strict` cookie are
each independently sufficient. No action required; do not "fix" this by rejecting absent `Origin`
without checking the CLI test suite and non-browser API callers first.

---

## Browser-facing header matrix

Console (`kernel/webui`) headers are applied by `setSecurityHeaders` (`webui.go:1311-1326`) inside
`secure()` (`webui.go:1260-1273`), which wraps the **entire** route registry (`webui.go:871-872`) —
so they cover public routes and 401 responses too.

| Control | Set? | Value / mechanism | Where |
|---|---|---|---|
| `Content-Security-Policy` | ✅ (console) | `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'` — **app does not comply, CLI-001** | `webui.go:1316-1325` |
| `Content-Security-Policy` (SVG artifacts) | ✅ | `sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:` | `artifact_route.go:50` |
| `Content-Security-Policy` (OAuth page :1455) | ❌ | none | `provider_oauth.go:237-238` — **CLI-003** |
| `X-Frame-Options` | ✅ (console) | `DENY` | `webui.go:1314` |
| `frame-ancestors` | ✅ (console) | `'none'` | `webui.go:1325` |
| `X-Frame-Options` (:1455) | ❌ | none | **CLI-003** |
| `X-Content-Type-Options` | ✅ | `nosniff` (global) + re-set on `/api/files/raw` | `webui.go:1313`, `files_route.go:383` |
| `Referrer-Policy` | ✅ (console) | `no-referrer` | `webui.go:1315` |
| **CORS** (`Access-Control-Allow-*`) | ❌ **none anywhere** | zero occurrences repo-wide (verified) — `connect-src 'self'` is the substitute | — |
| `Strict-Transport-Security` | ❌ none anywhere | acceptable: default bind is plaintext loopback; matters only behind an operator TLS proxy, where the proxy should set it | — |
| Content-Type — JSON APIs | ✅ | `application/json` | `webui.go:1760-1761` |
| Content-Type — file manager raw | ✅ | **always** `application/octet-stream` + `nosniff` | `files_route.go:381-383` |
| Content-Type — artifact raw | ✅ | allowlist of 8 types, else `application/octet-stream` | `artifact_route.go:41`, `:65-76` |
| Content-Type — static assets | ✅ | explicit by extension (deliberately not `mime.TypeByExtension`) | `webui.go:1490-1507` |
| Content-Type — TTS | ⚠ backend-supplied `mime`, unvalidated | `w.Header().Set("Content-Type", mime)` | `tts.go:55` — POST-only + token-gated + global `nosniff`; not browser-navigable. No finding. |
| Cookie `HttpOnly` | ✅ | `HttpOnly: true` | `session.go:239` |
| Cookie `SameSite` | ✅ | `SameSiteStrictMode` | `session.go:240` |
| Cookie `Secure` | ✅ conditional | `r.TLS != nil` OR `X-Forwarded-Proto: https` / `X-Forwarded-Ssl: on` | `session.go:241`, `:261-275` |
| Cookie `Path` / expiry | ✅ | `/`, `MaxAge = 12h`, sliding server-side TTL | `session.go:238`, `:242`, `:33-34` |
| Host allowlist (anti-rebinding) | ✅ | exact-match map; unknown DNS names rejected | `webui.go:1328-1343`, `:139-163` |
| Origin check on mutations | ✅ | `Sec-Fetch-Site` + `Origin` vs `Host` | `webui.go:1345-1362` |

---

## Verified safe

Each item below is a check that came back clean, with the evidence that makes it a conclusion
rather than an absence of results.

**No raw-HTML render path exists anywhere in the console SPA.** A tree-wide search of
`frontend/src` for `dangerouslySetInnerHTML`, `innerHTML`, `outerHTML`, `insertAdjacentHTML`,
`document.write`, `new Function`, `eval(`, `setTimeout`/`setInterval` with a string first argument,
`Function(`, ref-based DOM injection, and `createElement("script")` returns **two hits, both test
assertions** (`Chat.context.test.tsx:63`, `HelpDrawer.test.tsx:15`, each
`expect(container.innerHTML).toBe("")`). Recon's claim is independently confirmed.

**Agent output is rendered through React text nodes only.** `components/Markdown.tsx` is a
hand-rolled AST renderer (`:11-94`, leaves at `:147-180`); every leaf is `{tok.v}` inside JSX. The
comment at `:8-10` is accurate.

**Markdown link `href` is scheme-allowlisted — my initial lead here was wrong and I killed it.**
`Markdown.tsx:166` renders `href={tok.href}` with no local validation, but `tok.href` is already
filtered at construction: `lib/markdown.ts:106-108` runs every `[text](href)` match through
`safeHref` (`:42`), `/^(https?:\/\/|mailto:)/i`, and falls back to rendering the span as literal
text when it fails. Regression-tested at `lib/markdown.test.ts:43-45`
(`safeHref("javascript:alert(1)") === ""`).

**The other unguarded `href`/`window.open` sinks are not attacker-reachable.**
`Research.tsx:223` (`href={s.url}`) renders research sources, which looked like the strongest
candidate — but the server filters them at construction: `kernel/runtime/research.go:411` admits a
hit only when its URL has an `http://` or `https://` prefix, so `javascript:` cannot reach the
report. `ACPAgents.tsx:165`, `Channels.tsx:309`/`:743`, `QuickConnect.tsx:299` bind
catalog-constant doc URLs. The three `window.open(r.authorize_url, "_blank", "noopener,noreferrer")`
sites (`Channels.tsx:211`, `Models.tsx:362`, `Setup.tsx:223`) are server-minted OAuth URLs;
`javascript:` there is blocked both by modern browsers and by `script-src 'self'`. Hardening these
with the existing `safeHref` would be cheap and correct, but none is a live vulnerability.

**`dompurify` is not an unused sanitizer — recon's framing should be corrected.** It appears in
`frontend/package.json:50-53` under **`overrides`**, alongside `undici`, not under `dependencies`.
That is a transitive-dependency version pin (the standard shape of a CVE floor), not a direct
import someone forgot to wire up. It is correctly unreferenced by app code, because the app has no
raw-HTML path for it to guard. There is no evidence of a removed or planned raw-HTML path.

**CSRF is defended by three independent layers.** The write surface is the sharpest possible shape
for CSRF — `postAction` (`lib/api.ts:140-146`) issues `fetch(url, {method:"POST", headers:
authHeaders()})` with no body and no `Content-Type`, i.e. a *simple* request that an attacker can
reproduce with a plain cross-origin `<form>` and no preflight. It still fails:
1. **Auth does not ride.** The console token travels as `Authorization: Bearer` (`api.ts:48-52`),
   which browsers never attach cross-origin. The session cookie is `SameSite=Strict`
   (`session.go:240`), so it is not sent on any cross-site request, including top-level navigation.
2. **`Sec-Fetch-Site: cross-site` → 403** (`webui.go:1350-1352`).
3. **`Origin` host:port vs `r.Host` mismatch → 403** (`webui.go:1361`).
Layer 3 also closes the same-site/different-port case (`http://127.0.0.1:9000` → `:8787`), where
`SameSite` and `Sec-Fetch-Site` would both permit the request. All three run in `secure()` before
the router, so they cover public routes and 401s.

**DNS rebinding is blocked — this is the single most important control on this surface and it is
correctly implemented.** `hostAllowed` (`webui.go:1328-1343`) passes `localhost` and IP literals,
and for any DNS name consults an **exact-match** map populated only by explicit
`SetAllowedHosts` calls (`webui.go:139-163`). `rebind.attacker.com` is not in the map → 403 before
any handler runs. The match is a map lookup, not a prefix or suffix comparison, so
`evil-127.0.0.1.attacker.com` and `127.0.0.1.attacker.com` do not satisfy it. The IP-literal
passthrough at `:1337-1339` is not a rebinding hole: a rebound name stays a name in the `Host`
header.

**Clickjacking is closed on the console.** `X-Frame-Options: DENY` (`webui.go:1314`) *and* CSP
`frame-ancestors 'none'` (`webui.go:1325`), set before the auth check so even 401 responses carry
them. Pinned by `webui_test.go:1364`. Only gap is the separate `:1455` listener (CLI-003).

**CORS is absent, not misconfigured.** Zero occurrences of `Access-Control-Allow-Origin` or
`Access-Control-Allow-Credentials` in the entire repository. No origin reflection, no `*`, no
credentialed wildcard, no regex matching to bypass.

**No WebSocket server exists.** No `Upgrader`, no `CheckOrigin`, no upgrade handler anywhere. The
sole `github.com/coder/websocket` use is an **outbound client** in
`plugins/channels/nostr/nostr.go:158` (`websocket.Dial`). `sc-websocket` has no server-side surface
to assess here. One doc nit: `kernel/agentgw/types.go:4` advertises "scoped HTTP/WebSocket" — the
gateway registers HTTP routes only.

**SSE is same-origin-restricted and separately credentialed.** `/events` is `protectedRead`
(`webui.go:791`) and accepts an **ephemeral SSE-only token** in `?st=`
(`webui.go:1409-1417`, minted at `session.go:179-185`) specifically so the main console token never
enters a URL. `connect-src 'self'` confines `EventSource` to the daemon, and `Referrer-Policy:
no-referrer` keeps any query token out of `Referer`.

**Stored-XSS via agent-authored files is closed at the server.** `/api/files/raw` serves **every**
file as `application/octet-stream` with `X-Content-Type-Options: nosniff`
(`files_route.go:381-383`) — an agent writing `evil.html` or `evil.js` into the workspace root
cannot get it interpreted, which also forecloses the `script-src 'self'` bypass of loading a
same-origin agent-authored `.js`. `/api/artifact/raw` allowlists the MIME
(`artifact_route.go:65-76`) and additionally sandboxes SVG with its own CSP (`:43-51`), because SVG
is an active document — the reasoning in that comment is correct.

**Server-rendered HTML is escaped.** The only two `text/html` responses in the Go tree are
`oauthResultPage` (`webui.go:904-916`) and its twin `providerLoginPage`
(`provider_oauth.go:237-249`). Both interpolate only `title` (a compile-time constant) and `detail`,
and `detail` is passed through `htmlEscape`/`htmlEscapeProv` (`webui.go:918-921`,
`provider_oauth.go:251-254`) on every attacker-influenceable path — including the reflected
`?error=` query parameter at `webui.go:883`. The replacer omits `'`, which is harmless here because
both interpolation points are element text content, not attribute values. There is no
`text/template` or `html/template` anywhere in the tree.

**No secrets ship to the browser.** No `import.meta.env`, no `VITE_*`, no `process.env` in
`frontend/src`. The console token is read once from `location.search` and held **in memory only**
(`lib/api.ts:10-11`); `localStorage` holds UI preferences exclusively (theme, accent hue, wake-word
flag, dismissed alerts, conversation list, advanced-mode toggle) — no credential in any of the ~20
call sites checked.

**No `postMessage` handlers.** No `postMessage` sender or `addEventListener("message", …)`
receiver anywhere in `frontend/src`, so there is no missing-origin-check class here.

---

## Out of lane (noted, not filed)

`POST /hooks/<workflow>` accepts its per-workflow secret from `?secret=` as well as
`X-Agezt-Secret`, which puts a credential in access logs and proxy history. Browser-CSRF against it
*is* blocked (`Sec-Fetch-Site` + `Origin`, same as every other mutation), and
`Referrer-Policy: no-referrer` keeps it out of `Referer`. The residual risk is server/ops-domain
(credential-in-URL), so it belongs to another Phase 2 hunter, not this one.

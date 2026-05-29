# Phase Report — Milestone 1.x (browser.read tool)

> Status: **shipped** · Date: 2026-05-29
> Per DECISIONS A-TOOL (operator-visible tool set) and the M1.a
> "browser tool" deferral.
> Continues [PHASE-M1.w-REPORT.md](PHASE-M1.w-REPORT.md).

## Scope

Until M1.x, the agent's network reach was the `http` tool, which
returns raw response bodies. Real-world use cases — "summarise
this blog post," "find the API rate limit on the docs page,"
"check the GitHub release notes" — meant the agent paid hundreds
of tokens per page parsing through `<head>` boilerplate, `<script>`
blobs, and CSS in markup it shouldn't have to read.

M1.x ships `browser.read`: HTTP fetch + stdlib HTML→text
extraction. The agent gets readable prose, scoped to a host
allowlist, capped at a configurable character count.

```
browser.read(url=https://example.com/post)
→ {"url":"...", "status":200, "text":"Article body. With paragraphs.\n\nNext para.", "raw_bytes":24576, "text_chars":1843}
```

**Deliberately not a headless browser.** No Chrome, no Playwright,
no JavaScript execution. Server-rendered pages work cleanly;
single-page apps return a near-empty shell with a `<noscript>`
hint the agent can detect. A real `--render` mode that shells
out to operator-installed Chrome is a v2 conversation.

| Concern | Status |
|---|---|
| Stdlib-only (no `golang.org/x/net/html`, no chromedriver) | ✅ |
| `browser.read(url, max_chars?)` tool definition | ✅ tested |
| Default-deny host allowlist (matches `http` tool pattern) | ✅ tested |
| Wildcard `*.example.com` subdomain matching | ✅ tested |
| http/https only (file://, ftp://, javascript: refused) | ✅ tested |
| Non-2xx → tool error (model doesn't quote error pages) | ✅ tested |
| Configurable per-call + per-tool `max_chars` truncation | ✅ tested |
| Truncation marker in text + `truncated_text:true` in metadata | ✅ tested |
| Hard 4MB raw-download cap (safety net for hostile servers) | ✅ |
| Browser-like User-Agent (reduces WAF false positives) | ✅ tested |
| Strips `<script>`, `<style>`, `<noscript>` (well-formed AND unclosed) | ✅ tested |
| Strips HTML comments and doctype | ✅ tested |
| Decodes named + numeric + hex entities (`&amp;`, `&#x1F600;`) | ✅ tested |
| Preserves paragraph structure via block-tag → newline | ✅ tested |
| Collapses runs of whitespace | ✅ tested |
| Wired into daemon's tool registry as `browser.read` | ✅ |
| Daemon flags: `AGEZT_BROWSER_ALLOWED_HOSTS` + `AGEZT_BROWSER_ALLOW_ALL` | ✅ |
| Daemon emits warning when `BROWSER_ALLOW_ALL=1` (matches `http` pattern) | ✅ |

## Changes

### 1. `plugins/tools/browser/browser.go` — new file (~220 LoC)

The tool implementation. Defaults:

- `DefaultTimeout = 30s` (matches the http tool)
- `DefaultMaxChars = 64 * 1024` (most pages fit)
- `MaxFetchBytes = 4 * 1024 * 1024` (hard cap on raw download)
- `UserAgent = "Mozilla/5.0 (compatible; agezt-browser/0.1)"`

Three design choices worth recording:

**Why a separate tool from `http`.** Two reasons:

1. **Different output shape.** The http tool returns raw bodies +
   headers; the agent has to do the HTML parsing itself, paying
   per-token. browser.read returns extracted text + metadata; the
   agent reads the prose directly.
2. **Different allowlist semantics.** Operators routinely want to
   let the agent *read* a wide variety of public docs sites
   (`docs.python.org`, `developer.mozilla.org`, etc.) but POST
   only to a tightly-scoped internal API set. Folding both into
   one allowlist forces the wider-of-the-two; separating lets
   each be appropriately tight.

**Why a Mozilla-like User-Agent.** A few WAFs and CDN edge rules
treat any UA without `Mozilla/` as a bot and block. Setting a
browser-like UA pre-empts most of those without claiming to be
a real Chrome (the `agezt-browser` suffix makes the actual
identity discoverable from server logs).

**Why surface non-2xx as a tool error.** Returning the error page
as content would let the agent quote a 404 HTML body as if it
were the requested article. Treating it as a tool error gives the
agent a clean "the page didn't exist" signal it can react to.

### 2. `plugins/tools/browser/htmltext.go` — new file (~110 LoC)

Stdlib-only HTML→plain-text. Five passes:

1. **Strip noise blocks** (`<script>`, `<style>`, `<noscript>`):
   one regex per tag (RE2 has no backreferences, so we can't do
   `</\1>` — one pattern per tag is the trade-off).
2. **Strip unclosed remainder** of those same tags. Handles
   truncated downloads where a `<script>` block opened but never
   closed — without this we'd leak script source into the text.
3. **Strip comments + doctype + XML PIs**.
4. **Replace tags** with whitespace. Block-level tags
   (`<p>`/`<br>`/`<li>`/`<h*>`/...) become newlines so paragraph
   structure survives; everything else becomes a space.
5. **Decode entities** via stdlib `html.UnescapeString`.
6. **Normalise whitespace** — collapse horizontal runs, trim
   per-line, collapse vertical runs to at most two newlines.

Trade-offs honestly documented in the file: tables read as walls
of text; link URLs are dropped (text only); image alt-text is
dropped. These rarely block the "read this article" use case.

### 3. `cmd/agezt/main.go` — daemon registration

```go
out["browser.read"] = br
if br.AllowAll {
    registered = append(registered, "browser.read(allow_all=true)")
} else {
    registered = append(registered, fmt.Sprintf("browser.read(hosts=%d)", len(br.AllowedHosts)))
}
```

`AGEZT_BROWSER_ALLOWED_HOSTS` (comma-separated, supports
`*.example.com` wildcards), `AGEZT_BROWSER_ALLOW_ALL=1` for the
trust-everyone-on-purpose case (warning to stderr). Mirrors the
http tool's pattern exactly so the operator's mental model
transfers.

### 4. `plugins/tools/browser/browser_test.go` — 17 tests

| Test | Coverage |
|---|---|
| `TestHTMLToText_StripsScriptStyleNoscript` | All three noise blocks gone |
| `TestHTMLToText_DecodesEntities` | `&amp;` `&lt;` `&#x1F600;` `&copy;` all decoded |
| `TestHTMLToText_PreservesParagraphStructure` | Multiple paragraphs land on separate lines |
| `TestHTMLToText_StripsComments` | `<!-- ... -->` gone, real content preserved |
| `TestHTMLToText_HandlesUnclosedScript` | Truncated `<script>` doesn't leak source |
| `TestHTMLToText_CollapsesWhitespace` | Multi-space + multi-newline normalised |
| `TestHTMLToText_HandlesEmptyAndPlainText` | "" → ""; plain text unchanged |
| `TestHTMLToText_StripsDoctype` | `<!DOCTYPE html>` gone |
| `TestInvoke_FetchesAndExtracts` | End-to-end via httptest: GET → extract → metadata JSON |
| `TestInvoke_DefaultDenyForUnallowedHost` | No allowlist → host-denied |
| `TestInvoke_RejectsNonHTTPScheme` | `file://`, `javascript:` refused |
| `TestInvoke_RejectsMissingURL` | Empty input → clear error |
| `TestInvoke_Surfaces4xxAsToolError` | 404 → tool error, not content |
| `TestInvoke_TruncatesLongContent` | `MaxChars=500` → text capped + flag set |
| `TestInvoke_PerCallMaxCharsCapsBelowToolDefault` | Per-call `max_chars=100` overrides tool's larger default |
| `TestDefinition_NameAndSchema` | Tool name is `browser.read`; schema includes required url |
| `TestInvoke_WildcardHostMatch` | `*.example.com` matches subdomain, not bare; matches http tool semantics |

## Test summary

```
go test ./plugins/tools/browser/ -v -count=1
(17 tests — all PASS)

go test ./... -count=1
(all packages PASS)
```

Total suite: **515 passing** (from 498 after M1.w). +17 from
M1.x.

## Operator workflow examples

**Default deny — agent gets a clear error before any network call:**

```
# Operator hasn't set AGEZT_BROWSER_ALLOWED_HOSTS:
agent: browser.read(url=https://docs.python.org/3/)
tool: browser: host not in allowlist: docs.python.org
agent: I can't read that page — the operator needs to add docs.python.org to AGEZT_BROWSER_ALLOWED_HOSTS.
```

**Enable docs sites and let the agent research:**

```
export AGEZT_BROWSER_ALLOWED_HOSTS="docs.python.org,docs.anthropic.com,*.readthedocs.io,developer.mozilla.org"
agezt &  # restart with the new env

agt run "what's the syntax for asyncio.gather with return_exceptions in Python 3.12?"
# agent uses browser.read against docs.python.org, returns the answer
```

**Read a GitHub release notes page** (subdomain wildcard handles
the `*.github.com` variants):

```
export AGEZT_BROWSER_ALLOWED_HOSTS="github.com,*.github.com,*.githubusercontent.com"
agt run "summarise the breaking changes in the latest Go release"
```

**Local development — read-everything mode (DANGEROUS in prod):**

```
export AGEZT_BROWSER_ALLOW_ALL=1
agezt &
# stderr: WARNING: AGEZT_BROWSER_ALLOW_ALL=1 disables the browser host allowlist.
```

## What's intentionally NOT in M1.x

- **JavaScript rendering.** Single-page apps return a near-empty
  shell. The agent can detect this (text < 200 chars or contains
  `<noscript>` hint) and fall back to a different source. Future
  v2: opt-in `--render` mode that shells out to operator-installed
  `chrome --headless --dump-dom` when present.
- **Form submission / POST.** browser.read is GET-only. The
  existing `http` tool handles POST for the "submit a form to an
  API" case. A future `browser.submit` could wrap form parsing
  + multipart encoding; out of scope for v1.
- **Cookies / session state.** Each call is a one-shot request.
  No cookie jar across calls; no login flow. Operators who need
  authenticated reads pass cookies via `http` tool's POST path
  to the auth endpoint, then can't carry the session into
  `browser.read`. v2 could add an opt-in `browser.session` that
  keeps a jar per-correlation.
- **Link extraction as `[text](url)`.** Discussed in `htmltext.go`.
  Adds value but doubles the parser size; future.
- **Table reconstruction.** Adds value but a real parser
  (golang.org/x/net/html or a much larger state machine) is
  effectively required for it; future.
- **Screenshot capture.** Requires a headless browser binary.
  Could be a separate `browser.screenshot` tool that's only
  registered when the operator opts in by pointing
  `AGEZT_BROWSER_CHROME` at a chrome binary.
- **Search-engine front-end.** `browser.search(query)` that hits
  a configured search API (SerpAPI, Brave Search, Google CSE) and
  returns top-N results. Different shape entirely (search-result
  list, not page text); separate tool.

## Tool catalog (after M1.x)

| Tool | Purpose | Allowlist |
|---|---|---|
| `shell` | Execute commands (warden-isolated) | Warden profile + edict policy |
| `file` | Read/write/list within workspace | Filesystem root (`AGEZT_WORKSPACE`) |
| `http` | GET/POST any URL | `AGEZT_HTTP_ALLOWED_HOSTS` |
| **`browser.read`** | **Fetch a web page as readable text** | **`AGEZT_BROWSER_ALLOWED_HOSTS`** |

Four operator-visible tools, each with its own default-deny
allowlist, each wired through warden where applicable. The
agent's reach is fully operator-controlled.

## Files touched

- [plugins/tools/browser/browser.go](../plugins/tools/browser/browser.go) — new (~220 LoC).
- [plugins/tools/browser/htmltext.go](../plugins/tools/browser/htmltext.go) — new (~110 LoC).
- [plugins/tools/browser/browser_test.go](../plugins/tools/browser/browser_test.go) — new (~330 LoC, 17 tests).
- [cmd/agezt/main.go](../cmd/agezt/main.go) — import + 18-line registration block + env-var handling.

Zero changes to the kernel, the bus, the agent loop, the
scheduler, the planner, or any provider. The tool wedge stays
clean.

## Deferrals after M1.x

- **JS rendering / browser.submit / cookies / screenshots / search**
  — discussed above.
- **Out-of-process plugin host** (M1.y) — next pickup. Lets
  third parties ship tool plugins without recompiling agezt.
  Architecturally the biggest remaining chunk.
- **OS-keychain integration / passphrase rotation / argon2**
  (M1.w deferrals).
- **Planner v2** (re-planning, sub-planners, planner tools).
- **Pulse v2** (historical replay, TUI, dropped-events synthetic).
- **AWS credential-provider chain** (M1.m.x.x).
- **Non-Anthropic body shapes on Bedrock** (M1.m.y).

Picking up **out-of-process plugin host** next — the last
architectural chunk in the "M1 / agentic OS substrate" wedge.
After that, the project is at the "operator-tunable, deeply-tested,
externally-extensible agentic substrate" milestone the v1 vision
called for.

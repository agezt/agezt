# AGEZT — TypeScript/JavaScript Language Security Scan (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT`
**Commit:** `e0041337` (branch `main`)
**Produced by:** `sc-lang-typescript`
**Scope:** `frontend/src/` (React 19 console SPA), `sdk/typescript/` (published npm package
`@agezt/sdk`), and Node-side scripts. **Excluded:** `frontend/dist/`, `kernel/webui/dist/`,
`node_modules/`, `.dev-home/`.
**Not covered here (owned by the XSS/CSRF/CORS agent):** DOM XSS, CSRF, CORS, clickjacking,
WebSocket. Two observations in those classes are noted in one line each under §3 and handed off.

---

## 0. Headline

This is an unusually clean TypeScript codebase by language-level standards. `strict: true` is on in
both tsconfigs, `tsc --noEmit` passes clean on both trees, and there is **not a single
`@ts-ignore` / `@ts-expect-error` / `@ts-nocheck` in the entire repository**. The three highest-
frequency finding classes for this checklist — `eval`/`new Function`, DOM sinks, and client-side
secret storage — are all **genuinely absent**, not merely rare.

The one finding that matters is in the **published SDK**, not the console: a security fix landed on
2026-08-12 (commit `03694cdf`, SDK-001) that closed a capability-token leak, and it patched **one of
the two call sites that needed it**. The second is still live in `src/` and in the shipped `dist/`.

---

## 1. Findings — SDK (`sdk/typescript/`)

Threat model note: SDK code runs on *consumers'* machines and inside agent subprocesses. Flaws here
are supply-chain-shaped and are not mitigated by the daemon's loopback binding.

---

### TS-001 — `EventbusHandle.subscribe()` bypasses `resolveSocketPath()`, leaking the agent capability token to a planted socket

- **Severity:** High
- **Confidence:** 92
- **CWE:** CWE-522 (Insufficiently Protected Credentials), CWE-706 (Use of Incorrectly-Resolved Name), secondary CWE-1injection-channel via CWE-829
- **File:** `sdk/typescript/src/agent.ts:403` — and shipped as `sdk/typescript/dist/src/agent.js:317`

**The line:**

```ts
socketPath: (this.client as unknown as { socketPath: string }).socketPath,
```

**Compare the sibling call site 177 lines earlier** — `sdk/typescript/src/agent.ts:226`, inside
`AgentClient.request()`:

```ts
socketPath: resolveSocketPath(this.socketPath),
```

**Exploitation path.** `DEFAULT_SOCKET_PATH` is `"@agezt/agentgw.sock"`
(`agent.ts:43`). The daemon binds it with Go's `net.Listen("unix", "@agezt/agentgw.sock")`, and Go
maps a leading `@` to the **Linux abstract namespace**. Node/libuv does not — it copies the string
verbatim into `sun_path`, so the untranslated string is a **CWD-relative file path**,
`./@agezt/agentgw.sock`. The project's own doc comment states the consequence precisely
(`agent.ts:56-60`):

> "It fails OPEN into a credential leak. An agent subprocess whose CWD is attacker-writable can have
> `./@agezt/agentgw.sock` planted there; every request then hands `Authorization: Bearer <capability
> token>` to whoever is listening, who can replay it and feed forged tool results back as a
> prompt-injection channel."

That reasoning applies verbatim to `subscribe()`, which sets `Authorization: Bearer <token>` at
`agent.ts:406` and then connects to the **unresolved** path at `:403`. Concretely, on Linux:

1. An agent subprocess runs with its CWD in the agent workspace — which is exactly the directory the
   `file` tool writes to and `code_exec` runs in, both `LevelAllow` by default per the Phase 1 map.
2. Anything that can write there creates a listening unix socket at `./@agezt/agentgw.sock`.
3. The next `client.eventbus.subscribe(...)` call connects to the attacker's socket instead of the
   gateway and sends the scoped JWT capability token in the `Authorization` header.
4. The attacker replays the token against the real gateway (it is valid until `exp`; the Phase 1 map
   records that agentgw tokens have **no `kid` and no revocation**), and simultaneously feeds forged
   `BusEvent` frames back to the subscriber — a prompt-injection channel into the agent loop.

**Why this is not a false positive.**

- Both call sites were checked exhaustively: `resolveSocketPath` has exactly **one** use in `src/`
  (`agent.ts:226`), while `socketPath` is consumed by **two** `http.request` sites (`agent.ts:240`
  and `agent.ts:411`).
- `git show 03694cdf -- sdk/typescript/src/agent.ts` confirms the SDK-001 fix was a single-line
  change, `-socketPath: this.socketPath` → `+socketPath: resolveSocketPath(this.socketPath)`. The
  second site was never touched.
- The bug is present in the **committed build output** (`dist/src/agent.js:317`:
  `socketPath: this.client.socketPath`), i.e. it is in what npm would publish.
- `subscribe()` is public, exported API: `AgentClient.eventbus` is a `readonly` public field
  (`agent.ts:185`) and `EventbusHandle.subscribe` is a public method (`agent.ts:397`).
- The existing test (`sdk/typescript/test/agent.test.ts:22-44`) exercises `resolveSocketPath` **as a
  pure function only**. It never asserts that any call site invokes it, so a green suite (18/18 pass,
  see §5) is fully consistent with this bug. This is the "stale/insufficient test guarding a real
  bug" pattern — flagging per instruction rather than assuming the test is adequate.
- The Python SDK is **not** affected in the same way: `sdk/python/agezt/agent.py` funnels through a
  single `_connect` that applies `_resolve_socket_path` once (`agent.py:156`). The divergence is
  specific to the TypeScript SDK having two connect paths.

**Contributing cause worth fixing too.** The `as unknown as { socketPath: string }` double cast at
`:403` (and `as unknown as { token: string }` at `:406`) reaches around the class's own `private`
fields. That is precisely the type-safety erosion that let the second call site drift out of sync —
had `subscribe()` gone through an accessor, the compiler would have pointed at one chokepoint.

**Remediation.**

1. Apply the resolver at `agent.ts:403`, and re-run the build so `dist/` is regenerated:
   ```ts
   socketPath: resolveSocketPath((this.client as unknown as { socketPath: string }).socketPath),
   ```
2. Better, remove the escape hatch entirely: give `AgentClient` an internal accessor
   (e.g. `/** @internal */ get resolvedSocketPath(): string { return resolveSocketPath(this.socketPath); }`
   and `/** @internal */ get bearer(): string`) and have `subscribe()` use those. One chokepoint,
   no casts, and a future third connect path cannot repeat this.
3. Add a regression test that asserts the *call site*, not the helper — e.g. construct an
   `AgentClient`, stub `http.request`, invoke `subscribe()`, and assert
   `options.socketPath[0] === "\0"` on Linux. Per the repo's own practice, prove it fails before the
   fix.

---

### TS-002 — SSE parsers grow an unbounded buffer and rescan it quadratically; the stream has no timeout

- **Severity:** Low
- **Confidence:** 80
- **CWE:** CWE-400 (Uncontrolled Resource Consumption)
- **File:** `sdk/typescript/src/client.ts:275-297` (`parseSSE`), same pattern at
  `sdk/typescript/src/agent.ts:421-441`

`parseSSE` accumulates `buf += decoder.decode(value, { stream: true })` (`client.ts:283`) with **no
cap**, and calls `indexOfFrameEnd(buf)` (`client.ts:285`) which runs two `String.indexOf` scans from
index 0 on every read (`client.ts:302-304`). A peer that streams bytes containing no `\n\n`
separator makes the consumer's memory grow without bound while each successive chunk re-scans the
whole accumulated buffer — O(n²) CPU on top of O(n) memory.

Compounding this, the per-request timeout at `client.ts:246` is attached to the **fetch promise**,
which settles when response *headers* arrive; `.finally(() => clearTimeout(timer))`
(`client.ts:247-249`) therefore disarms the abort before a single body byte is read. So a streaming
response has no timeout at all. Note this makes the doc comment at `client.ts:210-211` — advising
consumers to raise `timeoutMs` so "the default 30s would [not] cut a quiet watch short" — describe
behaviour the code does not have.

**Why this is not a false positive:** the code path is unconditional for `runStream()` and
`mailboxWatch()`, and no length guard exists anywhere in the function.

**Why the severity is Low:** the peer here is the consumer's own daemon, reached over loopback or a
unix socket. This is a robustness/hardening gap against a compromised or buggy daemon, not a path an
unrelated attacker opens. Raising it above Low would overstate it.

**Remediation:** cap the buffer (reject a frame over, say, 1 MiB and error the stream); track a
scan offset so `indexOfFrameEnd` resumes rather than restarting; and if a stream-level deadline is
wanted, arm an idle timer that resets on each `read()` instead of relying on the fetch timer.

---

### TS-003 — SDK response bodies are type-asserted, never validated

- **Severity:** Low
- **Confidence:** 85
- **CWE:** CWE-20 (Improper Input Validation) / SC-TS-272
- **Files:** `sdk/typescript/src/client.ts:115`, `:235`, `:226`; `sdk/typescript/src/agent.ts:269`, `:434`

Every response crosses the boundary as a cast: `(await res.json()) as RunResult` (`client.ts:115`),
`(await res.json()) as T` (`client.ts:235`), `JSON.parse(respBody) as T` (`agent.ts:269`), and the
double cast `ev.data as unknown as Mail` (`client.ts:226`). There is no Zod/Valibot/io-ts anywhere in
the package (confirmed: zero matches). A cast is a compile-time claim, not a runtime check, so the
SDK hands consumers objects typed `Mail`/`RunResult` that may not have those shapes; the consumer's
own `.map`/property access is where it surfaces, as a `TypeError` in their code.

**Why the severity is Low, not High:** for the SDK the counterparty is the consumer's own trusted
daemon. This is an API-contract robustness issue rather than a trust-boundary bypass — I am
deliberately not inflating it.

**Remediation:** validate at minimum the fields the SDK itself dereferences
(`out.message`, `out.waiting`, `out.replies`, `out.topics`) before returning them, or ship a small
hand-rolled type guard per response shape. Zero-dependency is a stated goal of this package
(`index.ts:8`), so a dependency-free guard is the right shape.

---

### SDK packaging — clean

Checked and found correct, recorded because these are the usual supply-chain failure points:

- **`files` allowlist** (`package.json:15-19`) ships only `dist/src`, `dist/examples`, `README.md`.
  Source, `test/`, and `dist/test/` are **not** published.
- **No lifecycle scripts** — no `preinstall`/`postinstall`/`prepare`/`prepublishOnly` in either
  `sdk/typescript/package.json` or `frontend/package.json` (verified by grep, zero matches).
- **Zero runtime dependencies**; the only `devDependencies` are `@types/node` and `typescript`.
- **No TLS weakening** — uses platform `fetch` and `node:http` with no custom agent, no
  `rejectUnauthorized: false` anywhere.
- **No token leakage into errors** — `APIError` (`errors.ts:16-22`) carries only status/type/detail
  from the response body; `AgentError` at `agent.ts:278` interpolates `socketPath`, not the token.
  No `console.*` of credentials anywhere in the package.
- **Caller-supplied path segments cannot escape the base URL** — every interpolation is wrapped:
  `encodeURIComponent` at `client.ts:130`, `:170`, `:179` and `agent.ts:342`, `:357`, `:398`, `:527`,
  `:578`, `:580`, `:629`. `baseUrl` is normalised with `.replace(/\/+$/, "")` (`client.ts:94`).
  The single unwrapped interpolation is `&limit=${limit}` (`agent.ts:342`), where `limit` is typed
  `number` with a default — worth wrapping with `String(Number(limit))` for JS consumers, but not a
  finding on its own.
- **CI installs with `--ignore-scripts`** (`ci.yml:217`, `:270`; `publish-sdks.yml:78`), which is the
  correct posture and directly addresses SC-TS-224/243.

One minor gap: `publish-sdks.yml:90` runs `npm publish --access public` **without `--provenance`**
(SC-TS-236). Low; properly `sc-ci-cd`'s call, noted here for completeness.

---

## 2. Findings — Console (`frontend/src/`)

---

### TS-004 — No runtime validation at the API boundary anywhere in the console

- **Severity:** Medium
- **Confidence:** 88
- **CWE:** CWE-20 / SC-TS-272, SC-TS-273
- **Files:** `frontend/src/lib/api.ts:118`, `:137`, `:145`; downstream e.g.
  `frontend/src/lib/cursorPager.ts:59`, `:75`; `frontend/src/views/Research.tsx:186-220`

The entire transport layer terminates in a cast. `lib/api.ts:118`:

```ts
return res.json() as Promise<T>;
```

with identical shapes at `:137` (`postJSON`) and `:145` (`postAction`). Every one of ~150 views
calls these with a `<T>` describing the expected shape, and **no schema library exists in the
project** (zero matches for zod/valibot/io-ts/superstruct/ajv/yup). The generic pager does the same:
`const items = (data[itemsKey] as T[] | undefined) ?? []` (`cursorPager.ts:59`) — the `??` guards
`undefined` but not "the server sent a string/object where an array was expected".

**Exploitation path.** This is a *robustness* boundary, not an authentication one — the API is the
operator's own daemon. It matters because **agent-authored content reaches these responses**: run
answers, memory records, forged tool metadata, world-model entities, research reports. Where a view
dereferences an optional field with `!`, a shape it did not expect becomes a render-time
`TypeError` that unmounts the React subtree. `views/Research.tsx:189` renders
`report.claims!.map(...)`, `:220` renders `report.sources!.map(...)`, `:166`
`report.notes!.map(...)` — all `!` assertions on fields of an LLM-derived report object. A report
whose `claims` serialises as a non-array takes out the Research view.

**Why this is not a false positive:** the casts are real and unconditional, and `TS-005` below
demonstrates the same untrusted-growth channel is live. **However** — refuting the stronger version
of this claim — this is *not* an XSS or privilege vector: React escapes all text children, the
markdown renderer produces React text nodes only (§4), and there is no DOM sink to reach. The
realistic worst case is a client-side denial of view.

**Countervailing evidence that keeps this at Medium:** several views already do this correctly and
show the intended pattern — `views/World.tsx:82` guards with
`Array.isArray(world?.entities) ? (world!.entities as unknown[]) : null` before dereferencing, and
`:109-111` do the same for edges/relations. The discipline exists; it is just not systematic.

**Remediation.** Rather than retrofitting a schema library across 150 views, add a small
dependency-free `expectArray<T>(v: unknown): T[]` / `expectObject` helper in `lib/api.ts`, use it
inside `useCursorPager` (one chokepoint covering the 13 paged endpoints), and replace the `!`
assertions on optional array fields in `Research.tsx`, `Chains.tsx:312,320`, `Autonomy.tsx:620` and
`IncidentPage.tsx:787` with `Array.isArray(...)` guards. That converts a crash into an empty state.

---

### TS-005 — `Toolforge` renders every forged tool with no window, no pager, and no server-side cap

- **Severity:** Medium
- **Confidence:** 85
- **CWE:** CWE-400 (Uncontrolled Resource Consumption)
- **Files:** `frontend/src/views/Toolforge.tsx:352` + `:451`; server side
  `kernel/controlplane/toolforge.go:37-52`

Client (`views/Toolforge.tsx:352`, then `:451`):

```ts
const d = await getJSON<{ tools?: ScriptTool[] }>("/api/toolforge");
...
{tools.map((t) => (
```

No `limit` query arg, no `.slice(0, N)`, no `LoadMoreFooter`, no `useCursorPager`.

Server (`kernel/controlplane/toolforge.go:37-46`) confirms there is no cap to fall back on:

```go
func (s *Server) handleToolforgeList(conn net.Conn, req Request) {
	tools := s.k.ToolForge().List()
	out := make([]any, 0, len(tools))
	...
	for _, st := range tools {
		out = append(out, scriptToolView(st, false))
```

`List()` is returned whole — no `limit`, no cursor, unlike `/api/runs` which defaults to 20 and
caps at 1000 (`kernel/controlplane/runs.go:24-25`, `:258-273`).

**Exploitation path.** `tool_forge` is an agent-callable capability that the Phase 1 map records as
`LevelAllow` by default, and it is one of the four *persistence* primitives (`standing`, `schedule`,
`tool_forge`, `mcp`) by which a single prompt injection outlives its run. An agent that forges tools
in a loop grows this list without bound; the console then fetches and mounts every row on each visit.
Growth here is **agent-driven, not operator-driven**, which is what distinguishes this from the
naturally-bounded lists below.

**Why this is not a false positive:** both ends were verified — the client has no bound and the
server has no cap. This is a direct violation of the recorded owner law ("hiçbir liste sınırsız
fetch/render etmez"), which makes it a finding here rather than a style nit.

**Remediation:** apply the established in-repo pattern. Since `/api/toolforge` has no cursor support,
use the 60-item client windowing form already used by `views/Agents.tsx:189,444-449`,
`views/Artifacts.tsx:97,253-258`, `views/Board.tsx:40,860-865` and `views/Data.tsx:43,404-405`:
a `TOOLFORGE_WINDOW = 60` constant, `slice(0, win)`, and a `LoadMoreFooter`. Adding a server-side
`limit`/cursor to `handleToolforgeList` would be the more complete fix.

**Scope note — deliberately narrowed.** An initial sweep flagged 31 files that fetch and `.map`
without windowing. On verification most are naturally bounded (`views/Policy.tsx` = the 36 fixed
capabilities; `views/Models.tsx`, `views/ExecutionProfiles.tsx`, `views/Chains.tsx`,
`views/Routing.tsx` = operator-configured registries) or are already capped by the server or by an
explicit query arg (`views/Autonomy.tsx:79` passes `limit: "150"`;
`components/AgentActivity.tsx:79` passes `limit: "60"`). Only `Toolforge` combined *agent-driven
unbounded growth* with *no cap at either end*. The remaining 55/121 view+component files do use
`.slice(0, …)` and 13 use a pager hook, so the law is broadly followed.

---

### TS-006 — Monaco is fetched from a third-party CDN with no SRI, and the shipped CSP blocks it

- **Severity:** Medium
- **Confidence:** 90
- **CWE:** CWE-829 (Inclusion of Functionality from Untrusted Control Sphere), CWE-353 (Missing Integrity Check) / SC-TS-231, SC-TS-189
- **Files:** `frontend/src/lib/monaco.ts:11-30`; CSP at `kernel/webui/webui.go:1316-1319`

```ts
export const PINNED_MONACO_VERSION = "0.52.2";
export const MONACO_CDN_BASE = `https://cdn.jsdelivr.net/npm/monaco-editor@${PINNED_MONACO_VERSION}/min/vs`;
...
loader.config({ paths: { vs: MONACO_CDN_BASE } });
```

`ensureLoader()` runs at **module import time** (`monaco.ts:31`), not lazily.

The daemon's CSP (`kernel/webui/webui.go:1316-1319`) is:

```
default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; …
```

**Two conclusions, and they pull in opposite directions:**

1. **Availability (what actually happens today).** `script-src 'self'` admits no third-party origin,
   and `default-src 'none'` denies the `connect-src`/`font-src` the AMD loader also needs. Under the
   shipped CSP the Monaco editor **cannot load in the console at all** — every `<Editor/>` surface
   (`components/MonacoView.tsx`, the Files/Toolforge editors) is dead in the packaged product. This
   is very likely an unnoticed regression: the CSP comment at `webui.go:1303-1306` reasons only
   about the SPA's own same-origin hashed bundle and does not mention Monaco.

2. **Integrity (why the obvious fix is wrong).** The tempting repair is to add
   `https://cdn.jsdelivr.net` to `script-src`. **Do not.** That would grant a third-party CDN the
   ability to execute script in the origin that holds the daemon's full control plane — the same
   origin that can reach `POST /api/run`, `/api/config/set`, `/api/files/delete` and
   `/api/toolbox/install`. The AMD loader fetches many chunks by computed path, so SRI cannot
   meaningfully cover it either. A jsdelivr compromise, or anyone able to MITM/poison DNS for it,
   would own the console.

**Why this is not a false positive:** the CDN URL, the eager `ensureLoader()` call, and the CSP text
were each read directly; no SRI attribute is possible in the `loader.config({paths})` form.

**Remediation:** self-host. The file's own comment already prescribes it (`monaco.ts:7`): *"To
self-host later, point `paths.vs` at the vendored `monaco-editor/min/vs` directory."* Add
`monaco-editor@0.52.2` as a direct dependency, have Vite emit `min/vs` into the embedded bundle, and
point `MONACO_CDN_BASE` at the same-origin path. That restores the feature **and** keeps
`script-src 'self'` intact — the only outcome that fixes availability without widening the CSP.

---

### TS-007 — The console bearer token is read from the URL and left in the address bar

- **Severity:** Low
- **Confidence:** 95
- **CWE:** CWE-598 (Use of GET Request Method With Sensitive Query Strings) / SC-TS-035, SC-TS-110
- **File:** `frontend/src/lib/api.ts:10-11`

```ts
const TOKEN =
  typeof location !== "undefined" ? new URLSearchParams(location.search).get("token") || "" : "";
```

The SPA reads `?token=` once and — correctly — keeps it **in memory only, never in
`localStorage`** (the comment at `api.ts:1-6` states this and the code honours it). What it does not
do is remove the token from the URL afterwards. Verified: `history.replaceState` is used in this
codebase, but only for hash routing (`views/Board.tsx:463`, `views/Workboard.tsx:311,339`), and those
calls pass a bare `#hash`, which resolves against the current URL and therefore *preserves* the query
string. No call scrubs `?token=`.

**Exposure.** The full-authority console token remains visible in the address bar for the whole
session and is written to browser history. It survives screenshots and screen-shares — a realistic
concern for a tool whose users demo it. `Referrer-Policy: no-referrer` (`webui.go:1311-1326`) does
close the third-party referrer leak, which is why this is Low rather than Medium.

**Why this is not a false positive:** the read is unconditional and no scrub exists anywhere in
`frontend/src`.

**Remediation:** one line at module load, after `TOKEN` is captured:

```ts
if (TOKEN && typeof history !== "undefined") {
  const u = new URL(location.href);
  u.searchParams.delete("token");
  history.replaceState(null, "", u.pathname + u.search + u.hash);
}
```

Note this must run before any component reads `location.search` for other purposes, and the SSE
fallback at `api.ts:34` (which reuses `TOKEN`) keeps working because the value is already captured.

---

### TS-008 — No ESLint or any JS/TS static analysis in the tree or in CI

- **Severity:** Low
- **Confidence:** 95
- **CWE:** CWE-1053 / SC-TS-385
- **Evidence:** no `.eslintrc*` / `eslint.config.*` in `frontend/` or the repo root; no `lint` script
  and no `eslint` dependency in `frontend/package.json` or `sdk/typescript/package.json`;
  `.github/workflows/ci.yml` has `frontend-test` (Vitest) and `typescript-sdk` jobs but no lint or
  SAST step for TS.

The Go side has ratchets, `deadcodecheck`, and gitleaks; the 109k-LOC TypeScript side has type
checking and unit tests but no rule enforcement. Rules that would have *mechanically* caught findings
in this very report: `@typescript-eslint/no-unnecessary-type-assertion` and
`no-restricted-syntax` on `as unknown as` (TS-001's contributing cause),
`@typescript-eslint/no-non-null-assertion` (TS-004's `!` sites).

**Remediation:** add `typescript-eslint` with `no-explicit-any`, `no-non-null-assertion` (as `warn`),
and a `no-restricted-syntax` rule banning `as unknown as` outside a small allowlist; wire it as a
`lint` script and a CI step alongside the existing `frontend-test` job.

---

### TS-009 — Dual lockfiles, and a `dompurify` override for a package nothing imports

- **Severity:** Low
- **Confidence:** 95
- **CWE:** CWE-1395 / SC-TS-223, SC-TS-225
- **Files:** `frontend/package-lock.json` (2026-07-29) and `frontend/pnpm-lock.yaml` (2026-07-26);
  `frontend/package.json:50-53`

Both lockfiles are committed and the pnpm one is three days staler. The project uses **npm** (CI runs
`npm ci --ignore-scripts`, `ci.yml:217`), so `pnpm-lock.yaml` is an unmaintained artifact that will
drift further and could mislead a contributor into resolving a different dependency graph.

Separately, `package.json:51` pins `"dompurify": "^3.4.11"` in `overrides`. Confirmed by grep:
**nothing in `frontend/src` imports or references DOMPurify**. As an `overrides` entry this is
harmless-but-inert if some transitive dep pulls it; the risk is interpretive — it reads as though a
sanitizer is in the rendering path when none is. (In this codebase that turns out to be fine: the
markdown renderer needs no sanitizer by construction, see §4.)

**Remediation:** delete `frontend/pnpm-lock.yaml` and add it to `.gitignore`; either drop the
`dompurify` override or add a one-line comment recording which transitive dependency it is pinning
and why.

---

## 3. Handed off to the XSS/CSRF/CORS agent (noted, not analysed)

Both were already surfaced in the Phase 1 map; recording in one line each per the division of labour:

- `frontend/src/views/Files.tsx:170` — `<iframe src={href}>` for PDF preview with **no `sandbox`
  attribute**, unlike the correct sibling at `views/Artifacts.tsx:513` which sets
  `sandbox="" referrerPolicy="no-referrer"`.
- `views/Channels.tsx:211`, `views/Models.tsx:362`, `views/Setup.tsx:223` —
  `window.open(r.authorize_url, "_blank", "noopener,noreferrer")` with **no scheme validation** on a
  server-supplied URL (a `javascript:` value would execute in the opener's origin). Note the console
  already ships the right helper for this: `lib/markdown.ts:42-44` `safeHref()` allowlists
  `https?:`/`mailto:` and is the obvious thing to reuse at these three sites.

---

## 4. Categories verified CLEAN (with the evidence)

Recording these because absence-of-finding is a result, and several are the highest-frequency
categories in this checklist.

| Category | Result | Evidence |
|---|---|---|
| `eval` / `new Function` / string `setTimeout`/`setInterval` | **Clean — zero occurrences** | Regex sweep of `frontend/src` for `eval(`, `new Function(`, `setTimeout("`, `setInterval("`: no matches |
| DOM sinks (`innerHTML`, `outerHTML`, `document.write`, `insertAdjacentHTML`, `dangerouslySetInnerHTML`) | **Clean — zero occurrences** | Same sweep, no matches anywhere in `frontend/src` |
| Markdown rendering of agent content | **Clean by construction** | `lib/markdown.ts` is a hand-rolled AST parser whose leaves are React text nodes (React escapes them); `safeHref()` (`:42-44`) allowlists `https?:`/`mailto:` so `[x](javascript:…)` renders as literal text (`:108`) |
| ReDoS | **Clean — measured, not assumed** | Benchmarked `fileMentionRegex` (`lib/language.ts:60-63`), `TABLE_SEP_RE`, `INLINE_RE` (`lib/markdown.ts:34,127`) at n=1k→64k on adversarial near-miss inputs. All **linear**: worst case 0.6 ms at n=64,000. Full output in §5 |
| User-controlled regex (`new RegExp(userInput)`) | **Clean** | 4 `new RegExp` sites; `language.ts:60` and `markdown.ts:67` build from a static extension list; `voiceSession.ts:136,144` apply the standard metacharacter escape `replace(/[.*+?^${}()|[\]\\]/g, "\\$&")` before interpolating |
| `import.meta.env` / `VITE_*` secret leakage | **Clean — zero occurrences** | No `import.meta.env`, no `VITE_`, no `process.env` anywhere in `frontend/src`. Vite config has **no `define` block** (`vite.config.ts`) |
| Tokens/secrets in client-side storage | **Clean** | `localStorage` holds only UI prefs — theme, accent hue, console name, advanced toggle, notify opt-in, chat conversations, voice wake/agent, dismissed alert ids. The bearer token is **memory-only** by explicit design (`lib/api.ts:1-11`). No `sessionStorage`, no `indexedDB`, no `document.cookie` writes |
| Secrets logged to console | **Clean** | No `console.*` call in non-test `frontend/src` mentions token/secret/key/password/cred/auth |
| Secrets rendered into the DOM | **Clean** | Credential inputs use `type="password"` (`views/Setup.tsx:543,551,639,808`); no view renders a raw API-key value |
| Prototype pollution | **Clean — 3 candidates, all refuted** | (a) `components/FleetNowBar.tsx:225` `Object.assign(row, eventPhase(e))` — `eventPhase` (`:125-137`) returns an object literal with 5 fixed keys, no untrusted key spread. (b) `components/EventFeed.tsx:37` `m[k] = (m[k]\|\|0)+1` — `k` is `categoryOf(e.kind).key`, a value from a closed category table, not the raw event kind. (c) No `deepMerge`, no `__proto__` access, no `JSON.parse` reviver, no `structuredClone` anywhere |
| `@ts-ignore` / `@ts-expect-error` / `@ts-nocheck` | **Clean — zero in the entire repository** | Repo-wide grep across all `.ts`/`.tsx` |
| `strict` mode | **Clean — enabled both trees** | `frontend/tsconfig.json:15` `"strict": true` (+ `noFallthroughCasesInSwitch`); `sdk/typescript/tsconfig.json` `"strict": true` |
| `as any` | **Clean — 2 real sites, both benign** | 4 grep hits, 2 are prose in comments. Real: `views/Dashboard.tsx:331-332` `(a.payload as any)?.phase` — optional-chained reads for display only |
| Non-null assertions on external data | **Clean-ish — 25 sites, all locally guarded** | Each is preceded by a truthiness check in the same expression, e.g. `components/AgentRepair.tsx:314` `(denials?.length \|\| 0) > 0 && …denials!.length`. Flagged as a hardening item under TS-004 rather than as its own finding |
| `postMessage` / `message` listeners | **Clean — zero occurrences** | No `postMessage` and no `addEventListener("message")` anywhere in `frontend/src` |
| `Math.random` for security purposes | **Clean** | 6 sites, all React list keys / client-side correlation ids (`components/Inspector.tsx:74,117`, `lib/chatStore.tsx:46`, `views/Alerts.tsx:79`). `lib/conductorStore.ts:107` and `lib/councilStore.ts:122` prefer `crypto.randomUUID()` and fall back only when unavailable. No token/nonce/secret generation client-side |
| `child_process` in JS/TS | **Clean — zero occurrences** | Repo-wide grep excluding `node_modules`/`dist`: no `child_process`, `execSync`, or `spawnSync` in any `.ts`/`.tsx`/`.js`/`.mjs`/`.cjs` |
| Dynamic `import()`/`require()` with computed specifier | **Clean** | The only Node-side script is `scripts/dev/readlines.js`, a 5-line dev helper with a hardcoded path and no user input |
| Build config leakage | **Clean** | `vite.config.ts`: `sourcemap: false` (`:24`), no `define`, no `base` override; dev-only `server.proxy` targets `127.0.0.1:8787` and does not ship |
| `index.html` inline scripts | **Clean** | Single `<script type="module" src="/src/main.tsx">`; no inline script, consistent with `script-src 'self'` |
| SQL/NoSQL/ORM injection | **N/A** | No database, driver, or ORM in the tree (confirmed in Phase 1); no query construction in TS |
| Express/Fastify/Node server patterns | **N/A** | No Node HTTP server in this repo; the daemon is Go |
| npm lifecycle-script abuse | **Clean** | No `preinstall`/`postinstall`/`prepare`/`prepublishOnly` in either `package.json`; CI installs with `--ignore-scripts` |
| Frontend/SDK test–source drift | **Clean this run** | Full Vitest suite 1543/1543 pass; SDK 18/18 pass; both typecheck clean. **Exception:** the SDK suite is *insufficient* rather than stale — see TS-001, where a green suite coexists with the bug |

---

## 5. Tooling output

Everything below was actually executed; nothing is inferred.

**`tsc --noEmit` — frontend** (`cd frontend && npx --no-install tsc --noEmit -p tsconfig.json`)
```
EXIT=0
```
Clean, no diagnostics.

**`tsc -p tsconfig.json` — TypeScript SDK** (`cd sdk/typescript && npx --no-install tsc -p tsconfig.json`)
```
tsc-exit=0
```
Clean.

**Vitest — frontend** (`cd frontend && npx --no-install vitest run`)
```
 RUN  v4.1.9 D:/Codebox/PROJECTS/AGEZT/frontend

 Test Files  187 passed (187)
      Tests  1543 passed (1543)
   Duration  30.28s
```
*(A first attempt with `--reporter=basic` failed to start — `basic` is not a valid reporter in
Vitest 4 and it tried to load it as a custom reporter module. That was my flag error, not a project
failure; rerun with the default reporter, shown above.)*

**SDK unit tests** (`cd sdk/typescript && node --test dist/test/client.test.js dist/test/mailbox.test.js dist/test/agent.test.js`)
```
ℹ tests 18
ℹ pass 18
ℹ fail 0
```

**ESLint / lint script** — **not available.** No ESLint config, dependency, or script exists in
either package (see TS-008). Nothing was run; nothing is claimed.

**`npm audit`** — **not run**, per instruction not to touch the network or run `npm install`.
Dependency-CVE coverage for the npm tree is therefore **not** part of this report and should be
attributed to `sc-supply-chain`. Noted: CI references Dependabot for weekly `/frontend` bumps
(`ci.yml:203`).

**ReDoS benchmark** — ad-hoc Node script against the three regexes that touch agent-authored text,
using adversarial near-miss inputs (dot-rich runs with no valid extension, long path-segment chains,
near-miss table separators, unterminated code spans):

```
--- fileMentionRegex: dot-rich run, no valid extension ---
  n=1000: 0.1ms    n=4000: 0.0ms    n=16000: 0.2ms
--- fileMentionRegex: many path segments ---
  n=500: 0.0ms     n=2000: 0.1ms    n=4000: 0.1ms
--- TABLE_SEP_RE: near-miss dashes ---
  n=1000: 0.1ms    n=16000: 0.1ms   n=64000: 0.6ms
--- TABLE_SEP_RE: pipe+space runs ---
  n=200: 0.0ms     n=800: 0.0ms     n=1600: 0.0ms
--- INLINE_RE: unterminated code span ---
  n=1000: 0.1ms    n=16000: 0.0ms   n=64000: 0.1ms
```
Growth is linear in every case. **ReDoS refuted on measurement**, not on inspection.

---

## 6. Checklist coverage

| # | Category | Result |
|---|---|---|
| 1 | Input Validation & Sanitization | 1 finding (TS-004); URL-scheme validation handed to XSS agent (§3) |
| 2 | Authentication & Session Management | 1 finding (TS-007). No JWT in the console; token is memory-only |
| 3 | Authorization & Access Control | **N/A at language level** — enforcement is Go-side (Edict); no client-only authz gate found |
| 4 | Cryptography | **Clean** — no crypto in TS; `Math.random` never used for security values |
| 5 | Error Handling & Logging | **Clean** — no stack traces or credentials surfaced to the UI or console |
| 6 | Data Protection & Privacy | 1 finding (TS-007). Client-side storage otherwise clean |
| 7 | SQL/NoSQL/ORM Security | **N/A** — no database anywhere in the tree |
| 8 | File Operations | **N/A in TS** — all filesystem access is Go-side |
| 9 | Network & HTTP Security | 1 finding (TS-006, SRI/CDN). Headers + CSP are Go-side |
| 10 | Serialization & Deserialization | 1 finding (TS-002). No YAML/XML/BSON; `JSON.parse` reviver never used |
| 11 | Concurrency & Race Conditions | **Clean** — no shared mutable state across workers; no SharedArrayBuffer |
| 12 | Dependency & Supply Chain | 1 finding (TS-009) + `--provenance` note. Lifecycle scripts and `files` allowlist clean |
| 13 | Configuration & Secrets Management | **Clean** — no `VITE_*`, no `define`, no sourcemaps, no inline scripts |
| 14 | Prototype & Type Safety | 2 findings (TS-003, TS-004). Prototype pollution **clean** (3 candidates refuted) |
| 15 | TS/JS-Specific Patterns | **Clean** — no `eval`/`Function`/`vm`/dynamic-require/`with`/`Reflect` abuse |
| 16 | Framework-Specific: React/Next.js | 1 finding (TS-005, unbounded render). No Next.js; no `dangerouslySetInnerHTML` |
| 17 | Framework-Specific: Express/Fastify/Node | **N/A** — no Node server in this repo |
| 18 | API Security | 1 finding (TS-005 pagination). Auth/rate-limiting are Go-side |
| 19 | Testing & CI/CD Security | 1 finding (TS-008, no SAST). CI hygiene otherwise strong (`--ignore-scripts`, SHA-pinned actions) |
| 20 | Third-Party Integration Security | 1 finding (TS-006, CDN). Unsandboxed iframe handed to XSS agent (§3) |

---

## 7. Summary by severity

| ID | Title | Severity | Confidence | Location |
|---|---|---|---|---|
| TS-001 | `subscribe()` bypasses `resolveSocketPath()` → capability-token leak | **High** | 92 | `sdk/typescript/src/agent.ts:403` |
| TS-004 | No runtime validation at the console API boundary | Medium | 88 | `frontend/src/lib/api.ts:118` |
| TS-005 | `Toolforge` renders agent-creatable list unbounded | Medium | 85 | `views/Toolforge.tsx:352,451` |
| TS-006 | Monaco from CDN, no SRI, blocked by CSP | Medium | 90 | `frontend/src/lib/monaco.ts:11-30` |
| TS-002 | SDK SSE parser: unbounded buffer, quadratic rescan, no stream timeout | Low | 80 | `sdk/typescript/src/client.ts:275-297` |
| TS-003 | SDK responses type-asserted, never validated | Low | 85 | `sdk/typescript/src/client.ts:115,226,235` |
| TS-007 | Console token left in the address bar | Low | 95 | `frontend/src/lib/api.ts:10-11` |
| TS-008 | No ESLint/SAST for 109k LOC of TypeScript | Low | 95 | (absent config) |
| TS-009 | Dual lockfiles; unused `dompurify` override | Low | 95 | `frontend/package.json:50-53` |

**The one to fix first is TS-001** — it is the only finding that leaks a credential, it is in a
*published* package rather than a loopback-bound console, and it is a two-line fix to a security
patch that is already 90% landed.

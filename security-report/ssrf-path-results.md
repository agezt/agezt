# SSRF + Path Traversal scan — AGEZT

- **Skills:** `sc-ssrf` (CWE-918), `sc-path-traversal` (CWE-22 / CWE-59)
- **Repo:** `D:\Codebox\PROJECTS\AGEZT` — `main` @ `f815f56e`
- **Date:** 2026-08-12
- **Method:** source review of every outbound-HTTP construction site (`grep` for `http.Client{}`, `http.DefaultClient`, `NewRequest`) cross-referenced against `kernel/netguard` usage, plus every filesystem sink reachable from a request/tool argument.
- **Supersedes:** the previous `ssrf-path-results.md` (99d2e426).

---

## 0. Baseline verification: does `kernel/netguard` do what it claims?

**Claim (netguard.go:9-19):** the guard validates the *resolved* IP on every connection attempt — initial dial **and** every redirect hop — via `net.Dialer.Control`.

**Verdict: the claim holds for every request that actually goes through `Guard.HTTPClient`.**

- `Guard.Control` (netguard.go:168-186) splits the concrete `IP:port` the dialer is about to connect to, fails closed on an unparseable/unresolved address, and returns an error before `connect(2)`. Because `Control` runs per dial, DNS rebinding cannot win: the guard sees the address actually used, not the one that was checked.
- `Guard.HTTPClient` (netguard.go:198-209) builds a **fresh, non-shared** `http.Transport` whose `DialContext` is the guarded dialer, so redirect hops (which reuse the same Transport) are each re-dialed and re-screened. `Transport.Proxy` is left nil, so no `HTTP_PROXY` env var can route around the dialer.
- Classification (netguard.go:69-104) covers unspecified, `0.0.0.0/8`, loopback, link-local (incl. `169.254.169.254`), RFC1918 + ULA, CGNAT `100.64/10`, multicast, and `255.255.255.255`; `embeddedV4` (netguard.go:111-132) collapses NAT64 `64:ff9b::/96` and IPv4-compatible `::/96` so a v4 metadata address cannot be smuggled in as a v6 literal.
- `kernel/netguard/netguard_test.go:137-157` is a real end-to-end redirect test against `169.254.169.254`.

**Correctly wired consumers (verified guarded):** `plugins/tools/http`, `plugins/tools/fetch`, `plugins/tools/browser` (`browser.read`), `plugins/tools/websearch`, `kernel/mcp/http.go:75`, `kernel/update/update.go:147-166`, `kernel/catalog/sync.go:21`, `kernel/catalog/discovery.go:41`, `kernel/market/sync.go:38`, `kernel/webhook` (guarded client injected at `cmd/agezt/httpsurfaces.go:568`), `kernel/controlplane/channels.go:403` (WhatsApp-gateway + provider probes), `kernel/controlplane/channel_oauth.go:84`, `kernel/chatgptauth/chatgptauth.go:57`, `plugins/providers/voice/voice.go:209`.

**So the guard is not the weak point. The weak point is every egress path that never reaches it.** The three findings below are exactly those paths.

---

## SSRF-001 — `browser.action` egress guard is a pre-flight DNS check; the Playwright driver it hands the URL to has no guard at all

- **Title:** SSRF to cloud metadata / internal services via `browser.action` (redirect, in-page JS, or DNS rebinding)
- **Severity:** **High**
- **Confidence:** 92
- **CWE:** CWE-918 (SSRF), with CWE-367 (TOCTOU) as the enabling flaw
- **File:** `plugins/tools/browser/action.go:803-838` (`validateHostEgress`), called from `action.go:251` / `:330`; sink is `plugins/builtinskills/browseruse/scripts/browse.mjs:99-104`

### Description

Unlike `browser.read` — which performs its own fetch through a netguard client — `browser.action` does **not** make the request in Go at all. It validates the URL and then shells out to a Playwright driver:

```go
// action.go:803-838 — the ONLY egress control on this path
func (t *ActionTool) validateHostEgress(ctx context.Context, host string) error {
    ips, err = lookup(ctx, "ip", host)          // resolve once, here
    ...
    g := netguard.New(opts...)
    for _, ip := range ips {
        if ok, reason := g.Allowed(ip); !ok { return fmt.Errorf("egress blocked: ...") }
    }
    return nil                                   // …then hand the URL to node
}
```

```js
// browse.mjs:99-104 — the actual navigation. No page.route(), no interception.
await page.goto(spec.url, { waitUntil: "domcontentloaded", timeout });
for (const a of spec.actions || []) {
  switch (a.type) {
    case "goto": await page.goto(a.url, ...); break;
    case "click": await page.click(a.selector, ...); break;
```

A full `grep` of `browse.mjs` (395 lines) for `route|abort|guard|169.254` returns **nothing** — the browser context is created with no request interception. Three independent bypasses follow:

1. **HTTP redirect.** `validateHostEgress` checks only the URL the model supplied. Chromium follows 3xx itself; netguard never sees the second hop.
2. **In-page JavaScript.** The fetched page's own scripts can `fetch()` / `XMLHttpRequest` any address and write the answer into the DOM, which `extract: "text"|"html"` (browse.mjs:149-158) returns straight to the model. JS can also set the `Metadata-Flavor: Google` header GCP's IMDS requires.
3. **DNS rebinding.** The Go-side `LookupIP` and Chromium's own resolver are two separate resolutions; a TTL-0 record public at check time and `169.254.169.254` at navigation time defeats the check outright.

`click` on an attacker-controlled page is a fourth vector: the target URL of a click-triggered navigation is never validated.

### Exploit scenario (concrete)

The tool ships **host-open by default** — `plugins/builtintools/tools.go:237` sets `ba.AllowAll = true` when no allowlist is pinned, so any host passes the allowlist check; only netguard's IP check stands between the agent and the internal network.

1. A prompt-injected page (or a compromised site the agent was told to visit) causes the agent to call:
   `browser.action {"url":"https://attacker.example/x","extract":"text"}`
2. `validateHostEgress` resolves `attacker.example` → a public IP → allowed.
3. `attacker.example/x` replies `302 Location: http://169.254.169.254/latest/meta-data/iam/security-credentials/agezt-role`.
4. Chromium follows it; `page.innerText("body")` is returned as the tool result, and the IAM credentials land in the model's context — from where the same agent can exfiltrate them with `http`/`fetch` to the attacker's host.
5. Nothing is journaled: `OnBlock` never fires because netguard never saw the dial, so `agt netguard log` shows a clean egress record.

A screenshot is also taken by default (`browse.mjs:160-165`) and stored as a browsable artifact, giving a second copy of the internal response.

### Remediation

Install a Playwright request interceptor in the driver — `context.route('**/*', ...)` that resolves each request's host and aborts on the blocked ranges — and pass the effective `allow_loopback`/`allow_private` flags into the spec so the driver enforces the same policy. A pre-flight DNS check cannot be made sound for an out-of-process browser.

---

## SSRF-002 — mcpbridge SSE transport: the per-POST dialer guard the code documents does not exist

- **Title:** Announced MCP SSE endpoint bypasses its SSRF gate via redirect or DNS rebinding
- **Severity:** **Medium**
- **Confidence:** 95
- **CWE:** CWE-918 (SSRF), CWE-367 (TOCTOU)
- **File:** `plugins/external/mcpbridge/sse_transport.go:80-90` (client construction), `:140` (POST), `:183` (GET); false guarantee at `plugins/external/mcpbridge/sse_guard.go:113`

### Description

`sse_guard.go` exists specifically to stop a hostile MCP server from pivoting the bridge into internal space via the server-announced `endpoint` event. Its own comment states the design:

> "Resolution happens here at the time the endpoint event arrives (cheap, one-shot) **and is enforced again per-POST via the dialer — see dialerGuard below.**" — `sse_guard.go:112-113`

**There is no `dialerGuard`.** A repo-wide grep for `dialerGuard`, `Control:`, `CheckRedirect`, and `DialContext` finds no match anywhere in `plugins/external/mcpbridge/`. The transport's client is a bare stdlib client:

```go
// sse_transport.go:81-85
httpClient: &http.Client{
    // No client-side timeout: the SSE stream is long-lived
    // by design. Per-request POSTs use a fresh client below
    // with their own context.
},
```

No `Transport`, so no `Control` hook; no `CheckRedirect`, so Go's default 10-redirect follow applies. `classifyHost` (`sse_guard.go:175-198`) therefore degrades to a one-shot advisory check with a wide-open TOCTOU window.

### Exploit scenario

Threat model is the file's own: a malicious or hijacked MCP server that the operator registered (`MCPBRIDGE_SERVER_URL`).

1. Server announces `event: endpoint` with a **same-origin** relative path, e.g. `/messages` — passes the origin check, and its host resolves to the server's public IP, so `classifyHost` passes.
2. Every subsequent JSON-RPC `POST https://evil.example/messages` is answered `307 Location: http://169.254.169.254/latest/meta-data/…` (or `http://127.0.0.1:PORT/…` at a co-located admin service). `t.httpClient.Do` follows it (sse_transport.go:140) with no IP screening.
3. Alternatively, without any redirect: rebind `evil.example` to `169.254.169.254` after the endpoint event — the POST re-resolves at dial time.

The POST response body is discarded (`io.Copy(io.Discard, …)`, sse_transport.go:151), so this is a **blind** SSRF: it yields internal port scanning (timing/status via `resp.StatusCode`, surfaced in the returned error string at `:153`) and state-changing requests against unauthenticated internal services (307/308 preserve method and body), not direct data theft.

### Remediation

Give both the SSE `GET` and the per-request `POST` an `http.Transport` whose `DialContext` runs the same `ipPolicyReason` classification the guard already implements, and set `CheckRedirect` to re-run `resolveEndpoint`'s origin check on each hop. Then the comment at `:113` becomes true.

---

## SSRF-003 — Guard drift + missing embedded-IPv4 form (6to4) in both IP classifiers

- **Title:** Blocked-range classification gaps in `netguard` and its mcpbridge copy
- **Severity:** **Low**
- **Confidence:** 70
- **CWE:** CWE-918 (incomplete blocklist)
- **File:** `kernel/netguard/netguard.go:111-132` and `:83-104`; `plugins/external/mcpbridge/sse_guard.go:205-233`, `:240-261`

### Description

Two related gaps, both of the exact class the project already fixed once (M171, NAT64):

1. **6to4 (`2002::/16`) embedded IPv4 is not collapsed.** `embeddedV4` handles `64:ff9b::/96` and `::/96` but not `2002:a9fe:a9fe::`, which a host with a 6to4 route resolves toward `169.254.169.254`. Same one-line shape as the NAT64 case already handled. (6to4 is deprecated by RFC 7526 and needs a 6to4-capable route, hence Low.)
2. **The bridge's copy has drifted from the kernel's.** `ipPolicyReason` (`sse_guard.go:205-233`) omits netguard's `isZeroBlock` (the whole `0.0.0.0/8` "this host" range — Linux routes `0.x.y.z` to local interfaces) and its `isV4Broadcast` case. So `http://0.1.2.3/` is refused by the kernel guard and accepted by the bridge guard. The comment at `sse_guard.go:26-29` says the duplication is deliberate; nothing keeps the two in sync.

Also unblocked in both: `192.0.0.0/24` (IETF protocol assignments), `198.18.0.0/15` (benchmarking), `192.88.99.0/24` (6to4 relay anycast).

### Remediation

Add `2002::/16` to both `embeddedV4`/`collapseEmbeddedV4`; port `isZeroBlock` + `isV4Broadcast` into `sse_guard.go`; add a shared table-driven test vector list so the two classifiers cannot drift again.

---

## PATH-001 — Console file browser: a symlinked *intermediate directory* escapes the workspace root (read, delete, move)

- **Title:** Path traversal / link-following in `/api/files/{raw,tree,delete,rename,mkdir}`
- **Severity:** **Medium** (arbitrary read **and** arbitrary delete/move as the daemon user, post-authentication)
- **Confidence:** 90
- **CWE:** CWE-59 (link following) → CWE-22 (path traversal)
- **File:** `kernel/webui/files_route.go:104-122` (`resolveFileRoot`), sinks at `:246-264` (raw), `:353` (rename), `:383-408` (delete), `:162-175` (tree)

### Description

The resolver's own contract says it canonicalizes through symlinks:

```
//   target   := filepath.Join(rootAbs, relPosix) then resolved symlinks
//
// Anywhere along that walk that escapes `rootAbs` is a refusal with a 400.
//   — files_route.go:101-103
```

The implementation never resolves anything. `resolveFileRoot` is **purely lexical**:

```go
// files_route.go:114-120
targetAbs = filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(relPosix)))
if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
    return "", "", "", fmt.Errorf("path escapes root")
}
```

There is no `filepath.EvalSymlinks` anywhere in the file. The only link defence is a **last-component** `os.Lstat` in the raw and delete handlers (`:246`, `:383`). `lstat(2)` does not follow the final component but **does** follow every directory component before it. So for an in-root symlinked directory `link` → `/etc`:

- `resolveFileRoot("link/passwd")` → `<root>/link/passwd`, lexically inside root → **allowed**.
- `os.Lstat("<root>/link/passwd")` → stats `/etc/passwd`, a regular file → `ModeSymlink` is clear → the symlink guard passes.
- `os.Open` streams `/etc/passwd`; `os.Remove` deletes it; `os.Rename` moves it into the workspace.

`handleFileRename` (`:353`) has **no** link check at all on either side, and `handleFileTree` (`:162-175`) uses `os.Stat` + `os.ReadDir`, which follow the same intermediate links (its per-entry `ModeSymlink` skip at `:213` only hides the link from the listing — it does not stop traversal *through* one whose name you already know).

The existing tests confirm the gap is untested: `files_route_test.go:182-260` only plants a **final-component** symlink (`link.txt`) — never a symlinked directory.

### Exploit scenario (concrete)

Precondition: one symlink or Windows junction anywhere inside `AGEZT_FILE_ROOT`. Realistic sources, in descending likelihood:

1. **pnpm/npm-linked project.** `AGEZT_FILE_ROOT` is operator-settable from the console (`kernel/settings/schema.go:597`, "Workspace root"), and pointing it at a working project is the normal use of the Files view. A pnpm `node_modules` is a forest of symlinks into a global store outside the root — `GET /api/files/raw?path=node_modules/<pkg>/../../../../../.ssh/id_ed25519` resolves *through* the store link and out. Worse, `POST /api/files/delete {"path":"node_modules/<pkg>"}` with `recursive:true` deletes the shared global store content.
2. **Agent-planted.** When the operator points `AGEZT_FILE_ROOT` at the agent workspace (`<AGEZT_HOME>/workspace`), any agent holding `shell` or `code_exec` runs `ln -s / esc` there; the console then reads, moves, or deletes any file the daemon user can touch. (By default the two roots differ — `~/agezt/workspace` vs `~/.agezt/workspace` — so this variant needs the common convergent config.)
3. **Extracted archive / git clone** containing a symlink.

Reachability: `protectedRead` / `protectedMutation` — a valid console token or password session. The impact is that a **file-browser containment boundary the code explicitly promises** does not exist; a console-authenticated actor (or CSRF/XSS against the console) gets arbitrary read + delete + move outside the root.

### Remediation

Make `resolveFileRoot` do what it documents: after the lexical join, `filepath.EvalSymlinks` the deepest existing ancestor and re-assert containment against `rootAbs` — the pattern `plugins/tools/file/file.go:761-836` already implements correctly (`resolve` + `resolveNewWithinRoot`). Apply it to `from` and `to` in rename as well.

---

## PATH-002 — Windows no-follow containment check has no separator boundary

- **Title:** Sibling-prefix bypass in `openFileNoFollow`'s workspace containment
- **Severity:** **Low**
- **Confidence:** 85
- **CWE:** CWE-22
- **File:** `plugins/tools/file/nofollow_windows.go:80`

### Description

```go
if !strings.HasPrefix(finalPath, ws) {   // ws = cleaned workspace root, no trailing sep
    f.Close()
    return nil, fmt.Errorf("openNoFollow: resolved path %q is outside workspace %q ...", finalPath, ws)
}
```

`c:\users\x\workspace-evil\f.txt` has prefix `c:\users\x\workspace`, so the check passes. This is precisely the bug the webui route calls out and avoids (`files_route.go:116-118`: "so `/var/foo` doesn't match `/var/foobar`"), and the same file's Unix sibling relies on kernel `O_NOFOLLOW` instead of a prefix test.

Exploitability is limited: this check is a TOCTOU backstop that only matters when a junction is swapped in between `resolve()` (which does a correct `EvalSymlinks` + `withinRoot` check) and the `os.OpenFile`, and the junction must target a directory whose name extends the root's. Hence Low — but it is an unambiguous defect in a security check.

### Remediation

Compare with `withinRoot`-style semantics: `finalPath == ws || strings.HasPrefix(finalPath, ws + "\\")`.

---

## PATH-003 — Nil-pointer panic in the file-tree handler on a concurrently removed entry

- **Title:** `handleFileTree` dereferences a nil `FileInfo` when `DirEntry.Info()` fails
- **Severity:** **Low** (availability; adjacent to the traversal surface, so recorded here)
- **Confidence:** 90
- **CWE:** CWE-476
- **File:** `kernel/webui/files_route.go:201-215`

```go
fi, ferr := e.Info()
var size, modMS int64
if ferr == nil && !e.IsDir() { ... } else if ferr == nil { ... }
// ferr != nil ⇒ fi is nil ⇒ panic
if fi.Mode()&os.ModeSymlink != 0 { continue }
```

`os.DirEntry.Info()` returns `(nil, err)` when the entry vanished between `ReadDir` and the `lstat` (`ErrNotExist`) — routine in a directory an agent is actively writing. The `ModeSymlink` check then dereferences nil. `net/http` recovers the panic per connection, so the blast radius is one aborted request plus a stack trace in the log, but a caller who can churn files in the browsed directory can make the Files view fail non-deterministically.

### Remediation

`if ferr != nil { continue }` before the mode check.

---

## Verified clean (checked, no finding)

| Area | Why it is clean |
|---|---|
| `kernel/netguard` core | See §0 — dial-level, per-hop, fail-closed, no proxy escape, NAT64/IPv4-compat collapsed. |
| `plugins/tools/http` | netguard client + host allowlist **re-checked on every redirect hop** (`http.go:109-117`, M251) + 10-hop cap. |
| `plugins/tools/browser` (`browser.read`) | Same pattern (`browser.go:134-142`, M254). |
| `plugins/tools/fetch`, `plugins/tools/websearch` | netguard client; websearch's endpoint is a compile-time constant (`websearch.go:57`). |
| `plugins/tools/research` | No network of its own — delegates to `runtime.Research`, which drives the guarded `web_search`/`browser.read` tools. |
| `kernel/mcp/http.go` | Guarded (loopback/private allowed by design, link-local refused). |
| `kernel/update` | netguard dialer **plus** `CheckRedirect` enforcing HTTPS on every hop (`update.go:157-166`) **plus** SHA256+Ed25519. |
| `kernel/acpcatalog`, `cmd/agt/skill_registry_remote.go`, `cmd/agt/plugin_registry.go` | Unguarded clients, but the targets are compile-time constants or a URL the operator types on the CLI — FP categories 1 & 5. |
| `kernel/controlplane/{nodes,remote_mirror}.go` | `http.DefaultClient`, but peer URLs come from the operator's node-peer env spec and are validated http(s); internal ranges are the intended destination. |
| `plugins/tools/peer` | Destination is name-keyed into the configured peer table; the model supplies a peer *name*, never a URL. |
| `plugins/tools/mcptool` (`op=add`) | Agent self-install accepts `command`/`args` only — no URL field, so an agent cannot register a remote MCP endpoint. |
| `kernel/artifact` | Content-addressed; `validRef` enforces 64-char lowercase hex before any path join (`artifact.go:144-155`); `Index.Bytes/Delete` gate on map membership, IDs are server-minted ULIDs. |
| `plugins/tools/file` | Correct containment: `EvalSymlinks` on both the abs and rel branches, deepest-existing-ancestor resolution for new files (`file.go:806-836`), `entryEscapesRoot` during walks, `openFileNoFollow`, atomic `replace` with a pre-write `Lstat` symlink refusal. |
| `kernel/skill/bundle.go` | `cleanRel` (`:75-88`) rejects absolute + `..` after backslash normalization; bundles are `map[string][]byte`, so no archive entry types and no symlink entries. |
| `kernel/market` | `Pack.Validate` → `safeRelPath` (`market.go:186-199`, rejects `\`, leading `/`, and any `.`/`..`/empty segment) *and* the `BundleStore.cleanRel` layer underneath. |
| `cmd/agt/backup.go` restore | Subtree allowlist + `..` rejection + prefix-with-separator check + `O_EXCL` + non-regular entries skipped (`:483-514`). |
| `plugins/tools/codeexec` tar extraction | `sanitizeRelFile` (`runtimes.go:197-211`) rejects abs, `..`, NUL, and colon; symlink entries fall to `default: continue`. |
| `kernel/datalake`, `kernel/tenant`, `kernel/settings`, `kernel/resume`, `kernel/configcenter` | Slug regexes / map-membership gates before every path join; `tenant.baseDir` additionally re-asserts `filepath.Dir(dir) == root`. |
| `kernel/webui/rollback.go` | Apply takes an `id` only; `abs_path` comes from the daemon-written catalog, with `Lstat` symlink + directory refusals before write/remove. |
| `kernel/webui/artifact_route.go` | Ref-addressed, mime allowlist, SVG sandboxed by CSP, `sanitizeFilename` for `Content-Disposition`. |
| `plugins/tools/browser` session/artifact paths | `sessionDir` (`action.go:469-491`) and `isBrowserActionTempPath` (`:1001-1016`) both use `filepath.Rel` + `..` rejection. |

## Summary

| ID | Severity | Confidence | Area |
|---|---|---|---|
| SSRF-001 | High | 92 | `browser.action` → Playwright driver, unguarded |
| SSRF-002 | Medium | 95 | mcpbridge SSE POST, documented dialer guard absent |
| SSRF-003 | Low | 70 | 6to4 embedded v4 + kernel/bridge classifier drift |
| PATH-001 | Medium | 90 | webui Files: intermediate-directory symlink escape |
| PATH-002 | Low | 85 | Windows no-follow prefix check, no separator boundary |
| PATH-003 | Low | 90 | nil-deref panic in file-tree handler |

The unifying theme of the two real findings: **AGEZT's egress and containment controls are sound where they are enforced at the syscall boundary (netguard's `Dialer.Control`, the file tool's `EvalSymlinks` + `O_NOFOLLOW`), and unsound wherever a check-then-hand-off pattern was substituted** — a DNS resolution handed to an out-of-process browser (SSRF-001), a one-shot host classification handed to an unguarded `http.Client` (SSRF-002), a lexical join handed to `open`/`remove` (PATH-001). In all three cases the code's own comments describe the stronger guarantee that was intended.

# Dependency / Supply-Chain Audit — AGEZT

**Scope:** Go backend (`go.mod`/`go.sum`), React frontend (`frontend/package.json` + lockfile), TypeScript SDK (`sdk/typescript/package.json` + lockfile).
**Method:** Static reading of manifests + lockfiles. No live CVE-DB access; vulnerability claims rely on training knowledge and are confidence-tagged. "Vulnerable" is only used where a specific pinned version maps to a real known issue; otherwise "review / outdated".
**Date:** 2026-06-23

---

## Executive summary

The dependency surface is **unusually small and clean** for an application of this size, which is a strong supply-chain posture overall:

- **Go:** only 4 direct + 6 indirect modules. **No `replace` directives, no `toolchain` override, no local/forked paths.** No 3rd-party crypto/JWT/YAML/HTTP-router/templating libs are vendored — the app relies on the Go standard library for TLS, JSON, HTTP, and (apparently) any JWT/crypto work. This sharply reduces supply-chain exposure. The only notable point is an **unreleased beta IMAP library** (`go-imap/v2 v2.0.0-beta.8`).
- **Frontend:** A **bleeding-edge, latest-everything** stack (React 19, Vite 8 / Rolldown 1.0, Tailwind 4 / oxide, TypeScript 6, Vitest 4, jsdom 29). All deps are very recent — the risk here is *immaturity/churn*, not known CVEs. An explicit `undici@^7.28.0` override is in place (good hygiene). No typosquats found.
- **SDK:** Minimal — zero runtime dependencies, only `typescript` + `@types/node` as devDeps. Essentially no supply-chain surface.

**No high-confidence known-vulnerable pinned versions were identified in any ecosystem.** The most material concerns are (1) the IMAP beta, (2) reliance on very new/just-released build tooling (Rolldown 1.0.3, Vite 8, Tailwind oxide) that has had little time to accumulate audits, and (3) the usual caution that the app does its own crypto/auth on the stdlib — that should be verified in the code-level phases, since there is no battle-tested 3rd-party JWT/crypto lib to lean on.

---

## 1. Go ( `go.mod` / `go.sum` )

**Go directive:** `go 1.26.4` — current/modern. **No separate `toolchain` directive** (so the toolchain is not pinned above the language version; builds use whatever local toolchain >= 1.26.4 is present). **No `replace` directives** — clean, no forks or local-path redirects. Good.

### Direct dependencies

| package | version | role | issue | severity | confidence | recommendation |
|---|---|---|---|---|---|---|
| github.com/btcsuite/btcd/btcec/v2 | v2.5.0 | secp256k1 crypto (ECDSA/Schnorr) | Security-relevant (crypto). v2.5.0 is recent; no known CVE for this version. | Info | High | Keep current; track btcec advisories since this is a crypto primitive. |
| github.com/coder/websocket | v1.8.15 | WebSocket transport | Maintained fork of nhooyr/websocket (now `coder/websocket`). Current. No known CVE. | Info | High | OK. Verify origin checking / message-size limits at code level. |
| github.com/emersion/go-imap/v2 | **v2.0.0-beta.8** | IMAP client (email channel) | **Pre-release beta** of a v2 rewrite. Parses untrusted server/network input. Beta = unstable API + less hardening; pre-1.0 means no stability/security guarantees. | **Medium** | High | Pin tightly and watch for the stable v2 release; this is the highest-churn security-relevant dep. Treat IMAP-parsed data as untrusted in code. |
| lukechampine.com/blake3 | v1.4.1 | BLAKE3 hashing | Current. No known CVE. | Info | High | OK. |

### Indirect dependencies

| package | version | role | issue | severity | confidence | recommendation |
|---|---|---|---|---|---|---|
| github.com/btcsuite/btcd/chainhash/v2 | v2.0.0 | hashing for btcec | transitive; current | Info | High | OK |
| github.com/decred/dcrd/crypto/blake256 | v1.1.0 | crypto (blake256) | transitive of btcec; current | Info | High | OK |
| github.com/decred/dcrd/dcrec/secp256k1/v4 | v4.4.1 | secp256k1 impl | transitive crypto; current | Info | High | OK |
| github.com/emersion/go-message | v0.18.2 | MIME message parsing | Parses untrusted email MIME (attachments, headers). v0.18.2 is current; no known CVE. MIME parsers are a historical attack surface (decompression/encoding). | Low–Med | Med | Keep current; enforce size/recursion limits when handling parsed messages. |
| github.com/emersion/go-sasl | v0.0.0-2024…b788ff22d5a6 | SASL auth | Pseudo-version (commit-pinned, no tagged release). Common for emersion libs; not a fork. | Low | High | Acceptable; prefer a tagged release if/when published. |
| github.com/klauspost/cpuid/v2 | **v2.0.9** | CPU feature detection | **Notably old** (v2.0.9; current line is v2.2.x). Pulled in transitively (likely via blake3). Not security-critical (CPU detection), but stale. | Low | High | Bump via `go get -u` of the parent; v2.2.x has bugfixes. No known security impact. |

**`go.sum` transitive observations:**
- Old `golang.org/x/crypto`, `x/net`, `x/sys`, `x/text`, `x/tools` entries (e.g. `x/text v0.3.x`, `x/net 2021/2022 pseudo-versions, `x/crypto 2019/2021) appear in `go.sum` but **only as `/go.mod` hash lines** (no `h1:` content hashes) — meaning they are pulled in for module-graph resolution but **are not actually compiled into the build**. So the known-vulnerable old `x/text`/`x/net`/`x/crypto` lines are **not a live risk** here; flagged only so a future direct import doesn't silently inherit a stale floor.
- No YAML, no XML, no JWT, no templating, no archive (zip/tar), no 3rd-party HTTP router/client modules present → those concerns map to **stdlib usage**, to be confirmed in code-level phases (zip-slip, TLS config, JWT signing-alg confusion, etc. would live in app code, not deps).

---

## 2. Frontend ( `frontend/package.json` + `package-lock.json`, lockfileVersion 3 )

**Overall:** Deliberately "latest everything" (per project memory). All direct deps are at or near current major versions. The risk profile is **newness/churn**, not known CVEs. ~234 resolved packages; no typosquats detected (`obug@2.1.2` looks suspicious by name but is a legitimate `debug`-style logger by `sxzz`, a transitive of the Rolldown/oxc toolchain — funded, real package, not a typosquat). An `overrides` block forces `undici@^7.28.0` (resolved 7.28.0) — good, this pre-empts older-undici advisories pulled via jsdom.

### Direct dependencies (runtime)

| package | version | issue | severity | confidence | recommendation |
|---|---|---|---|---|---|
| @fontsource-variable/inter | ^5.2.8 | font assets only | Info | High | OK |
| @radix-ui/react-dropdown-menu | ^2.1.17 | UI primitive | Info | High | OK |
| @radix-ui/react-scroll-area | ^1.2.11 | UI primitive | Info | High | OK |
| @radix-ui/react-tabs | ^1.1.14 | UI primitive | Info | High | OK |
| @radix-ui/react-tooltip | ^1.2.9 | UI primitive | Info | High | OK |
| @xyflow/react | ^12.11.0 (12.11.0) | flow canvas; pulls d3-* + zustand | Info | High | OK; current. |
| class-variance-authority | ^0.7.1 | styling util | Info | High | OK |
| clsx | ^2.1.1 | classnames util | Info | High | OK |
| lucide-react | ^1.18.0 | icons | review/outdated-claim N/A — note this is a **post-1.0 lucide-react**, unusually high major; confirm it resolves to the intended package (it is the legit icon lib, recently versioned). | Info | Med | OK; verify on install. |
| react | ^19.2.7 | framework | Info | High | OK (React 19, current). |
| react-dom | ^19.2.7 | framework | Info | High | OK |
| tailwind-merge | ^3.6.0 | styling util | Info | High | OK |

### Direct dependencies (dev / build — these run in CI and on dev machines, so still supply-chain relevant)

| package | version | issue | severity | confidence | recommendation |
|---|---|---|---|---|---|
| vite | ^8.0.16 (8.0.16) | **Very new major (Vite 8) using Rolldown** as bundler (`rolldown@1.0.3`, `@rolldown/binding-*` native). Vite has had several past dev-server CVEs (`server.fs.deny` bypass / path traversal, CVE-2024-/2025- series) — those were patched in earlier majors; v8 is post-fix. Risk here is *maturity of the brand-new Rolldown path*, not a known CVE. | Low–Med | Med | Keep patched; **never expose the Vite dev server on a public interface** (`server.host`/CORS). Watch Vite 8 advisories closely given its newness. |
| rolldown (transitive of vite) | 1.0.3 | Rust bundler at its **first stable (1.0.x)** release. Little audit history; native binaries per-platform. | Low–Med | Med | Acceptable but immature; pin via lockfile (already is). |
| @vitejs/plugin-react | ^6.0.2 | build plugin | Info | High | OK |
| @tailwindcss/vite + tailwindcss | ^4.3.1 | Tailwind 4 (oxide, native `@tailwindcss/oxide-*` + lightningcss native bins) | New major with native binaries; large platform-binary fan-out increases the number of distinct artifacts trusted at install. No known CVE. | Low | Med | OK; rely on lockfile integrity hashes (present). |
| typescript | ^6.0.3 | **TypeScript 6** — very new major. Build-time only. | Info | High | OK |
| vitest | ^4.1.8 | test runner (new major v4) | Info | High | OK |
| jsdom | ^29.1.1 (29.1.1) | DOM impl for tests; pulls `tough-cookie@6.0.1`, `undici@7.28.0`, `parse5`, `whatwg-url`. Historically jsdom/tough-cookie had ReDoS/prototype issues in **old** versions; these resolved versions are current and **not** the vulnerable ones. | Low | High | OK; override keeps undici current. |
| @playwright/test / playwright | ^1.60.0 (1.60.0) | e2e; downloads browser binaries | Info | High | OK; ensure browser downloads come from official CDN. |
| @testing-library/dom | ^10.4.1 | test util | Info | High | OK |
| @testing-library/react | ^16.3.2 | test util | Info | High | OK |
| @types/node | ^25.9.3 | types only | Info | High | OK |
| @types/react / @types/react-dom | ^19.2.x | types only | Info | High | OK |

### Notable transitive packages (frontend)

| package | version | note | severity | confidence |
|---|---|---|---|---|
| undici | 7.28.0 | forced via `overrides`; current. Past undici had CVEs (proxy-auth leak, integrity bypass) in **<5.28 / <6.x** — not applicable here. | Info | High |
| tough-cookie | 6.0.1 | current; the prototype-pollution CVE (CVE-2023-26136) affected **<4.1.3** — not applicable. | Info | High |
| nanoid | 3.3.12 | the predictable-ID issue (CVE-2024-/GHSA) affected **<3.3.8** — 3.3.12 is patched. | Info | High |
| postcss | 8.5.15 | the line-return parsing CVE (CVE-2023-44270) affected **<8.4.31** — patched here. | Info | High |
| d3-* (color/zoom/etc.) | current via @xyflow | old `d3-color` had a ReDoS (GHSA) in very old versions; xyflow pulls current. | Info | Med |
| zustand (via @xyflow) | — | state lib; no concern | Info | High |
| esbuild | peer range `^0.27 || ^0.28` | not separately pinned (Vite 8 uses rolldown/oxc, esbuild is an optional/peer path). The esbuild dev-server CORS issue (GHSA-67mh, fixed 0.25.0) is moot at these versions. | Info | Med |

No `git+`/`http(s)` URL deps, no `file:`/local-path deps, no GitHub-tarball deps in the frontend lockfile → all from the npm registry with integrity hashes. Good.

---

## 3. TypeScript SDK ( `sdk/typescript/package.json` + lockfile )

**Overall:** **Cleanest of the three.** `@agezt/sdk@1.1.0`, `engines.node >=18`, **zero runtime dependencies** — it ships only `dist/src` and uses the Node built-in test runner. The only deps are devDeps.

| package | version (resolved) | issue | severity | confidence | recommendation |
|---|---|---|---|---|---|
| typescript | ^5.7.2 (5.9.3) | build-only; current 5.x | Info | High | OK |
| @types/node | ^22.10.0 (22.19.20) | types-only; pulls `undici-types@6.21.0` (types only, no runtime undici) | Info | High | OK |
| undici-types | 6.21.0 | **types only**, no executable code; not the undici runtime | Info | High | OK |

**Supply-chain note:** SDK declares no runtime deps, so a consumer installing `@agezt/sdk` pulls **no transitive runtime code** — minimal blast radius. `repository` points at the canonical `github.com/agezt/agezt`. No concerns.

---

## Cross-ecosystem recommendations (priority order)

1. **Go IMAP beta (`go-imap/v2 v2.0.0-beta.8`)** — Medium. Highest-risk single dep: pre-release, parses untrusted network/email input. Track for the stable v2 release; ensure code applies size/recursion limits to parsed IMAP/MIME data.
2. **Bump `klauspost/cpuid/v2` off v2.0.9** — Low. Stale transitive; refresh with `go get -u`.
3. **Treat Vite 8 / Rolldown 1.0 / Tailwind-oxide as "new and watch"** — Low–Med. No known CVEs, but first-stable Rust toolchains with large native-binary fan-out. Keep the lockfile authoritative (it has integrity hashes), keep deps patched, and never expose the Vite dev server publicly.
4. **Verify app-level crypto/auth in code phases** — the app vendors *no* 3rd-party JWT/TLS/YAML/XML/zip libs, so those classic dependency risks instead live in hand-written code on the Go stdlib. The supply chain is clean precisely because the app does this itself — which shifts the audit burden to the SAST/code-review phases (JWT alg confusion, TLS config, zip-slip in any stdlib `archive/zip` usage, SSRF in stdlib `net/http` clients).
5. **Keep the `undici@^7.28.0` override** — good existing hygiene; don't drop it.

**No `replace` directives, forked sources, local-path deps, git/tarball deps, or typosquats were found in any ecosystem.**

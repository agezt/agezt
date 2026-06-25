# Dependency Audit — AGEZT

Read-only supply-chain / dependency review of `go.mod`/`go.sum` and `frontend/package.json`/`package-lock.json`.

**Date:** 2026-06-24
**Scope:** declared direct + indirect dependencies, replace/git/non-canonical sources, lockfile integrity, known-CVE version flagging.

---

## Executive summary

This is a **deliberately lean dependency tree** — one of the strongest aspects of the project's security posture.

- **Go:** only **4 direct** + 6 indirect modules. No web framework, no JWT library, no `gorilla/*`, no `yaml.v2`, no ORM. The HTTP server, control-plane protocol, crypto and auth are all in-tree (`kernel/*`, `plugins/*`), which removes whole classes of third-party CVE exposure (but shifts the risk to the in-house code — out of scope for this phase).
- **Frontend:** modern, recently bumped to bleeding-edge (React 19, Vite 8, TS 6, Tailwind 4, lucide 1). All sources are the canonical npm registry; no git deps.
- **No `replace =>` directives, no `toolchain` pin, no vendored/forked modules, no git-based or non-canonical sources** in either ecosystem.

There are **no high-confidence known-CVE versions** in the current dependency set. The concerns below are mostly **pre-release/beta pins** and **maintenance/freshness** items, not active CVEs.

---

## 1. Go — version & direct dependencies

**Go toolchain declared:** `go 1.26.4` (modern; well past the EOL'd 1.19/1.20 lines that carried the bulk of stdlib CVEs). No separate `toolchain` directive, so the build uses whatever local/CI Go satisfies `>=1.26.4`. **Recommendation:** the security-relevant surface for a Go project is largely the **stdlib version of the building toolchain** — ensure CI builds with a patched 1.26.x (Go security fixes ship in the toolchain, not in these modules). Confidence: high.

### Direct (`require` block 1)

| Module | Version | Notes / Risk | Remediation |
|---|---|---|---|
| `github.com/btcsuite/btcd/btcec/v2` | v2.5.0 | secp256k1 ECDSA/Schnorr. Maintained, widely used. No known CVE at this version. Used presumably for Ed25519/secp signature verify (update-signing / agentgw). | None required; keep current. Confidence: med-high. |
| `github.com/coder/websocket` | v1.8.15 | The maintained successor to `nhooyr.io/websocket` (renamed). Actively maintained, no known CVE. **Good choice** — notably *not* `gorilla/websocket` (which was archived for a period and has had advisories). | None. Confidence: high. |
| `github.com/emersion/go-imap/v2` | **v2.0.0-beta.8** | **Pre-release/beta pin.** v2 of go-imap is still beta; API and security fixes can land without semver guarantees. IMAP parsing handles attacker-influenced (mailbox) data, so parser bugs here are reachable from the email channel. | Track go-imap v2 releases; move to a stable v2.0.0 when published, and re-pin promptly on beta bumps. Confidence: med (risk is "beta, not stabilized" rather than a specific CVE). |
| `lukechampine.com/blake3` | v1.4.1 | BLAKE3 hashing. Small, maintained, no known CVE. | None. Confidence: high. |

### Indirect

| Module | Version | Notes |
|---|---|---|
| `github.com/btcsuite/btcd/chainhash/v2` | v2.0.0 | transitive of btcec. Fine. |
| `github.com/decred/dcrd/crypto/blake256` | v1.1.0 | transitive (secp256k1). Fine. |
| `github.com/decred/dcrd/dcrec/secp256k1/v4` | v4.4.1 | the actual secp256k1 impl behind btcec. Maintained. Fine. |
| `github.com/emersion/go-message` | v0.18.2 | MIME/message parsing — **attacker-influenced input** (email bodies/headers). Maintained; no known CVE at 0.18.2, but parser-class risk. Keep updated. Confidence: med. |
| `github.com/emersion/go-sasl` | v0.0.0-20241020182733 (pseudo-version) | SASL auth. Pseudo-version = pinned to a commit, no tagged release. Common for this module; acceptable but means no semver. Confidence: med. |
| `github.com/klauspost/cpuid/v2` | **v2.0.9** | Noticeably **old** (2021-era) relative to the rest of the tree (current line is v2.2.x). Pulled in transitively (likely via blake3). Low security impact (CPU feature detection), but it's the one clearly stale module. | Allow `go get -u` to advance it to v2.2.x. Confidence: med. |

**Not present (positive findings):** no `golang.org/x/crypto` or `golang.org/x/net` in the *required build* (they appear in `go.sum` only as historical `/go.mod` hashes from transitive graph resolution — see §4), no `dgrijalva/jwt-go` / `golang-jwt`, no `gorilla/*`, no `gopkg.in/yaml.v2`, no `text/template`-driven web libs. This avoids the classic Go CVE cluster.

---

## 2. Frontend — direct dependencies (`package.json`)

Project memory notes a recent "her şey son sürüm" bump to latest (Vite 8 / TS 6 / lucide 1). Confirmed — the tree is at or near the bleeding edge.

### Runtime dependencies

| Package | Range | Notes / Risk | Remediation |
|---|---|---|---|
| `react` / `react-dom` | ^19.2.7 | React 19, current. No known CVE. | None. |
| `@xyflow/react` | ^12.11.0 | React Flow (Flow Studio canvas). Maintained. No known CVE. | None. |
| `@radix-ui/react-*` (dropdown/scroll-area/tabs/tooltip) | ^2.1.17 / ^1.x | Maintained, security-clean. | None. |
| `lucide-react` | ^1.18.0 | Icons. v1 line (very new). Cosmetic, negligible security surface. | None. |
| `@fontsource-variable/inter` | ^5.2.8 | Self-hosted font (good — avoids 3rd-party CDN/font exfil). | None. |
| `class-variance-authority` | ^0.7.1 | CVA. Pre-1.0 but stable and tiny. | None. |
| `clsx` | ^2.1.1 | Tiny classname util. Fine. | None. |
| `tailwind-merge` | ^3.6.0 | Fine. | None. |

### Dev dependencies (build/test only — not shipped to the embedded SPA bundle)

| Package | Range | Notes |
|---|---|---|
| `vite` | ^8.0.16 | Vite 8, very new. **Caret range** means CI can float within v8. Historically Vite has had **dev-server** advisories (path traversal / `server.fs.deny` bypass, e.g. CVE-2025-3072x series in v5/v6) — these affect the **dev server only**, not the `go:embed`-ded production build, so impact is low for shipped artifacts. Keep current. Confidence: med. |
| `vitest` | ^4.1.8 | v4, current. Test-only. Past `vitest`/`vite-node` advisories were dev-server RCE via the API server — test-only exposure. | 
| `typescript` | ^6.0.3 | TS 6. Build-only. No security surface. |
| `tailwindcss` / `@tailwindcss/vite` | ^4.3.1 | Tailwind 4. Build-only. |
| `@vitejs/plugin-react` | ^6.0.2 | Build-only. |
| `@playwright/test` | ^1.60.0 | E2E, dev-only. |
| `@testing-library/*`, `jsdom ^29`, `@types/*` | — | Test/types only. `jsdom` historically had ReDoS/prototype-pollution advisories in old majors; v29 is current. Test-only. |

### `overrides`

```json
"overrides": { "undici": "^7.28.0" }
```
**Positive finding.** This forces `undici` to `^7.28.0` across the transitive tree (the lock shows a transitive `undici: ^7.25.0` dependency at line 2505). `undici` has had multiple CVEs in older majors (proxy-auth leak, decompression DoS, CRLF). Pinning to a recent v7 is a **deliberate, correct supply-chain hardening move**. Confidence: high.

---

## 3. Replace / git / forked / vendored code

- **`go.mod`:** **No `replace =>` directives, no `exclude`, no `toolchain` pin, no `// vendored` modules.** No forked/local code hidden behind a replace. Clean. Confidence: high.
- **`frontend/package-lock.json`:** every `"resolved"` points at `https://registry.npmjs.org/`. **No `git+https`, `git+ssh`, `git://`, `github:`, or `codeload` sources, no `file:` links.** All deps come from the canonical registry. Confidence: high.

No vendored directory (`/vendor`) is referenced in the build path for hidden third-party Go code.

---

## 4. Lockfile integrity & non-canonical sources

- **`go.sum`:** present and consistent with `go.mod`. Every required module has both an `h1:` (zip) and `/go.mod` hash. The extra `golang.org/x/*` and `yuin/goldmark` entries (lines 21–51) are **`/go.mod`-only hashes** (no `h1:` zip hash) — these are *graph-resolution residue* (modules consulted during MVS but **not in the final build set**), which is normal and **not** a sign those packages are linked into the binary. Their absence of `h1:` lines confirms they aren't downloaded/built. No integrity mismatch observed. Confidence: high.
- **`package-lock.json`:** `lockfileVersion: 3` (npm 7+), with integrity hashes. Sources canonical (§3). No anomalies. Confidence: high.
- **Recommendation:** enforce reproducible installs in CI — `go mod verify` + `go build -mod=readonly` (or `-mod=mod` discipline), and `npm ci` (not `npm install`) so the lockfile is the source of truth and floating caret ranges don't silently advance build inputs.

---

## 5. Unmaintained / archived dependencies

- **None positively identified as archived.** Notably the project **avoided** the common archived/abandoned traps: it uses `coder/websocket` (the maintained rename) rather than the formerly-archived `gorilla/websocket`, and uses no `dgrijalva/jwt-go` (deprecated).
- **Watch items (maintenance, not abandonment):**
  - `github.com/klauspost/cpuid/v2 @ v2.0.9` — clearly stale relative to the tree; advance to v2.2.x.
  - `github.com/emersion/go-imap/v2 @ beta.8` and `go-sasl @ pseudo-version` — pre-stable pins on the **email-channel parsing path**, which processes attacker-influenced input. Highest-priority freshness items because of reachability, even absent a named CVE.

---

## Prioritized remediation list

| # | Item | Version | Risk | Action | Confidence |
|---|---|---|---|---|---|
| 1 | `emersion/go-imap/v2` | v2.0.0-**beta.8** | Beta parser on attacker-influenced email input; no semver/security guarantees | Track releases; move to stable v2.0.0 when published; re-pin promptly on beta bumps | med |
| 2 | `emersion/go-message` | v0.18.2 | MIME parser on attacker input | Keep current; subscribe to advisories | med |
| 3 | Go toolchain | build-time | Stdlib CVEs ship in the toolchain, not modules | Ensure CI builds with a patched **1.26.x**; consider adding a `toolchain` directive to pin a known-good patched version | high |
| 4 | `klauspost/cpuid/v2` | v2.0.9 | Stale (2021); low security impact | `go get -u` → v2.2.x | med |
| 5 | `vite` / `vitest` (dev) | ^8 / ^4 | Dev-server advisory class (path traversal/RCE) — **dev only**, not shipped | Keep current; never expose the Vite dev server beyond localhost | med |
| 6 | CI install discipline | — | Floating caret ranges + `npm install` can drift build inputs | Use `npm ci` + `go build -mod=readonly` + `go mod verify` | high |

**No action items rated high-severity** — there is no high-confidence known-CVE version in the current build set. The strongest residual exposure is the **beta email-parsing stack (#1/#2)** because of input reachability.

---

*End of dependency audit. CVE judgments are based on version knowledge only (no network/scanner run); confidence is marked per finding.*

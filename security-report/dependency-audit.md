# SC: Dependency Audit — AGEZT

> Scanner: `sc-dependency-audit` (security-check pipeline, Phase 1)
> Repo root: `D:\Codebox\PROJECTS\AGEZT`
> Method: **static manifest + lock-file analysis** (no `npm audit` / network advisory calls).
> CVE/version flags are **based on version heuristics — verify against a live advisory DB
> (osv.dev, GitHub Advisories, govulncheck) before acting.**

## Scope

Manifests audited (worktree copies under `.claude/worktrees/*` and `.worktrees/*` and
all `node_modules/**/package.json` were **excluded** — only canonical repo manifests):

| Ecosystem | Manifest(s) | Lock file(s) |
|---|---|---|
| Go | `go.mod`, `go.sum` (single root module, **no nested plugin modules**) | `go.sum` |
| Node/TS (webui) | `frontend/package.json` | `frontend/pnpm-lock.yaml` **and** `frontend/package-lock.json` (both present) |
| Node/TS (SDK) | `sdk/typescript/package.json` | `sdk/typescript/package-lock.json` |
| Python (SDK) | `sdk/python/pyproject.toml` | none (zero deps) |
| Rust (SDK) | `sdk/rust/Cargo.toml` | `sdk/rust/Cargo.lock` (zero deps) |
| Docs | `DEPENDENCIES.md` (justified-deps policy doc) | — |

Notable absence: no root `package.json`, no `requirements.txt`/`Pipfile`, no `.npmrc`/`pip.conf`.

---

## Severity-tagged findings

| ID | Severity | Conf. | Ecosystem | Package / Artifact | Type | Finding |
|---|---|---|---|---|---|---|
| DEP-001 | **Medium** | 85 | npm | `frontend/` — dual lock files | Supply-chain integrity | Both `pnpm-lock.yaml` AND `package-lock.json` are committed for the same `package.json`. Two sources of truth diverge over time; CI/devs using different package managers resolve different trees. Pick one (the repo builds with pnpm — lockfileVersion 9.0) and delete the other. |
| DEP-002 | **Medium** | 80 | npm | `undici` override `^7.28.0` | Lock drift / unpinned | `frontend/package.json` declares `overrides.undici: ^7.28.0`, but `pnpm-lock.yaml` resolved `undici@7.27.2` and contains **no `overrides:` block**. The lock predates the override → the security-motivated pin is **not actually enforced** in the installed tree. Run `pnpm install` to re-resolve and commit the updated lock. (undici <7.x has had SSRF/redirect CVEs; the intent of the pin is good but currently ineffective.) |
| DEP-003 | **Low–Medium** | 70 | npm | `typescript ^6.0.3`, `vite ^8.0.16`, `vitest ^4.1.8`, `@vitejs/plugin-react ^6.0.2`, `@types/node ^25.9.3` (frontend) / `^26.0.1` (sdk), `jsdom ^29.1.1` | Bleeding-edge / recently-bumped dev-deps | TS 6.0, Vite 8, Vitest 4, plugin-react 6, @types/node 25/26 are **very new major versions** (well ahead of the typical stable line at audit time). The prompt's noted "@types/node 22→26, typescript 5.9→6.0" bumps land here. All are **dev/build-only** (compiled into the static `dist/` bundle; Node never runs at AGEZT runtime), so runtime exposure is low — but new majors widen the window for a malicious-release / regression. Pin exact versions (drop `^`) for build reproducibility and watch advisories. |
| DEP-004 | **Low** | 75 | npm | `@types/node` skew: `^25.9.3` (frontend) vs `^26.0.1` (sdk/typescript) | Version inconsistency | Two different `@types/node` majors across the two TS projects. Harmless functionally but a maintenance smell; align them. |
| DEP-005 | **Low** | 90 | npm | `lucide-react ^1.18.0` | Version heuristic | `lucide-react` historically tracked `0.x`. A `1.x` line is unusual — **verify this is the genuine `lucide-react` at the resolved version and not a republished/typosquat artifact.** (No url/git dep was found in the lock, which is reassuring; resolution should be checked against the npm registry record.) |
| DEP-006 | **Low** | 88 | Go | `github.com/emersion/go-imap/v2 v2.0.0-beta.8` | Pre-release / unpinned-quality | A `beta` dependency is a **direct, compiled** dep (email channel inbound IMAP). Beta APIs/behaviour can change and may carry unaudited parsing code paths (IMAP/MIME parsing is attack-surface for malicious mail). Track for a stable release; treat untrusted mail input defensively. |
| DEP-007 | **Low** | 92 | Go | `github.com/klauspost/cpuid/v2 v2.0.9` | Outdated transitive | Pulled in by `lukechampine.com/blake3` for SIMD detection. v2.0.9 is **old** (the v2 line has advanced well past .0.9). Low risk (CPU-feature detection, pure-Go), but it lags. Bumps come for free via `go get -u lukechampine.com/blake3` / `go mod tidy`. |
| DEP-008 | **Info/Low** | 95 | Go | `golang.org/x/{net,crypto,text,sys,sync,term,tools,mod,xerrors}`, `testify`, `go-spew`, `go-difflib`, `goldmark`, `yaml.v3` | Graph-only — NOT built | These appear in `DEPENDENCIES.md` and `go.sum` but **`go.sum` holds only their `/go.mod` hashes, not module `h1:` hashes** → they are graph-only and **compiled into zero AGEZT binaries**. The often-flagged old `golang.org/x/net v0.6.0` / `golang.org/x/crypto` pseudo-versions are therefore **not a runtime exposure** here. No action needed beyond awareness; `go mod tidy` may prune some. |
| DEP-009 | **Info** | 99 | Go | `go 1.26.4` directive | Toolchain | `go.mod` requires Go 1.26.4. Ensure the build toolchain matches and is itself patched (Go std-lib CVEs are fixed by toolchain upgrades, independent of these modules). |

No **Critical** or **High** findings. No git/URL/`file:`/`link:` dependencies, no `postinstall`/`preinstall`/`prepare` lifecycle scripts, and no `requiresBuild`/`hasInstallScript`/`deprecated`/`patchedDependencies` markers were found in `frontend/pnpm-lock.yaml`.

---

## Per-ecosystem detail

### Go (core kernel + plugins) — cleanest ecosystem
- **Single module** `github.com/agezt/agezt`; no nested `go.mod`.
- **9 modules actually compiled** (have `h1:` hashes): `btcec/v2 v2.5.0`, `coder/websocket v1.8.15`, `chainhash/v2 v2.0.0`, `go-spew v1.1.1`, `dcrd/crypto/blake256 v1.1.0`, `dcrd/dcrec/secp256k1/v4 v4.4.1`, `go-imap/v2 v2.0.0-beta.8`, `go-message v0.18.2`, `go-sasl …b788ff22d5a6`, `cpuid/v2 v2.0.9`, `blake3 v1.4.1`. (`go-spew` is test-only.)
- **No `replace` directives** (the SKILL flags local/non-standard replaces — none here, good).
- All direct deps are pure-Go, MIT/ISC-compatible, and individually justified in `DEPENDENCIES.md`. `go.sum` integrity is intact (paired hashes present for built modules).
- Direct: 4. Indirect (manifest): 6. Graph-only/test (go.sum, not built): ~14.

### Node/TS — webui (`frontend/`)
- Build/dev tooling only ships as a static `go:embed`-ded `dist/` bundle; **Node is never executed at AGEZT runtime**, which substantially lowers the blast radius of dev-dep CVEs.
- Both lock files resolve **234 packages** (≈471 snapshot lines in pnpm-lock). Runtime deps are React 19, Radix UI, `@xyflow/react`, `clsx`, `tailwind-merge`, `class-variance-authority`, `lucide-react`, `@fontsource-variable/inter`.
- Build chain: Vite 8 / Rollup / esbuild (`@esbuild/win32-x64@0.25.12` seen on disk — **≥0.25.0, so NOT affected by the esbuild dev-server CORS advisory GHSA-67mh-4wv8-2f99**), `lightningcss 1.32.0`, `nanoid 3.3.12` (≥3.3.8, not the predictable-ID CVE).
- Concerns: DEP-001/002/003/004/005 above.

### Node/TS — SDK (`sdk/typescript/`)
- **Zero runtime dependencies.** Dev-only `@types/node ^26.0.1` + `typescript ^6.0.3`. `engines.node >=18`. Lowest-risk Node surface. (Version skew vs frontend — DEP-004.)

### Python SDK (`sdk/python/`)
- `dependencies = []` — **stdlib only** (urllib+json). `requires-python >=3.9`, `setuptools>=61` build backend (standard, no custom build scripts). No risk.

### Rust SDK (`sdk/rust/`)
- `Cargo.toml` empty `[dependencies]`; `Cargo.lock` contains only the crate itself. **Zero deps, no `build.rs`.** No risk.

### Licenses
- Project + all four SDKs are MIT. Go deps are MIT/ISC. No GPL/AGPL copyleft, no unlicensed or proprietary deps observed. The npm graph was not exhaustively license-scanned (234 transitive); React/Radix/Vite ecosystem is overwhelmingly MIT — no obvious copyleft, but a full `license-checker` pass is recommended for completeness.

### Typosquat / dependency-confusion
- No `.npmrc`/registry config and no scoped→public confusion vectors found. SDK packages are correctly scoped (`@agezt/sdk`). Only typosquat-worth-verifying item is `lucide-react@1.x` (DEP-005), flagged for its unusual major version, not because of a name mismatch.

---

## Dependency Audit Summary

- **Total dependencies:** ~480 across all ecosystems
  - Go: ~24 in graph (direct 4, indirect-manifest 6, graph-only/test ~14) — **11 compiled, 9 prod**
  - npm webui: 234 resolved (direct 13 runtime + 13 dev)
  - npm SDK: 2 (dev-only)
  - Python SDK: 0
  - Rust SDK: 0
- **Ecosystems scanned:** Go, npm (×2 projects), PyPI, crates.io
- **Known vulnerabilities found (version-heuristic):** 0 Critical, 0 High, 2 Medium (DEP-001 integrity, DEP-002 ineffective security-pin), 6 Low/Info
- **Typosquatting risks:** 1 (DEP-005 — verify `lucide-react@1.x`)
- **Dependency-confusion risks:** 0
- **License concerns:** 0 obvious (full npm license sweep recommended)
- **Outdated dependencies:** 2 notable (DEP-007 `cpuid v2.0.9`; DEP-006 `go-imap beta.8`) + bleeding-edge dev majors (DEP-003)
- **Supply-chain hygiene wins:** no postinstall scripts, no git/url deps, no `replace` directives, no CGO in core, SDKs are zero-dep, Go core compiles only 9 prod modules.

## Recommended actions (priority order)
1. **DEP-002** — `pnpm install` to re-resolve so the `undici ^7.28.0` override actually applies, commit the refreshed lock.
2. **DEP-001** — delete whichever of `package-lock.json` / `pnpm-lock.yaml` is not the source of truth (keep pnpm).
3. **DEP-005** — confirm `lucide-react@1.x` provenance on the npm registry.
4. **DEP-003/004** — pin exact dev-dep versions; align `@types/node` across the two TS projects.
5. **DEP-006/007** — track `go-imap` stable release; `go get -u lukechampine.com/blake3 && go mod tidy` to refresh `cpuid`.
6. Run live `govulncheck ./...` (Go) and `pnpm audit` / `osv-scanner` (npm) when network is available to convert these heuristic flags into confirmed advisories.

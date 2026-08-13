# Dependency Audit — AGEZT

**Skill:** `sc-dependency-audit` (Phase 1b)
**Target:** `D:/Codebox/PROJECTS/AGEZT`
**Date:** 2026-08-13
**Scope:** `go.mod`/`go.sum`, `frontend/package.json` + lockfile, `sdk/typescript`, `sdk/python`, `sdk/rust`, `.github/workflows/*` + `.github/actions/*`, installers, and runtime-fetched package surfaces (MCP/ACP catalogs, Monaco CDN).

---

## Supply chain posture

**Strong on paper, with the real exposure sitting outside the manifests.**

The declared dependency footprint is genuinely minimal and unusually well governed: 5 direct Go modules behind a CI-enforced allowlist (`tools/depscheck`) and a written justification table (`DEPENDENCIES.md`); three SDKs with **zero** runtime dependencies (Python stdlib-only, Rust std-only, TypeScript platform-`fetch`-only); every GitHub Action pinned to a full commit SHA; `persist-credentials: false` on every checkout; workflow-wide `contents: read`; `--ignore-scripts` on every `npm ci`; checksum-verified staticcheck download; pinned `govulncheck`, `gitleaks`, `build`, `twine`. `go mod verify` passes and `govulncheck ./...` reports **no vulnerabilities** against the local go1.26.5 toolchain.

The risk is concentrated in three places the manifests do not describe:

1. **~43 unpinned third-party MCP/ACP packages** shipped as one-click presets, executed via `npx -y …@latest` / `uvx …` with daemon privileges. Four of them are **npm-deprecated / abandoned**. This is a far larger executable-code surface than the entire Go + npm dependency tree combined.
2. **All three SDK package names are unclaimed on their registries** while the README already instructs users to `pip install agezt` — a live, squattable dependency-confusion window.
3. **The two `overrides` pins in `frontend/package.json` have both gone stale** — each now resolves to exactly the top of a newly published vulnerable range, so the remediation they encode no longer remediates.

Offline/network note: `npm audit`, `govulncheck`, and registry metadata lookups all **succeeded** during this audit, so vulnerability findings below are evidence-backed, not inferred. `osv-scanner` is not installed locally. Items I could not confirm are explicitly marked.

---

## Dependency Audit Summary

- **Total dependencies: 337** (direct: 27, transitive: 310)
- **Ecosystems scanned:** Go, npm (frontend), npm (TS SDK), PyPI (Python SDK), crates.io (Rust SDK), GitHub Actions
- **Known vulnerabilities found: 4** (Critical: 0, High: 2, Medium: 2, Low: 0) — all npm, all in `frontend/`; **2 of the 4 are dev-tree only**
- **Typosquatting / name-squat risks: 3** (unclaimed SDK names on npm, PyPI, crates.io)
- **Dependency confusion risks: 2** (unclaimed publish targets; never-published preset package)
- **License concerns: 0 blocking** (12 MPL-2.0 + 1 dual MPL/Apache, no GPL/AGPL, no missing-license packages)
- **Outdated / abandoned dependencies: 5** (4 npm-deprecated MCP presets + 1 beta-grade Go parser)
- **Unpinned dependencies: ~43** (MCP/ACP catalog packages) + 1 CDN-loaded editor + 2 floating build-tool versions

### Per-ecosystem inventory

| Ecosystem | Manifest | Lock | Direct | Transitive | Total |
|---|---|---|---:|---:|---:|
| Go | `go.mod` | `go.sum` ✅ | 5 | 19 | **24** (`go list -m all`) |
| npm — frontend | `frontend/package.json` | `package-lock.json` v3 ✅ | 27 (13 prod / 14 dev) | 282 | **309** (76 prod, 230 dev, 88 optional, 4 peer) |
| npm — TS SDK | `sdk/typescript/package.json` | `package-lock.json` v3 ✅ | 2 (both dev) | 1 | **3** (0 runtime) |
| PyPI — Python SDK | `sdk/python/pyproject.toml` | *(none — no deps)* | 0 runtime, 1 build (`setuptools>=61`) | 0 | **0 runtime** |
| crates.io — Rust SDK | `sdk/rust/Cargo.toml` | `Cargo.lock` ✅ (self only) | 0 | 0 | **0** |
| GitHub Actions | `.github/workflows/*`, `.github/actions/*` | n/a | 4 distinct actions | — | **4** |

No missing lock files. No `replace` directives in `go.mod`. `GOPROXY=proxy.golang.org,direct`, `GOSUMDB=sum.golang.org`, no `GOPRIVATE`/`GONOSUMDB` bypass. No `.npmrc` anywhere in the repo (no alternate-registry or scope-redirect configuration to abuse).

---

## Findings

### DEP-001 — Shipped MCP/ACP presets execute ~43 unpinned third-party packages at runtime

- **Severity:** High
- **Confidence:** 95
- **Package:** ~43 npm/PyPI packages (see evidence)
- **Ecosystem:** npm + PyPI (runtime-fetched, not in any lockfile)
- **Vulnerability Type:** Unpinned dependency / Build-time (install-time) code execution
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-1104 (Use of Unmaintained Third Party Components)

**Description.** `frontend/src/views/Mcp.tsx` (lines ~160–205) ships a 43-preset MCP catalog, and `plugins/builtinmarket/builtinmarket.go` (lines 65–119) seeds market packs, both of which launch servers as `npx --yes <package>` or `uvx <package>`. Ten presets carry an explicit floating `@latest` tag (`@playwright/mcp@latest`, `tavily-mcp@latest`, `@supabase/mcp-server-supabase@latest`, `@azure/mcp@latest`, `@sentry/mcp-server@latest`, `slack-mcp-server@latest`, `redis-mcp-server@latest`, `awslabs.aws-documentation-mcp-server@latest`, …); the rest are untagged, which `npx`/`uvx` also resolve to latest. `kernel/acpcatalog/acpcatalog.go` adds `npm install -g` instructions for three agent CLIs.

`npx --yes` downloads **and runs npm lifecycle scripts** for whatever version is latest at that moment. Nothing in the repo pins a version, records a hash, or constrains the resolved artifact. Twelve of the names are **unscoped** (`airtable-mcp-server`, `slack-mcp-server`, `firecrawl-mcp`, `tavily-mcp`, `exa-mcp-server`, `chroma-mcp`, `arxiv-mcp-server`, `mongodb-mcp-server`, `redis-mcp-server`, `duckduckgo-mcp-server`, `excel-mcp-server`, `aws-documentation-mcp-server`), so they carry no scope-ownership protection; one (`@kimtaeyoon83/mcp-server-youtube-transcript`) sits in an individual maintainer's personal scope.

**Impact.** A single compromised or hijacked release of any of these packages executes arbitrary code as the AGEZT daemon user — the same process that holds provider API keys, the credential vault, and agent tool capabilities. Because resolution is `@latest`, the compromise lands on the *next* preset launch with no diff, no PR, and no lockfile change for an operator to review. The `--ignore-scripts` discipline correctly applied to CI `npm ci` is **not** applied to these runtime `npx` invocations.

**Remediation.** Pin every catalog preset to an exact version (`@playwright/mcp@1.2.3`, `uvx --from pkg==1.2.3`) and treat bumps as reviewed changes; consider recording expected integrity hashes. At minimum, surface the resolved version to the operator before first launch and drop `@latest` from all shipped defaults.

**References:** `frontend/src/views/Mcp.tsx`, `plugins/builtinmarket/builtinmarket.go`, `kernel/acpcatalog/acpcatalog.go`, `kernel/acpcatalog/registry.go:474-480`

---

### DEP-002 — All three SDK package names are unclaimed on their registries while the README tells users to install them

- **Severity:** High
- **Confidence:** 98
- **Package:** `agezt` (PyPI), `agezt` (crates.io), `@agezt/sdk` (npm)
- **Ecosystem:** PyPI / crates.io / npm
- **Vulnerability Type:** Dependency Confusion / Typosquatting
- **CWE:** CWE-427 (Uncontrolled Search Path Element), CWE-494

**Description.** Verified live against each registry during this audit:

| Name | Registry | Status |
|---|---|---|
| `agezt` | `https://pypi.org/pypi/agezt/json` | **HTTP 404 — unclaimed** |
| `agezt` | `https://crates.io/api/v1/crates/agezt` | **HTTP 404 — unclaimed** |
| `@agezt/sdk` | `https://registry.npmjs.org/@agezt%2fsdk` | **HTTP 404 — unclaimed** (org `agezt` also 404) |

Meanwhile `README.md:217` states ``pip install agezt``, `sdk/python/README.md:13` states `pip install agezt`, `sdk/rust/README.md:18` states `agezt = "1.0"`, and `sdk/typescript/README.md:15` states `npm install @agezt/sdk`. `.github/workflows/publish-sdks.yml` is wired to publish to exactly these names once the registry tokens are added.

**Impact.** PyPI and crates.io have no namespace scoping — anyone can register `agezt` on either **today**. A squatter's PyPI `agezt` runs arbitrary code at `pip install` time (setup.py / build backend) on the machine of every user who follows the published README. On npm the `@agezt` *organization* is likewise unregistered, so the scope offers no protection until claimed. The project would then be unable to publish under its own documented name without a registry dispute.

**Remediation.** Claim all three names immediately (a placeholder `0.0.0` release is enough): register the `agezt` npm org, publish a stub `agezt` to PyPI, and reserve `agezt` on crates.io. Until claimed, soften the README install lines to "not yet published — install from source". Consider also reserving obvious variants (`agezt-sdk`, `agezt_sdk`, `ageztai`).

**References:** `README.md:217`, `sdk/python/README.md:13`, `sdk/rust/README.md:18`, `sdk/typescript/README.md:15`, `.github/workflows/publish-sdks.yml`

---

### DEP-003 — Both `overrides` pins in `frontend/package.json` have gone stale and no longer remediate

- **Severity:** Medium
- **Confidence:** 100
- **Package:** `dompurify@3.4.12`, `undici@7.28.0`
- **Ecosystem:** npm
- **Vulnerability Type:** Known advisory (GHSA)
- **CWE:** CWE-79 (dompurify), CWE-444 / CWE-200 / CWE-93 (undici)

**Description.** `frontend/package.json` carries `"overrides": { "dompurify": "^3.4.11", "undici": "^7.28.0" }` — clearly added as prior remediations. Both caret ranges now resolve to the **exact top of a freshly published vulnerable range**. Confirmed by a live `npm audit` in `frontend/`:

| Package | Resolved | Advisory | Vulnerable range | Severity | Tree |
|---|---|---|---|---|---|
| `dompurify` | 3.4.12 | [GHSA-55q2-fjhq-7xh7](https://github.com/advisories/GHSA-55q2-fjhq-7xh7) — IN_PLACE hook removal leaves a detached subtree executable (XSS) | `<=3.4.12` | moderate | prod (via `monaco-editor`) |
| `monaco-editor` | 0.55.1 | depends on vulnerable `dompurify` | `>=0.54.0-dev-20250909` | moderate | prod |
| `undici` | 7.28.0 | GHSA-8xcm-r25x-g524, GHSA-4cwx-7wf7-3272, GHSA-m8rv-5g2x-5cg5, GHSA-jr45-8vmc-qm54, GHSA-v3r7-h72x-cjcm | `7.0.0 - 7.28.0` | high | **dev only** (via `jsdom`) |

`npm audit` reports `fixAvailable: true` for all four.

**Impact.** The `undici` advisories are **dev-tree only** (`jsdom` → vitest), so they affect the test harness, not the shipped daemon — real severity Low in this context. The `dompurify` advisory is nominally prod, but see DEP-004: `monaco-editor` is **not actually bundled** into `kernel/webui/dist`, so the vulnerable `dompurify` never reaches a browser today. The durable problem is the *pattern*: a hardcoded caret override silently stops protecting the moment a new advisory extends the range, and nothing in CI notices.

**Remediation.** Bump both overrides past the advisory ranges (`dompurify` `^3.4.13`+, `undici` `^7.29.0`+ — verify the fixed versions exist before pinning) and add `npm audit --audit-level=high` as a CI gate in the `frontend-test` job so a stale override fails loudly instead of silently.

**References:** live `npm audit` output, `frontend/package.json:50-53`

---

### DEP-004 — The code editor is loaded at runtime from a third-party CDN, at a version no lockfile governs

- **Severity:** Medium
- **Confidence:** 90
- **Package:** `monaco-editor@0.52.2` (jsdelivr CDN) vs `monaco-editor@0.55.1` (lockfile)
- **Ecosystem:** npm (CDN-delivered)
- **Vulnerability Type:** Unpinned/unverified remote code, Audit blind spot
- **CWE:** CWE-494, CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)

**Description.** `frontend/src/lib/monaco.ts:11-13` configures the Monaco AMD loader against `https://cdn.jsdelivr.net/npm/monaco-editor@0.52.2/min/vs`, deliberately keeping ~3 MB of editor code out of the `go:embed`-ed bundle. Confirmed: there is **no monaco chunk in `kernel/webui/dist/assets/`** (134 assets, none monaco) and no `dompurify`/`DOMPurify` string anywhere in the shipped bundle.

Two consequences:

1. **Audit blind spot.** The version npm audits (`0.55.1`, from the lockfile) is *not* the version that executes (`0.52.2`, from the CDN). `npm audit`, Dependabot, and the lockfile all govern a package that never ships, while the code that would actually run in the operator's browser is governed by nothing. The DEP-003 `dompurify` finding is an artifact of this: it describes a package that isn't deployed.
2. **Third-party runtime trust.** An AMD loader fetching many files dynamically cannot use Subresource Integrity, so a jsdelivr compromise (or a compromise of the upstream npm tarball it mirrors) would execute in a page holding an authenticated session to a machine-controlling agent daemon.

**Mitigating control (and the trap).** The daemon's CSP at `kernel/webui/webui.go:1316-1325` is `default-src 'none'; script-src 'self'; connect-src 'self'; …` — it does **not** allow `cdn.jsdelivr.net`. Reading the headers, the CDN script should be blocked outright, which neutralizes the supply-chain risk *and* means the Monaco editor almost certainly never loads in production (`MonacoView` degrades to its `<pre>` fallback). I did not runtime-verify this in a browser — **marked unverified**, but the header text is unambiguous.

The danger is the obvious "fix": whoever next debugs the broken editor will be tempted to widen `script-src`/`connect-src` to allow jsdelivr, converting a dormant risk into a live one.

**Remediation.** Self-host Monaco — vendor `monaco-editor/min/vs` into the build and point `paths.vs` at a same-origin path (the code comment at `lib/monaco.ts:6` already anticipates this). Do **not** widen the CSP. If Monaco is genuinely unused, drop `@monaco-editor/react` + `monaco-editor` from `dependencies` entirely, which also removes the phantom `dompurify`/`marked` prod entries.

**References:** `frontend/src/lib/monaco.ts:11-13`, `frontend/src/components/MonacoView.tsx:20-24`, `kernel/webui/webui.go:1311-1326`

---

### DEP-005 — Four npm-deprecated, abandoned MCP servers ship as one-click presets

- **Severity:** Medium
- **Confidence:** 100
- **Package:** `@modelcontextprotocol/server-github@2025.4.8`, `server-gdrive@2025.1.14`, `server-postgres@0.6.2`, `server-google-maps@0.6.2`
- **Ecosystem:** npm
- **Vulnerability Type:** Abandoned / unmaintained component
- **CWE:** CWE-1104

**Description.** Registry metadata fetched live confirms all four carry an npm **deprecation notice** ("Package no longer supported"), with last releases dating to early/mid 2025:

| Preset | `frontend/src/views/Mcp.tsx` | Latest | Deprecated |
|---|---|---|---|
| `github` | line 189 | 2025.4.8 | **yes** |
| `gdrive` | line 203 | 2025.1.14 | **yes** |
| `postgres` | line 179 | 0.6.2 | **yes** |
| `googlemaps` | line 176 | 0.6.2 | **yes** |

(By contrast `server-memory`, `server-filesystem`, `server-sequential-thinking`, `server-everything` are all current — 2026.7.x, not deprecated.)

**Impact.** These four will receive no security patches. Three of them are precisely the high-value credential sinks: the `github` preset is configured to receive `GITHUB_PERSONAL_ACCESS_TOKEN`, `gdrive` takes OAuth credentials, `postgres` takes a full connection string with embedded password. A vulnerability in any of them will never be fixed upstream, and an abandoned package is also a prime account-takeover target for a malicious republish.

**Remediation.** Replace with maintained equivalents (e.g. GitHub's own `github-mcp-server`) or remove the presets. This is the same "archived/never-published package" curation law the MCP catalog work already established — these four slipped past it.

**References:** live npm registry metadata, `frontend/src/views/Mcp.tsx:176,179,189,203`

---

### DEP-006 — Seeded market pack launches a package that does not exist on npm

- **Severity:** Low
- **Confidence:** 100
- **Package:** `@modelcontextprotocol/server-fetch`
- **Ecosystem:** npm
- **Vulnerability Type:** Never-published package reference
- **CWE:** CWE-1104

**Description.** `plugins/builtinmarket/builtinmarket.go:65` seeds a market pack that runs `npx -y @modelcontextprotocol/server-fetch`. That name returns **HTTP 404** on the npm registry — it has never been published. The correct artifact is the PyPI one, and `frontend/src/views/Mcp.tsx:164` already gets it right (`uvx mcp-server-fetch`, verified HTTP 200). So the Go-side seed and the UI catalog disagree, and the Go-side one is broken.

**Impact.** Functionally the preset simply fails. Security-wise the name is inside the `@modelcontextprotocol` scope, which upstream controls, so a third party **cannot** squat it — this is why the severity is Low rather than High. It remains a shipped reference to an unpublished name, which is the exact shape of a dependency-confusion foothold if the scope's ownership ever changes.

**Remediation.** Change `builtinmarket.go:65` to `uvx mcp-server-fetch`, matching the UI catalog.

**References:** `plugins/builtinmarket/builtinmarket.go:65`, `frontend/src/views/Mcp.tsx:164`

---

### DEP-007 — Installer downloads and extracts a Go toolchain with no integrity verification

- **Severity:** Medium
- **Confidence:** 95
- **Package:** `go${GO_VERSION}.linux-${arch}.tar.gz` from `go.dev/dl`
- **Ecosystem:** n/a (installer script)
- **Vulnerability Type:** Download of code without integrity check
- **CWE:** CWE-494

**Description.** `install.sh:118-124` does:

```sh
url="https://go.dev/dl/${tarball}"
curl -fsSL "$url" -o "$tmp"
rm -rf /usr/local/go
tar -C /usr/local -xzf "$tmp"
```

No SHA-256 check, despite go.dev publishing a checksum for every release and the repo's own CI demonstrating the correct pattern for staticcheck (download `.sha256`, compare, fail on mismatch). The extraction runs as root into `/usr/local`. `install.sh:385` additionally does `curl -fsSL https://tailscale.com/install.sh | sh` — an unverified pipe-to-shell, though it is the vendor's documented method.

Positives: the NodeSource, Cloudflare, and ngrok paths all install GPG keys and use signed apt repositories. `install.ps1` performs no comparable binary downloads.

**Impact.** TLS is the only integrity control. Anyone able to intercept the transfer (corporate MITM proxy, compromised mirror, hostile network during a fresh provision) substitutes a Go toolchain that is then used to compile the AGEZT daemon — a textbook compiler-level supply-chain compromise. Both paths gate behind `require_remote_install_opt_in`, which limits blast radius to users who opted in.

**Remediation.** Fetch the matching checksum from `https://go.dev/dl/?mode=json`, verify with `sha256sum -c` before `tar -xzf`, and abort on mismatch — mirroring the staticcheck block in `.github/workflows/ci.yml`. For tailscale, download to a file, verify, then execute.

**References:** `install.sh:105-130`, `install.sh:385`, `.github/workflows/ci.yml` (staticcheck checksum block, as the model to copy)

---

### DEP-008 — Production IMAP stack rests on a beta-versioned parser handling untrusted input

- **Severity:** Low
- **Confidence:** 85
- **Package:** `github.com/emersion/go-imap/v2@v2.0.0-beta.8`
- **Ecosystem:** Go
- **Vulnerability Type:** Pre-release dependency on an untrusted-input path
- **CWE:** CWE-1104

**Description.** The email channel (`plugins/channels/email`) depends on a `-beta.8` pre-release, which in turn pulls `github.com/emersion/go-message@v0.18.2` and `github.com/emersion/go-sasl@v0.0.0-20241020182733-…` (a pseudo-version, i.e. an untagged commit). All three parse attacker-influenced data: IMAP protocol responses, RFC 5322 message bodies, MIME structures, and SASL exchanges. `DEPENDENCIES.md` documents and accepts the choice (stdlib has no IMAP), and `govulncheck` reports no known advisories against any of them.

**Impact.** No known vulnerability today. The concern is structural: pre-1.0 software carries no API/security-fix guarantee, the pseudo-versioned `go-sasl` has no release cadence at all, and the parsing surface is directly reachable from remote input. A memory-safety or panic bug here is a remote DoS against the daemon's email channel.

**Remediation.** Track upstream for a stable `v2.0.0` and bump promptly. Ensure IMAP/MIME parsing runs with panic recovery and hard size limits at the channel boundary (verify separately — outside this skill's scope).

**References:** `go.mod:8,17,18`, `DEPENDENCIES.md`

---

### DEP-009 — `DEPENDENCIES.md` has drifted from `go.mod`; CI enforces names but not versions

- **Severity:** Low
- **Confidence:** 100
- **Package:** `github.com/klauspost/cpuid/v2`, `golang.org/x/sys`
- **Ecosystem:** Go
- **Vulnerability Type:** Governance / inventory drift
- **CWE:** CWE-1059 (Insufficient Technical Documentation)

**Description.** The justified-dependency inventory no longer matches the module file:

- `DEPENDENCIES.md` records `klauspost/cpuid/v2` at **v2.0.9**; `go.mod:19` requires **v2.4.0**.
- `DEPENDENCIES.md` heads its table "Indirect deps (**6**)" and lists six; `go.mod` declares **seven** indirect requires — `golang.org/x/sys v0.47.0` is absent from the table entirely.

`tools/depscheck` enforces only that every module in `go list -m all` appears in `allowlist.txt` (24 entries, matching the 24-module build list). It checks **names, not versions**, so version drift in the justification table is invisible to CI.

**Impact.** No direct exploitability. But the inventory is the artifact a reviewer or auditor consults to answer "what are we shipping and why", and it is now wrong on two counts. Governance controls that quietly drift stop being controls.

**Remediation.** Correct both rows and extend `tools/depscheck` to diff the versions in `DEPENDENCIES.md` against `go list -m all`, not just the names.

**References:** `go.mod:19,20`, `DEPENDENCIES.md` ("Indirect deps (6)" table), `tools/depscheck/allowlist.txt`

---

### DEP-010 — Floating toolchain and build-tool versions in otherwise fully pinned CI

- **Severity:** Low
- **Confidence:** 100
- **Package:** Go toolchain (`go-version: stable`, `check-latest: true`), `setuptools>=61`
- **Ecosystem:** GitHub Actions / PyPI
- **Vulnerability Type:** Unpinned version
- **CWE:** CWE-1104

**Description.** Against a CI that SHA-pins every action and version-pins every downloaded tool, three things still float:

- `.github/actions/setup-go-safe/action.yml:46-47` — `go-version: stable` with `check-latest: true` (both the initial and the retry install). The action's own comment makes this deliberate: *"We stay on `go-version: stable` … and NEVER pin back to an older minor."*
- `sdk/python/pyproject.toml:2` — `requires = ["setuptools>=61"]`, an open-ended build requirement that resolves to whatever setuptools is latest at build time, including in the PyPI publish job.
- `go.mod:3` — `go 1.26.4` with no `toolchain` directive. A floor, not a pin.

**Impact.** For the Go toolchain this floating is arguably *protective* — it guarantees the newest patched stdlib, which is why `govulncheck` is clean. The `setuptools>=61` float is the weaker one: it sits in the release pipeline that produces the sdist/wheel published to PyPI, so a bad setuptools release flows into a shipped artifact. Note also that `govulncheck` was run here with go1.26.5; a builder using exactly the `go.mod` floor of **1.26.4** was not tested — I could not confirm offline or online whether 1.26.4 carries stdlib advisories that 1.26.5 fixes. **Marked unverified.**

**Remediation.** Pin `setuptools` to an exact version in `pyproject.toml`'s build requires, as `build==1.2.2` / `twine==6.1.0` already are in `publish-sdks.yml`. Leave the Go `stable` float as-is (documented, and the safer direction), but consider adding an explicit `toolchain` directive to `go.mod` so the minimum is a decision rather than a side effect.

**References:** `.github/actions/setup-go-safe/action.yml:42-52,114-120`, `sdk/python/pyproject.toml:2`, `go.mod:3`

---

## Checks that came back clean

Recorded so a future audit does not re-litigate them.

**GitHub Actions — fully SHA-pinned.** All 4 distinct external actions are pinned to full 40-char commit SHAs with a version comment, across all 21 `uses:` sites:

| Action | SHA | Tag |
|---|---|---|
| `actions/checkout` | `3d3c42e5aac5ba805825da76410c181273ba90b1` | v7.0.1 |
| `actions/setup-node` | `820762786026740c76f36085b0efc47a31fe5020` | v7.0.0 |
| `actions/setup-python` | `5fda3b95a4ea91299a34e894583c3862153e4b97` | v7.0.0 |
| `actions/setup-go` | `40f1582b2485089dde7abd97c1529aa768e1baff` | v5.6.0 |
| `dtolnay/rust-toolchain` | `29eef336d9b2848a0b548edc03f92a220660cdb8` | stable |

Zero tag-pinned or branch-pinned actions. Workflow-wide `permissions: contents: read`, with `contents: write` scoped to the single job that needs it. `persist-credentials: false` on every checkout. Dependabot configured for all four ecosystems (gomod, npm ×2, github-actions), weekly.

**npm install-time script execution — contained.** Only 2 of 309 lockfile entries declare an install script: `fsevents@2.3.2` and `fsevents@2.3.3`, both `dev`, both `optional`, both `os: ["darwin"]` — they never install on the Linux CI runners. Every `npm ci` in both workflows uses `--ignore-scripts`, with a well-reasoned comment explaining why it is not a repo-level `.npmrc`. The TS SDK's 3-package dev tree declares no install scripts.

**Go module integrity.** `go mod verify` → *all modules verified*. `govulncheck ./...` → *No vulnerabilities found* (go1.26.5). No `replace` directives. No local-path or non-standard-URL module sources. `GOSUMDB=sum.golang.org` active with no `GOPRIVATE`/`GONOSUMDB` bypass.

**Licenses — no conflicts.** All 309 frontend packages declare a license; none is missing. Distribution: MIT 252, ISC 18, MPL-2.0 12, Apache-2.0 9, BSD-3-Clause 7, OFL-1.1 3, MIT-0 2, BSD-2-Clause 2, `MPL-2.0 OR Apache-2.0` 1, BlueOak-1.0.0 1, CC0-1.0 1, 0BSD 1. **No GPL or AGPL anywhere.** The 12 MPL-2.0 packages are file-level copyleft and compatible with the project's MIT license as long as those files are used unmodified — no action needed, noted for completeness. Go deps are MIT/BSD per `DEPENDENCIES.md`. Rust and Python SDKs have no dependencies to reconcile.

**Typosquat sweep — clean.** Every frontend direct dependency resolves to the genuine upstream: React 19.2.7, Radix UI, `@xyflow/react`, `lucide-react`, `tailwind-merge`, `clsx`, `class-variance-authority`, `@fontsource-variable/*`. No character-transposed, hyphen/underscore-swapped, or scope-confused names (no `types-react`-style unscoped impostors). The `@modelcontextprotocol`, `@playwright`, `@google`, `@zed-industries`, `@openai` preset packages were each verified present on npm at their real scoped names.

**No alternate-registry configuration.** No `.npmrc`, `.yarnrc`, or `pip.conf` anywhere in the tree — no mixed public/private registry resolution to exploit.

**Minor hygiene.** An empty, gitignored `node_modules/` directory sits at the repo root with no accompanying `package.json` — a leftover; harmless, safe to delete. `frontend/.nvmrc` pins Node 24 while the `typescript-sdk` and `npm` publish jobs hardcode `node-version: '22'`; not a security issue, but the two will drift.

---

## Verification notes & limits

- `npm audit` (both lockfiles), `govulncheck`, `go mod verify`, and direct npm/PyPI/crates.io registry queries **all executed successfully**. Findings DEP-002, DEP-003, DEP-005, and DEP-006 rest on live registry responses captured during this audit.
- `osv-scanner` is not installed locally and was not run. Given `govulncheck` (Go, reachability-aware) and `npm audit` (npm) both ran, the coverage gap is small.
- **Unverified — requires runtime check:** whether the CSP actually blocks the jsdelivr Monaco load in a real browser (DEP-004). Inferred with high confidence from the literal `script-src 'self'` header, but not observed.
- **Unverified — could not confirm:** whether Go **1.26.4** specifically (the `go.mod` floor) carries stdlib advisories fixed in 1.26.5. `govulncheck` was run against the locally installed 1.26.5 and reported clean; I did not query the vulnerability database per-patch-version, and I will not assert a CVE I have not confirmed (DEP-010).
- Registry results are point-in-time (2026-08-13). The unclaimed-name findings in DEP-002 in particular should be re-checked before acting — and acted on quickly, since the exposure window is open right now.
- `node_modules/`, `frontend/dist/`, `kernel/webui/dist/` (read only for bundle-content verification), and the vendored module cache under `.dev-home/` were excluded as scan targets per instructions.

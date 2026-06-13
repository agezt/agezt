# Security Audit — Dependency / Supply-Chain, CI/CD, IaC/Docker, Self-Update Integrity

Scanner: HUNTER subagent (sc-dependency-audit, sc-ci-cd, sc-docker, sc-iac)
Target: AGEZT @ D:/Codebox/PROJECTS/AGEZT
Date: 2026-06-13

---

## Summary (counts by severity)

| Severity | Count |
|---|---|
| Critical | 0 |
| High | 2 |
| Medium | 3 |
| Low | 4 |
| Informational / False-positive notes | 3 |

Top 3 findings:
1. **CICD-001 (High)** — Self-hosted CI runners execute untrusted fork-PR code on `pull_request`
2. **UPD-001 (High)** — Custom update endpoint is trusted to supply its own SHA256 (no signature / no out-of-band integrity anchor)
3. **UPD-002 (Medium)** — Self-update download follows redirects manually and does not pin HTTPS / forbid HTTP downgrade

Overall posture is strong: the LEAN-DEPS policy is real and enforced (2 Go modules, both justified + allowlisted + CI-gated). Plugin hash-pinning is correctly fail-closed. No Docker/Compose/Terraform/K8s present. The findings concentrate on the self-update trust chain and self-hosted CI exposure.

---

## Dependencies / Supply Chain

### Go modules — CLEAN
- `go.mod` declares exactly **one direct** dep (`lukechampine.com/blake3 v1.4.1`) + **one indirect** (`github.com/klauspost/cpuid/v2 v2.0.9`). Both are pinned in `go.sum` with hashes.
- Both are justified in `DEPENDENCIES.md` and allowlisted in `tools/depscheck/allowlist.txt`; the `deps-check` CI job (`go run ./tools/depscheck`) fails the build on any module not in the allowlist. Memory note's LEAN-DEPS policy is genuinely enforced, not aspirational.
- **blake3 v1.4.1**: no known CVE / security advisory. It is the canonical pure-Go MIT implementation. Usage is unkeyed BLAKE3-256 for content-addressing and pin verification — appropriate, not a cryptographic-auth use that would need a keyed construction. No issue.
- `cpuid/v2 v2.0.9` is older (2021-era) but is a transitive SIMD-feature-detection dep of blake3 with no security-relevant surface and no advisory. **Low/informational** (see DEP-001).

### Frontend (npm) — CLEAN
`frontend/package.json` resolved versions (from lockfile) are all current:
- react / react-dom **19.2.7**, vite **6.4.3**, esbuild **0.25.12** (well above the esbuild dev-server CORS advisory GHSA-67mh-4wv8-2f99 fixed in 0.25.0), vitest **4.1.8**, jsdom **29.1.1**, tailwindcss **4.3.0**, lucide-react 0.469.0, @xyflow/react 12.x.
- No prototype-pollution-class packages (lodash/minimist/json5 old) in the production tree; `json5@2.2.3` (transitive) is post-CVE-2022-46175. No abandoned/typosquat packages observed.
- These are **build-time only** — Vite compiles them into the committed `kernel/webui/dist/` bundle that the daemon `go:embed`s. Node is never run at runtime, so even a hypothetical frontend CVE has no runtime-server attack surface. Correctly documented in DEPENDENCIES.md.

### SDKs — CLEAN
- Python SDK (`sdk/python/pyproject.toml`): `dependencies = []` — stdlib only.
- TypeScript SDK (`sdk/typescript/package.json`): zero runtime deps, single devDep `typescript`.
- Rust SDK: std-only per CI comment.

---

### Finding DEP-001 — Outdated transitive dependency (cpuid/v2 v2.0.9)
- **Severity:** Low  **Confidence:** 60  **Ecosystem:** Go
- **Package:** `github.com/klauspost/cpuid/v2@v2.0.9`
- **CWE:** CWE-1104 (Use of Unmaintained Third Party Components) — informational
- **File:** `go.mod:7`, `go.sum:1-2`
- **Description:** Pulled in transitively by blake3 for CPU SIMD feature detection. v2.0.9 dates to ~2021; current is v2.2.x. No CVE applies.
- **Impact:** None known; feature-detection code has no untrusted-input surface.
- **Remediation:** Optional — `go get -u lukechampine.com/blake3` would bump blake3 and likely the transitive cpuid. Not required for security.

### Finding DEP-002 — Dual frontend lockfiles (npm + pnpm) can diverge
- **Severity:** Low  **Confidence:** 70  **Ecosystem:** npm
- **CWE:** CWE-1357 (Reliance on Insufficiently Trustworthy Component) — process risk
- **File:** `frontend/package-lock.json` (tracked, used by CI) vs `frontend/pnpm-lock.yaml` (untracked, present per git status); same for `sdk/typescript/`.
- **Description:** CI installs with `npm ci` against `package-lock.json`. A `pnpm-lock.yaml` also exists in the working tree. If a developer builds with pnpm locally, the resolved dependency graph (and thus the committed `dist/` bundle) can differ from what CI verifies, and a pnpm resolution could pull a version npm would not.
- **Impact:** Reproducibility / supply-chain drift; the `frontend-dist-in-sync` CI gate would catch a divergent bundle, limiting real impact.
- **Remediation:** Pick one package manager. Either gitignore/remove the pnpm lockfiles, or switch CI to pnpm with `--frozen-lockfile`. Do not commit both.

---

## CI/CD Security (.github/workflows/)

Two workflows: `ci.yml` and `publish-sdks.yml`. Reviewed in full.

**Positives:**
- No `pull_request_target` trigger anywhere — the dangerous "checkout PR head with write token" pattern is absent.
- No untrusted `${{ github.event.* }}` interpolation inside any `run:` block — no expression-injection vector (the classic PR-title/branch-name RCE). All `run:` blocks use static commands or matrix vars.
- `publish-sdks.yml` correctly scopes `permissions: contents: read` and gates each publish behind a token-presence check.
- Secret scanning (gitleaks), govulncheck, staticcheck all run in CI.

### Finding CICD-001 — Self-hosted runners execute untrusted fork-PR code (High)
- **Severity:** High  **Confidence:** 80
- **CWE:** CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)
- **File:** `.github/workflows/ci.yml:16-19` (`on: pull_request`) + every job's `runs-on: [self-hosted, Linux, X64]`
- **Description:** `ci.yml` triggers on `pull_request` and runs **every** job on self-hosted WSL runners (`wsl-runner-1..3`). For a public/forkable repo, a `pull_request` from a fork checks out and **executes attacker-controlled code** on the self-hosted machine: `go test ./...`, `go build`, `npm ci` (runs arbitrary lifecycle scripts from the PR's package.json), `npx playwright install`, `cargo test`, and `bash scripts/e2e-smoke.sh`. Self-hosted runners are **persistent and non-ephemeral** — a malicious PR can read the runner's filesystem, exfiltrate any cached credentials / `~/.ssh` / env, pivot into the WSL host and the owner's Windows machine, and poison the build cache for later legitimate runs. GitHub's own guidance: "we recommend that you do not use self-hosted runners with public repositories" precisely because of this.
- **Impact:** Full code execution on the owner's development host via a drive-by fork PR; cache poisoning of the shipped `dist/`/binaries; credential theft.
- **Remediation:**
  - If the repo is private with only trusted collaborators, residual risk is lower — but document that assumption.
  - If public: require approval for fork-PR workflow runs (Settings → Actions → "Require approval for all outside collaborators" / first-time contributors), or split into a `pull_request_target`-free design where untrusted PR jobs run only on **ephemeral** GitHub-hosted (or ephemeral self-hosted, e.g. one-shot containers via ARC) runners. Never run fork-PR builds on a long-lived runner that shares a host with developer credentials.
  - Add `permissions: contents: read` at the top of `ci.yml` (currently no `permissions:` block — defaults to the repo's token scope; explicit least-privilege is best practice).

### Finding CICD-002 — No top-level `permissions:` block in ci.yml (Medium)
- **Severity:** Medium  **Confidence:** 75
- **CWE:** CWE-250 (Execution with Unnecessary Privileges)
- **File:** `.github/workflows/ci.yml` (no `permissions:` key; `publish-sdks.yml` has one, ci.yml does not)
- **Description:** Without a `permissions:` block the `GITHUB_TOKEN` inherits the repository/organization default, which is frequently `read-write` to `contents` and more. Combined with CICD-001 (fork code executing on the runner), an attacker who achieves execution can use the ambient token to push commits, create releases, or move tags.
- **Impact:** Privilege escalation of a compromised job into repo write / supply-chain tampering.
- **Remediation:** Add `permissions: contents: read` at workflow top of `ci.yml`; elevate per-job only where a job genuinely needs more (none here do — all jobs are read/build/test).

### Finding CICD-003 — Third-party actions pinned to mutable tags, not SHAs (Medium)
- **Severity:** Medium  **Confidence:** 70
- **CWE:** CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)
- **File:** `.github/workflows/ci.yml` (`actions/checkout@v4`, `actions/setup-go@v5`, `actions/setup-node@v4`, `actions/setup-python@v5`, `dtolnay/rust-toolchain@stable`); same in `publish-sdks.yml`
- **Description:** All actions are referenced by mutable tag/branch. `actions/*` are first-party GitHub (lower risk, common false positive per the checklist), but `dtolnay/rust-toolchain@stable` is a **third-party action pinned to a moving branch ref** (`@stable`) — if that repo or branch is compromised, malicious code runs in CI (and, given CICD-001, on the self-hosted host). This action also runs in `publish-sdks.yml`, which holds the crates.io token.
- **Impact:** Supply-chain compromise via a moved tag/branch; in `publish-sdks.yml` it sits in the same job as `CARGO_REGISTRY_TOKEN`.
- **Remediation:** Pin third-party actions to a full commit SHA (`dtolnay/rust-toolchain@<sha> # stable`). Consider SHA-pinning the `actions/*` set too for defense-in-depth. Enable Dependabot for GitHub Actions to keep SHAs fresh.

> Note: `publish-sdks.yml` runs on `ubuntu-latest` (GitHub-hosted, ephemeral) and is correctly gated by `permissions: contents: read` + token-presence checks. The secret-leak surface there is small. Its main residual risk is the unpinned `dtolnay/rust-toolchain@stable` co-located with `CARGO_REGISTRY_TOKEN` (covered by CICD-003).

---

## Self-Update Integrity (kernel/update/update.go) — HIGH PRIORITY

The mechanism: `Check` (GitHub Releases API or custom endpoint) → `downloadBinary` → `validateSHA256` → atomic rename → spawn new process via `os.StartProcess`. Reviewed line-by-line.

**Positives:**
- `validateSHA256` correctly **rejects an empty SHA256** (`update.go:455`) — fail-closed; a blank hash never validates.
- Atomic staging+rename swap; fail-safe (does not auto-restart on validation failure); concurrency lock via `O_CREATE|O_EXCL`.
- Default HTTP client has sensible timeouts and TLS handshake timeout.

### Finding UPD-001 — Update endpoint is its own integrity authority; no signature / anchor (High)
- **Severity:** High  **Confidence:** 75
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-345 (Insufficient Verification of Data Authenticity)
- **File:** `kernel/update/update.go:343-385` (`checkEndpoint`) + `:172-238` (`Apply`) + `:451-473` (`validateSHA256`)
- **Description:** For `SourceEndpoint`, the manifest's `sha256` and `url` come from the **same endpoint** over the **same channel**. `validateSHA256` checks the downloaded binary against the manifest's own claimed hash — so an attacker who controls (or MITMs, or DNS-hijacks) the endpoint simply serves `{version, sha256=hash(evil), url=evil}` and the check passes. The SHA256 here is an **integrity-against-corruption** check, **not** an authenticity check. There is no code signature, no detached signature over the manifest, no pinned public key, and no out-of-band trust anchor. Since `Apply` ends by `os.StartProcess`-ing the swapped binary as the daemon, a compromised update channel is **remote code execution as the daemon**.
- **Impact:** Full RCE on every deployment configured with `AGEZT_UPDATE_ENDPOINT` if the endpoint, its TLS, or its DNS is compromised.
- **Remediation:** Sign release binaries (minisign/cosign/ed25519). Embed the public key in the binary at build time and verify a detached signature over the downloaded artifact **before** the atomic swap — the signature, not a self-asserted SHA, is the trust anchor. SHA256-from-the-same-server adds zero authenticity.

### Finding UPD-002 — Manual redirect handling can downgrade HTTPS→HTTP; no scheme enforcement (Medium)
- **Severity:** Medium  **Confidence:** 70
- **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information) / CWE-494
- **File:** `kernel/update/update.go:388-449` (`downloadBinary`), redirect block `:402-420`; endpoint/URL never validated for `https://` scheme (`:343-385`, `cmd/agezt/main.go:1184-1218`)
- **Description:** `downloadBinary` manually follows a single 301/302/303 by reading the `Location` header and re-issuing the request with `req.URL.Parse(redirectURL)`. A `Location` of `http://...` (or any attacker-chosen host) is followed without re-validating the scheme or host. Combined with the lack of any `https://`-only check on the configured `Endpoint` / asset URL, the binary can be fetched in cleartext, where an on-path attacker swaps it. With UPD-001 (no signature) this is the only line of defense and it is weak. Also note: only ONE redirect hop is handled — many CDNs chain redirects, so this is also a robustness bug.
- **Impact:** On-path attacker downgrades the binary fetch to HTTP and serves a malicious binary; with no signature, it is then executed.
- **Remediation:** Require `https://` for `Endpoint` and all download/asset URLs (reject `http://` at config time and on every redirect target). Prefer letting the stdlib `http.Client` follow redirects (it enforces sane policies) instead of hand-rolling, or explicitly refuse cross-scheme/downgrade redirects. Set `TLSClientConfig{MinVersion: tls.VersionTLS12}` on the transport.

### Finding UPD-003 — GitHub-source updates never populate SHA256 → either unverifiable or unusable (Medium)
- **Severity:** Medium  **Confidence:** 85
- **CWE:** CWE-494 (Download of Code Without Integrity Check)
- **File:** `kernel/update/update.go:296-340` (`checkGitHub` builds `UpdateInfo` with **no `SHA256` field**) vs `:452-456` (`validateSHA256` rejects empty)
- **Description:** `checkGitHub` returns `UpdateInfo{Version, URL, Notes}` and **never sets `SHA256`**. In `Apply`, `validateSHA256` is then called with an empty want-hash, which returns `"empty SHA256 in manifest"` and aborts the update. So the GitHub source either (a) **cannot complete an update at all** (fail-safe but broken feature), or (b) if a future change makes the empty hash a skip-validation path, every GitHub-sourced binary would run **unverified**. The GitHub Releases API exposes asset names but the code never fetches a `.sha256`/checksums asset to anchor integrity.
- **Impact:** Today: broken self-update for the GitHub channel (availability). Risk: a "fix" that bypasses the empty-hash guard would silently disable integrity verification.
- **Remediation:** Have `checkGitHub` locate and fetch the release's checksums asset (e.g. `*_checksums.txt` or per-asset `.sha256`) and populate `UpdateInfo.SHA256` from it — and, per UPD-001, verify a signature over that checksums file. Add a test asserting GitHub-sourced `UpdateInfo.SHA256 != ""`.

### Finding UPD-004 — Update lockfile is not PID-validated; stale lock can wedge updates (Low)
- **Severity:** Low  **Confidence:** 60
- **CWE:** CWE-667 (Improper Locking)
- **File:** `kernel/update/update.go:477-489` (`acquireLock`) + `:183` (`defer os.Remove(lockPath)`)
- **Description:** The lock is an `O_EXCL` file containing the PID. If the daemon is killed mid-`Apply` (crash/OOM/SIGKILL) the deferred `os.Remove` never runs, leaving a stale `update.lock`; every subsequent `Apply` returns `ErrUpdateInProgress` forever until manual deletion. The PID written into the file is never read back to detect a dead holder.
- **Impact:** Self-update denial-of-service after an unclean shutdown; minor.
- **Remediation:** On `ErrExist`, read the PID and check whether that process is alive (`os.FindProcess` + signal 0 on Unix); if dead, reclaim the lock. Or stamp the lock with a timestamp and treat it as stale after a bounded TTL.

---

## Docker / IaC

**No Dockerfile, no docker-compose, no Terraform (`*.tf`), and no Kubernetes/Helm manifests exist in the repository.** Confirmed via glob over `**/Dockerfile*`, `**/docker-compose*`, `**/*.tf`. The product ships as a single static Go binary with an embedded SPA, so there is no container/IaC attack surface to assess. The `.github/workflows/*.yml` are the only IaC-class files and are covered above. No DOCK-/IAC-class findings.

---

## False Positives / Notes (explicitly cleared)

- **blake3 v1.4.1** — not a vulnerable/abandoned dep; canonical, maintained, MIT, no advisory. Cleared.
- **Frontend npm packages flagged by version-age heuristics** — all resolved versions are current and post-CVE; build-time only, never run at server runtime. Cleared.
- **`actions/checkout@v4` etc. unpinned** — first-party GitHub actions; lower risk per the sc-ci-cd checklist's documented false-positive #1. Flagged only as defense-in-depth under CICD-003; the real concern there is the **third-party** `dtolnay/rust-toolchain@stable`.
- **`publish-sdks.yml` token handling** — correctly gated (read-only permissions, token-presence guard, ephemeral ubuntu-latest, registries reject duplicate versions). Not a finding.
- **Update redirect `http://` URLs in `update_test.go`** — test fixtures only, not production config. Not a finding.

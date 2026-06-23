# Infrastructure / CI-CD / IaC Security Hunt — AGEZT

Scope: `.github/workflows/ci.yml`, `.github/workflows/publish-sdks.yml`,
`.github/actions/setup-go-safe/action.yml`, `scripts/ci-go-retry.sh`,
`scripts/e2e-smoke.sh`, `scripts/webui-e2e.sh`. Globbed entire repo for
Dockerfile*, docker-compose*, *.tf/*.tfvars, k8s/helm/charts/deploy/manifests.

## Summary of posture (what is GOOD — context for the findings)

- **No `pull_request_target`** anywhere. CI triggers on plain `pull_request`.
- **Every CI job is fork-gated**: `if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository`. Fork PRs are skipped entirely, so untrusted fork code never executes on the self-hosted WSL runners. This is the correct mitigation for the "self-hosted + fork PR = RCE" class and it is applied consistently to all 14 jobs.
- **`permissions: contents: read`** is set at the top level of BOTH workflows (least privilege; not default write-all).
- **No `${{ github.event.* }}` (PR title/body/branch/author) is interpolated into any `run:` block.** The only `github.event.*` usage is inside `if:` expressions compared against `github.repository`. No `${{ github.head_ref }}` / `github.event.pull_request.*` reaches a shell. → No GHA script-injection sink found.
- **No Dockerfiles, docker-compose, Terraform, or k8s/helm manifests exist** in the repo (confirmed by repo-wide glob). IaC attack surface for this hunt is empty.
- `publish-sdks.yml` runs only on `release: published` and `workflow_dispatch` (both maintainer-gated, trusted events). Registry tokens (`PYPI_API_TOKEN`, `NPM_TOKEN`, `CARGO_REGISTRY_TOKEN`) are never exposed to fork/PR code. Publish steps no-op when the token secret is absent. This is well-scoped.

---

## Findings

### INFRA-1 — Unpinned third-party actions (mutable tags, not SHA-pinned)
- **Severity:** Medium
- **CWE:** CWE-1357 (Reliance on Insufficiently Trustworthy Component) / CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)
- **Location:**
  - `.github/workflows/ci.yml:37,53,70,86,107,108,131,132,150,152,176,177,193,210,242,243,261,278,339,342` — `actions/checkout@v4`, `actions/setup-go@v5`, `actions/setup-node@v4`, `actions/setup-python@v5`, `dtolnay/rust-toolchain@stable`
  - `.github/actions/setup-go-safe/action.yml:41,88` — `actions/setup-go@v5`
  - `.github/workflows/publish-sdks.yml:31,32,57,58,85,86` — same families + `dtolnay/rust-toolchain@stable`
- **Attack:** All third-party actions are referenced by mutable Git tags (`@v4`, `@v5`, `@stable`) rather than full commit SHAs. If any upstream action (or its tag) is compromised — a hijacked maintainer account or a moved tag — the malicious version is pulled and executed automatically on the next run. On the SELF-HOSTED WSL runners this is code execution on persistent infrastructure (the runners reuse one WSL VM and share `/dev/shm`), not an ephemeral cloud VM. `dtolnay/rust-toolchain@stable` is a branch ref, which is the most mutable of all.
- **Impact:** Supply-chain compromise → arbitrary code execution on the self-hosted runners and, in `publish-sdks.yml`, potential theft of PyPI/npm/crates.io publish tokens. Self-hosted runner persistence makes lateral movement / credential harvesting worse than on hosted runners.
- **Confidence:** High that they are unpinned (factual). Medium on real-world exploitability (requires upstream compromise; these are first-party `actions/*` and a reputable `dtolnay` action).
- **Remediation:** Pin every external action to a full 40-char commit SHA with the version in a trailing comment, e.g. `uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1`. Enable Dependabot for `github-actions` to bump the pins. Pin `dtolnay/rust-toolchain` to a SHA (a branch ref is especially risky).

### INFRA-2 — CI installs build/scan tooling via `curl | tar` and `go install ...@latest` (unpinned binary supply chain)
- **Severity:** Medium
- **CWE:** CWE-494 (Download of Code Without Integrity Check)
- **Location:**
  - `.github/workflows/ci.yml:297-301` — `curl ... api.github.com/.../releases/latest` then `curl ... staticcheck_linux_amd64.tar.gz -o /tmp/sc.tgz; tar xzf` — no checksum/signature verification, latest release resolved dynamically.
  - `.github/workflows/ci.yml:322` — `go install golang.org/x/vuln/cmd/govulncheck@latest`
  - `.github/workflows/ci.yml:350` — `go install github.com/zricethezav/gitleaks/v8@latest`
- **Attack:** The staticcheck tarball is downloaded over HTTPS but its integrity is never verified against a published checksum/signature; the release tag is whatever `releases/latest` returns at run time. `@latest` for govulncheck and gitleaks pulls whatever the proxy serves now. A compromised upstream release, a hijacked module version, or a MITM of the GitHub API/download (defense-in-depth) yields a malicious binary executed on the self-hosted runner with the workspace checkout present.
- **Impact:** Arbitrary code execution on the persistent self-hosted runners during the `lint` and `secrets` jobs. These run on every push to `main` and on internal PRs.
- **Confidence:** Medium (factual that there is no integrity check; exploitation requires upstream/registry compromise).
- **Remediation:** Pin staticcheck to a specific release tag AND verify the published SHA256 of the tarball before extracting. Pin `govulncheck` and `gitleaks` to specific module versions (`...@vX.Y.Z`) rather than `@latest`, and rely on `go.sum`/GONOSUMCHECK defaults for integrity. Consider running these scanners via SHA-pinned actions instead of ad-hoc installs.

### INFRA-3 — Self-hosted runners with shared mutable state (defense-in-depth concern)
- **Severity:** Low (given INFRA-mitigation: fork PRs are gated off — see Posture)
- **CWE:** CWE-668 (Exposure of Resource to Wrong Sphere)
- **Location:** `.github/workflows/ci.yml` (all `runs-on: [self-hosted, Linux, X64]` jobs); `.github/actions/setup-go-safe/action.yml:35-37,57,84` (purge/stage to a shared `/dev/shm` and a hardcoded fallback `$HOME/actions-runner-2/_work/_tool`).
- **Attack:** The three runners share one WSL VM and one `/dev/shm`. There is no `actions/checkout` `persist-credentials: false`, and the runners are not ephemeral, so any job that did execute attacker-influenced code (today blocked by the fork gate) could persist artifacts in `/dev/shm`/`$HOME` across jobs/branches and read the cached Go module tree and any runner-level credentials. The fork-PR gate is the *only* thing preventing untrusted code here; if that single `if:` is ever removed or weakened, the whole runner fleet is exposed.
- **Impact:** Cross-job/cross-branch contamination and credential persistence on long-lived runners IF the fork gate is bypassed. Currently mitigated.
- **Confidence:** Medium. This is a latent/defense-in-depth risk, not an active exploit — the fork gate holds today.
- **Remediation:** Keep the fork-PR gate on every job (treat removing it as a security change). Add `with: persist-credentials: false` to `actions/checkout` steps. Prefer ephemeral runners (fresh VM/container per job) for the test legs. Do not rely on a hardcoded `actions-runner-2` fallback path. Consider requiring approval for first-time contributors' workflow runs in repo settings as a second layer.

---

## Notes / non-findings (checked, no issue)

- `publish-sdks.yml` runs on `ubuntu-latest` (GitHub-hosted) while the comment says hosted minutes are billing-blocked — operational, not a security issue. Token handling is correct (env-scoped, guarded by presence check, trusted triggers only).
- No secret is `echo`'d / printed. e2e/webui-e2e scripts use a keyless echo mock (`AGEZT_DEMO_ECHO=1`) and bind only to `127.0.0.1`; the bearer tokens they grep from logs are ephemeral per-boot daemon tokens in a temp `AGEZT_HOME`, not repo secrets.
- No `curl | bash`, no `eval` of remote content, no privileged docker, no host mounts (no docker artifacts at all).
- `concurrency` + `cancel-in-progress` is set; gitleaks scans full history (`fetch-depth: 0`) — good.

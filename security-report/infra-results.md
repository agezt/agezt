# Infrastructure / CI-CD / IaC / Docker — Security Results

> Domain: CI/CD pipelines, Infrastructure-as-Code, Docker. Scanners: `sc-ci-cd`, `sc-iac`, `sc-docker`.
> Repo root: `D:\Codebox\PROJECTS\AGEZT`. Method: full read of both workflow YAMLs, the composite
> action, every CI-invoked script, the Makefile, and dependabot config; each candidate finding taken
> through Discovery → Verify (is the trigger reachable by an external contributor? does it expose a
> secret?). The recon claim "CI already least-privilege & SHA-pinned" was treated as a hypothesis to
> falsify, not trusted.

---

## 0. Scanner surface confirmation

| Scanner | Surface present? | Evidence |
|---------|------------------|----------|
| **sc-docker** | **No surface** | `**/Dockerfile*`, `**/docker-compose*`, `**/.dockerignore`, `**/*.dockerfile` → 0 matches. The product is a self-hosted Go daemon distributed as native binaries; there is no container build. **No findings possible — scanner has no input.** |
| **sc-iac (terraform / k8s / helm / CFN)** | **No surface** | `**/*.tf`, `**/*.tfvars`, `**/k8s/**`, `**/kubernetes/**`, `**/helm/**` → 0 matches. No CloudFormation. **No findings possible.** The only IaC-shaped artifacts are the GitHub Actions YAMLs, which are covered under sc-ci-cd below. |
| **sc-ci-cd** | **Yes** | `.github/workflows/ci.yml`, `.github/workflows/publish-sdks.yml`, `.github/actions/setup-go-safe/action.yml`, `.github/dependabot.yml`, plus CI-invoked `scripts/*.sh`. This is the entire real surface. |

So this report is **entirely a CI/CD pipeline review**. Docker and IaC scanners are recorded as *not applicable* (no target files).

---

## Executive summary

This is one of the better-hardened GitHub Actions setups reviewable in the wild. The recon claim holds up:

- **`permissions: contents: read`** is set at the top level of **both** workflows (`ci.yml:26-27`, `publish-sdks.yml:23-24`) — no job widens it. GITHUB_TOKEN cannot push, comment, or release.
- **Every** third-party and first-party action is **pinned to a full 40-char commit SHA** with the human-readable tag in a trailing comment (`actions/checkout`, `actions/setup-go`, `actions/setup-node`, `actions/setup-python`, `dtolnay/rust-toolchain`). No `@v4` / `@main` / `@master` mutable refs anywhere.
- **`persist-credentials: false`** on every single `actions/checkout` (12 occurrences) — the `GITHUB_TOKEN` is not left in `.git/config` for later steps/tools to read.
- **No `${{ github.event.* }}` interpolation into any `run:` block** — the only `github.event.*` use is in `if:` job-gates (a trusted expression context, not shell), and even there only `.pull_request.head.repo.full_name`. **Zero expression-injection sinks.**
- **No `pull_request_target`** and **no `workflow_run`** triggers anywhere. CI runs on plain `pull_request`, which checks out PR code in a *read-only, secret-less* context by GitHub's default.
- The SDK-publish workflow runs on **GitHub-hosted `ubuntu-latest`** (not the self-hosted runners), is gated to `release: published` / `workflow_dispatch` (maintainer-only), guards every registry token with an `if [ -z "$TOKEN" ]` skip, and never echoes a secret.
- Pinned-version + checksum-verified supply chain for the tools fetched at runtime (staticcheck tarball SHA-256 verified, govulncheck/gitleaks pinned `@vX.Y.Z` so the Go checksum DB validates them).
- Dependabot covers gomod, both npm trees, **and `github-actions`** — so the SHA pins stay current instead of rotting.

There are **no Critical or High findings.** The two findings below are genuine residual risks inherent to running CI on **persistent self-hosted runners**, plus one Low hygiene gap. None is exploitable by an external contributor in the current configuration; they are defense-in-depth gaps that matter *if* the human gate or repo settings ever slip.

---

## Findings

### CICD-001 — Self-hosted runners are persistent (non-ephemeral); fork-PR safety depends entirely on one `if:` gate + repo settings, with no second layer

- **Severity:** Medium
- **Confidence:** 80
- **File:** `.github/workflows/ci.yml:32` (and the identical gate replicated on all 14 jobs: lines 33, 50, 69, 87, 112, 138, 157, 187, 204, 225, 251, 284, 301, 379); `runs-on: [self-hosted, Linux, X64]` at `:33` et al.
- **Vulnerability Type:** CWE-693 (Protection Mechanism Failure) / CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)
- **Description:** All 14 CI jobs run on three **persistent** self-hosted WSL runners (`wsl-runner-1/2/3`) that share one VM and one `/dev/shm` (per `setup-go-safe/action.yml:20-26`). The only thing preventing an external fork's PR code from executing on those long-lived machines is the per-job guard:
  ```yaml
  if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository
  ```
  This is the **correct** pattern and it is applied consistently to every job — that is why this is Medium, not High. But it is a *single* control with no backstop:
  - Self-hosted runners do **not** reset between jobs. A malicious or buggy build that escapes the workspace (writes to `$HOME`, `~/go`, `/dev/shm`, the runner's tool cache, or installs a shim on `$PATH`) **persists** and can poison the *next* trusted `push`/PR build on that runner — classic state-bleed / cache-poisoning on a persistent runner. The workflow deliberately re-fetches the Go module cache each run (`setup-go-safe` `cache: false`) which removes one poisoning vector, but `~/go/bin` (where `go install govulncheck/gitleaks` land at `ci.yml:365,397`), `/dev/shm/goroot-$RUNNER_NAME`, and `RUNNER_TOOL_CACHE` are all reused across runs and writable by job code.
  - The fork-gate is enforced in YAML, so it is only as strong as the repo's **Actions settings**. GitHub's recommended hard control — *"Require approval for all outside collaborators"* / *"… for first-time contributors"* and **not** offering self-hosted runners to public-fork PRs — is a repo-level setting that is **not visible in the codebase** and therefore cannot be confirmed here. If that setting were ever relaxed (or the repo made public with default fork-PR settings), the YAML `if:` is the *only* thing standing between an attacker's `package.json`/`go test` TestMain and arbitrary code execution on the owner's daily-driver WSL VM.
- **Verify:** Reachable by external contributor *today*? **No** — the `if:` gate is present and correct on all jobs; a fork PR's jobs are skipped, so fork code never reaches the runner. Exposes a secret? Not directly (no secrets are referenced in `ci.yml` at all). The risk is **latent**: it materializes only if the single gate is removed/edited or repo Actions settings permit fork PRs to use self-hosted runners. This is a defense-in-depth / blast-radius finding, not a live exploit.
- **Impact:** If the gate ever fails open: arbitrary code execution on a persistent runner that the owner also uses for development, with state-bleed into subsequent trusted builds (supply-chain poisoning of release binaries). Even today, a *trusted* PR from a compromised maintainer account runs unsandboxed on a shared, non-ephemeral host.
- **Remediation:**
  1. At the org/repo level (outside this repo, but the authoritative control): set Actions → *"Require approval for all external collaborators"*, and ensure public-fork PRs are **never** offered self-hosted runners. Confirm and document this; the YAML gate should be the *second* layer, not the only one.
  2. Add a label/environment **approval gate** for any job that must run on self-hosted runners for PRs, so a maintainer explicitly opts each external PR in.
  3. Move toward **ephemeral** runners (`--ephemeral` registration / one-shot VM or WSL instance per job) so escaped state cannot bleed between runs. At minimum, have the runner wipe `~/go/bin`, `/dev/shm/goroot-*`, and `$RUNNER_TOOL_CACHE/go` in a pre-job hook.
  4. Keep the `if:` gate but treat it as belt-and-suspenders, and add a CI lint (or CODEOWNERS-protected `.github/`) so the gate can't be silently dropped from a future job.
- **References:** https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#hardening-for-self-hosted-runners ; https://securitylab.github.com/research/github-actions-preventing-pwn-requests/

---

### CICD-002 — `.github/` directory is not protected by CODEOWNERS; the workflow-trust gate has no change-review backstop

- **Severity:** Low
- **Confidence:** 70
- **File:** repo-wide — no `CODEOWNERS` file exists (`.github/CODEOWNERS` and `**/CODEOWNERS` → 0 matches).
- **Vulnerability Type:** CWE-693 (Protection Mechanism Failure)
- **Description:** The entire fork-PR safety of CICD-001, the SHA pins, the `permissions: contents: read` cap, and the `persist-credentials: false` lines all live in `.github/`. There is **no `CODEOWNERS`** requiring a specific reviewer for changes to workflows/actions, and no evidence in-repo of a branch-protection rule mandating review of `.github/**`. A future PR (or a self-merge, given project memory notes `main` has at times been **unprotected** with no required checks) could weaken any of these controls — e.g. flip a `run:` to interpolate `${{ github.event.pull_request.title }}`, drop the `if:` gate, or unpin an action — and it would merge without a forced second pair of eyes on the most security-sensitive directory in the repo.
- **Verify:** Not an exploit by itself; it is the *absence of a guardrail* that would catch a regression of CICD-001 or an introduced expression-injection. Confidence 70 because branch-protection rules are server-side and could exist without an in-repo artifact — but no CODEOWNERS at all is a concrete, verifiable gap.
- **Impact:** Silent weakening of CI security controls without mandatory review; raises the probability that CICD-001's single gate is one day removed unnoticed.
- **Remediation:** Add `.github/CODEOWNERS` requiring a trusted owner to review `/.github/**` (workflows, actions, dependabot), and enable branch protection on `main` with *"Require review from Code Owners"* + required status checks. This is the cheapest durable backstop for everything else in this report.
- **References:** https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners

---

## Items checked and found SAFE (verification of the recon claim + standard CI checklist)

| Checklist item | Result | Evidence |
|----------------|--------|----------|
| Unpinned action versions (`@main`/`@vN` tag) | **PASS** — all SHA-pinned | every `uses:` in `ci.yml` & `publish-sdks.yml` uses a 40-char SHA + tag comment; `dtolnay/rust-toolchain@29eef33…`, `actions/checkout@34e1148…`, etc. |
| `pull_request_target` misuse | **PASS** — not used | grep across `.github/` → 0 hits; trigger is plain `pull_request` (`ci.yml:19`). |
| `workflow_run` privilege-escalation pattern | **PASS** — not used | 0 hits. |
| Script injection `${{ github.event.* }}` → `run:` | **PASS** — none | the only `github.event.*` use is in `if:` job gates (`ci.yml:32` etc.), never in a `run:`/`env:` shell context. |
| Excessive `GITHUB_TOKEN` permissions | **PASS** — least privilege | top-level `permissions: contents: read` in both workflows; no job overrides upward. |
| `persist-credentials` left default (true) | **PASS** — explicitly false | all 12 `actions/checkout` set `persist-credentials: false`. |
| Secrets printed / passed to untrusted steps | **PASS** | `ci.yml` references **no** secrets at all. `publish-sdks.yml` puts each registry token in `env:` (never inline in a command), guards with `if [ -z "$TOKEN" ]`, and never echoes/encodes it. |
| SDK-publish token scope & trigger abuse | **PASS** | runs on hosted `ubuntu-latest`; trigger = `release: published` + `workflow_dispatch` (maintainer-only); tokens are scoped publish tokens documented as repo secrets; duplicate-version republish fails safely. No fork-reachable path to publish. |
| Cache poisoning (`actions/cache`) | **PASS** | no explicit `actions/cache`; `setup-node` npm cache is keyed to committed lockfiles; `setup-go-safe` deliberately disables module-cache restore (`cache: false`). |
| Artifact upload of sensitive paths | **PASS** — N/A | no `actions/upload-artifact` anywhere; nothing is exported off the runner. |
| Supply chain of runtime-fetched tools | **PASS** | staticcheck tarball SHA-256 verified against publisher sidecar before extract (`ci.yml:329-340`); govulncheck `@v1.4.0`, gitleaks `@v8.30.1`, both pinned so Go checksum DB validates (`ci.yml:365,397`). |
| Retry wrappers masking real failures | **PASS (by design)** | `ci-go-retry.sh` and the staticcheck/govulncheck retry loops re-run on transient WSL `compile` corruption only; a deterministic failure still fails every attempt (documented, bounded `max=5`). Not a security issue. |
| Dependabot coverage | **PASS** | `dependabot.yml` covers gomod, frontend npm, sdk/typescript npm, **and github-actions** — keeps SHA pins fresh. |
| Local dev scripts (`dev.ps1`, `dev/*.ps1`, `*.ps1`) | **Out of CI scope / safe** | not invoked by any workflow; `dev.ps1` only sources a gitignored `.env`, seeds an isolated `.dev-home`, and never enters env into untrusted contexts. |

---

## Severity counts

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 0 |
| Medium | 1 (CICD-001) |
| Low | 1 (CICD-002) |
| **Total** | **2** |

Docker scanner: **no surface** (no Dockerfile/compose). IaC scanner: **no surface** (no Terraform/k8s/helm/CFN). Both recorded as not-applicable rather than clean-with-findings.

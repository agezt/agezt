# Infrastructure Security Audit — AGEZT

Scope: CI/CD, Docker, IaC, repo scripts, committed secrets. Read-only review.
Auditor: Infrastructure hunter. Date: 2026-06-24.

## Executive summary

The GitHub Actions pipeline is **unusually well-hardened** — among the best-configured aspects of this
repo. It already implements most CI/CD supply-chain best practices: least-privilege `GITHUB_TOKEN`,
all third-party actions SHA-pinned, no PR-head code exec with secrets, no script-injection sinks,
`persist-credentials: false`, checksum-verified tool downloads, pinned tool versions.

There is **no Docker, no docker-compose, no Terraform, and no Kubernetes/IaC** in the repository, so
those classes are N/A (see §2/§3).

The real risk surface is **(a) the self-hosted WSL runners combined with `pull_request` triggers**, and
**(b) a set of loose, committed root utility scripts** — `list_vault.py`, the `*scout*` scripts, and
`start-daemon.*` — which are debugging/operator scratch files that should not ship in a repo. Plus a
local **`.env` containing a real `AGEZT_VAULT_PASSPHRASE` value** that, while correctly gitignored, sits
unencrypted on disk and is referenced by `dev.ps1`.

Findings below, highest severity first.

---

## §1. CI/CD

### INFRA-01 — Self-hosted runners execute on `pull_request` (fork-PR code-exec exposure) — MEDIUM

- **Where:** `.github/workflows/ci.yml:16-19` (`on: pull_request:` with no `types`/branch filter) +
  every job `runs-on: [self-hosted, Linux, X64]` (e.g. `ci.yml:33,51,70,88,113,139,158,188,205,225,252,285,302,380`).
- **Issue:** All CI jobs run on **persistent self-hosted WSL runners** (project memory: 3 WSL systemd
  runners, `wsl-runner-1..3`, all in one VM sharing one `/dev/shm`). The workflow triggers on
  `pull_request`, which fires for **fork PRs**. Self-hosted runners are long-lived and non-ephemeral, so
  a malicious fork PR that reaches the runner can persist on the host, poison the shared toolchain in
  `/dev/shm`/`~/go/pkg/mod`, read other repos' checkouts under `_work`, or pivot into the developer's
  WSL/Windows host.
- **Mitigating controls already present (good):** every job is gated by
  `if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository`
  (`ci.yml:32,50,69,87,...`). This **skips all jobs for fork PRs** — only same-repo (branch) PRs and
  pushes run. `permissions: contents: read` (`ci.yml:26-27`) and `persist-credentials: false` further
  limit blast radius. This is the correct pattern and substantially closes the hole.
- **Residual impact:** The `if:` guard is the *only* thing standing between a fork PR and the
  self-hosted runner. It is correct today, but it is a per-job opt-in that is easy to forget on a newly
  added job (a job without the guard would run fork code on the runner). GitHub's own guidance is that
  **self-hosted runners should never be used on public repos with fork PRs**, because the runner is
  non-ephemeral. There is no repo-level/org-level "require approval for fork PRs" backstop documented.
- **Severity:** MEDIUM (LOW if the repo is private; the guard is currently effective, but the
  architecture is fragile — one un-guarded job re-opens it).
- **Fix:**
  1. Restrict the trigger explicitly: `on: pull_request: branches: [main]` does NOT block forks — instead
     set repo Settings → Actions → "Require approval for all outside collaborators" / "for first-time
     contributors", and ideally "Require approval for all fork pull request runs".
  2. Prefer **ephemeral** self-hosted runners (one job per runner, torn down after) over persistent
     systemd runners, so PR code cannot persist on the host.
  3. Add a CI lint/guard test that fails if any job is missing the
     `pull_request.head.repo.full_name == github.repository` gate, so the protection can't silently
     regress.

### INFRA-02 — `setup-go-safe` re-purge step deletes a broad toolcache path with `rm -rf` driven by env — LOW

- **Where:** `.github/actions/setup-go-safe/action.yml` — "Purge corrupt Go toolcache" step
  (`rm -rf "${RUNNER_TOOL_CACHE:-$HOME/actions-runner-2/_work/_tool}/go/1.26.4"`) and "Re-purge"
  (`rm -rf "${RUNNER_TOOL_CACHE:-...}/go" "/dev/shm/goroot-${RUNNER_NAME:-shared}"`).
- **Issue:** These `rm -rf` paths interpolate `$RUNNER_TOOL_CACHE` and `$RUNNER_NAME`. On GitHub-hosted
  runners these are controlled by the platform, but on the **self-hosted** runners the environment is
  under the operator's control, not an attacker's — so this is not directly exploitable. However, a
  fallback hardcoded to another runner's home (`$HOME/actions-runner-2/...`) is a footgun: if
  `RUNNER_TOOL_CACHE` is unset on runner-1 or runner-3, the purge targets `actions-runner-2`'s tree,
  which on a shared WSL VM could delete the **wrong runner's** toolcache mid-job.
- **Impact:** Reliability/integrity (cross-runner cache deletion), not a direct security breach.
- **Severity:** LOW.
- **Fix:** Drop the cross-runner hardcoded fallback; fail fast if `RUNNER_TOOL_CACHE`/`RUNNER_NAME` are
  unset rather than guessing another runner's path.

### CI/CD — verified GOOD (no finding)

- **GITHUB_TOKEN least privilege:** `permissions: contents: read` at workflow level in both
  `ci.yml:26` and `publish-sdks.yml:23`. No `write-all`, no missing block.
- **Action pinning (supply-chain):** ALL third-party actions are pinned by full commit SHA with a
  version comment — `actions/checkout@34e1148…` , `actions/setup-go@40f1582…`,
  `actions/setup-node@49933ea…`, `actions/setup-python@a26af69…`, `dtolnay/rust-toolchain@29eef33…`.
  No mutable `@v4`/`@main` tags.
- **No script injection:** no `${{ github.event.* }}` (PR title/body/branch/comment) is interpolated
  into any `run:` shell. Matrix vars (`${{ matrix.goos }}` etc.) are static, not attacker-controlled.
- **No `pull_request_target` / `issue_comment` / `workflow_run`** triggers anywhere (grep clean) — the
  classic "checkout PR head + secrets" RCE pattern is absent.
- **Secrets handled correctly:** `publish-sdks.yml` reads registry tokens only into `env:` and guards
  with `if [ -z "$TOKEN" ]`; tokens are never echoed. The `--redact` flag is used on gitleaks
  (`ci.yml:398`). No `echo`/`printf` of any secret.
- **Tool download integrity:** staticcheck tarball is downloaded over HTTPS and **SHA256-verified
  against the publisher's signed sidecar** before extraction (`ci.yml:333-341`); `govulncheck`,
  `gitleaks`, staticcheck are all **version-pinned** (`@v1.4.0`, `@v8.30.1`, `2026.1`) rather than
  `@latest` (CWE-494 addressed).
- **Secret scanning** runs in CI (gitleaks, full history `fetch-depth: 0`, `ci.yml:374-398`).

---

## §2. Docker — N/A

No `Dockerfile`, `Dockerfile.*`, `docker-compose*.yml`, or `.dockerignore` exists anywhere in the repo
(excluding `node_modules`). The product ships as static Go binaries (`agezt`, `agt`) with a go:embed'd
web UI; there is no container build to review. **No finding.**

## §3. IaC — N/A

No Terraform (`*.tf`), no Kubernetes/Helm manifests, no CloudFormation, no Pulumi, no Ansible. There is
no infrastructure-as-code in this repository. Open security groups / public buckets / missing
encryption / hardcoded cloud creds are not applicable here. **No finding.**

---

## §4. Repo scripts

### INFRA-03 — `list_vault.py` is a committed vault-probing scratch script — MEDIUM

- **Where:** `D:/Codebox/PROJECTS/AGEZT/list_vault.py` (tracked in git).
- **Issue:** This script invokes `agt vault status --json` and then enumerates the **process
  environment for secrets** (`[k for k in os.environ if 'ANTHROPIC' in k or 'LLMGATEWAY' in k or
  'API_KEY' in k.upper()]`) and **prints them** (`print("Relevant env vars:", env_keys)`). It prints
  variable *names* only, not values — so it does not leak secret material directly. But:
  - It is an **operator debugging artifact aimed squarely at the encrypted vault**, committed to the
    repo. Shipping a "list_vault" tool normalizes vault-introspection and is exactly the kind of script
    an attacker who lands code-exec would look for and extend (one-line change `os.environ[k]` →
    prints the value).
  - It depends on `agt` being on PATH and a running daemon; it has no place in version control.
- **Impact:** Information disclosure aid / supply-chain hygiene; not a direct leak as written.
- **Severity:** MEDIUM (because of intent + name + it touches the vault).
- **Fix:** Delete `list_vault.py` from the repo and add it (and the other root scratch scripts) to
  `.gitignore`, mirroring how the gateway-auth scratch scripts (`decode_jwt.py`, `verify_sig.py`, …) are
  already gitignored at `.gitignore:64-74`.

### INFRA-04 — Loose committed operator/debug scripts pollute the repo — LOW

- **Where (all tracked):** `find_302ai.py`, `find_model.py`, `fix_scout.py`, `readlines.js`,
  `update_scout_soul.py`, `update_scout_soul.go`, `scout_soul.ps1`, `set_scout_soul.ps1`,
  `update_scout.bat`, `start-daemon.bat`, `start-daemon.ps1`.
- **Issue:** These are one-off operator scratch files (model lookups against `models.dev`, the agent
  "scout" persona setters, a `readlines.js` slicer, daemon launchers). They:
  - `find_model.py` hardcodes a machine-specific absolute path
    `C:\Users\ersin\.agezt\catalog\api.json` — leaks the operator's username and home layout into the
    repo.
  - `scout_soul.ps1` / `set_scout_soul.ps1` hardcode `D:\Codebox\PROJECTS\AGEZT\…` and call `agt.exe`
    from PATH/CWD with shell-expanded content; `update_scout.bat` runs PowerShell with
    `-ExecutionPolicy Bypass`. None take untrusted input, so there is no injection vector, but
    `-ExecutionPolicy Bypass` on a committed launcher is a poor default to ship.
  - `find_302ai.py` makes an unpinned `urllib` HTTPS request to `https://models.dev/api.json` (fine,
    HTTPS, but it's debug-only).
- **Impact:** Repo hygiene, username/path disclosure, no exploit.
- **Severity:** LOW.
- **Fix:** Move these to a gitignored `scratch/` dir or delete them. They are not part of the build and
  most reference a hardcoded developer machine path.

### INFRA-05 — `start-daemon.ps1` launches the daemon hidden, no integrity/path checks — LOW

- **Where:** `start-daemon.ps1`, `start-daemon.bat`.
- **Issue:** Both launch `agezt.exe` from a **hardcoded absolute path** with `-WindowStyle Hidden` /
  `start ""`. Hidden-window auto-launch of a long-running network daemon is an anti-pattern (no console
  to observe, easy to forget it's running, resembles a persistence mechanism). No code-signing or hash
  check on the binary before launch.
- **Severity:** LOW.
- **Fix:** Run visibly (or as a managed service with logging), and resolve the binary relative to the
  script, not a hardcoded `D:\…` path.

### Scripts — verified GOOD (no finding)

- **No `curl | bash` / `iwr | iex`** anywhere. The only network fetches are HTTPS to `models.dev`
  (debug) and the CI staticcheck download (checksum-verified).
- **`build.sh`** uses `set -euo pipefail`, fixed CGO posture, no secrets. Clean.
- **`scripts/ci-go-retry.sh`, `e2e-smoke.sh`, `webui-e2e.sh`** use `mktemp -d` (safe temp dirs), bind
  test servers to `127.0.0.1` only (no `0.0.0.0`), and pass bearer tokens via headers, not argv leaks.
  No secrets committed. Clean.
- **`dev.ps1`** explicitly isolates to `.\.dev-home` (never the real `~/.agezt`), parses `.env`
  "without echoing secrets to the console," and passes only an allowlist of env vars through. Good
  hygiene — see INFRA-06 for the one residual concern (the `.env` it reads).

---

## §5. .gitignore / committed secrets

### INFRA-06 — `.env` on disk contains a real `AGEZT_VAULT_PASSPHRASE` value (gitignored, but plaintext) — MEDIUM

- **Where:** `D:/Codebox/PROJECTS/AGEZT/.env` (line `AGEZT_VAULT_PASSPHRASE=NoPass…`), read by
  `dev.ps1` (passes it through to `env:AGEZT_VAULT_PASSPHRASE`).
- **Issue:** The `.env` is **correctly NOT tracked** by git (`git ls-files` shows nothing; `.gitignore`
  covers `.env`, `.env.*`, `*.env` at lines 57-59) and has **never been committed** (`git log --all --
  oneline -- .env` is empty). That part is good. However:
  - The file's own comment says *"Vault passphrase — … DO NOT commit a value here"* and *"set via
    environment variable or OS secrets manager at runtime"* — yet a literal passphrase value
    (`AGEZT_VAULT_PASSPHRASE=NoPass…`) IS written into the file, contradicting the file's own guidance.
  - It also contains commented-out **real-looking provider API keys** (`MINIMAX_API_KEY=sk-cp-…`,
    `DEEPSEEK_API_KEY=sk-da0…`). Even commented, they are recoverable secret material sitting in
    plaintext on disk.
  - The vault is machine-bound-encrypted at rest (M934), but a passphrase in a world-readable `.env`
    (perms `-rw-r--r--`) on the dev box undermines that — anyone/anything with read access to the repo
    directory gets the passphrase and the commented keys.
- **Impact:** Local plaintext secret exposure; one mis-`git add -f` or backup/sync away from leaking.
  Not a committed-secret incident today.
- **Severity:** MEDIUM (LOW if these keys are already-rotated/dummy; the passphrase being literal
  against the file's own instruction is the concern).
- **Fix:** Remove the literal `AGEZT_VAULT_PASSPHRASE` value (provide it via the OS keychain /
  `export` at runtime as the comment itself instructs); scrub the commented real-looking API keys;
  tighten file perms (e.g. `icacls`/`chmod 600`). Treat any key that ever sat in this file as
  compromised and rotate.

### §5 — verified GOOD (no finding)

- `.env`, `.env.*`, `*.env`, `creds.json`, `agentgw.secret`, `*.token`, `token.txt`,
  `*.bak`, `.dev-home/`, `bin/` are all gitignored (`.gitignore:57-90`). The gateway-auth scratch
  scripts are explicitly gitignored (`.gitignore:64-74`).
- **No live secrets in tracked files.** A `git grep` for `sk-…`, `AIza…`, `ghp_…`, `xox[bp]-…`
  patterns over tracked files returned only **regex patterns and documentation examples**
  (`kernel/configcenter/classifier.go:85,90` are the secret-detector's own regexes;
  `.project/PHASE-M15-REDACTION-REPORT.md:98` is a redaction test fixture). No real credential is
  committed.
- No `*.pem`/`*.key`/`*.p12`/`*.pfx` files are tracked (the `kernel/agentgw/secret.go` hit is source
  code, not a key file).
- CI runs gitleaks over full history as a backstop (§1).

---

## Findings table

| ID | Severity | Area | File | Issue |
|----|----------|------|------|-------|
| INFRA-01 | MEDIUM | CI/CD | ci.yml:16-19 + self-hosted runners | `pull_request` on persistent self-hosted WSL runners; protected only by per-job `if:` fork-guard (currently effective, fragile) |
| INFRA-03 | MEDIUM | Scripts | list_vault.py | Committed vault-probing scratch script that enumerates secret env-var names |
| INFRA-06 | MEDIUM | Secrets | .env | Literal `AGEZT_VAULT_PASSPHRASE` + commented real-looking API keys in plaintext on disk (gitignored, never committed) |
| INFRA-02 | LOW | CI/CD | setup-go-safe/action.yml | `rm -rf` fallback hardcoded to `actions-runner-2`; can delete wrong runner's toolcache on shared VM |
| INFRA-04 | LOW | Scripts | find_model.py, *scout*, readlines.js | Loose committed operator/debug scripts; hardcoded user path leaks `C:\Users\ersin\…`; `-ExecutionPolicy Bypass` launcher |
| INFRA-05 | LOW | Scripts | start-daemon.ps1/.bat | Hidden-window daemon launch from hardcoded path, no binary integrity check |

**Net:** 3 MEDIUM, 3 LOW. No HIGH/CRITICAL. No Docker/IaC. The GitHub Actions pipeline itself is
strongly hardened (SHA-pinned actions, least-priv token, no PR-head-with-secrets exec, no script
injection, checksum-verified downloads, pinned tool versions). The standout architectural risk is the
**non-ephemeral self-hosted runner + `pull_request` trigger** (mitigated today by the fork-guard `if:`),
and the standout hygiene issue is the **loose committed scratch scripts** (`list_vault.py` especially)
plus the **plaintext `.env` vault passphrase**.

Report written to: D:/Codebox/PROJECTS/AGEZT/security-report/infra-results.md

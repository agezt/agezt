# Infrastructure, Supply Chain & Data Exposure — Security Results

**Repo:** `D:\Codebox\PROJECTS\AGEZT` · **Branch:** main @ `f815f56e` · **Date:** 2026-08-12
**Skills run:** `sc-ci-cd`, `sc-dependency-audit`, `sc-data-exposure`
**Supersedes:** the prior `infra-results.md` (from `99d2e426`) — overwritten.

## Scope note: container / IaC scanning SKIPPED

There is **no Dockerfile, docker-compose file, Kubernetes manifest, Helm chart, Terraform,
CloudFormation, Ansible, or Pulumi config anywhere in this repository**. Verified by a
full-tree filename sweep. AGEZT ships as static Go binaries (`cmd/agezt`, `cmd/agt`)
cross-built by the `multi-arch` CI job, with the Web UI `go:embed`-ded. Container and
IaC checks are therefore **not applicable** and were not run — this is an accurate
"nothing to scan" result, not an untested gap.

## Not re-reported (confirmed clean upstream)

Per the pipeline brief, `govulncheck` is currently **CLEAN** (no known-vulnerable Go
module or stdlib path) and `gitleaks` is **CLEAN** (zero committed secrets). Neither is
re-litigated below. One remark on the latter: the `.gitleaks.toml` allowlist was
independently reviewed and is **tight and well-justified** — it allowlists exactly seven
named `*_test.go` paths that hold deliberately synthetic fixtures, uses no regex/stopword
escape hatches, and leaves all production code in scope. The "clean" verdict is
meaningful, not manufactured.

---

# Findings summary

| ID | Severity | Title | Confidence |
|----|----------|-------|-----------|
| CICD-001 | **High** | Dependabot npm PRs execute unreviewed install scripts on non-ephemeral self-hosted runners | 90 |
| CICD-002 | **High** | Fork-guard enforcement lint (`internal/ciguard`) was deleted; 16 guards are now unenforced convention | 95 |
| CICD-003 | **High** | `frontend-dist-rebuild` auto-commits a shipped build artifact to `main` with `[skip ci]`, bypassing review and every CI gate | 92 |
| CICD-004 | **High** | `publish-sdks.yml` is `workflow_dispatch`-able from any ref with no environment gate and long-lived registry tokens | 85 |
| CICD-005 | Medium | staticcheck integrity check fetches the checksum from the same origin as the tarball (no real verification) | 95 |
| CICD-006 | Medium | Fixed `/tmp` and `/dev/shm` paths shared by three concurrent runners in one VM — build-artifact substitution / TOCTOU | 85 |
| CICD-007 | Medium | `ci-go-retry.sh` wildcard-deletes other concurrently-running jobs' `GOCACHE`/`GOTMPDIR` | 95 |
| CICD-008 | Medium | GOROOT staged into shared `/dev/shm`; a co-resident job can tamper with another job's Go compiler | 70 |
| CICD-009 | Medium | `GITHUB_TOKEN` embedded in a `git push` URL argument (process-list exposure on a shared runner) | 80 |
| CICD-010 | Low | `playwright install --with-deps` implies passwordless `sudo` on the self-hosted runner | 70 |
| CICD-011 | Low | npm publish without `--provenance` / no OIDC attestation on any of the three registries | 90 |
| DEP-001 | Medium | Monaco editor loaded at runtime from `cdn.jsdelivr.net` into the admin console — outside the lockfile, allowlist, SRI and CSP | 95 |
| DEP-002 | Medium | Shipped `dist` requests Monaco `0.55.1` while reviewed source pins `0.52.2` | 95 |
| DEP-003 | Medium | No npm vulnerability scanning anywhere in CI — 309 transitive frontend packages have zero coverage | 95 |
| DEP-004 | Medium | `setuptools>=61` is unpinned in the PyPI publish path despite `build`/`twine` being pinned | 90 |
| DEP-005 | Low | `emersion/go-imap/v2` **beta** parser on the attacker-controlled inbound-email path | 85 |
| DEP-006 | Low | `tools/depscheck` allowlist is a superset of actual usage (e.g. `goldmark` is imported nowhere) | 95 |
| DEP-007 | Low | Stale transitive module graph (`x/tools 0.6.0`, `x/mod 0.8.0`, `x/sync 0.1.0`, `testify 1.8.0`) | 90 |
| EXPOSE-001 | **High** | Journal segments are world-readable (0644 in 0755) while every sibling secret store uses 0700/0600 | 98 |
| EXPOSE-002 | **High** | `agentgw` audit writes directly to the append-only hash chain, bypassing the redactor entirely | 95 |
| EXPOSE-003 | **High** | `/api/webhook_log` returns full outbound sink URLs — for Slack/Discord/Teams the URL *is* the credential | 90 |
| EXPOSE-004 | Medium | Redaction is nil-by-default outside `cmd/agezt`; three redactors run with an empty literal set | 92 |
| EXPOSE-005 | Medium | Upstream provider error bodies relayed verbatim to clients on `/api/transcribe`, `/api/tts`, `/api/run` | 90 |
| EXPOSE-006 | Medium | Full Web UI admin token printed in a URL query string to stdout/logs and passed as a process argument | 95 |
| EXPOSE-007 | Medium | `/metrics` is computed from the primary kernel but reachable with a per-tenant token | 85 |
| EXPOSE-008 | Medium | `/api/tool_log` serves raw tool-call arguments and outputs (shell commands, URLs, file contents) | 88 |
| EXPOSE-009 | Medium | Redactor pattern gaps: cookies, `Basic`/`X-Api-Key` headers, Stripe `sk_live_`, Mistral, `key=value` assignments | 92 |
| EXPOSE-010 | Medium | `*fs.PathError` absolute host paths leak through ~15 `http.Error(w, err.Error(), …)` sites | 90 |
| EXPOSE-011 | Low | Event `Subject`/`Actor`/`CorrelationID` are never redacted, even on the redacted path | 95 |
| EXPOSE-012 | Low | Memory store and config-center audit log also use 0644/0755 | 90 |
| EXPOSE-013 | Low | `/oauth/callback` renders upstream error text to an unauthenticated browser | 80 |

---

# 1. CI/CD Pipeline Security (`sc-ci-cd`)

## What is genuinely well built

Before the findings, these are verified strengths and should not be regressed:

- **Zero GitHub Actions expression injection.** Every `${{ … }}` interpolation in both
  workflows and the composite action was enumerated. Not one untrusted context
  (`github.event.pull_request.title/body`, `github.event.issue.*`,
  `github.event.comment.body`, `github.event.head_commit.message`, `github.head_ref`)
  reaches a `run:` block, a step `name:`, or any shell context. The only interpolations
  are `github.workflow`, `github.ref`, `matrix.*`, and `secrets.*`. This is the single
  most common Critical GitHub Actions bug and it is absent.
- **No `pull_request_target`.** Both workflows use `pull_request` / `push` / `release` /
  `workflow_dispatch` only. There is no privileged-context checkout of PR head code.
- **100% SHA pinning.** All five distinct third-party actions (`actions/checkout`,
  `actions/setup-go`, `actions/setup-node`, `actions/setup-python`,
  `dtolnay/rust-toolchain`) are pinned to full 40-char commit SHAs with a version
  comment, in both workflows *and* inside the composite action. No mutable tags anywhere.
- **Least-privilege default.** `permissions: contents: read` at workflow level in both
  files, with exactly one job (`frontend-dist-rebuild`) escalating to `contents: write` —
  and doing so at job scope, not workflow scope.
- **`persist-credentials: false` on every checkout.** Verified: all 17 `actions/checkout`
  invocations across both workflows carry it. No exceptions.
- **Tool version pinning.** staticcheck `2026.1`, govulncheck `v1.4.0`, gitleaks
  `v8.30.1`, `build==1.2.2`, `twine==6.1.0` — no `@latest` in any security-gate tool.

The findings below are about the **self-hosted runner threat model**, which is where the
residual risk concentrates.

---

## CICD-001 — Dependabot npm PRs execute unreviewed install scripts on non-ephemeral self-hosted runners

- **Severity:** High · **Confidence:** 90
- **CWE:** CWE-829 (Inclusion of Functionality from Untrusted Control Sphere), CWE-1357 (Reliance on Insufficiently Trustworthy Component)
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\.github\dependabot.yml:8-18` (npm ecosystems for `/frontend` and `/sdk/typescript`)
  - `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:281-298` (`frontend-test`, `npm ci`)
  - `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:191-213` (`frontend-dist-in-sync`, `npm ci`)
  - `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:300-328` (`webui-e2e`, `npm ci` + `npx playwright install --with-deps`)

### Description

The fork guard used by every job is:

```
if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository
```

This correctly blocks **fork** PRs. But Dependabot does not open fork PRs — it pushes
branches (`dependabot/npm_and_yarn/...`) **inside the repository**, so
`github.event.pull_request.head.repo.full_name == github.repository` evaluates **true**
and the guard passes.

The result: every weekly Dependabot npm PR causes three separate jobs to run `npm ci`
against a **newly published, entirely unreviewed** package version, on a **non-ephemeral
self-hosted WSL runner**, *before a human has looked at the diff*. `npm ci` executes
`preinstall`/`install`/`postinstall` lifecycle scripts. `webui-e2e` additionally runs
`npx playwright install --with-deps`, which downloads browser binaries and installs
system packages.

The runner is not disposable. Per
`D:\Codebox\PROJECTS\AGEZT\.github\actions\setup-go-safe\action.yml:29-36`, the three
runners `wsl-runner-1/2/3` live in **one shared WSL VM**, keep a persistent `$HOME`, a
persistent Go module cache, and a persistent npm cache across every job of every branch.

### Concrete exploit scenario

1. An attacker compromises the npm publishing account of any package in the frontend's
   309-package transitive tree (or a maintainer account of a direct dep such as
   `lucide-react`, `clsx`, `tailwind-merge`) — the standard `event-stream` /
   `ua-parser-js` / `debug`-chain pattern.
2. They publish a patch version containing a `postinstall` script.
3. Within seven days Dependabot opens a PR bumping `frontend/package-lock.json`.
4. `frontend-test`, `frontend-dist-in-sync` and `webui-e2e` all fire on the self-hosted
   runner. The `postinstall` script runs as the runner user with network access.
5. The payload writes a persistent backdoor into the runner's `$HOME` (shell profile,
   `~/.npmrc`, a `git` credential helper, or the npm cache), which then survives into
   **every subsequent job on that VM** — including `frontend-dist-rebuild`, the job that
   holds `contents: write` and pushes directly to `main` (see CICD-003).
6. No human has yet reviewed the Dependabot PR. Compromise precedes review.

### Remediation

- Add `--ignore-scripts` to every CI `npm ci` (`npm ci --ignore-scripts`). Nothing in
  this tree needs install scripts: the only two `hasInstallScript` packages in
  `frontend/package-lock.json` are `fsevents` and `vite/node_modules/fsevents`, both
  macOS-only optional deps that are no-ops on Linux.
- Route Dependabot PRs to GitHub-hosted runners, or gate them behind
  `github.actor != 'dependabot[bot]'` on the self-hosted jobs.
- Make the self-hosted runners **ephemeral** (`--ephemeral` registration flag), so
  runner-state persistence is impossible.

---

## CICD-002 — The fork-guard enforcement lint was deleted; 16 guards are now unenforced convention

- **Severity:** High · **Confidence:** 95
- **CWE:** CWE-693 (Protection Mechanism Failure), CWE-1188 (Insecure Default Initialization)
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:233` — *"it exists here only so the ciguard fork-guard lint passes"*
  - `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:244` — *"persist-credentials: false is required by the ciguard lint"*
  - `D:\Codebox\PROJECTS\AGEZT\docs\DEAD-CODE-AUDIT.md:10` — `internal/ciguard/ciguard.go` … **DELETED 2026-07-08**

### Description

`ci.yml` (last modified 2026-07-29) documents in two places that a lint called
**ciguard** mechanically enforces two critical invariants: that every self-hosted job
carries the fork guard, and that every checkout sets `persist-credentials: false`.

**That lint no longer exists.** A full-tree search finds no `ciguard` package, no
`ciguard` binary under `tools/`, no `ciguard` CI job, and no test anywhere that parses
`.github/workflows/*.yml`. The only surviving trace is a stale git ref
(`origin/feat/delete-ciguard-deadcode`) and the audit row above, which retired it as
"production helpers only used by tests — UNREACHABLE_DECL".

That classification was wrong in consequence, even if right in form: the helpers looked
unreachable because their only consumer *was* a test — and that test **was the control**.
Deleting it removed the enforcement, and the workflow comments were never updated, so
the file still asserts a protection that is gone.

The guards themselves are currently **all correct** — I verified all 16 jobs
(`ci.yml:32, 78, 111, 148, 166, 191, 235, 281, 302, 332, 349, 370, 396, 429, 448, 535`)
and all 17 checkouts. This finding is not about a present hole; it is about the
**absence of the mechanism that keeps the hole from reappearing**, on a pipeline where
the failure mode is arbitrary fork code on a persistent runner.

### Concrete exploit scenario

A contributor adds a new job to `ci.yml` — say `bench (linux)` — by copying the
`runs-on: [self-hosted, Linux, X64]` block but omitting the eight-word `if:` line, which
is visually easy to drop and semantically invisible in review. CI stays green, so nothing
signals the omission. From that moment, **any GitHub user** can open a pull request from
a fork and have arbitrary code (`go test ./...` runs every `TestXxx` in the PR's tree)
execute on the shared WSL VM that hosts all three runners, with a persistent `$HOME` and
the npm/Go caches used by every other build — including the `contents: write` job.

### Remediation

Restore the lint as a CI job (it need not be Go — a short script or `actionlint` with a
custom rule works). It must assert, for every job in every workflow: if
`runs-on` contains `self-hosted`, the job has the exact fork-guard `if:` expression; and
every `actions/checkout` step sets `persist-credentials: false`. Then correct or remove
the two stale comments at `ci.yml:233` and `ci.yml:244`.

---

## CICD-003 — `frontend-dist-rebuild` auto-commits a shipped artifact to `main` with `[skip ci]`

- **Severity:** High · **Confidence:** 92
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-829, CWE-693
- **File:** `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:215-277`

### Description

This job runs on every push to `main`, rebuilds the Web UI on the self-hosted runner,
and if the output differs from what is committed, **commits and force-pushes the result
straight to `main`**:

```
git add kernel/webui/dist/
git commit -m "chore(frontend): auto-rebuild dist [skip ci]"
git push "https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git" HEAD:refs/heads/main
```

Three properties compound into a real risk:

1. **The artifact is executable code that ships.** `kernel/webui/dist/` is `go:embed`-ded
   into the `agezt` binary. Whatever lands here is served to every operator's browser as
   the admin console for an autonomous agent daemon.
2. **It bypasses human review entirely.** No PR, no approval — a machine-authored commit
   lands on the default branch.
3. **It bypasses every CI gate.** The commit carries `[skip ci]`, and (per the job's own
   comment) pushes made with `GITHUB_TOKEN` do not retrigger workflows anyway. So the
   auto-committed bundle is never scanned by `gitleaks`, never checked by
   `frontend-dist-in-sync`, never exercised by `webui-e2e`. It is the **only** code path
   into `main` with zero review and zero automated inspection.

The design intent (self-healing `main`) is sound; the problem is that the healing agent
is a build running on a persistent runner whose input is a 309-package npm tree.

### Concrete exploit scenario

Chaining from CICD-001: a compromised npm postinstall (or a tampered runner `$HOME` from
any earlier job) alters the Vite build so `npm run build` emits a `dist` bundle
containing an extra exfiltration routine — the console already handles provider API keys,
vault contents and the `?token=` admin credential, so a few lines suffice to beacon them.
On the next push to `main`, `frontend-dist-rebuild` observes a `dist` diff (it will —
that is the payload), commits it as `github-actions[bot]`, and pushes to `main` with
`[skip ci]`. Every subsequent `go build` embeds the backdoored console into the shipped
binary. Because the commit skips CI and is authored by a bot, it reads as routine
maintenance noise in the log.

Note the repository's `main` branch is documented as unprotected, so no branch-protection
rule intercepts this push.

### Remediation

- Have the job **fail loudly** on drift rather than self-heal, and let a human open the
  fix PR. `frontend-dist-in-sync` already provides exactly this signal on PRs; the
  push-to-main variant should be a duplicate alarm, not an auto-writer.
- If auto-healing is kept: drop `[skip ci]` so the resulting commit is at minimum
  gitleaks-scanned, and use a PAT/App token that *does* retrigger CI, accepting one extra
  run in exchange for the artifact being inspected.
- Better still, stop committing build output: build `dist` in the release pipeline and
  drop it from the tree.

---

## CICD-004 — `publish-sdks.yml` is dispatchable from any ref, ungated, with long-lived registry tokens

- **Severity:** High · **Confidence:** 85
- **CWE:** CWE-284 (Improper Access Control), CWE-522 (Insufficiently Protected Credentials)
- **File:** `D:\Codebox\PROJECTS\AGEZT\.github\workflows\publish-sdks.yml:18-21, 50-60, 81-90, 108-117`

### Description

The workflow that pushes packages to **PyPI, npm and crates.io** triggers on:

```
on:
  release:
    types: [published]
  workflow_dispatch:
```

There is **no `environment:` block** on any of the three jobs. That means:

- No required reviewers on the publish step.
- No environment-scoped secrets — `PYPI_API_TOKEN`, `NPM_TOKEN` and
  `CARGO_REGISTRY_TOKEN` are plain repository secrets, readable by any workflow run that
  references them.
- No deployment branch restriction. `workflow_dispatch` lets the caller **choose the
  ref**, and for `workflow_dispatch` GitHub uses the *workflow file from the selected
  ref* — so the attacker-chosen branch supplies both the SDK source **and** the publish
  logic, including the `if [ -z "$TOKEN" ]` skip guard, which they can simply delete.

The credentials are also **long-lived static API tokens**. PyPI, npm and crates.io all
support OIDC Trusted Publishing / short-lived credentials, which would remove the
standing secret entirely.

### Concrete exploit scenario

An attacker obtains repository write access — via a compromised maintainer session, a
leaked PAT, or by pivoting from the self-hosted runner compromise in CICD-001/003. They:

1. Push a branch `chore/ci-tweak` containing a trojaned `sdk/python/agezt/__init__.py`
   and an edited `publish-sdks.yml` with the version bumped.
2. Open the Actions tab, select **Publish SDKs**, choose branch `chore/ci-tweak`, and
   press Run.
3. The `pypi` job builds from that branch and `twine upload`s to PyPI as the official
   `agezt` package. Same for `@agezt/sdk` on npm and `agezt` on crates.io.
4. Every downstream consumer who installs the SDK pulls the trojan. Nothing on `main`
   changed, no release exists, and the only trace is one workflow run.

Because these are the *official* client SDKs for an agent platform, the blast radius is
every integrator's environment, not just this repo.

### Remediation

- Add an `environment: release` to all three jobs, configured with **required reviewers**
  and a **deployment branch rule** restricted to `main` / tags. Move the three tokens to
  that environment so they are unreachable from an ad-hoc branch dispatch.
- Migrate to OIDC Trusted Publishing on all three registries (`id-token: write` +
  `pypa/gh-action-pypi-publish`, npm provenance, crates.io Trusted Publishing) and delete
  the static tokens.
- Restrict `workflow_dispatch` inputs, or drop the trigger and publish only on
  `release: published` from a tag.

---

## CICD-005 — staticcheck "checksum verification" fetches the checksum from the same origin as the artifact

- **Severity:** Medium · **Confidence:** 95
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-345 (Insufficient Verification of Data Authenticity)
- **File:** `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:481-498`

### Description

The step downloads the staticcheck tarball and its `.sha256` sidecar from the **same
GitHub release URL**, then compares them:

```
base="https://github.com/dominikh/go-tools/releases/download/${SC_TAG}"
curl -fsSL "${base}/staticcheck_linux_amd64.tar.gz"        -o /tmp/sc.tgz
curl -fsSL "${base}/staticcheck_linux_amd64.tar.gz.sha256" -o /tmp/sc.sha256
want=$(awk '{print $1}' /tmp/sc.sha256)
got=$(sha256sum /tmp/sc.tgz | awk '{print $1}')
```

Both operands come from the same mutable source. Anyone able to replace the tarball —
a compromised `dominikh` account, a stolen release token, or GitHub-side asset
replacement — replaces the sidecar in the same action, and the comparison passes. The
in-file comment calls it *"the publisher's signed .sha256 sidecar"*, but the file is a
bare hex digest with **no signature** and no independent trust anchor; nothing is
verified against a key. The check therefore detects transport corruption only, which
`curl -f` over TLS already covers.

The version pin (`SC_TAG=2026.1`) is real and valuable — but a git tag on a release is
mutable, so the pin alone does not close this.

### Concrete exploit scenario

`dominikh/go-tools`' release-publishing credential is compromised. The attacker replaces
`staticcheck_linux_amd64.tar.gz` on the `2026.1` release with a build that behaves
normally but also drops a payload, and updates `staticcheck_linux_amd64.tar.gz.sha256` to
match. The next AGEZT `lint` run downloads both, the digests agree, and
`/tmp/staticcheck/staticcheck ./...` executes the attacker's binary on the self-hosted
runner with full access to the persistent `$HOME`, the Go module cache and the network —
feeding directly into the CICD-001/003 persistence chain.

### Remediation

Pin the **literal expected digest** in the workflow, so integrity is anchored in the
reviewed repo rather than in the artifact's own origin:

```
SC_SHA256=<digest recorded at pin time>
echo "${SC_SHA256}  /tmp/sc.tgz" | sha256sum -c -
```

Bump the digest in the same commit as any `SC_TAG` bump. Extract to a `mktemp -d` path
rather than the fixed `/tmp` locations (see CICD-006).

---

## CICD-006 — Fixed `/tmp` and `/dev/shm` paths shared by three concurrent runners in one VM

- **Severity:** Medium · **Confidence:** 85
- **CWE:** CWE-377 (Insecure Temporary File), CWE-367 (TOCTOU Race Condition)
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\.github\actions\setup-go-safe\action.yml:29-36` — *"the three self-hosted runners (wsl-runner-1/2/3) all live in the SAME WSL VM and therefore share ONE /dev/shm"*
  - `ci.yml:158-162` (`e2e`: builds and then **executes** `/tmp/agezt`, `/tmp/agt`)
  - `ci.yml:316-328` (`webui-e2e`: builds and then **executes** the same two fixed paths)
  - `ci.yml:425` (`multi-arch`: all six matrix legs write `/tmp/out/`)
  - `ci.yml:489-497` (`lint`: `/tmp/sc.tgz`, `/tmp/sc.sha256`, `/tmp/staticcheck/`)

### Description

The composite action explicitly documents that all three runners share one VM — and
therefore one `/tmp` and one `/dev/shm`. It solves this for `GOROOT`, `GOCACHE` and
`GOTMPDIR` by using per-runner paths (`/dev/shm/goroot-${RUNNER_NAME}`), citing real
collisions (*"cp: cannot create directory: File exists", "compile: Text file busy"*).

The workflow itself never applies that lesson. Five separate job families write to
**fixed, predictable, shared paths**, and two of them build a binary at a fixed path and
then execute it:

- `e2e` and `webui-e2e` **both** `go build -o /tmp/agezt` / `-o /tmp/agt`, then run those
  binaries against a live daemon. These two jobs have no ordering constraint and run
  concurrently on different runners in the same VM.
- All six `multi-arch` legs emit into one shared `/tmp/out/`, mixing `linux/arm64`,
  `darwin/amd64` and `windows/amd64` artifacts in a single directory.
- `lint` extracts staticcheck to a fixed `/tmp/staticcheck/` and executes it from there.

`/tmp` is mode `1777`. The sticky bit stops deletion of another user's entries, but the
runners plausibly share a uid, and in any case nothing prevents one job from overwriting
a path another job is about to exec.

### Concrete exploit scenario

Two CI runs are in flight (a PR and a push to `main` — routine, and `concurrency` only
cancels within the same ref). Run A's `webui-e2e` finishes `go build -o /tmp/agezt`.
Between Run B's `e2e` build step and its `bash scripts/e2e-smoke.sh /tmp/agezt /tmp/agt`
step, Run A's build overwrites `/tmp/agezt`. Run B now boots and smoke-tests **a binary
built from a different commit** and reports the result as its own — a silent
false-green/false-red. Weaponized: a contributor with push access adds a test that loops
`cp ./evil /tmp/agezt`, and the concurrently-running `e2e` job of an unrelated build
executes the planted binary with that job's environment and credentials.

### Remediation

Apply the same discipline the composite action already uses. Replace every fixed path
with a per-job temp dir:

```
BIN="$(mktemp -d)"
go build -o "$BIN/agezt" ./cmd/agezt
```

and pass `$BIN/agezt` downstream. Same for `/tmp/out/` and the staticcheck extraction.
`e2e-smoke.sh` and `webui-e2e.sh` already do this correctly for their own state
(`TMP="$(mktemp -d)"` at `scripts/e2e-smoke.sh:19` and `scripts/webui-e2e.sh:22`) — only
the workflow's binary paths are unsafe.

---

## CICD-007 — `ci-go-retry.sh` wildcard-deletes other concurrently-running jobs' caches and exec dirs

- **Severity:** Medium · **Confidence:** 95
- **CWE:** CWE-362 (Race Condition), CWE-379 (Creation of Temp File in Directory with Incorrect Permissions)
- **File:** `D:\Codebox\PROJECTS\AGEZT\scripts\ci-go-retry.sh:31`

### Description

On any retry, the script runs:

```
rm -rf /dev/shm/gocache-* /dev/shm/gotmp-* 2>/dev/null || true
```

The glob spans **all three runners**. `setup-go-safe` deliberately chose per-runner paths
(`/dev/shm/gocache-${RUNNER_NAME}`, `/dev/shm/gotmp-${RUNNER_NAME}`) precisely so the
three runners-in-one-VM would not collide — and this one line defeats that isolation on
every retry.

`GOTMPDIR` is where `go test` writes each package's test binary and then **execs it**.
Deleting another job's `GOTMPDIR` mid-run therefore yanks executables out from under a
live test process on a different runner.

This is partly a reliability bug, but it is a security-relevant one: it is a cross-job
destructive write into shared mutable state, and it manufactures exactly the transient
"flaky compiler" failures that the retry logic then papers over — so induced failures are
indistinguishable from the genuine WSL corruption, and an attacker-triggered disruption
would be attributed to known flakiness.

### Concrete exploit scenario

Runner-1's `go test ./...` hits the WSL corruption and retries. Its `rm -rf` wipes
`/dev/shm/gotmp-wsl-runner-2` and `/dev/shm/gocache-wsl-runner-3` while runner-2 is
mid-`go test` and runner-3 is mid-`go build`. Both fail with exec/cache errors, both
retry (invoking the same wildcard delete), and the three runners can cascade into
mutual disruption. The retry ceiling is 5 per command, so a busy CI window can burn all
attempts and fail the build for reasons unrelated to the code — and a contributor who
wants to hide a real failure can rely on the noise.

### Remediation

Scope the cleanup to the current job only:

```
rm -rf "${GOCACHE:?}" "${GOTMPDIR:?}" 2>/dev/null || true
mkdir -p "$GOCACHE" "$GOTMPDIR"
```

Both variables are already exported into `$GITHUB_ENV` by `setup-go-safe`
(`action.yml:196-199`), so no new plumbing is needed.

---

## CICD-008 — GOROOT staged into shared `/dev/shm`; a co-resident job can tamper with another job's compiler

- **Severity:** Medium · **Confidence:** 70
- **CWE:** CWE-427 (Uncontrolled Search Path Element), CWE-732 (Incorrect Permission Assignment for Critical Resource)
- **File:** `D:\Codebox\PROJECTS\AGEZT\.github\actions\setup-go-safe\action.yml:59-70, 179-199`

### Description

The action copies the entire Go toolchain into `/dev/shm/goroot-${RUNNER_NAME}` and
points `GOROOT` there for the remainder of the job. `/dev/shm` is a world-writable
(`1777`) tmpfs shared by all three runners in the VM. `cp -a` preserves the source
permissions, so the staged tree is owner-writable — and if the three runner services run
under a **common user account** (the usual systemd-runner setup, and consistent with the
"same WSL VM" note), then a job on runner-2 can write into
`/dev/shm/goroot-wsl-runner-1/pkg/tool/linux_amd64/compile`.

That file is the Go compiler binary that runner-1's job is about to exec thousands of
times. Replacing it is arbitrary code execution inside another job's build, with that
job's environment and tokens.

Confidence is 70 rather than higher because I could not inspect the runners to confirm
they share a uid; if each runs under a distinct account the cross-job write is blocked by
ordinary DAC and this reduces to a hardening note. The path being in world-writable
`/dev/shm` at all is confirmed.

### Concrete exploit scenario

A same-repo PR (or a Dependabot PR, per CICD-001) runs a Go test that spawns a background
goroutine writing a trojaned `compile` binary into every `/dev/shm/goroot-*` it can open.
Concurrently, the push-to-`main` build's `frontend-dist-rebuild` and `multi-arch` jobs
compile with the tampered toolchain. The compiler injects a payload into the emitted
binaries; `frontend-dist-rebuild` then commits and pushes to `main` with `[skip ci]`
(CICD-003). The tampered toolchain lives in RAM and vanishes on reboot, leaving no
artifact to audit.

### Remediation

- Confirm whether the three runner services share a uid; if so, give each its own user.
- Stage GOROOT under a mode-`0700` directory the job owns, e.g.
  `install -d -m 700 /dev/shm/goroot-${RUNNER_NAME}` before `cp -a`, and verify the mode
  after staging.
- Longer term, fix the root cause: the whole tmpfs apparatus exists to dodge WSL2 ext4
  read corruption. Moving CI to ephemeral containers or a non-WSL host removes this
  entire class of workaround, along with CICD-006 and CICD-007.

---

## CICD-009 — `GITHUB_TOKEN` embedded in a `git push` URL argument

- **Severity:** Medium · **Confidence:** 80
- **CWE:** CWE-214 (Invocation of Process Using Visible Sensitive Information), CWE-522
- **File:** `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:276`

```
git push "https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git" HEAD:refs/heads/main
```

### Description

The token is passed as a **command-line argument**. `git` forwards the URL to a
`git-remote-https` child process, so the credential appears in `/proc/<pid>/cmdline`,
readable by any process running as the same user for the lifetime of the push. On this
pipeline, "any process as the same user" plausibly includes **jobs running concurrently
on the other two runners in the same VM** (see CICD-008).

The intent — avoiding credential persistence in `.git/config`, given
`persist-credentials: false` — is correct. The implementation trades a durable leak for a
transient but broader one. This token carries `contents: write` on the default branch.

Actions log-masking protects the *log*; it does not protect the process table. Git may
also surface the full remote URL in an error message on push failure.

### Concrete exploit scenario

A malicious test in a concurrently-running same-repo/Dependabot PR polls
`/proc/*/cmdline` for `x-access-token:`. When `frontend-dist-rebuild` pushes, the token
is captured and used within its remaining lifetime to push an arbitrary commit to `main`
— a direct write to the default branch of a repo whose `main` is unprotected.

### Remediation

Pass the credential via stdin-fed config rather than argv:

```
git -c http.extraheader="AUTHORIZATION: basic $(printf 'x-access-token:%s' "$GITHUB_TOKEN" | base64 -w0)" \
    push origin HEAD:refs/heads/main
```

or write a `~/.git-credentials` entry with mode `0600` and remove it in a `trap`. If
CICD-003 is remediated by removing the auto-push, this finding disappears with it.

---

## CICD-010 — `playwright install --with-deps` implies passwordless sudo on the self-hosted runner

- **Severity:** Low · **Confidence:** 70
- **CWE:** CWE-250 (Execution with Unnecessary Privileges)
- **File:** `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml:322-325`

`npx playwright install --with-deps chromium` runs `sudo apt-get install` for the browser's
system libraries. For this to succeed non-interactively, the runner account must have
**passwordless sudo**. That privilege is not scoped to this step — every job on the VM,
including any that executes unreviewed dependency code (CICD-001), inherits the ability to
become root and modify the host persistently. It also means CI mutates the shared VM's
system package set on every run.

**Remediation:** pre-install the Chromium system dependencies once during runner
provisioning and drop `--with-deps` (use plain `npx playwright install chromium`, which
needs no root). Then remove passwordless sudo from the runner account.

---

## CICD-011 — No build provenance / attestation on any published SDK

- **Severity:** Low · **Confidence:** 90
- **CWE:** CWE-345 (Insufficient Verification of Data Authenticity)
- **File:** `D:\Codebox\PROJECTS\AGEZT\.github\workflows\publish-sdks.yml:90, 60, 117`

`npm publish --access public` is issued without `--provenance`; the PyPI and crates.io
jobs likewise publish without attestations. Consumers of `@agezt/sdk`, `agezt` (PyPI) and
`agezt` (crates.io) have no cryptographic way to confirm a given release was built from
this repository by this workflow — which is precisely the check that would expose a
CICD-004 branch-dispatch publish.

**Remediation:** add `permissions: id-token: write` to the jobs and publish with
`npm publish --provenance`, `pypa/gh-action-pypi-publish` (Trusted Publishing), and
crates.io Trusted Publishing. This also removes the static tokens flagged in CICD-004.

---

# 2. Supply Chain & Dependency Audit (`sc-dependency-audit`)

## Dependency Audit Summary

```
Total dependencies:
  Go        24 modules in the build graph (5 direct + 7 indirect declared in go.mod;
            the remainder are test/tool-only graph entries). go.sum: 57 lines.
  npm (frontend)      28 declared (13 runtime + 15 dev) → 309 packages resolved in the lockfile
  npm (sdk/typescript) 2 declared (dev-only)            → 3 packages resolved
  PyPI (sdk/python)   0 runtime dependencies (stdlib only)
  crates.io (sdk/rust) 0 runtime dependencies (std only)
  Unmanaged/shadow    1 (monaco-editor, CDN-loaded — see DEP-001)

Ecosystems scanned: Go modules, npm, PyPI, crates.io
Known vulnerabilities: 0 reported by govulncheck (clean, per brief); npm tree UNSCANNED (DEP-003)
Typosquatting risks: 0
Dependency confusion risks: 0
License concerns: 0
Lock file coverage: 100% of manifests that need one
Outdated/stale: 5 (DEP-005, DEP-007)
```

## Verified strengths

- **Lock files complete.** `go.sum`, `frontend/package-lock.json`,
  `sdk/typescript/package-lock.json`, `sdk/rust/Cargo.lock` all present and committed.
  `sdk/python` needs none — zero runtime deps.
- **No `replace` or `exclude` directives in `go.mod`.** No local-path or vanity-URL
  redirection; every module resolves through the Go module proxy and checksum database.
- **Install-script surface is essentially nil.** Only two packages in the 309-package
  frontend tree declare `hasInstallScript`: `fsevents` and `vite/node_modules/fsevents`,
  both macOS-only optional deps. (This is what makes the `--ignore-scripts` fix in
  CICD-001 free.)
- **`overrides` are live and effective**, not dead config: `frontend/package-lock.json`
  resolves `dompurify` to **3.4.12** (a transitive dep requests `3.2.7`) and `undici` to
  **7.28.0** (a dep requests `^7.25.0`). Both overrides are doing real work.
- **No typosquatting.** Every direct npm dependency is a well-known package under its
  canonical name/scope (`@radix-ui/*`, `@xyflow/react`, `lucide-react`, `clsx`,
  `tailwind-merge`, `class-variance-authority`, `@fontsource-variable/*`). No character
  transposition, scope confusion, or hyphen/underscore variants.
- **No dependency confusion vector.** No private registry config, no `.npmrc` pointing at
  mixed sources, and the published packages are either scoped (`@agezt/sdk`) or already
  owned (`agezt` on PyPI/crates.io).
- **Zero-dependency SDKs.** Python (urllib+json), Rust (std), TypeScript (platform fetch)
  ship no runtime deps at all — an unusually strong posture for published client libraries,
  and it eliminates the entire downstream transitive-risk surface.
- **All licenses compatible.** Project is MIT; the dependency set is MIT/BSD/Apache-2.0.
  No GPL/AGPL, no unlicensed, no proprietary.
- **Dependabot covers the right ecosystems** — `gomod`, both npm directories, and
  `github-actions` (which is what keeps the SHA pins fresh).

---

## DEP-001 — Monaco editor loaded from a third-party CDN into the admin console, outside every supply-chain control

- **Severity:** Medium · **Confidence:** 95
- **CWE:** CWE-829 (Inclusion of Functionality from Untrusted Control Sphere), CWE-1104 (Use of Unmaintained/Unmanaged Third-Party Components)
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\frontend\src\lib\monaco.ts:11-13, 22`
  - `D:\Codebox\PROJECTS\AGEZT\frontend\src\components\MonacoView.tsx:21`
  - shipped: `D:\Codebox\PROJECTS\AGEZT\kernel\webui\dist\assets\vendor-BuaPXTA3.js`

### Description

```ts
export const PINNED_MONACO_VERSION = "0.52.2";
export const MONACO_CDN_BASE = `https://cdn.jsdelivr.net/npm/monaco-editor@${PINNED_MONACO_VERSION}/min/vs`;
…
loader.config({ paths: { vs: MONACO_CDN_BASE } });
```

`monaco-editor` is **not a declared dependency**. It appears nowhere in
`frontend/package.json` and nowhere in `frontend/package-lock.json`. It is a bare version
string in a source file, fetched at runtime from a public CDN.

Consequences — this ~3 MB of JavaScript is:

- **outside the lockfile** — no integrity hash, no resolved-version guarantee;
- **outside Dependabot** — never bumped, never advisory-checked;
- **outside `tools/depscheck`** — the Go side enforces a strict 24-module allowlist where
  every entry needs a justification row in `DEPENDENCIES.md`; the largest single
  third-party code blob in the product is governed by none of that;
- **without SRI** — the AMD loader fetches further chunks by path, so subresource
  integrity cannot be applied to them;
- destined for the **admin console** of an autonomous agent daemon, a page that handles
  provider API keys, vault contents, workflow definitions and the `?token=` credential.

There is a saving grace, and it is worth stating precisely because it is fragile: the
daemon's CSP at `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:1316-1327` is
`default-src 'none'; script-src 'self'; connect-src 'self'; …`, which **blocks
cdn.jsdelivr.net outright**. So today the fetch fails and Monaco does not load — the
editor is broken rather than dangerous. That is an accidental mitigation, not a designed
one, and the obvious "fix" a developer reaches for when the editor doesn't render is to
add `https://cdn.jsdelivr.net` to `script-src` — converting a broken feature into a
third-party-code-execution channel in the admin console.

### Concrete exploit scenario

A developer notices the code editor is blank, traces it to a CSP violation in the console,
and relaxes `script-src` to `'self' https://cdn.jsdelivr.net` (plus `connect-src` for the
worker chunks). The console now executes ~3 MB of JavaScript fetched at page load from a
CDN, with no integrity check, on an origin that holds an authenticated admin session.
A jsDelivr compromise, a BGP/DNS hijack of the CDN, or a malicious `monaco-editor`
publish then yields script execution inside the console — with access to
`connect-src 'self'` and thus to every `/api/*` route, including provider credentials and
the vault.

### Remediation

Self-host it, exactly as the source comment already anticipates (*"To self-host later,
point `paths.vs` at the vendored `monaco-editor/min/vs` directory"*):

1. `npm i -D monaco-editor@<version>` so it lands in `package.json` + `package-lock.json`
   and comes under Dependabot and npm audit.
2. Copy `monaco-editor/min/vs` into the Vite build output and point
   `loader.config({ paths: { vs: "/assets/vs" } })` at it.
3. Keep the CSP as `script-src 'self'` — with self-hosting it needs no relaxation, and
   the bundle becomes a reviewed, integrity-checked, `go:embed`-ded artifact like the rest
   of `dist`.

If the ~3 MB binary-size cost is unacceptable, the alternative is to drop the editor —
not to open the CSP.

---

## DEP-002 — Shipped bundle requests Monaco `0.55.1`; reviewed source pins `0.52.2`

- **Severity:** Medium · **Confidence:** 95
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-1104
- **Files:**
  - source pin: `D:\Codebox\PROJECTS\AGEZT\frontend\src\lib\monaco.ts:11` → `"0.52.2"`
  - shipped artifact: `D:\Codebox\PROJECTS\AGEZT\kernel\webui\dist\assets\vendor-BuaPXTA3.js` → `monaco-editor@0.55.1`

### Description

Grepping the committed, `go:embed`-ded bundle for the CDN URL yields
`https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs`, while the TypeScript source
that produces that string declares `PINNED_MONACO_VERSION = "0.52.2"`.

The reviewed source and the artifact that actually ships **disagree about which
third-party version to execute**. Whichever direction the drift runs, the property that
matters is broken: reading `monaco.ts` does not tell you what the product loads.

This is the same class as the project's known "stale checkout" hazard, but with
supply-chain rather than merely cosmetic consequences — and it is possible here *only*
because DEP-001 keeps Monaco outside the lockfile. A locked dependency cannot drift this
way; an interpolated string in a committed build artifact can.

The `frontend-dist-in-sync` CI gate should catch this on the next run of a job that
rebuilds `dist`, which suggests the drift is recent or that `dist` was hand-resolved
during a rebase (a documented hazard in this repo, where rebase conflicts are routinely
dist-only).

### Concrete exploit scenario

A reviewer audits `monaco.ts`, confirms `0.52.2` is a version they have vetted, and
approves. Operators run a binary that fetches `0.55.1` — a version nobody reviewed, whose
advisories nobody checked. If `0.55.1` carries a known XSS in the editor's rendering path,
or is later yanked/compromised, the vetting record points at the wrong artifact entirely
and no tooling contradicts it.

### Remediation

Resolve the drift (rebuild `dist` from source and commit) as an immediate step, then
remove the possibility by adopting DEP-001's remediation: once `monaco-editor` is a real
locked npm dependency bundled into `dist`, source and artifact cannot disagree.

---

## DEP-003 — No npm vulnerability scanning anywhere in CI

- **Severity:** Medium · **Confidence:** 95
- **CWE:** CWE-1104 (Use of Unmaintained Third-Party Components), CWE-937
- **Files:** `D:\Codebox\PROJECTS\AGEZT\.github\workflows\ci.yml` (entire file — absence of a gate)

### Description

The Go side is well guarded: `govulncheck` (`ci.yml:509-528`), a hard module allowlist
(`tools/depscheck`, 24 entries), an SDK-parity check, and a dead-code check.

The JavaScript side has **no vulnerability gate of any kind**. There is no `npm audit`,
no `osv-scanner`, no Snyk/Trivy, no SBOM generation — verified by searching the whole
`.github/` tree and the `Makefile`. The `frontend-test` job runs `knip` (dead-code) and
Vitest; neither looks at advisories.

That leaves **309 resolved packages** — which are compiled into `kernel/webui/dist/` and
`go:embed`-ded into the shipped `agezt` binary — with zero automated advisory coverage.
The `overrides` block pinning `dompurify` and `undici` is evidence that someone
*manually* tracked two advisories; manual tracking is what a gate replaces.

The asymmetry is the finding: the Go dependency set (24 modules, all backend) is governed
by an allowlist requiring written justification per module, while the larger and more
directly attacker-facing JavaScript set (309 packages rendering an authenticated admin
console) is governed by nothing.

### Concrete exploit scenario

A prototype-pollution or XSS advisory lands against a transitive frontend package — the
markdown/sanitization path is the obvious candidate given `dompurify` is already in the
tree via `jsdom`, and the console renders agent-produced and channel-inbound content.
Nothing in CI reports it. Dependabot may open a PR eventually, but nothing *fails*, so
the vulnerable bundle keeps shipping inside the binary. Meanwhile agent output — which is
attacker-influenceable via the ~27 inbound comm channels — is rendered by the vulnerable
component in a page holding an admin session.

### Remediation

Add a gate to the `frontend-test` job:

```
- run: cd frontend && npm audit --audit-level=high
```

or, for lower noise and better lockfile fidelity, `osv-scanner --lockfile=frontend/package-lock.json`
(which also covers `sdk/typescript/package-lock.json`, `go.sum` and `Cargo.lock` in one
pass). Treat `high`/`critical` as failing and allowlist exceptions explicitly, mirroring
how `.gitleaks.toml` handles its known-noise cases.

---

## DEP-004 — `setuptools>=61` is unpinned in the PyPI publish path

- **Severity:** Medium · **Confidence:** 90
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-829
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\sdk\python\pyproject.toml:1-3`
  - `D:\Codebox\PROJECTS\AGEZT\.github\workflows\publish-sdks.yml:40-49`

### Description

The publish workflow pins its tools deliberately and documents why:

```
python -m pip install 'build==1.2.2' 'twine==6.1.0'
```

with the comment: *"a malicious or breaking release on PyPI would otherwise flow straight
into the release pipeline and ship a tarball we never reviewed."* Exactly right — and the
pin is incomplete. `python -m build` creates a **PEP 517 isolated build environment** and
installs whatever `[build-system] requires` names:

```toml
[build-system]
requires = ["setuptools>=61"]
build-backend = "setuptools.build_meta"
```

`setuptools>=61` is an unbounded floating range resolved from PyPI **at build time**, and
a build backend executes arbitrary Python during `build_meta` invocation. So the exact
threat the comment describes remains open through the back door: an unreviewed, unpinned
package runs code in the release pipeline, in the same job that holds `PYPI_API_TOKEN`.

Dependabot does not close this either — `.github/dependabot.yml` has no `pip` ecosystem
entry, so the build requirement is never bumped or watched.

### Concrete exploit scenario

`setuptools`' PyPI account or one of its release automations is compromised and a
malicious version is published. The next AGEZT release triggers `publish-sdks.yml`;
`python -m build` fetches the newest matching `setuptools` into the isolated env and
invokes `build_meta`, executing attacker code inside the `pypi` job. The job's environment
contains `TWINE_PASSWORD` (the PyPI API token) in the very next step, and the payload can
also alter the wheel contents before `twine upload` ships them as the official `agezt`
package.

### Remediation

Pin the build backend with an upper bound and a hash where practical:

```toml
[build-system]
requires = ["setuptools==<pinned>", "wheel==<pinned>"]
build-backend = "setuptools.build_meta"
```

Add a `pip` entry to `.github/dependabot.yml` for `/sdk/python` so the pin is maintained.
Optionally pass `--no-isolation` in CI with a pre-pinned, hash-checked environment so the
build cannot reach PyPI at all during packaging.

---

## DEP-005 — Beta-quality IMAP parser on the attacker-controlled inbound-email path

- **Severity:** Low · **Confidence:** 85
- **CWE:** CWE-1104 (Use of Unmaintained/Pre-Release Third-Party Component)
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\go.mod:8` — `github.com/emersion/go-imap/v2 v2.0.0-beta.8`
  - `D:\Codebox\PROJECTS\AGEZT\go.mod:17` — `github.com/emersion/go-message v0.18.2` (indirect)
  - consumer: `D:\Codebox\PROJECTS\AGEZT\plugins\channels\email\inbound.go`

### Description

`go-imap/v2` is at **`v2.0.0-beta.8`** — an explicitly pre-release version, with no
stability or security-support commitment from upstream — and it is used to parse
**inbound email**, i.e. wholly attacker-controlled bytes arriving from the public
internet. `go-message` (the MIME/header parser it pulls in) sits on the same path.

Parsers of hostile input are the highest-risk dependency category there is, and this is
the one module in the entire 24-module Go graph that is both pre-release and
adversary-facing. `govulncheck` is clean today, which means no *known* advisory — beta
software's characteristic risk is the advisory that has not been filed yet, and a
pre-release project is less likely to have had the fuzzing and audit attention that a
stable one accumulates.

Severity is Low rather than higher because there is no known vulnerability, the email
channel is opt-in, and the surrounding architecture (capability governance, warden,
default-deny Edict on unmapped tools) constrains what a parser compromise reaches.

### Concrete exploit scenario

An attacker sends a crafted message to a mailbox an AGEZT email channel polls — malformed
MIME boundaries, a pathological header, a deeply nested multipart structure. A parsing
defect in the beta code yields a panic (denial of service for the channel, or the daemon
if unrecovered), unbounded allocation (memory exhaustion), or in the worst case a memory-
safety-adjacent logic error that lets header content cross into a trusted field. The
attacker needs only the mailbox address, which is by definition public.

### Remediation

- Track `emersion/go-imap/v2` and upgrade to `v2.0.0` stable as soon as it is tagged; add
  it to a watch list so the bump is not missed.
- In the meantime, harden the boundary rather than the dependency: ensure
  `plugins/channels/email/inbound.go` runs parsing under a `recover()`, caps message and
  attachment size before parsing, and bounds multipart nesting depth. A panic in a channel
  goroutine should degrade that channel, never the daemon.
- Consider fuzzing the inbound path (`go test -fuzz`) against the parser — cheap, and it
  targets exactly the risk beta status represents.

---

## DEP-006 — `tools/depscheck` allowlist is a superset of actual usage

- **Severity:** Low · **Confidence:** 95
- **CWE:** CWE-1061 (Insufficient Encapsulation) / policy drift
- **File:** `D:\Codebox\PROJECTS\AGEZT\tools\depscheck\allowlist.txt`

### Description

The allowlist contains 24 module paths. Its stated policy is that *"Every entry MUST also
have a row in DEPENDENCIES.md justifying it (POLICY §1.1)"* — a strong, deliberate
control, and the reason the Go dependency surface is as tight as it is.

But the list has drifted into a superset. `github.com/yuin/goldmark` is allowlisted while
being **imported by no Go file in the repository** — its only occurrences are inside
`tools/depscheck/main_test.go` (as test data for the checker itself). Several `golang.org/x/*`
entries (`x/mod`, `x/tools`, `x/sync`, `x/xerrors`, `x/term`, `x/text`) and the testify
chain (`stretchr/testify`, `davecgh/go-spew`, `pmezard/go-difflib`) are likewise graph-only
or test-only rather than production dependencies.

An allowlist entry is a **standing grant**. Every stale entry is a module that can be
introduced into production code later — with all its transitive weight — and the gate
designed to force that conversation will stay silent.

### Concrete exploit scenario

A contributor (or an agent operating on the codebase) adds `goldmark` to a production
markdown-rendering path. `deps-check` passes, because `goldmark` is pre-approved. No
`DEPENDENCIES.md` row is written, no reviewer is prompted to weigh a new parser on a
content path, and the module — currently pinned at the aging `v1.4.13` — enters the
shipped binary without the scrutiny the policy exists to guarantee.

### Remediation

Split the file into `allowlist.txt` (production imports) and `allowlist-test.txt`
(test/tool-only graph entries), and have `depscheck` verify that every production entry is
actually imported by non-test code — turning the allowlist from a permission list into a
two-way assertion. Prune `goldmark` if it is genuinely unused. Reconcile the result against
`DEPENDENCIES.md`.

---

## DEP-007 — Stale transitive module graph

- **Severity:** Low · **Confidence:** 90
- **CWE:** CWE-1104 (Use of Unmaintained Third-Party Components)
- **File:** `D:\Codebox\PROJECTS\AGEZT\go.mod` / `go.sum` (resolved graph)

`go list -m all` shows a graph split sharply by age. The maintained edge is current
(`golang.org/x/net v0.57.0`, `golang.org/x/crypto v0.54.0`, `golang.org/x/sys v0.47.0`),
but a cluster of entries is years behind:

| Module | Resolved | Note |
|--------|----------|------|
| `golang.org/x/tools` | v0.6.0 | ~Feb 2023 |
| `golang.org/x/mod` | v0.8.0 | ~2023 |
| `golang.org/x/sync` | v0.1.0 | ~2022 |
| `github.com/stretchr/testify` | v1.8.0 | ~2022 |
| `github.com/yuin/goldmark` | v1.4.13 | ~2022 |

These are test/tool-only graph entries with no known advisory (`govulncheck` clean), so
impact today is nil — this is recorded as a maintenance signal, not a live vulnerability.
The reason it matters at all is DEP-006: because these modules are already allowlisted, a
future production import would pull a years-old version by default.

**Remediation:** run `go get -u ./... && go mod tidy` on a maintenance branch, verify
`deps-check` and `govulncheck` stay green, and commit. Low priority.

---

# 3. Sensitive Data Exposure (`sc-data-exposure`)

## How redaction is wired (needed to read the findings)

`kernel/redact` is a good redactor with a well-chosen pattern set, and it is installed at
the correct architectural chokepoint: `cmd/agezt/main.go:873` and `:1068` call
`Bus().SetRedactor(redactor)`, and `kernel/bus/bus.go:198` redacts **before**
`journal.Append` — so redaction happens before hashing and before the write, which is the
only ordering that works for an append-only chain. It is on by default
(`daemonconfig.go:512`: `Redact = AGEZT_REDACT != "off"`), and it is seeded with the
**actual literal values** from the credentials vault, so a configured provider key is
scrubbed wherever it appears regardless of shape. `bus.go:106` even disables JSON HTML
escaping so `&`/`<`/`>` inside a secret cannot hide from the literal scrubber. That is
careful, deliberate work.

The findings below are about the paths that go **around** that chokepoint, the shapes the
pattern set misses, and the file permissions on the store it protects.

One structural fact governs the whole section: **the journal is append-only and
hash-chained**. There is no purge, redact-after-the-fact, or rewrite path — the only
recovery is `Restore` into an empty directory (`kernel/journal/journal.go:334`). Every
redaction miss is therefore **permanent and unremediable**. That raises the severity of
every gap below relative to the same gap in a rotating log.

---

## EXPOSE-001 — Journal segments are world-readable while every sibling secret store is 0700/0600

- **Severity:** High · **Confidence:** 98
- **CWE:** CWE-732 (Incorrect Permission Assignment for Critical Resource), CWE-532 (Insertion of Sensitive Information into Log File), CWE-200
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\journal\journal.go:73, 369, 381, 494, 517`

### Description

Every journal path is created world-readable:

| Line | Call | Mode |
|------|------|------|
| `journal.go:73` | `os.MkdirAll(dir, 0o755)` — `Open` | **0755 dir** |
| `journal.go:369` | `os.MkdirAll(dir, 0o755)` — `Restore` | **0755 dir** |
| `journal.go:381` | `os.OpenFile(path, …, 0o644)` — restored segment | **0644 file** |
| `journal.go:494` | `os.OpenFile(…, 0o644)` — rotation | **0644 file** |
| `journal.go:517` | `os.OpenFile(path, flag, 0o644)` — `openCurrent` | **0644 file** |

This is inconsistent with **every other secret-bearing store in the tree**, all of which
were deliberately locked down:

| Store | File | Mode |
|-------|------|------|
| Credentials vault | `kernel/creds/creds.go:212` | `0600` |
| Artifacts | `kernel/artifact/artifact.go:52, 79` | `0700` dir / `0600` file |
| Auth token files | `kernel/auth/tokenfile.go:30, 34` | `0700` dir / `0600` file |
| Data lake | `kernel/datalake/datalake.go:116, 504` | `0700` / `0600` |
| Agent gateway secret | `kernel/agentgw/secret.go:50` | `0700` |
| **Journal** | `kernel/journal/journal.go` | **0755 / 0644** |

The journal is not a low-value store. Confirmed contents:

- **Full user prompts, verbatim and uncapped** — `kernel/agent/run_setup.go:68`
  (`"intent": userIntent`), published at `kernel/agent/agent.go:947`.
- **LLM answers** — `kernel/agent/agent.go:1204-1209` (`"answer"`, length-capped).
- **Raw tool arguments** — `kernel/agent/run_tools.go:230-234` (`"input": tc.Input`, the
  raw provider JSON) and `kernel/toolexec/toolrun.go:90-96`.
- **Tool outputs** — `kernel/agent/run_tools.go:409`, `kernel/toolexec/toolrun.go:115`.
- **HTTP request headers including `Authorization`, and response headers including
  `Set-Cookie`** — `plugins/tools/http/http.go:160, 171, 254`.

Because `internal/paths/paths.go:22-31` does not create `~/.agezt` itself, `journal.Open`'s
`MkdirAll(…, 0o755)` typically creates the **base directory at 0755 as well**.

### Concrete exploit scenario

AGEZT runs on a shared Linux host — a build server, a jump box, a lab machine, or a VPS
with several operator accounts. A low-privileged local user with no AGEZT credentials
whatsoever runs:

```
cat /home/operator/.agezt/journal/*.jsonl
```

They obtain the complete history of every agent run: every prompt the operator typed
(business context, internal hostnames, incident detail), every shell command the agent
executed with full argv, every file the agent read, every URL fetched including query
strings, and every HTTP header the `http` tool sent or received — `Authorization: Basic …`,
`X-Api-Key: …`, `Set-Cookie: session=…` — none of which the redactor covers (EXPOSE-009).

The control-plane token, the console password gate, the tenant routing and the
capability governor are all bypassed: none of them mediate the filesystem. And because
the log is append-only and hash-chained, the exposure covers the **entire retained
history**, not a window.

### Remediation

Bring the journal in line with its siblings — a four-line change:

```go
os.MkdirAll(dir, 0o700)                       // journal.go:73, :369
os.OpenFile(path, flag, 0o600)                // journal.go:381, :494, :517
```

Ship a one-time startup migration that `chmod`s an existing `journal/` directory and its
segments down to `0700`/`0600`, since fixing the constants alone leaves already-created
files exposed. Add a permission assertion to the journal test suite so this cannot
regress.

---

## EXPOSE-002 — `agentgw` audit writes directly to the append-only chain, bypassing the redactor

- **Severity:** High · **Confidence:** 95
- **CWE:** CWE-532, CWE-200, CWE-693 (Protection Mechanism Failure)
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\audit.go:97` — `if _, err := a.j.Append(spec); err != nil {`
  - wiring: `D:\Codebox\PROJECTS\AGEZT\kernel\runtime\runtime.go:937` — `agentGW.SetAuditJournal(j)`
  - entry struct: `D:\Codebox\PROJECTS\AGEZT\kernel\agentgw\types.go:116-128`

### Description

There are exactly **two** non-test callers of `journal.Append` in the codebase.
`kernel/bus/bus.go:198` is the correct one — it redacts first. The other,
`kernel/agentgw/audit.go:97`, holds a direct `*journal.Journal` reference (injected at
`runtime.go:937`) and appends **straight to the hash chain with the bus, and therefore the
redactor, entirely out of the path**.

Nothing between `AuditEntry` and the permanent record touches `kernel/redact`. The entry
carries the request method, `r.URL.Path`, the client `RemoteAddr`, the capability, the
run ID, and a free-text **error string** (`kernel/agentgw/gateway.go:271-284`).

This is a protection-mechanism failure rather than a missing feature: the redactor exists,
is enabled, and is correctly ordered on the *other* write path. One component reaches
past it into the same immutable store.

### Concrete exploit scenario

An in-process agent calls the gateway with a credential in the request path, or receives an
upstream error whose text embeds one — for example a config-center fetch failing with
`invalid api key: sk_live_…`, or a path carrying `?token=…`. The audit hook writes the
entry verbatim into the hash-chained journal. Even with `AGEZT_REDACT` on, the vault
literals seeded and every pattern matching, the secret is stored in cleartext — and
because the chain is append-only, **it can never be removed**. An operator who later
discovers the leak has no remediation short of destroying and re-creating the journal,
losing the entire audit history in the process.

### Remediation

Route the audit through the bus so it inherits the redactor, which is what every other
producer does:

```go
// kernel/agentgw/audit.go — replace the direct journal handle with the bus
a.bus.Publish(ctx, spec)
```

If a direct handle is required for ordering or to avoid subscriber fan-out, inject the
`*redact.Redactor` into the audit writer and apply `RedactBytes` to `spec.Payload` before
`Append`, mirroring `bus.go:92-120` exactly. Add a test asserting that a seeded literal
placed in `AuditEntry.Error` does not appear in the journal file.

---

## EXPOSE-003 — `/api/webhook_log` returns full outbound sink URLs, which for Slack/Discord/Teams *are* the credential

- **Severity:** High · **Confidence:** 90
- **CWE:** CWE-200, CWE-522 (Insufficiently Protected Credentials), CWE-598 (Information Exposure Through Query String)
- **Files:**
  - emitter: `D:\Codebox\PROJECTS\AGEZT\kernel\webhook\webhook.go:163, 183` — `"url": sink.URL`
  - handler: `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\webhook_log.go:20-56` (field `url`)
  - aggregation: `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\webhook_log.go:103-105` (`by_url` breakdown)
  - route: `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:389`

### Description

Outbound webhook delivery telemetry records the **full configured sink URL**, and
`/api/webhook_log` serves it, with `handleWebhookStats` additionally keying an entire
`by_url` aggregation on those URLs.

For the three most common webhook targets the URL is not an address — it is a bearer
credential embedded in a path:

```
https://hooks.slack.com/services/T00000000/B00000000/<24-char secret>
https://discord.com/api/webhooks/<id>/<68-char token>
https://outlook.office.com/webhook/<guid>@<guid>/IncomingWebhook/<hash>/<guid>
```

Anyone holding that string can post arbitrary messages into the channel, indefinitely,
with no further authentication.

The redactor does not save this. `kernel/redact/redact.go:55-100` has **no pattern for
webhook URLs**, and the literal seed set is drawn only from the credentials vault
(`cmd/agezt/main.go:2631-2641`) — an `AGEZT_WEBHOOKS` sink secret is never registered as a
literal. So the URL passes redaction untouched into the journal, and out through the API.

### Concrete exploit scenario

An operator configures a Slack sink for agent alerts. Every delivery attempt journals the
full URL. Two paths to compromise, both realistic:

1. **Local read.** Combined with EXPOSE-001, any local user on the host reads
   `~/.agezt/journal/*.jsonl` and lifts the Slack webhook URL — no AGEZT credential
   needed. They then post convincing messages into the operator's alert channel,
   impersonating the agent, which is an excellent phishing primitive precisely because
   that channel is already trusted to carry automated security notices.
2. **Authenticated read at the wrong tier.** Any holder of a `TierUser` console
   credential — or a password-only session when `AGEZT_WEB_PASSWORD` is set without strict
   mode (`kernel/webui/webui.go:1443-1452`) — fetches `/api/webhook_log` and reads the
   credential directly from the API. That is a privilege escalation from "can view the
   console" to "can post as the organization's alerting bot".

### Remediation

- Stop recording the full URL. Record a **stable non-reversible sink identifier**
  (a configured sink name, or `sha256(url)[:12]`) plus scheme+host for diagnostics:
  `hooks.slack.com/services/…` truncated at the path root. That preserves every
  operational use of the field — grouping, error attribution, `by_url` stats — with none
  of the credential.
- Add redact patterns for the known webhook shapes as defense in depth
  (`hooks\.slack\.com/services/\S+`, `discord(app)?\.com/api/webhooks/\S+`,
  `outlook\.office\.com/webhook/\S+`).
- Seed `AGEZT_WEBHOOKS` sink secrets into the redactor's literal set alongside the vault
  credentials at `cmd/agezt/main.go:2631-2641`.

---

## EXPOSE-004 — Redaction is nil-by-default outside `cmd/agezt`, and three redactors run with an empty literal set

- **Severity:** Medium · **Confidence:** 92
- **CWE:** CWE-1188 (Insecure Default Initialization), CWE-532
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\bus\bus.go:93-95` — `if b.redactor == nil { return spec }`
  - only wiring: `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\main.go:873`, `:1068`
  - empty-literal redactors: `kernel/openaiapi/openaiapi.go:50`, `kernel/controlplane/remote_mirror.go:256`, `plugins/builtintools/plugins.go:85`

### Description

Two related default-posture problems.

**(a) The redactor is opt-in at the library layer.** `bus.Bus.redactor` is `nil` until
someone calls `SetRedactor`, and the *only* callers are in `cmd/agezt/main.go`. A consumer
who embeds the kernel — via `kernelruntime.Open` or `bus.New(j)` directly, which is
exactly what the published Agent SDK and any downstream integrator would do — gets a
**completely unredacted, permanent, hash-chained journal** with no warning and no log line
saying so. The safe configuration lives in the binary rather than in the library, so every
embedder must rediscover it.

**(b) Three redactors run without literals.** `redact.New()` starts with an empty literal
set; only `SetSecrets` populates it. These three call sites never call it:

| Site | Protects | Literal set |
|------|----------|-------------|
| `kernel/openaiapi/openaiapi.go:50` (used `:53`, `:713`, `:765`, `responses.go:309`) | Upstream provider error strings echoed to HTTP clients | **empty** |
| `kernel/controlplane/remote_mirror.go:256` | Event payloads mirrored to a **remote peer** | **empty** |
| `plugins/builtintools/plugins.go:85` (used `:99`, `:158`) | External plugin stdout/stderr log lines | **empty** |

So on these three paths the operator's own configured provider keys are invisible to the
scrubber — only the generic patterns apply. The remote-mirror case is the sharpest: it is
the one path that ships event payloads **off the host**.

### Concrete exploit scenario

An operator configures a provider whose key shape is not in the pattern list — a Mistral
key (bare 32-char alphanumeric, EXPOSE-009), an Azure OpenAI key, or a self-hosted gateway
token. On the primary bus this is still caught, because vault literal seeding covers *any*
shape. But when that key appears in a payload mirrored to a remote peer via
`remote_mirror.go:256`, the fresh redactor has no literals and no matching pattern, and the
key is transmitted in cleartext to another machine. The one control that would have caught
an unknown-shape secret is precisely the one these three sites lack.

### Remediation

- Make the safe path the default: have `bus.New` install a pattern-only `redact.New()`
  automatically, so `SetRedactor` **upgrades** the literal set rather than enabling
  protection. An embedder then gets pattern coverage for free and full coverage when they
  wire the vault.
- Share one redactor instance. Inject the process-wide redactor (seeded at `main.go:647`,
  refreshed on reload at `:665`) into `openaiapi`, `remote_mirror` and `builtintools`
  instead of each constructing its own — a single `SetSecrets` on rotation then updates
  every sink at once.
- Log the redaction posture at boot for embedders, as `main.go:1167` already does for the
  daemon.

---

## EXPOSE-005 — Upstream provider error bodies relayed verbatim to clients

- **Severity:** Medium · **Confidence:** 90
- **CWE:** CWE-209 (Generation of Error Message Containing Sensitive Information), CWE-200
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\webui\transcribe.go:60` — `writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\webui\tts.go:49` — same shape
  - `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:1120` — `/api/run` SSE: `write(map[string]any{"kind":"error","error": err.Error()})`; also `:1161`, `:1195`
  - error origins: `plugins/providers/voice/voice.go:256, :307`; `deepgram.go:86, :140`; `elevenlabs.go:62, :113`; `cartesia.go:72`; `plugins/providers/openai/openai.go:122`; `plugins/providers/anthropic/anthropic.go:119`
  - **the mitigation that already exists elsewhere:** `D:\Codebox\PROJECTS\AGEZT\kernel\openaiapi\openaiapi.go:42-53`

### Description

The voice and run paths return `err.Error()` straight to the HTTP client. Those errors are
constructed from the **raw upstream response body**:

```go
fmt.Errorf("voice: STT status %d: %s", resp.StatusCode, string(respBytes))   // voice.go:256
fmt.Sprintf("openai: status %d: %s", e.Status, e.Body)                        // openai.go:122
```

Provider error bodies are not neutral. On a 401 OpenAI returns a message of the form
`Incorrect API key provided: sk-proj-…ABCD`, echoing a prefix and suffix of the key that
was sent. Others echo organization IDs, project IDs, account identifiers, rate-limit quotas
and internal request IDs.

What makes this a clear defect rather than a judgement call is that **the project already
identified and fixed this exact issue on a neighbouring surface**.
`kernel/openaiapi/openaiapi.go:42-53` installs a dedicated `errRedactor` with the comment
that live HTTP responses bypass the journal redactor and so must be scrubbed at egress
(logged as VULN-012). That mitigation was never propagated to `kernel/webui`. The
inconsistency means the OpenAI-compat surface is protected while the console's own voice
and run endpoints are not.

### Concrete exploit scenario

An operator's STT key expires or is entered with a typo. They open Voice Mode; the browser
POSTs to `/api/transcribe`; the provider returns 401 with a body echoing the key prefix;
`transcribe.go:60` serializes it into the JSON response. The key fragment now sits in the
browser's network log, in any frontend error-reporting pipeline, and in the browser console
where it is trivially screenshotted into a bug report or support ticket. On the `/api/run`
SSE path (`webui.go:1120`) the same body is streamed into the chat transcript itself —
which is journaled, turning a transient provider error into a permanent record.

### Remediation

Reuse the existing mitigation rather than inventing a second one. Apply `openaiapi`'s
`redactErr` pattern to `kernel/webui`:

```go
// kernel/webui — one shared egress redactor, seeded from the process redactor
writeJSON(w, http.StatusBadGateway, map[string]any{"error": s.redactErr(err)})
```

Better still, return a generic client-facing message ("speech-to-text provider
unavailable") plus a correlation ID, and log the detailed upstream body server-side at
`slog.Debug`. Apply to `transcribe.go:60`, `tts.go:49`, and `webui.go:1120, :1161, :1195`.

---

## EXPOSE-006 — Full Web UI admin token printed in a URL query string and passed as a process argument

- **Severity:** Medium · **Confidence:** 95
- **CWE:** CWE-598 (Information Exposure Through Query String), CWE-214 (Invocation of Process Using Visible Sensitive Information), CWE-532
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:153` — `consoleURL := localURL + "/?token=" + token`
  - `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:163` — `openBrowser(consoleURL)`
  - `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:230-241` — `openBrowser`

### Description

The console admin token is emitted **in full** in a URL query string, printed to stdout in
the boot banner, and then passed as a command-line argument to the OS browser opener:

```go
cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)  // windows
cmd = exec.Command("open", url)                                      // darwin
cmd = exec.Command("xdg-open", url)                                  // default
```

The token authorizes at `kernelauth.TierAdmin` (`kernel/webui/webui.go:1459`) — it is the
most powerful credential the daemon mints.

The inconsistency is instructive: the *less* privileged API tokens on the same screen are
handled correctly. `httpsurfaces.go:443` and `:525` write the OpenAI and REST tokens to
`0600` files and print **only a prefix** in the banner. The admin token gets neither
treatment.

Exposure channels, all concrete:

- **Process table.** `xdg-open`/`open`/`rundll32` receive the token as `argv[1]`, readable
  in `/proc/<pid>/cmdline` (or Process Explorer) by any local user while the process lives
  — and these helpers commonly `exec` further children, propagating it.
- **Log files.** Whenever stdout is redirected, the token is captured verbatim. The
  project's own CI proves this is recoverable: `scripts/webui-e2e.sh:50-51` greps the token
  straight out of `daemon.log` with
  `grep -oE "http://127\.0\.0\.1:$PORT_WEB/\?token=[a-f0-9]+"`.
- **Browser history**, shell scrollback, and terminal multiplexer buffers.

Mitigations that *are* present and worth preserving: `Referrer-Policy: no-referrer`
(`kernel/webui/webui.go:1319`) keeps the token out of `Referer` headers, and the CSP blocks
external subresource loads that might otherwise carry it.

### Concrete exploit scenario

An operator starts `agezt` under systemd or `nohup … > daemon.log`, which is the normal way
to run a daemon. The token lands in `daemon.log`. Because that file is created with the
shell's default `umask` (typically `0644`) — and, per EXPOSE-001, the surrounding
`~/.agezt` tree is `0755` — any local user reads it and gains **full admin access to the
console**: agent creation, tool execution, vault reads, workflow authoring. On a shared
host the same result is available without touching the filesystem at all, by polling
`/proc/*/cmdline` during the seconds the browser opener runs.

### Remediation

- **Do not put the token in the URL.** Mint a **single-use, short-TTL handoff code**, open
  `http://127.0.0.1:PORT/?c=<code>`, and have the SPA exchange it once at `/api/login` for
  a session cookie. A burned code is worthless in a log or a process table.
- Print only a token **prefix** in the banner, exactly as `httpsurfaces.go:443` and `:525`
  already do for the API tokens, and write the full value to a `0600` file.
- Seed the console token (and `AGEZT_WEB_PASSWORD`) into the redactor's literal set at
  `cmd/agezt/main.go:2631-2641` so that if it does reach an event payload it is scrubbed
  before the journal.

---

## EXPOSE-007 — `/metrics` is computed from the primary kernel but reachable with a per-tenant token

- **Severity:** Medium · **Confidence:** 85
- **CWE:** CWE-200, CWE-863 (Incorrect Authorization)
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\restapi\restapi.go:212-214` — route registration at `TierUser`
  - `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:508` — `rest.SetMetrics(func() []restapi.Metric { return restMetrics(k) })`
  - gauge list: `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:642-659`
  - tenant auth: `D:\Codebox\PROJECTS\AGEZT\kernel\httpserver\auth.go:70-78`, wired at `restapi.go:519`

### Description

First, the concern raised in scope is a **non-issue and should be recorded as such**:
`/metrics` has **no label cardinality at all**. `restapi.Metric` (`restapi.go:123-128`)
carries only `Name/Help/Type/Value`, and the exposition writer (`restapi.go:288-300`) emits
`agezt_<name> <value>` with no label set. There are no agent names, model names, provider
names, user identifiers, file paths or URLs in the output. It is also **authenticated**
(`TierUser`, 401 without a token — asserted by `kernel/restapi/metrics_test.go:25-31`), not
public.

The real issue is tenancy. `handleMetrics` never calls `s.bind(r)`, and the metrics closure
is bound at construction to the **primary** kernel. But `TierUser` is satisfiable by a
*per-tenant* token via `TenantAuthorize`. So a tenant-scoped credential reads
**daemon-global** state:

`agezt_spend_today_microcents`, `agezt_budget_ceiling_microcents`, `agezt_active_runs`,
`agezt_journal_head_seq`, `agezt_journal_bytes`, `agezt_memory_records`,
`agezt_world_entities`, `agezt_pending_approvals`, `agezt_disk_free_bytes`.

The six diagnostic log endpoints get this right — all are `TenantRouted: true`
(`kernel/controlplane/registry.go:61-75`). `/metrics` is the outlier.

### Concrete exploit scenario

A multi-tenant deployment issues tenant A a scoped token, expecting it to see only tenant
A's data. Tenant A polls `/metrics` every 15 seconds and derives the **whole daemon's**
financial and operational profile: total LLM spend to date, the configured budget ceiling,
the global in-flight run count, and the journal head sequence — from which the total event
rate across all tenants follows by differencing. `agezt_spend_today_microcents` is
commercially sensitive on its own; combined with `active_runs` it reveals competitors'
usage volume. `journal_head_seq` and `disk_free_bytes` additionally give an attacker a
precise oracle for timing a disk-exhaustion denial of service.

### Remediation

- Bind metrics to the request's tenant: call `s.bind(r)` in `handleMetrics` and have the
  metrics provider take the resolved kernel, matching how the log endpoints resolve theirs.
- If a global view is wanted for operators, expose it as a separate `TierAdmin` route
  (`/metrics/global`) rather than widening the tenant-facing one — the codebase already
  uses this pattern for mailbox and self-update (`restapi.go:226-239`).
- Consider raising the financial gauges to `TierAdmin` regardless.

---

## EXPOSE-008 — `/api/tool_log` serves raw tool-call arguments and outputs

- **Severity:** Medium · **Confidence:** 88
- **CWE:** CWE-200, CWE-532
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\tool_log.go:234` — `return p.CallID, previewString(string(p.Input))`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\tool_log.go:271` — output preview
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\tool_log.go:25` — `const toolOutputPreviewRunes = 100`
  - route: `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:314`

### Description

`/api/tool_log` returns, per row: `actor`, `correlation_id`, `tool`, `call_id`, **`input`**,
**`output`**, `error`, `duration_ms`, plus observation-trust metadata.

`input` is the raw `tool.invoked` payload — whitespace-collapsed and truncated to 100
runes, but otherwise **verbatim**. That means full shell command lines, `http` tool URLs
including query strings, and file paths. `output` is the first 100 runes of the tool
result: file contents, command stdout, HTTP response bodies.

The 100-rune cap is a *size* bound, not a security control. 100 characters comfortably
holds a bearer token, an API key, a `?token=`-bearing URL, a database password, or the
first lines of a private key. The only protection between these values and the API response
is ingest-time pattern redaction on the bus — which has the coverage gaps catalogued in
EXPOSE-009, and which is absent entirely for the shapes that matter most here
(`Authorization: Basic`, `X-Api-Key`, cookies).

The endpoint is correctly authenticated (`TierUser`) and tenant-routed. The finding is about
**tier and content**, not an open door: `/api/tool_log` carries materially more sensitive
data than `/api/status`, yet sits behind the identical single console credential — and
behind a **password-only session** when `AGEZT_WEB_PASSWORD` is set without strict mode
(`kernel/webui/webui.go:1443-1452`).

### Concrete exploit scenario

An operator sets a console password for convenience on a loopback bind (strict mode does
not auto-arm for loopback, `webui.go:139-163`). An attacker who guesses or phishes that
single password — no bearer token needed — opens `/api/tool_log?limit=500` and reads a
transcript of everything every agent has done: `shell` invocations with credentials passed
as flags, `http` calls to internal endpoints with tokens in query strings, and the leading
content of every file read. This is a complete reconnaissance package for the operator's
entire environment, obtained from one endpoint with one factor.

### Remediation

- Introduce a distinct authorization tier for the diagnostic log endpoints — `TierAdmin` at
  minimum — so viewing operational status does not imply reading agent tool transcripts.
- Redact at egress in addition to ingest (the `openaiapi.go:42-53` pattern), so that
  patterns added to the redactor later apply to **already-journaled** rows. Because the
  journal is immutable, egress redaction is the only way to improve coverage retroactively
  — this is the single highest-leverage fix in the section.
- Consider structurally masking known-sensitive argument keys (`headers`, `auth`, `token`,
  `password`) in `tool.invoked` payloads at the tool-dispatch boundary, before they are
  ever published.

---

## EXPOSE-009 — Redactor pattern gaps against shapes that demonstrably flow through this system

- **Severity:** Medium · **Confidence:** 92
- **CWE:** CWE-532, CWE-200
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\redact\redact.go:55-100` (`namedPatterns`), `:121-143` (`templatedPatterns`)

### Description

The pattern set is genuinely good — 15 named patterns covering OpenAI/Anthropic `sk-`,
`AKIA`, `gh[pousr]_`, `github_pat_`, `xox[baprs]-`, `xapp-`, Telegram bot tokens, `gsk_`,
`xai-`, `pplx-`, `fw_`, `AIza`, JWTs, `Bearer …`, and PEM private-key blocks, plus
context-preserving templates for `scheme://user:pass@host` and labeled AWS secret keys.
DeepSeek, OpenRouter and Cerebras are covered incidentally via the `sk-` substring rule.

The gaps below are ranked by whether the shape **actually traverses this codebase**, not by
general prevalence:

1. **Cookies and session IDs — zero coverage.** No `Set-Cookie`, `Cookie:`, `session=`,
   `sid=`, `JSESSIONID` or `connect.sid` pattern exists. Not hypothetical: the `http` tool
   writes full response headers into the tool result
   (`plugins/tools/http/http.go:254`), so `Set-Cookie` values are journaled.
2. **`Authorization: Basic <base64>` — not covered.** `redact.go:97` matches only
   `(?i)bearer\s+…`. The `http` tool journals request headers
   (`plugins/tools/http/http.go:160, 171`), so Basic credentials go in cleartext.
3. **`X-Api-Key:` / `api-key:` — not covered.** These are the **Azure OpenAI** and
   **Anthropic** authentication headers — first-class providers for this product.
4. **Stripe `sk_live_` / `rk_live_` — not covered.** The `sk-` rule requires a **hyphen**;
   Stripe uses an **underscore**. A live Stripe secret key passes through untouched. This
   is the most dangerous single miss by blast radius.
5. **Mistral keys — not covered.** Bare 32-character alphanumerics with no prefix; no
   pattern can match them, so only vault-literal seeding catches them — which is exactly
   what the three empty-literal redactors in EXPOSE-004 lack.
6. **Generic `key=value` assignments — no rule at all.** `password=`, `api_key=`,
   `apikey:`, `secret=`, `client_secret=`, `token=`, `PGPASSWORD=` all pass. There is no
   key-name list and no entropy heuristic anywhere in the package; the only label-keyed
   rule is the AWS one at `:140`.
7. **Non-URI connection strings — not covered.** ODBC/SQL Server `Server=…;Password=…;`,
   JDBC `?password=`, `.pgpass`. Only `scheme://user:pass@host` is handled.
8. **AWS partial coverage.** `ASIA…` temporary access key IDs are not matched (only
   `AKIA`), `aws_session_token` is not matched, and a bare secret access key is caught only
   when preceded by its label.
9. **Other provider/CI shapes:** GitLab `glpat-`, Hugging Face `hf_`, Replicate `r8_`,
   NVIDIA `nvapi-`, SendGrid `SG.`, npm `npm_`, PyPI `pypi-AgEI…`, Docker `dckr_pat_`,
   Azure SAS `sig=`, Google OAuth refresh tokens `1//…`.
10. **PEM edge cases:** `-----BEGIN PGP PRIVATE KEY BLOCK-----` does not match (the regex
    requires the delimiter to end in `PRIVATE KEY-----`); PuTTY `PuTTY-User-Key-File-3:` is
    uncovered. OpenSSH and encrypted PEM keys *do* match.
11. **JSON-escaping blind spot in literal matching.** Redaction runs over already-marshaled
    JSON (`bus.go:106-115`), but the literal check is a plain `strings.Contains`
    (`redact.go:225`). A literal secret containing `"` or `\` is JSON-escaped in the
    marshaled bytes and will **not** match. The team already solved the analogous problem
    for `&`/`<`/`>` via `SetEscapeHTML(false)` — this is the remaining half of that bug.

### Concrete exploit scenario

An agent uses the `http` tool to call an internal API authenticated with
`X-Api-Key: <32-char key>`, or an Azure OpenAI deployment. The header is a tool argument, so
it is journaled at `plugins/tools/http/http.go:160` inside `tool.invoked.input`. No pattern
matches `X-Api-Key`, and the key is not a vault literal because it belongs to a third-party
service the agent was told about at runtime. The key is written **permanently** into the
append-only hash chain, then served by `/api/tool_log` (EXPOSE-008) and readable on disk by
any local user (EXPOSE-001). Because the chain is immutable, adding the pattern later fixes
nothing retroactively.

### Remediation

Ordered by value:

1. Add a **key-name rule** — the single highest-coverage addition. Match
   `(?i)(pass(word)?|passwd|secret|api[-_]?key|apikey|token|client[-_]secret|auth)\s*[:=]\s*\S+`
   and redact the value while preserving the key, using the existing `templatedPatterns`
   mechanism at `redact.go:121-143`. One rule closes gaps 6, 7 and much of 9.
2. Add **header rules** for `(?i)authorization:\s*basic\s+\S+`, `(?i)x-api-key:\s*\S+`,
   `(?i)api-key:\s*\S+`, `(?i)set-cookie:\s*\S+`, `(?i)cookie:\s*\S+` — these cover the
   shapes this codebase provably journals.
3. Add `sk_live_`/`rk_live_` (Stripe), `glpat-`, `hf_`, `dckr_pat_`, `npm_`.
4. Fix the JSON-escaping blind spot: when seeding literals, also register the JSON-escaped
   form of each literal (`json.Marshal` the value and strip the quotes).
5. Add an **entropy fallback** for bare high-entropy tokens (Mistral, Together, Cohere): a
   Shannon-entropy threshold on 24+ character alphanumeric runs, applied only to values in
   known-risky positions (header values, `key=value` right-hand sides) to keep false
   positives low.

Pair every addition with a fixture in `kernel/redact/*_test.go` — that directory is already
allowlisted in `.gitleaks.toml`, so secret-shaped test data is expected there.

---

## EXPOSE-010 — Absolute host paths and internal state leak through `err.Error()` responses

- **Severity:** Medium · **Confidence:** 90
- **CWE:** CWE-209, CWE-200
- **Files:**
  - **root cause:** `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\respond.go:11-13` — `Response{Type: RespError, Error: err.Error()}`, relayed verbatim by the webui proxies at `kernel/webui/webui.go:1606, :1653, :1680, :1728`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\webui\files_route.go:159, 168, 177, 240, 252, 308, 313, 318, 345, 350, 354, 377, 389, 400, 405, 429` — `http.Error(w, err.Error(), …)`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\provider_keys.go:66, 128, 165, 198` — `"load vault: " + err.Error()`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\sandbox.go:136` — `"read: " + err.Error()`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\projections.go:78-81`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\channels.go:292, 351, 429`
  - `D:\Codebox\PROJECTS\AGEZT\kernel\controlplane\provider_keys.go:134, 170, 204` — `result["reload_error"]` on a **200 OK**

### Description

One structural chokepoint (`respond.go:11-13`) turns every control-plane error into a
client-visible string, fanning out across roughly 250 routes. The concrete leaks:

- **Absolute host paths.** `files_route.go` returns `*fs.PathError` from
  `os.Stat`/`os.ReadDir` verbatim — `C:\Users\<username>\agezt\workspace\… : Access is
  denied.` — disclosing the operator's OS username and real filesystem layout across 16
  sites.
- **Vault location and decrypt internals.** `provider_keys.go:66` surfaces the absolute
  path to `vault.json` plus decryption-failure detail.
- **Sandbox root disclosure.** `sandbox.go:136` leaks the resolved absolute project path
  *after* the confinement check passes — revealing the root the confinement is anchored to,
  which is precisely what an attacker probing for a traversal escape wants.
- **Journal segment paths.** `projections.go:78-81` sits inside the shared engine for all
  six log endpoints, so a corrupt segment returns its absolute path through `/api/tool_log`
  and friends.
- **Network topology.** `channels.go:429` (`/api/provider/probe`) wraps a `*url.Error`
  containing the full probed URL; `:292` and `:351` confirm internal/LAN hostnames, and
  loopback and private ranges are deliberately permitted (`channels.go:403`).
- **Errors on the success path.** `provider_keys.go:134, :170, :204` place `reload_error`
  inside a **200 OK** body — these evade any error-response scrubbing added later, because
  they are not error responses.

### Concrete exploit scenario

An attacker with a low-tier console credential systematically probes `files_route.go`
endpoints with paths designed to fail, harvesting `*fs.PathError` messages. Within a few
requests they know the operator's OS username, the absolute AGEZT home, the workspace root,
and (via `sandbox.go:136`) the sandbox anchor. They then use `/api/provider/probe` to map
internal hostnames and private-range services the daemon can reach. None of this requires
an exploit — it is the reconnaissance phase, and the error messages supply it turnkey.

### Remediation

- Fix at the chokepoint. In `respond.go:11-13`, map errors to a **stable client-facing
  code** plus a correlation ID, and log the full text server-side. That single change covers
  ~250 routes.
- Where a message must be returned, sanitize paths: strip the base directory prefix so
  `os.ReadDir` failures report `workspace/foo` rather than the absolute path. A small
  `sanitizePathErr(err, baseDir)` helper applied across `files_route.go` handles the 16
  sites uniformly.
- Move `reload_error` off the 200 path into a structured warning field that is explicitly
  sanitized.

---

## EXPOSE-011 — Event `Subject`, `Actor` and correlation IDs are never redacted

- **Severity:** Low · **Confidence:** 95
- **CWE:** CWE-532
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\bus\bus.go:92-120`

`redactSpecLocked` scrubs **only** `spec.Payload` (`bus.go:96-111`) and `spec.Tags`
(`bus.go:112-119`). The `event.Event` struct (`kernel/event/event.go:68-81`) also carries
`Subject`, `Actor`, `CorrelationID` and `CausationID`, and none of them pass through the
redactor.

These are normally structural identifiers, which is why the finding is Low. But `Subject` is
caller-supplied free text, and `Actor` is set from `e.TokenID` on the agent-gateway path
(`kernel/agentgw/audit.go`) — so a producer that puts a URL, an email address, or a
credential-bearing identifier into `Subject` writes it permanently into the hash chain with
no scrubbing.

**Remediation:** extend `redactSpecLocked` to cover `Subject` and `Actor`:
`spec.Subject = b.redactor.Redact(spec.Subject)`. Leave the correlation IDs alone if they
are guaranteed to be generated UUIDs, but assert that guarantee in a test.

---

## EXPOSE-012 — Memory store and config-center audit log are also world-readable

- **Severity:** Low · **Confidence:** 90
- **CWE:** CWE-732, CWE-532
- **Files:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\jsonstore\jsonstore.go:54, 73` — `0o755` dir / `0o644` file
  - consumer: `D:\Codebox\PROJECTS\AGEZT\kernel\memory\memory.go:222, 295` (`memory.json`)
  - `D:\Codebox\PROJECTS\AGEZT\kernel\configcenter\audit.go:104, 109` — `0o755` / `0o644`

The same permission gap as EXPOSE-001, on two smaller stores. `jsonstore` backs the **memory
store**, which holds distilled operator-profile facts and agent memories written raw —
`kernel/memory/manager.go:76` (`Remember`) does not pass through `kernel/redact`. The
config-center audit log writes value previews: the **full value** for public-rated keys
(`audit.go:81`) and the first 8 characters plus a hash for restricted ones (`audit.go:87`).

Neither is as severe as the journal — the content is narrower and the config-center
classifier already masks the sensitive tier — but both are readable by any local user for
the same reason, and both should follow the same 0700/0600 convention as the vault and
artifact stores.

**Remediation:** change `jsonstore.go:54, 73` and `configcenter/audit.go:104, 109` to
`0o700`/`0o600`, and include them in the one-time permission migration proposed in
EXPOSE-001. Consider routing `memory.Remember` through the redactor.

---

## EXPOSE-013 — `/oauth/callback` renders upstream error text to an unauthenticated browser

- **Severity:** Low · **Confidence:** 80
- **CWE:** CWE-209
- **Files:** `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:895` (`oauthResultPage(w, false, err.Error())`), registered public at `:864-867`; error origin `kernel/controlplane/channel_oauth.go:210`

`/oauth/callback` is one of only eight public, unauthenticated routes (`webui.go:775-790`,
`:857-867`), and on failure it renders `err.Error()` into the returned HTML page. The message
can carry the upstream OAuth provider's raw error body from `channel_oauth.go:210`, which for
token-exchange failures often includes the client ID, the redirect URI, and provider-side
diagnostic detail.

The output **is** HTML-escaped (`webui.go:908, 918-921`), so this is information disclosure
rather than XSS. It is the only error-text leak in scope reachable with no credential at all,
which is why it is listed despite the narrow content.

**Remediation:** render a generic failure page ("Authorization failed — check the daemon
log") plus a correlation ID, and log the upstream detail server-side.

---

# Appendix: What was verified clean

Recording these explicitly so a future pass does not re-investigate them.

- **No container or IaC assets exist** — no Dockerfile, compose file, Kubernetes manifest,
  Helm chart, Terraform, CloudFormation, Ansible or Pulumi config anywhere in the tree.
- **No GitHub Actions expression injection** — every `${{ … }}` in both workflows and the
  composite action was enumerated; no untrusted context reaches any shell.
- **No `pull_request_target`**; all 16 CI jobs carry the fork guard; all 17 checkouts set
  `persist-credentials: false`; all third-party actions are SHA-pinned.
- **`/metrics` carries no labels** — no agent, model, provider, user, path or URL
  identifiers in the exposition output, and it requires a token.
- **`netguard_log` is clean** on the URL/query-string concern — the emitter
  (`cmd/agezt/main.go:3715-3720`) publishes only `{ip, reason, tool}`.
- **`provider_log` carries no keys, headers, bodies, prompts or completions** — only
  provider/model names, chain topology and a failure `reason`.
- **`warden_log` records `argv0` only**, not full argv, so no secrets-in-arguments leak.
- **`policy_log` carries no arguments or payloads** — actor, tool, capability, allow/deny.
- **Inbound webhook bodies, headers and signatures are not logged** — `handleWorkflowHook`
  strips `secret` from the query at `kernel/webui/webui.go:1025-1029`.
- **All six `_log` endpoints are authenticated** (`TierUser`) and tenant-routed
  (`kernel/controlplane/registry.go:61-75`). None is on an unauthenticated path.
- **LLM prompt and completion text is *not* journaled on the `llm.request`/`llm.response`
  events** — only counts and metadata (`kernel/agent/agent.go:1144-1151`, `:1167-1174`).
- **Secret masking on config/key read paths is correct and complete.** No struct on any HTTP
  response path serializes an unmasked key or token: `/api/provider/keys` returns
  `{Label, Active, Last4}` (`kernel/creds/keyring.go:72`); `/api/config/values` returns
  presence-only for secret fields (`kernel/controlplane/settings.go:56-67`);
  `/api/configcenter/*` masks by rating (`configcenter_handler.go:404-419`,
  regression-tested); `/api/config` returns env-var names only (`config.go:500-505`). The
  pattern is exclusion-by-construction rather than `json:"-"`, which is the stronger choice.
- **API tokens are stored correctly** — `openai.token` and `rest.token` written `0600` with
  only a prefix shown in the banner (`cmd/agezt/httpsurfaces.go:409, 443, 469, 525`).
- **`.gitleaks.toml` allowlist is tight** — seven named test files, no regex or stopword
  escape hatches, all production code in scope.
- **npm install-script surface is nil** — only `fsevents` (macOS-only, optional).
- **No typosquatting, no dependency confusion, no `replace` directives, no license
  conflicts**; lock file coverage is complete.

---

# Recommended remediation order

1. **EXPOSE-001** — journal file permissions. Four constants plus a migration; closes the
   largest exposure in the report and is the precondition that makes EXPOSE-003 and
   EXPOSE-006 locally exploitable.
2. **CICD-001** — add `--ignore-scripts` to every CI `npm ci`. One flag, zero behaviour
   change (only `fsevents` has scripts), removes the unreviewed-code-execution path onto the
   persistent runner.
3. **CICD-003 / CICD-004** — stop auto-committing `dist` to `main`, and put `publish-sdks`
   behind a protected `environment`. Both are configuration changes that close direct
   supply-chain write paths.
4. **EXPOSE-002** — route the `agentgw` audit through the bus. Small, and every day it
   remains writes unredactable records.
5. **EXPOSE-003** — stop journaling full webhook sink URLs.
6. **CICD-002** — restore the fork-guard lint so the 16 correct guards stay correct.
7. **EXPOSE-008 / EXPOSE-009** — egress redaction plus the key-name and header patterns.
   Egress redaction is the only way to improve coverage over the immutable history.
8. **DEP-003 / DEP-001** — add npm vulnerability scanning; self-host Monaco.
9. Everything else, in severity order.

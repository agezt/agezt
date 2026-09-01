# AGEZT — Infrastructure / Supply-Chain Results (Phase 2, `sc-ci-cd`)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` · **Branch:** `main`
**Scope:** CI/CD workflows, the release/publish path, self-hosted runners, installers, the
self-update channel, and whether the documented CI gates are real controls.
**Method:** every workflow, service unit, composite action and installer read line by line, then a
refutation pass. Several CI gates were **executed locally** rather than read about. Read-only `gh`
API calls were used to establish what actually runs on GitHub. One recon claim is **refuted below**
with evidence rather than repeated.

---

## Docker / Kubernetes / Terraform — evidence of absence

Independently verified; recon's claim holds.

| Check | Command | Result |
|---|---|---|
| Dockerfile / compose | `git ls-files \| grep -Eic 'dockerfile\|docker-compose\|compose\.ya?ml'` | **0** |
| Terraform | `git ls-files \| grep -Eic '\.tf$\|\.tfvars$'` | **0** |
| k8s / Helm / charts / deploy | `git ls-files \| grep -Eic 'Chart\.yaml\|values\.yaml\|^k8s/\|^helm/\|^charts/\|^deploy/'` | **0** |
| Untracked too | `find . -not -in {node_modules,.git,.dev-home} -iname 'Dockerfile*' -o -iname '*.tf' -o -iname 'docker-compose*' -o -iname 'compose.y*ml' -o -iname 'Chart.yaml' -o -iname 'Jenkinsfile' -o -iname '.gitlab-ci.yml'` | **0 hits** |
| Every tracked `*.yml`/`*.yaml` in the repo | `git ls-files '*.yml' '*.yaml'` | exactly **4**: `.github/workflows/ci.yml`, `.github/workflows/publish-sdks.yml`, `.github/actions/setup-go-safe/action.yml`, `.github/dependabot.yml` |

`sc-docker` and `sc-iac` are correctly **not applicable**. `kernel/executionprofile/k8s.go` is a
*runtime execution driver* the agent can dispatch work to — not a deployment descriptor for this
repo, and it belongs to `sc-lang-go`, not here.

---

## Findings

### INFRA-001 — Every CI security gate is decorative: `main` is unprotected and no CI run has completed since 2026-08-07
- **Severity:** High · **Confidence:** 98 · **CWE-1269** (improper protection of alternate path) / **CWE-693** (protection mechanism failure)
- **Files:** `.github/workflows/ci.yml:21-23`, `.github/CODEOWNERS:1-3`, plus live GitHub API state

Three independent facts, each verified directly:

1. **No branch protection.** `gh api repos/agezt/agezt/branches/main/protection` →
   `{"message":"Branch not protected","status":"404"}`. `gh api repos/agezt/agezt/rulesets` → `[]`.
   There are therefore **no required status checks** — not one of the 16 CI jobs can block a merge
   or a push.
2. **CODEOWNERS is inert by its own admission.** `.github/CODEOWNERS:3`:
   > `# Enforced only when branch protection on 'main' has "Require review from Code Owners" enabled`

   That protection does not exist (fact 1). So the `/.github/` ownership rule written *specifically*
   to stop a silent weakening of the workflow guardrails enforces nothing.
3. **The gates do not even execute.** `gh run list -L 100` conclusions:
   `cancelled: 87`, `success: 11`, `failure: 1`, in-flight: 1. **All 23 most recent `push` and
   `pull_request` CI runs (2026-08-07 → 2026-08-13) concluded `cancelled`**, and the newest
   (`31671479194`, HEAD `e0041337`) has been `queued` for **3h38m**. Root cause is structural:
   ```yaml
   # ci.yml:21-23
   concurrency:
     group: ci-${{ github.workflow }}-${{ github.ref }}
     cancel-in-progress: true
   ```
   With the self-hosted runners unable to drain the queue, every run sits `queued` until the next
   push to the same ref cancels it. The gate never runs, and a permanently-queued check looks
   identical to "still running", never to "failed".

**Attack path:** the repo owner — or anything holding their credentials, which on this machine
includes an AGEZT daemon that ships `shell` at trust level L4 and can invoke `git` — pushes any
commit directly to `main`. `gitleaks`, `govulncheck`, `staticcheck`, `depscheck`, `deadcodecheck`,
the race detector, `e2e`, and `webui-e2e` all fail to observe it. The five Dependabot `pull_request`
runs on 2026-08-07 were likewise all `cancelled`, so dependency bumps carry zero gate coverage.

**Not a false positive:** this is not inferred from a report — it is the GitHub API's own answer for
protection, rulesets, and run conclusions. The one `failure` and eleven `success` entries in the last
100 are dominated by `dynamic`-event runs, not the `CI` workflow on a push.

**Remediation:** enable a ruleset on `main` requiring the CI workflow's jobs as status checks and
Code Owner review; restore runner capacity (or move the leaf jobs to hosted runners) so the queue
drains; consider `cancel-in-progress: false` for `push` on `main` so a superseding push cannot
silently void the previous commit's only verification.

---

### INFRA-002 — `frontend-dist-rebuild` hands `contents: write` to a job that first executes third-party npm/vite code, on a persistent runner
- **Severity:** High · **Confidence:** 85 · **CWE-269** (improper privilege management) / **CWE-829**
- **File:** `.github/workflows/ci.yml:241-289`

The job's step order is the problem:

| Line | Step |
|---|---|
| 249-252 | `permissions: contents: write` — the only job in the repo that raises the workflow-wide `contents: read` |
| 254-259 | `actions/checkout` (`persist-credentials: false`) |
| 261-265 | `actions/setup-node` |
| 266-271 | `npm ci --ignore-scripts` **then `npm run build`** |
| 272-288 | `env: GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` … `git push "https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git" HEAD:refs/heads/main` |

`--ignore-scripts` suppresses `preinstall`/`postinstall` lifecycle hooks. It does **not** stop
`npm run build` — which is `tsc --noEmit && vite build` (`frontend/package.json` scripts) — from
*importing and executing* every build-time dependency and vite plugin in `node_modules`, in the same
job, seven lines before a `contents: write` token enters the environment.

**Attack path:** a malicious or compromised transitive frontend dependency executes during
`vite build`. A GitHub Actions `run` step can append to `$GITHUB_PATH` and `$GITHUB_ENV`, both of
which take effect for *subsequent steps in the same job*. Prepending a directory containing an
attacker `git` binary makes the commit step at line 288 hand the push token — with write access to
an unprotected `main` (INFRA-001) — straight to attacker code. Alternatively the dependency simply
persists on the runner (INFRA-003, non-ephemeral) and reads the token from the job process later.

**Not a false positive:** the elevated permission (252), the token in the environment (275) and in a
URL (288), and the npm build in the same job (266-271) are all literally present. The
`persist-credentials: false` note at :256-259 protects `.git/config` but is irrelevant to the
in-process environment. The `[skip ci]` footer and the GITHUB_TOKEN no-retrigger behaviour address
*loops*, not privilege.

**Remediation:** split into two jobs — an unprivileged `build` job that uploads `dist/` as an
artifact, and a minimal `commit` job (`needs: build`) that downloads the artifact and pushes,
executing no third-party code. Or move the rebuild to a hosted ephemeral runner.

---

### INFRA-003 — Self-hosted runners are persistent, run as the owner's own user, and keep their registration credentials inside the job's working tree
- **Severity:** High · **Confidence:** 90 · **CWE-250** (execution with unnecessary privileges) / **CWE-427**
- **Files:** `ops/wsl-runners/README.md:10-15`, `:170-173`, `.github/actions/setup-go-safe/action.yml:20-25`, `:67`, `:186-187`, `scripts/ci-go-retry.sh:35`

- Runners live at **`/home/ersinkoc/actions-runner-{1,2,3}`** (`README.md:10`) — the *owner's* user
  account, not a dedicated service account.
- Registration is **non-ephemeral**: `./config.sh --url … --name wsl-runner-N-new --labels
  wsl-runner --unattended --replace` (`README.md:172-173`) — no `--ephemeral` — with
  `Restart=always` (`README.md:11`). State written by one job survives into the next.
- Jobs execute under `<runner-dir>/_work/…`, so `.runner`, `.credentials`,
  `.credentials_rsaparams` (enumerated at `README.md:171`) sit **one directory above the workspace**
  and are readable by any job step. Exfiltrating `.credentials_rsaparams` yields a permanent runner
  identity that can claim future jobs for this repo.
- All three runners share **one WSL VM and one `/dev/shm`**, acknowledged in
  `action.yml:20-25`. GOROOT, GOCACHE and GOTMPDIR are staged there at fully predictable paths —
  `/dev/shm/goroot-${RUNNER_NAME}` (`action.yml:67`), `/dev/shm/gocache-${RUNNER_NAME}` and
  `/dev/shm/gotmp-${RUNNER_NAME}` (`action.yml:186-187`). Same-uid processes ⇒ any job can write
  a sibling concurrent job's `pkg/tool/linux_amd64/compile`, i.e. substitute the Go compiler used to
  build another job's binaries. `scripts/ci-go-retry.sh:35` confirms cross-runner reach by design:
  `rm -rf /dev/shm/gocache-* /dev/shm/gotmp-*` — a glob that deletes **every** runner's cache, not
  just this one's.
- The VM runs on the owner's daily-driver Windows host (`README.md:12` references
  `C:\Users\ersin\.wslconfig`), and WSL2 mounts the host filesystem at `/mnt/c` by default.

**Attack path:** any code that runs in a job (INFRA-004's dependency path, or a merged PR) reads the
runner credentials from `../.credentials*`, plants a persistent implant that survives to the next job
including the privileged `frontend-dist-rebuild` (INFRA-002), poisons a concurrent sibling job's
tmpfs GOROOT, and reaches the owner's Windows filesystem through `/mnt/c`.

**Not a false positive:** the paths, the missing `--ephemeral`, and the shared `/dev/shm` are stated
by the repo's own operations doc and its own composite action. The per-runner path naming prevents
*accidental* collision; it provides no security boundary between same-uid processes.

**Remediation:** run the runners as a dedicated unprivileged user with the workspace outside the
runner's own directory; move to `--ephemeral` runners (ideally one VM per runner); or restrict
self-hosted jobs to `push` on `main` only.

---

### INFRA-004 — Dependabot PRs execute freshly-bumped third-party code on the persistent runners; `--ignore-scripts` closes only half the gap
- **Severity:** Medium · **Confidence:** 85 · **CWE-829**
- **Files:** `.github/workflows/ci.yml:202-218`, `:304-310`, `:334-338`; `.github/dependabot.yml:9-19`

The fork guard `if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name ==
github.repository` is present on **all 16 jobs** and is correct — fork PRs never touch the
self-hosted runners. But Dependabot branches live *in this repo*, so they pass it. This is not
theoretical: `gh run list` shows five `pull_request` CI runs on
`dependabot/npm_and_yarn/frontend/{lucide-react,jsdom,radix-ui,fontsource,multi-…}` branches
(2026-08-07).

`ci.yml:202-213` states the rationale for `--ignore-scripts` and claims it closes this. It does not,
fully:

- `ci.yml:304-310` — `npm ci --ignore-scripts` then `npm run deadcode; npm test;
  npm run test:coverage:voice`. `knip` and `vitest` **import** the bumped packages; module top-level
  code runs. `--ignore-scripts` only blocks install lifecycle hooks.
- `ci.yml:214-218` and `:266-271` — `npm run build` = `tsc --noEmit && vite build`; vite loads and
  executes plugin code from `node_modules`.
- `ci.yml:338` — `npx playwright install --with-deps chromium`. `--with-deps` shells out to
  `sudo apt-get install`; for this step to succeed the runner user must have **passwordless sudo**,
  which makes any job root-capable on the VM. The same step downloads and executes browser binaries
  from a CDN on every run.

**Remediation:** treat the residual as the real boundary — an ephemeral runner (INFRA-003) makes this
survivable. Pre-install Playwright system deps once, out of band, and drop `--with-deps`. Consider
excluding Dependabot PRs from the self-hosted label.

---

### INFRA-005 — `install.sh` root-downloads and extracts the Go toolchain with no checksum or signature, via a predictable `/tmp` path
- **Severity:** Medium · **Confidence:** 92 · **CWE-494** (download of code without integrity check) / **CWE-59** (link following)
- **File:** `install.sh:117-124` (reached as root: `install_all:273 need_root` → `:274 ensure_prereqs` → `:146 ensure_go` → `:109`)

```sh
tarball="go${GO_VERSION}.linux-${arch}.tar.gz"
url="https://go.dev/dl/${tarball}"
tmp="/tmp/${tarball}"
curl -fsSL "$url" -o "$tmp"
rm -rf /usr/local/go
tar -C /usr/local -xzf "$tmp"
```

No SHA-256 comparison exists anywhere in this function, and the toolchain it installs is then used to
build the daemon binaries that get installed to `/usr/local/bin`.

Two paths:
- **(a) Integrity.** Any successful interception of `go.dev` (compromised CDN, a mis-issued cert, a
  corporate TLS-terminating proxy, DNS hijack) yields root code execution on the build host at
  `tar -C /usr/local` time, and thereafter a backdoored compiler builds AGEZT.
- **(b) Local privilege escalation via `/tmp`.** `/tmp/go1.26.4.linux-amd64.tar.gz` is fully
  predictable. `/tmp`'s sticky bit stops an unprivileged user deleting *others'* files but not
  creating their own, so they can pre-create that path as a symlink before the operator runs the
  installer. `curl -o` opens with `O_WRONLY|O_CREAT|O_TRUNC` and **follows symlinks**, so a
  root-privileged truncate/overwrite lands on the attacker's chosen target.

**Not a false positive:** the contrast inside this same repo proves the omission is not policy —
`ci.yml:499-508` downloads staticcheck *and its `.sha256` sidecar* and hard-fails on mismatch, and
`install.ps1` routes every dependency through winget/choco (which verify). Only `install.sh`'s Go
download is unverified.

**Remediation:** pin and verify the tarball SHA-256 (published at `go.dev/dl/?mode=json`); use
`mktemp` for the staging path, or `curl -o "$tmp" --no-clobber` into a root-owned 0700 directory.

---

### INFRA-006 — `install.sh expose ngrok` installs a globally-trusted apt key and an unsigned-by repository
- **Severity:** Medium · **Confidence:** 90 · **CWE-345** (insufficient verification of data authenticity)
- **File:** `install.sh:403-404`

```sh
curl -fsSL https://ngrok-agent.s3.amazonaws.com/ngrok.asc | tee /etc/apt/trusted.gpg.d/ngrok.asc >/dev/null
echo 'deb https://ngrok-agent.s3.amazonaws.com bookworm main' > /etc/apt/sources.list.d/ngrok.list
```

A key dropped in `/etc/apt/trusted.gpg.d/` is trusted for **every** apt repository on the host, and
the `deb` line carries no `[signed-by=…]` restriction. ngrok's signing key — or anyone who obtains
it — can therefore sign a replacement for *any* Ubuntu package the machine installs or upgrades,
including the base system.

**Not a false positive:** the correct pattern is used twice elsewhere in the same file —
cloudflared at `:331-333` (`signed-by=/usr/share/keyrings/cloudflare-main.gpg`) and NodeSource at
`:135-137` (`signed-by=/etc/apt/keyrings/nodesource.gpg`). ngrok is the only outlier.

**Remediation:** `gpg --dearmor -o /etc/apt/keyrings/ngrok.gpg` and add
`[signed-by=/etc/apt/keyrings/ngrok.gpg]` to the `deb` line, matching the cloudflared block.

---

### INFRA-007 — The installers run `npm ci` **without** `--ignore-scripts`, as root / Administrator
- **Severity:** Medium · **Confidence:** 90 · **CWE-829**
- **Files:** `install.sh:176-182`, `install.ps1:182-188`

```sh
# install.sh:176-182 — reached only after need_root (install_all:273)
cd "$AGEZT_SRC/frontend"
if [ -f package-lock.json ]; then npm ci; else npm install; fi
npm run build
```
`install.ps1:184` is the same (`if (Test-Path 'package-lock.json') { npm ci } else { npm install }`)
under `Require-Admin` (`install.ps1:317`).

Commit `3987bf7c fix(ci): npm install scripts ran unreviewed on the self-hosted runners` added
`--ignore-scripts` to **all five** `npm ci` invocations under `.github/` — verified — but not to the
installer, which is the strictly higher-privilege path: root/Administrator, on the operator's
production host, at the moment the daemon binary is being built. The `npm install` fallback branch is
worse still: it can resolve dependencies outside the lockfile.

**Remediation:** add `--ignore-scripts` to both installers; drop the `npm install` fallback and fail
if the lockfile is missing.

---

### INFRA-008 — `install.sh expose` publishes the REST API while the docs say "Web", and the console is left on its built-in default password
- **Severity:** Medium · **Confidence:** 88 · **CWE-1188** (insecure default) / **CWE-1021**
- **Files:** `install.sh:40`, `:285`, `:349`, `:391`, `:412`; `cmd/agezt/httpsurfaces.go:82`, `:101-108`; `kernel/restapi/restapi.go:207-208`

Verified port collision:
- `install.sh:40` — `AGEZT_REST_ADDR="${AGEZT_REST_ADDR:-127.0.0.1:8787}"`.
- `cmd/agezt/httpsurfaces.go:81-83` — the **web console's own default** is `addr = "127.0.0.1:8787"`.
- `httpsurfaces.go:101-108` — when that bind fails, the console silently falls back to
  `net.Listen("tcp", "127.0.0.1:0")` (an OS-assigned random port).

So on a systemd install the REST API wins `:8787` and the console moves. Yet `install.sh:285` prints
`REST/Web binding: http://$AGEZT_REST_ADDR` — conflating the two surfaces — and every `expose`
recipe tunnels *that* address: `:349` installs a permanent `Restart=always` cloudflared unit
(`--url http://$AGEZT_REST_ADDR`) publishing it to `*.trycloudflare.com`; `:391` (`tailscale serve
--http=8787`) and `:412` (`ngrok http`) do the same.

The operator believes they exposed the console. They exposed the REST API — including its two
unauthenticated routes, `kernel/restapi/restapi.go:207-208`:
```go
router.Handle("/healthz", publicRoute, s.handleLive)
router.Handle("/readyz",  publicRoute, s.handleReady)
```
Meanwhile `install.sh` never sets `AGEZT_WEB_ADDR` or `AGEZT_WEB_PASSWORD`, so the console keeps
running on a random loopback port under the daemon's built-in default password
(`httpsurfaces.go:119-121` documents that default explicitly).

**Remediation:** give the console its own port in the generated env file (`AGEZT_WEB_ADDR`), make the
`expose` recipes name which surface they publish, force `AGEZT_WEB_PASSWORD` at install time, and fix
the `:285` banner to print both bindings separately.

---

### INFRA-009 — REFUTED as reported, restated correctly: update signature verification is **not** inert — every `Apply` path fails closed. The real defect is that the trust anchor cannot be set in a production build at all
- **Severity:** Low today, latent Critical · **Confidence:** 92 · **CWE-1188** / **CWE-1059**
- **Files:** `kernel/update/update.go:376`, `:380`, `:390-403`, `:426-456`, `:524-532`, `:275`, `:677-679`; `kernel/update/signature_test.go:22`; `cmd/agezt/boot_ops.go:47`, `:90`, `:102`; `kernel/controlplane/update_control.go:145-150`; `kernel/restapi/update_handlers.go:105-110`

The recon note ("signature verify shipped but the default public key may be unset, making
verification inert — an inert signature check on an auto-update path is a top-severity finding") is
**incorrect**, and I am recording the refutation rather than inheriting it.

What is true: `update.go:380` — `var DefaultPublicKeyHex = ""` — confirmed empty, so
`resolvePublicKey()` (`:390-403`) returns nil in every shipped binary. What follows is *not* an
accepted update:

1. `verifySignature` with `pub == nil` (`:439-441`) refuses unless
   `info.Provenance == ProvenanceGitHubRelease`.
2. **Every operator-reachable caller builds `UpdateInfo` by hand**, leaving `Provenance` at its zero
   value (`ProvenanceUnverified`): `kernel/controlplane/update_control.go:145-150`,
   `kernel/restapi/update_handlers.go:105-110`, and the CLI `agezt update --apply`
   (`cmd/agezt/boot_ops.go:47`, which round-trips version/sha256/url through the control plane).
   All three are refused with `ErrSignatureKeyNotConfigured`. **This is the UPD-001 fix working.**
3. The only caller that preserves `Provenance` is the in-process background checker
   (`boot_ops.go:90` → `:102`, passing `result.Update` directly). But `checkGitHub` (`:524-532`)
   constructs `UpdateInfo{Version, URL, Notes, Provenance}` and **never sets `SHA256`** — and `Apply`
   validates the checksum at `:275` *before* the signature, where `validateSHA256` (`:677-679`)
   returns `errors.New("update: empty SHA256 in manifest")`.

**Net: both branches abort. The self-update apply path is entirely non-functional in shipped builds,
and it fails closed.** No exploitable finding.

Two real residuals worth recording:

- **The trust anchor is unsettable in production.** `update.go:376` and the operator-facing error
  string at `:357` both instruct: *"set … at runtime via SetPublicKey"*. **`SetPublicKey` is defined
  only in `kernel/update/signature_test.go:22`** — there is no production API, and the
  `updatePubKey` var (`:385`) is never written outside tests. An operator who *wants* signed updates
  must produce a custom `-ldflags` build; the documented runtime path does not exist.
- **Latent Critical.** The `ProvenanceGitHubRelease` exemption at `:439-442` — accept an unsigned
  manifest because "GitHub Releases' TLS is the anchor" — is currently unreachable only because of
  the missing-SHA256 bug. The moment someone fixes `checkGitHub` to populate `SHA256` without first
  embedding `DefaultPublicKeyHex`, the background auto-updater will apply any GitHub release asset
  with **no signature check at all**, then `os.Exit(0)` for the watchdog to run it
  (`boot_ops.go:118`).

**Remediation:** delete the `ProvenanceGitHubRelease` exemption (make the signature mandatory for all
provenances), ship a real `SetPublicKey` or remove the claim from `:376` and `:357`, and fix
`checkGitHub` to publish and consume a signed checksum — in that order.

---

### INFRA-010 — Update payload is written to disk before any verification, with no size bound
- **Severity:** Low · **Confidence:** 90 · **CWE-400** (uncontrolled resource consumption)
- **File:** `kernel/update/update.go:270`, `:275`, `:287`, `:654`

`Apply` calls `downloadBinary` (`:270`) — which writes to `<baseDir>/bin/<binary>.new` — *before*
`validateSHA256` (`:275`) and *before* `verifySignature` (`:287`). The write itself is
`io.Copy(f, resp.Body)` (`:654`) with no `io.LimitReader` and no `MaxBytes` cap.

An admin-token holder posting a URL to `/api/v1/update/apply`, or a compromised
`AGEZT_UPDATE_ENDPOINT`, can therefore fill the daemon's base directory even though the update is
subsequently refused. The file is never renamed to the live path and never `chmod +x`'d
(`:305`, `:310-316` are unreachable on the failure path), so this is denial of service, not code
execution.

**Remediation:** wrap the body in `io.LimitReader` with a sane ceiling, and move `verifySignature`
ahead of the download (the signature covers `version||sha256`, both known pre-download).

---

### INFRA-011 — Gate-rot markers: `ci.yml` cites a lint that was deleted, and `.gitleaks.toml` allowlists a file that no longer exists
- **Severity:** Low (informational) · **Confidence:** 97 · **CWE-1110**
- **Files:** `.github/workflows/ci.yml:245`, `:256`; `.gitleaks.toml:42`; `docs/DEAD-CODE-AUDIT.md:10`

`ci.yml` justifies two of its guardrails by pointing at an enforcement mechanism that does not exist:
- `:245` — *"it exists here only so the ciguard fork-guard lint passes for this push-only-main job"*
- `:256` — *"`persist-credentials: false` is required by the ciguard lint"*

`internal/ciguard/ciguard.go` was **deleted on 2026-07-08** (`docs/DEAD-CODE-AUDIT.md:10`).
`git grep -rn ciguard` over the tree returns only those two comments and that doc line — no Go code.
I searched independently for any replacement (`git grep -l '\.github/workflows' -- '*.go'` → no
matches; no `*_test.go` references `persist-credentials`, `pull_request.head.repo`, or a fork guard).
**Nothing in this repo verifies that the fork guard, `persist-credentials: false`, or SHA pinning
survive a future edit** — which is precisely what `.github/CODEOWNERS:10-13` was written to backstop,
and CODEOWNERS is itself unenforced (INFRA-001).

Separately, `.gitleaks.toml:42` allowlists `cmd/agezt/plugin_log_test.go`; that file does not exist
(the other six allowlist paths all resolve). Harmless in effect — a dead allowlist entry only
widens nothing — but it is the same drift signature.

**Remediation:** reinstate a workflow lint as a `_test.go` in a package the build actually compiles
(the "test-only ≠ dead code" pattern), asserting: every job carries the fork guard, every `uses:`
is SHA-pinned, every `checkout` sets `persist-credentials: false`, and no `run:` block interpolates
`github.event.*`. Prune the stale gitleaks path.

---

### INFRA-012 — The publish path has no environment gate, no review requirement, and emits no provenance
- **Severity:** Medium · **Confidence:** 88 · **CWE-353** (missing integrity check) / **CWE-284**
- **File:** `.github/workflows/publish-sdks.yml:18-24`, `:53`, `:83`, `:110`, `:90`

```yaml
on:
  release:
    types: [published]
  workflow_dispatch:

permissions:
  contents: read
```

- **No `environment:` on any of the three publishing jobs**, so `PYPI_API_TOKEN`, `NPM_TOKEN` and
  `CARGO_REGISTRY_TOKEN` are available with no required reviewer and no deployment-branch
  restriction.
- `workflow_dispatch` lets anyone with write access publish **from any ref they select**, including
  an unreviewed branch. Combined with INFRA-001 (no branch protection, no required checks), a single
  compromised write-access credential publishes a malicious SDK to npm, PyPI and crates.io with no
  human in the loop and no CI gate having run.
- **No provenance or attestation:** no `id-token: write`, no `npm publish --provenance`
  (line 90 is bare `npm publish --access public`), no `actions/attest-build-provenance`, no sigstore
  for the Python or Rust artifacts. A consumer of `@agezt/*` cannot verify the tarball came from
  this repository at this commit.
- Minor: `npm publish` (:90) follows `npm run build` (:79) in the same job — the INFRA-002 pattern —
  though far weaker here, since these jobs run on ephemeral GitHub-hosted `ubuntu-latest` (:29,
  :64, :95), not on the self-hosted runners. That choice is correct and is recorded as safe below.

**Remediation:** put each publish job behind a protected `environment:` with a required reviewer;
restrict the workflow to `release: published` (drop `workflow_dispatch`, or gate it on `github.ref`
being a tag); add `permissions: id-token: write` + `npm publish --provenance`, and attest the PyPI
and crates artifacts.

---

## CI gate inventory

Every gate in `ci.yml`. "Runs?" and "Blocks?" reflect **live GitHub state**, not the YAML's intent.

| # | Gate | Line | Does it run? | Does it block? | References live code? | Verified locally |
|---|---|---|---|---|---|---|
| 1 | `go vet ./...` | 41 | **No** — queued/cancelled since 2026-08-07 | **No** — main unprotected, no required checks | Yes | — |
| 2 | `go test ./...` | 42 | No | No | Yes | — |
| 3 | 100 % coverage ratchet (10 pkgs) | 43-65 | No | No | **Yes — all 10 package dirs exist** | dirs verified |
| 4 | `go build ./...` | 66 | No | No | Yes | — |
| 5 | race-breadth (`-race ./...`) | 87-88 | No | No | Yes | — |
| 6 | race-depth (`-race -count=20`, 8 pkgs) | 124-138 | No (hosted runner, but same queue/cancel) | No | Yes — all 8 pkgs exist | — |
| 7 | e2e smoke (`scripts/e2e-smoke.sh`) | 161-162 | No | No | Yes — script present, `set -e` semantics intact | — |
| 8 | codegen-in-sync (`git diff --exit-code contract/gen/`) | 175-187 | No | No | Yes — `.project/agezt-contract.jsonc`, `contract/gen/types.gen.go` both exist | **RAN — clean, no drift** |
| 9 | frontend-dist-in-sync | 214-225 | No | No | Yes | — |
| 10 | frontend-dist-rebuild (auto-commit) | 266-289 | No | n/a (it *writes*, not gates) | Yes | see INFRA-002 |
| 11 | frontend `knip` deadcode + vitest + voice coverage | 304-310 | No | No | Yes — all three npm scripts exist | — |
| 12 | webui-e2e (real daemon + Playwright) | 334-340 | No | No | Yes — `scripts/webui-e2e.sh` present | — |
| 13 | python-sdk unittest | 354-357 | No | No | Yes | — |
| 14 | typescript-sdk `npm test` | 375-378 | No | No | Yes | — |
| 15 | rust-sdk `cargo fmt --check` + `cargo test` | 400-404 | No | No | Yes | — |
| 16 | cross-build 6 GOOS/GOARCH | 433-437 | No | No | Yes | — |
| 17 | **depscheck** (dependency allowlist) | 448-449 | No | No | Yes | **RAN — rc=0, "24 core dependencies, all justified"** |
| 18 | **sdkparity** | 450-451 | No | No | Yes — `docs/SDK-PARITY.md` exists | **RAN — rc=0** |
| 19 | **deadcodecheck** | 452-453 | No | No | Yes | **RAN — rc=0, "no unexpected dead code"** |
| 20 | repo hygiene (`git ls-files -ci`) | 468-476 | No | No | Yes | **RAN — empty (clean)** |
| 21 | gofmt | 478-485 | No | No | Yes | **RAN — clean; see false-positive note below** |
| 22 | staticcheck 2026.1 (checksum-verified, 5 retries) | 493-520 | No | No | Yes | not run (needs Linux binary) |
| 23 | govulncheck v1.4.0 (5 retries) | 526-540 | No | No | Yes | not run |
| 24 | **gitleaks v8.30.1** (full history, `fetch-depth: 0`) | 561-566 | No | No | Yes — but `.gitleaks.toml:42` allowlists a **deleted** file (INFRA-011) | not run |
| 25 | *"ciguard fork-guard lint"* (cited at :245, :256) | — | **Does not exist** | No | **No — `internal/ciguard` deleted 2026-07-08** | INFRA-011 |

**Verdict:** the workflow file describes a genuinely strong gate set — 24 real, well-constructed
checks, all `|| true`-free and none marked `continue-on-error` (the single `continue-on-error: true`
at `setup-go-safe/action.yml:78` is a toolchain-integrity probe with a non-tolerant re-verify
fallback at `:135-152`, not a gate bypass). **Every one of them is currently unenforced.** `main`
takes no required checks, and no CI run has reached a conclusion other than `cancelled` since
2026-08-07. The five gates I could execute locally are all green — the code is fine; the *control*
is not.

**One false positive I killed:** `gofmt -l` over the working tree lists ~500 files here. That is the
documented Windows artifact — `git config core.autocrlf` returns `true`, so the working tree is CRLF
while the index (and therefore the CI checkout) is LF. Not a CI red; not reported.

---

## Workflow trigger / permission matrix

| Workflow | Triggers | Untrusted-input trigger? | Workflow `permissions` | Job overrides | Runner | Secrets reachable |
|---|---|---|---|---|---|---|
| `ci.yml` | `push: [main]`, `pull_request` | **None.** No `pull_request_target`, no `workflow_run`, no `issue_comment`, no `schedule` | `contents: read` (`:26-27`) | **one**: `frontend-dist-rebuild` → `contents: write` (`:249-252`) | 15 jobs `[self-hosted, Linux, X64]`; `race-depth` `ubuntu-latest` (`:116`) | `GITHUB_TOKEN` only, and only in `frontend-dist-rebuild` (`:275`) |
| `publish-sdks.yml` | `release: [published]`, `workflow_dispatch` | None (both require write access) | `contents: read` (`:23-24`) | none | all three `ubuntu-latest` (`:29`, `:64`, `:95`) | `PYPI_API_TOKEN` (:53), `NPM_TOKEN` (:83), `CARGO_REGISTRY_TOKEN` (:110) — **no `environment:` gate** |
| `.github/actions/setup-go-safe` | composite, called by 8 jobs | n/a | inherits | n/a | inherits | none |

Fork guard `if:` present on **16/16** `ci.yml` jobs; `frontend-dist-rebuild` uses the stricter
`(push && ref == refs/heads/main) || same-repo PR` variant (`:247`).

---

## Verified safe

Checks that came back clean, with the evidence:

1. **No expression injection anywhere.** The complete inventory of `${{ }}` occurrences across
   `.github/` is **12**: `matrix.os` (:14, a comment), `github.workflow`+`github.ref` in
   `concurrency.group` (:22), `github.ref` in `checkout.with.ref` (:260), `secrets.GITHUB_TOKEN` in
   an `env:` block (:275), `matrix.goos`/`matrix.goarch` in a job `name:` (:407) and an `env:` block
   (:435-436), and the three registry secrets in `env:` blocks of `publish-sdks.yml`. **Not one
   `github.event.*` value reaches a `run:` block.** Line 288 correctly uses shell variables
   (`${GITHUB_TOKEN}`, `${GITHUB_REPOSITORY}`) rather than expressions — the recommended pattern.
2. **28/28 external action uses are full-SHA-pinned**, each with a version comment:
   `actions/checkout@3d3c42e5…` ×17, `actions/setup-node@820762786…` ×5,
   `actions/setup-python@5fda3b95…` ×2, `actions/setup-go@40f1582b…` ×2,
   `dtolnay/rust-toolchain@29eef336…` ×2 (the only third-party publisher, pinned with an explicit
   `toolchain: stable` because SHA-pinning defeats ref inference — `ci.yml:388-393`). Plus 8 uses of
   the local composite `./.github/actions/setup-go-safe`. No floating tags anywhere.
3. **`persist-credentials: false` on all 17 `actions/checkout` steps** — including the one job that
   needs to push, which authenticates with a one-shot token URL instead (`:256-259`, `:288`).
4. **No `pull_request_target`, `workflow_run`, `issue_comment`, or `schedule` trigger** in either
   workflow — the entire class of "untrusted input with secrets" triggers is absent.
5. **Fork PRs cannot reach the self-hosted runners.** The guard expression is on all 16 jobs and is
   the correct `github.event.pull_request.head.repo.full_name == github.repository` form.
6. **All third-party binaries fetched during CI are pinned and (where possible) verified.**
   staticcheck 2026.1 is downloaded *with its publisher `.sha256` sidecar* and compared, hard-failing
   on mismatch (`ci.yml:499-508`); `govulncheck@v1.4.0` (:533) and `gitleaks@v8.30.1` (:565) go
   through `go install` at a fixed version, which verifies against the Go checksum database. No
   `@latest` anywhere.
7. **`--ignore-scripts` on all 5 `npm ci` invocations under `.github/`** (`ci.yml:216`, `:269`,
   `:307`, `:336`, `:377`, plus `publish-sdks.yml:78`) — CICD-001 discipline is complete *within CI*.
   (The installers are the gap — INFRA-007.)
8. **No cache-poisoning surface.** `actions/cache` is not used directly anywhere; caching is only
   `setup-node`'s built-in, keyed on `cache-dependency-path` lockfile hashes. No cache key derives
   from attacker-influencable input.
9. **The publish jobs run on ephemeral GitHub-hosted runners** (`ubuntu-latest`), not the persistent
   self-hosted ones — the registry tokens never touch the shared WSL VM. This is the right call and
   deserves recording.
10. **Both workflows declare `permissions: contents: read` at the top**, with exactly one narrowly
    scoped job-level escalation.
11. **Systemd hardening in `install.sh:245-263`:** dedicated `agezt` system user with
    `/usr/sbin/nologin` (`:98`), `NoNewPrivileges=true`, `PrivateTmp=true`, `ProtectSystem=full`,
    `ProtectHome=true`, `ReadWritePaths` scoped to `$AGEZT_HOME`, `LockPersonality=true`. Plus
    `ensure_service_home_path` (`:84-90`) which **refuses** an `AGEZT_HOME` under `/home` or `/root`
    because it would be unreachable behind `ProtectHome=true` — a thoughtful guard.
12. **`install.sh` env file permissions:** created `0640` (`:228`) and `chown root:agezt` (`:233`),
    so the daemon user can read provider keys but other local users cannot.
13. **`install.ps1:106` strips secrets before writing service config:** `Get-EnvFilePairs` filters
    any name matching `(?i)(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL)` out of the values passed to
    `nssm set … AppEnvironmentExtra` (`:304-307`), keeping credentials out of the NSSM registry key.
14. **Both installers require a pinned release ref by default** and force an explicit opt-in for
    branch installs (`install.sh:66-78`, `install.ps1:81-90`) and for any third-party remote
    installer (`install.sh:80-82`, `install.ps1:92-96`). `AGEZT_REF` defaults to `v1.0.0`.
15. **`kernel/update` transport is sound:** the download client dials through a netguard-screened
    dialer (`update.go:182-196`), `requireHTTPS` is enforced on the initial URL *and* on every
    redirect hop via a `CheckRedirect` hook (`:192-194`, `:586-600`, `:632-634`) — closing the
    HTTPS→HTTP downgrade the comment at `:185-191` describes; the binary swap is
    write-temp→rename→rename (`:649-670`, `:305`); and the concurrency lock uses
    `O_CREATE|O_EXCL` (`:701`).
16. **UPD-001 is genuinely fixed.** A caller-assembled `UpdateInfo` (REST body, control-plane arg,
    CLI) inherits `ProvenanceUnverified` as the zero value and is refused — verified by reading all
    three construction sites (`update_control.go:145-150`, `update_handlers.go:105-110`,
    `boot_ops.go:47`). The comment at `update.go:59-75` accurately describes the code.
17. **No `continue-on-error`, `|| true`, or warn-only step masks any gate.** Every `exit 0` in the
    workflows is inside a legitimate conditional (skip-on-hosted-runner, skip-publish-when-token-
    absent, retry-loop success). Verified by grepping all of `.github/`.
18. **Docker / k8s / Terraform / Jenkins / GitLab CI: absent** — see the evidence table at the top.

---

## Notes for the orchestrator

- INFRA-001 is the finding everything else hangs off. Fixing any individual gate is wasted effort
  while nothing is required and nothing completes.
- INFRA-002 + INFRA-003 compose into a full runner-compromise chain, and INFRA-004 supplies the
  entry point. They should be triaged as one item.
- The recon's update-channel claim is **refuted** (INFRA-009). I did not carry it forward. Its
  companion note about `ca2366f0 fix(update): the trust anchor followed the service config, not the
  manifest` is accurate and the fix is correctly implemented (verified-safe #16).
- Not filed, per the brief: the daemon's default-allow capability posture.

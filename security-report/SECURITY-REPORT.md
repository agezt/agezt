# Security Assessment Report

**Project:** AGEZT — self-hosted autonomous multi-agent platform (Go kernel + daemon/CLI, React/TS web console, Go/TS/Python/Rust SDKs)
**Date:** 2026-08-12
**Scanner:** security-check (4-phase pipeline: Recon → Hunt → Verify → Report), 9 hunt agents + 2 adversarial verifiers
**Branch / HEAD:** `main` @ `f815f56e`
**Supersedes:** the 2026-06-27 assessment at `99d2e426` (last refreshed 2026-07-06). 1,396 files changed between them.

> **Threat model.** AGEZT is a **localhost-first, single-operator, token-gated daemon**. The REST and
> OpenAI-compatible APIs are off by default; the **Web UI console is ON by default** at
> `127.0.0.1:8787`, loopback-bound and credential-gated. The agent gateway is a unix/loopback socket.
> Operators can reverse-proxy or tunnel these (a documented deployment), and the **15 channel webhook
> listeners are internet-facing by nature** — a webhook provider must reach them. Severities below are
> weighted for that reality, not zeroed.
>
> The previous report asserted every listener was "off by default"; that was wrong for the console and
> was load-bearing in its reasoning. Corrected here and in the source comment that caused it.

---

## Executive summary

Nine domain agents produced ~100 raw findings; two adversarial verifiers then attacked the Highs that
the lead had not personally confirmed. **One High was refuted outright, one was refuted as written,
and six were downgraded.** Every High that remains was verified against source by hand.

**The codebase's defensive core is genuinely strong, and this was tested rather than assumed.** The
RCE agent could not break `ShellQuote`, the traversal guards, the tar extractor, or the Edict
JSON-escape matcher, and recorded 13 checks as verified-safe. The SSRF agent could not get past
`netguard` — `Control` runs post-resolution so DNS rebinding fails, and `HTTPClient` builds a fresh
non-shared Transport so every redirect hop re-dials through the guard. The client agent found **zero**
XSS: no raw-HTML rendering path exists anywhere in the console. Nine of twenty Go checklist categories
came back clean, including zero `InsecureSkipVerify` and zero unchecked type assertions.

**Every finding is *around* those guards, never through them.**

### The dominant finding

**Eleven verified cases where documentation asserts a guarantee the code does not implement.** This is
one failure mode, not eleven bugs, and it is the most important result of this assessment:

| # | Documented claim | Reality |
|---|---|---|
| 1 | `sdkparity`: "regenerate to fix" | Would have deleted a correct route table |
| 2 | `httpsurfaces.go:61`: "unset = off" | Console is **on** by default |
| 3 | Prior threat model: "every listener off by default" | Web UI is on |
| 4 | `sse_guard.go:113`: "see `dialerGuard` below" | **No such function exists in the repo** |
| 5 | `standingTrustCeiling`: "must not run uncapped by omission" | Runs uncapped under the default ask policy |
| 6 | `ci.yml`: "the ciguard fork-guard lint" | Package deleted in `3e5b609d` |
| 7 | `files_route.go`: "then resolved symlinks" | Purely lexical — zero `EvalSymlinks` in the file |
| 8 | `warden.go:12`: "namespaces + cgroups + seccomp" | Only `Setpgid` + `prlimit` |
| 9 | Update flow `Check → Apply` | `checkGitHub` sets no SHA256; the flow **cannot complete** |
| 10 | Self-repair cooldown + attempt cap | Fingerprint mutates per incident; both inert |
| 11 | Prior report: "`gitleaks` came back empty" | The gate had been red for 17 days |

Items 1, 2, 3 and 11 were found and fixed *before* this scan began, which is what identifies the
pattern as systemic. The same mechanism that let three CI gates rot unnoticed is operating inside the
security model.

On #8, in fairness: `warden_linux.go:27-32` is honest about the gap. It is the public `Profile`
constant doc that overclaims.

On #6, a caution worth carrying: `internal/ciguard` was deleted as unreachable **because its only
consumer was a test — but that test was the control.** Weigh that before deleting the next
"test-only" symbol the deadcode gate flags.

---

## Confirmed findings

Severities are post-verification and reflect this project's stated posture: **default-allow is
deliberate**, so "capability X is permitted by default" is never a finding here.

### High

| ID | Finding | Where |
|---|---|---|
| UPD-001 | **Self-update installs a caller-supplied binary.** `verifySignature` picks its trust anchor from `cfg.Source`, but the REST and control-plane handlers build the manifest from the request body. With `SourceGitHub` + empty `DefaultPublicKeyHex`, an admin-token holder POSTs `{version, sha256, url}` at their own binary; it is validated against their own checksum and staged over `bin/agezt`. Persists across restart and token rotation. Found independently by two agents. | `kernel/update/update.go:391`, `restapi/update_handlers.go:105`, `controlplane/update_control.go:145` |
| CH-001 | **Inbound webhook verification is fail-open.** `buildZalo`/`buildFeishu`/`buildIMessage`/`buildDingTalk` start a listener on `_ADDR` alone; `zalo.go:143` wraps the entire signature *and* freshness check in `if c.cfg.Secret != ""`. Internet-facing by nature. The correct fail-closed pattern already exists in-tree (`buildLine`, `buildNextcloudTalk`). | `plugins/builtinchannels/factories.go`, `plugins/channels/*` |
| AUTH-001 | **Password-strict mode disarmed at boot.** `SetAllowedHosts` raises `passwordStrict` for a non-loopback host; the next line's `SetPasswordStrict(env)` assigns unconditionally and clears it. Separately, `hostAllowed` accepts any IP literal, so a `0.0.0.0` bind is LAN-reachable while registering no allowed host at all. Operator gets token **OR** password where they believe token **AND** password. | `cmd/agezt/httpsurfaces.go:114,125`; `kernel/webui/session.go:138` |
| SR-001 | **Self-repair rate guards never bind.** Cooldown and `MaxAttempts` key on a fingerprint embedding `failures=%d`/`count=%d` and the newest error string, so it differs every incident. The `misconfigured` path in the same file uses stable strings and works. | `kernel/selfrepair/selfrepair.go:458,462,569,590,614` |
| SR-002 | **LLM free text drives persisted fleet-wide config.** The manager agent's answer selects `force_chain`; `setTaskModelChain` writes the **global** governor map keyed by task type and persists it. The wake intent directs that agent to read mailbox and logs — injectable. | `kernel/selfrepair/selfrepair.go:1229`, `overseertool/kernelsource.go:671` |
| SSRF-001 | **`browser.action` bypasses netguard entirely.** The Go guard is a pre-flight DNS check; navigation happens in Playwright. The shipped driver has **zero** `page.route` and no egress control, so a 302 to `169.254.169.254` reaches cloud metadata — and `OnBlock` never fires, so it is invisible in `agt netguard log`. Opt-in (`AGEZT_BROWSER_ACTIONS`), which limits reach. | `plugins/builtinskills/browseruse/scripts/browse.mjs` |
| SDK-001 | **SDK socket path cannot reach the daemon, and fails into a token-capture.** Go binds `@agezt/agentgw.sock` to Linux's abstract namespace; Node maps abstract sockets with a leading NUL, not `@`, so it is a CWD-relative file path. A planted file in a writable CWD receives `Authorization: Bearer <capability token>`. Same constant in the Python SDK. | `sdk/typescript/src/agent.ts:43`, `sdk/python/agezt/agent.py:45` |
| PE-001 | **Trust ceilings L1–L3 are inert under the shipped default.** `DecideWithCeiling` clamps correctly, then L1–L3 fall in the Ask band which `AskAllow` (the default, and the fallback for unknown values) folds to Allow. Only an L0 ceiling bites. **Downgraded from the reported Critical:** this grants no new access, it fails to restrict — but `standingTrustCeiling` defaults unattended orders to L2 with a comment saying they must not run uncapped, and every seeded guardian ships `TrustCeiling: "L2"`. | `kernel/edict/edict.go:754`, `cmd/agezt/main.go:3845` |
| PE-005 | **Overseer `op=edit` has no self-target or monotonicity check.** Registered unconditionally with no env gate; `Invoke` discards the context, so a caller-side check is structurally impossible. Clearing one's own `tool_deny` escapes the one per-agent restriction that survives the AskAllow fold; raising `max_daily_mc` bypasses a hard governor refusal. Boot reconcile re-clamps only `if p.System`. | `plugins/tools/overseertool/`, `builtintools/tools.go:70` |
| WF-001 | **No panic firewall on goroutines running third-party code.** `kernel/workflow` and `kernel/selfrepair` have **zero** `recover()`; `standing`, `cadence`, `pulse` and `agent` each have one or two, and the standing-order `FireFunc` carries an eight-line comment explaining exactly this hazard. `workflowrun.go:753` invokes plugin subprocesses and MCP tools directly. | `kernel/workflow/runner.go:147,179,195` |
| CICD-001 | **Weekly Dependabot npm bumps execute install scripts on a non-ephemeral self-hosted runner.** `.github/dependabot.yml` is live for `/frontend` and `/sdk/typescript`; `ci.yml` has no `paths:` filter; the fork guard passes for same-repo branches; three jobs run `npm ci` with no `--ignore-scripts` and there is no `.npmrc`. One-flag fix. | `.github/workflows/ci.yml`, `.github/dependabot.yml` |

### Medium

`EXPOSE-001` journal segments `0o644` in `0o755` while auth tokens are `0o600`/`0o700` — four
constants, but ship with a `chmod` migration or existing installs stay exposed ·
`EXPOSE-003` `/api/webhook_log` returns full sink URLs, which *are* the credential for Slack/Discord/Teams,
and no redactor pattern matches (workaround exists today: `AGEZT_REDACT_EXTRA`) ·
`PATH-001` `resolveFileRoot` is lexical, so one symlinked directory inside the root gives arbitrary
read/delete/rename · `SSRF-002` `dialerGuard` documented but nonexistent; blind SSRF via 307 ·
`RCE-001` credential buckets keyed off the **requested** warden profile, never the **effective** one, so
on non-Linux hosts secrets scoped to an isolated tier land in plaintext for an un-isolated `cmd /C` ·
`RCE-002` `ProfileNamespace` reports `isolation=namespace` while implementing only setpgid + prlimit ·
`RCE-003` `code_exec` projects are daemon-global, contradicting the package doc ·
`PE-003` workboard claim theft (attribution + lease denial, not capability) ·
`PE-006` `op=repair` missing the System-guardian guard `EditAgent` has; durable harm limited to a
guardian `Soul` rewrite, which boot reconcile does not re-clamp ·
`CICD-003` `frontend-dist-rebuild` auto-commits the embedded bundle to `main` with `[skip ci]` — no
fork or PR can drive it, so an amplifier of CICD-001 rather than an entry point ·
`CICD-004` `publish-sdks.yml` dispatchable from any ref with no `environment:` gate — **severity is
contingent on whether the registry secrets exist; the operator must confirm** ·
`CICD-005` staticcheck's "checksum verification" fetches the digest from the same origin as the tarball ·
`CICD-007` `ci-go-retry.sh:31` `rm -rf /dev/shm/gocache-*` destroys the other two runners' live caches ·
`SUPPLY-001` Monaco (~3 MB) loads from a CDN, is absent from `package.json`, and is outside the
lockfile, Dependabot and `depscheck`; the shipped bundle requests 0.55.1 while source pins 0.52.2 ·
`SUPPLY-002` no npm vulnerability scanning anywhere in CI (309 transitive packages) while Go has
govulncheck + allowlist + deadcode gates · `SEC-002` unbounded `kdf_iter` ·
`SEC-003` `credential_process` inherits the full env including vault passphrase and provider keys ·
`GO-001` genuine data race in `provider_oauth.go:176-177`

### Refuted or materially downgraded by verification

| ID | Reported | Verdict |
|---|---|---|
| PRIVESC-002 | High — "resume tickets are unauthenticated" | **REFUTED.** `WithAgentProfile` re-applies the roster's own ceiling and `WithTrustCeiling` keeps the **minimum**, so a forged ticket can only *tighten*. Authority is re-derived from the roster, not reconstructed from the ticket. The write also requires arbitrary host execution, at which point `roster.json` and `standing.json` are strictly more powerful. |
| EXPOSE-002 | High — "agentgw audit bypasses the redactor" | **REFUTED as written.** The bypass is structurally real, but the only non-test `AuditEntry` construction never sets `Error`, `Path` excludes the query string, and `TokenID` is a ULID. The headline scenario has no code path. Fix cheaply to stop a future caller; not a High. |
| PRIVESC-001 | Critical | **Downgraded to High** — fails to restrict, does not grant. |
| EXPOSE-001, EXPOSE-003, PRIVESC-003, PRIVESC-006, CICD-003, CICD-004 | High | **Downgraded to Medium**, each for a stated reason above. |

---

## Recommended order

1. **CICD-001** — one flag (`--ignore-scripts`), weekly exposure, executes on a persistent runner.
2. **AUTH-001** — reorder two adjacent lines; make `SetPasswordStrict` never lower an auto-raised flag.
3. **UPD-001** — the manifest's provenance, not the service's configured source, must select the trust
   anchor. Note that the signed path is not wired end-to-end today, so this is a design fix, not a
   one-liner: embedding `DefaultPublicKeyHex` alone would make every `Apply` fail.
4. **SDK-001** — strip the `@` default in both SDKs, or map it to a NUL prefix for Node.
5. **SR-001 + SR-002 together** — a stable fingerprint is worthless while model text still writes
   global routing, and vice versa.
6. **CH-001** — *posture change, needs an owner decision:* making the four factories refuse without a
   secret will break existing operator configs that currently run unverified.
7. **PE-001 + PE-004 together** — `defaultSystemGuardianTrustCeiling = "L4"` is moot today but becomes
   a live bypass the moment ceilings start biting.
8. **WF-001, EXPOSE-001, PATH-001, SSRF-002** — each self-contained.

## Remediation status

Tracked here rather than by editing the findings above, so the assessment stays a snapshot of what was
true at scan time and this section carries what changed since. Verify against source before acting on
any row — this table goes stale the same way the finding tables do.

| ID | Status | Commit |
|---|---|---|
| CICD-001 | **Fixed.** `--ignore-scripts` on every `npm ci` in `ci.yml` and `publish-sdks.yml`. Deliberately *not* a repo-level `.npmrc`, which would also skip `fsevents` for macOS developers locally and quietly drop vite's native file watching. | `3987bf7c` |
| AUTH-001 | **Fixed.** An unspecified-address bind now raises `passwordStrict` from the listener's own resolved address, and the env var only ever *overrides* when explicitly set — it can no longer silently clear an auto-raised flag. | `ebb1e4df` |
| UPD-001 | **Fixed.** The trust anchor is selected by a new `Provenance` on the manifest whose **zero value is untrusted**, so a body-built manifest cannot inherit `SourceGitHub` by omission. Only `checkGitHub`/`checkEndpoint` set it. | `ca2366f0` |
| SDK-001 | **Fixed.** Both SDKs map a leading `@` to a NUL-prefixed abstract socket on Linux and leave it alone elsewhere, so the default no longer resolves to a CWD-relative file that could capture a bearer token. | `03694cdf` |
| SR-001 | **Fixed.** All six fingerprint builders stripped of the mutating fields (`failures=`, `count=`, newest error string); cooldown and `MaxAttempts` now key on a stable incident identity, matching the `misconfigured` path that already worked. | `e6d45e0e` |
| SSRF-002 / SSRF-003 | **Fixed.** The documented-but-nonexistent `dialerGuard` is now real: the SSE transport dials through `netguard`, and the package's drifted classifier copy was deleted in favour of delegating to `netguard.Allowed`. | `cb344a08` |
| PATH-001 | **Fixed.** Resolution is no longer lexical. Also walks components with `os.Readlink`, because on Windows a directory **junction** is `ModeIrregular` and `EvalSymlinks` returns it unchanged — `EvalSymlinks` alone passed a real junction in local testing. | `d009dc14` |
| EXPOSE-001 | **Fixed.** Journal directories `0o700`, segments `0o600`, plus a best-effort `chmod` migration for existing installs (best-effort, not fatal, per the boot-resilience law). | `44be317a` |
| WF-001 | **Fixed, all four sites.** `kernel/workflow`'s runner first (`899ed24c`), then the three that commit explicitly left open: both `kernel/selfrepair` goroutines and the two detached run paths in `kernel/controlplane/workflow.go`. | `899ed24c`, `50ce6a58` |
| GO-001 | **Fixed.** The sign-in status handler took `provLoginMu`, copied the `*providerLogin` out, unlocked, and *then* read `status`/`errMsg` — locking the pointer while racing the fields it points at. Both reads moved inside the critical section. | `9a943f82` |
| SEC-002 | **Fixed.** `kdf_iter` now has a ceiling as well as a floor, checked *before* the derivation. Decrypt derived with the envelope's own attacker-supplied count, and PBKDF2 is O(iter) by design — on a file opened at daemon boot, that is a wedge, not a slow unlock. | `09bdc83d` |
| CH-001 | **Blocked on an owner decision** — see item 6 above; fail-closed breaks existing operator configs. | — |
| SR-002 | **Blocked on an owner decision.** Scoping the write, approval-gating it, and restricting it by task type are three different products, not three spellings of one fix. | — |
| PE-001 + PE-004 | **Blocked on an owner decision**, and must ship together. Interacts directly with the default-allow posture law. | — |
| CICD-004 | **Resolved to latent-Low, no owner input needed after all.** `PYPI_API_TOKEN`, `NPM_TOKEN` and `CARGO_REGISTRY_TOKEN` do not exist: `repos/agezt/agezt/actions/secrets` and `.../actions/organization-secrets` both return `total_count: 0`, and the repo has no environments. There is nothing for an unauthorised dispatch to exfiltrate today. The missing `environment:` gate is still worth adding *before* the secrets are created, not after. | — |
| CICD-007 | **REFUTED — the citation is fabricated.** `.github/scripts/ci-go-retry.sh` does not exist and never has (`git log --all` on that path is empty; there is no `.github/scripts/` directory). No `rm -rf /dev/shm/gocache-*` wildcard appears anywhere in `.github/`. The real staging in `setup-go-safe/action.yml:186` is already per-runner — `gocache-${RUNNER_NAME}` — and carries a comment explaining precisely the three-runners-share-one-`/dev/shm` collision this finding describes. See the Medium-tier note below. | — |
| SUPPLY-001 | **Materially corrected; the core concern stands.** Two of the three claims are wrong: Monaco is *not* absent from `package.json` (`@monaco-editor/react` is a declared dependency), and the bundle does *not* request 0.55.1 — `lib/monaco.ts` calls `loader.config()` at module scope and `MonacoView` imports it statically, while `Editor` is `lazy()`, so the pin is applied strictly before `init()` runs. The `0.55.1` literal in the vendor chunk is `@monaco-editor/loader`'s **overridden default**; `0.52.2` is in the Markdown chunk and wins. What remains true and unfixed: the ~3 MB editor **core** is fetched from jsDelivr at runtime, is outside the lockfile, Dependabot and `depscheck`, and carries no integrity check. | — |

### The Medium tier was not hand-verified, and it shows

The Method note below is precise about its own scope: **every High** was read in source before being
recorded. The Mediums were not, and a later verification pass over their citations found the tier is
measurably less reliable than the tier above it:

- **CICD-007 cites a file that has never existed in this repository.** Not moved, not renamed — no
  such path in `git log --all`. The hazard it describes is real in the abstract and the repo already
  defends against it, with a comment saying so.
- **SUPPLY-001 got two of its three factual claims wrong** in the direction of alarm, while its
  actual concern (a CDN-loaded dependency outside the lockfile, with no integrity check) is sound.
- **CICD-004's "the operator must confirm"** needed no operator: two API calls settled it.

Treat an unverified Medium as a lead, not a finding. Where a Medium is cheap to check, check it — two
of the four examined here did not survive contact with the source, and one of those was pure
fabrication. This is the same failure mode as the report's own dominant finding (a claim asserted
without the thing it claims), turned back on the report.

## Method note

Every High in this report was read in source by the lead before being recorded here; agent claims were
not relayed on trust. Two Highs died that way, and one of the lead's own proposed refutations
(that CICD-001 would collapse because Dependabot might not be configured) was itself disproved by a
verifier. Each `*-results.md` carries a "verified clean" appendix recording what was checked and
dismissed, so a future pass does not re-derive it.

**Not covered:** container and IaC scanning (no Dockerfile, compose, K8s or Terraform exists in the
tree) and SQL/NoSQL/GraphQL/SSTI injection (no `database/sql`, no ORM, no GraphQL, and zero files
importing `text/template` or `html/template`). These are accurate "nothing to scan", not gaps.

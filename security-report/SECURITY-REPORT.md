# Security Assessment Report

**Project:** AGEZT — self-hosted autonomous multi-agent platform (Go kernel + daemon/CLI, React/TS web console, Go/TS/Python/Rust SDKs)
**Date:** 2026-06-27 (last refreshed: 2026-07-06)
**Scanner:** security-check v1.1.0 (4-phase pipeline: Recon → Hunt → Verify → Report)
**Branch / HEAD at scan:** `main` @ `99d2e426`; **current HEAD:** `ef7b412d`
**Risk Score:** **2.6 / 10 — Low Risk**

> **Threat model.** AGEZT is a **localhost-first, single-operator, token-gated daemon**. Every network listener
> (Web UI, REST, OpenAI-compatible API, agent gateway) is **off by default and loopback-bound**; the always-on
> control plane is loopback + bearer-token. Operators *can* reverse-proxy/tunnel these (a documented deployment),
> so internet-facing reachability is **weighted, not zeroed**, throughout this report.

---

## Executive Summary

A full security assessment was performed across **12 vulnerability domains** (injection, code-execution, client-side,
access-control, secrets/crypto, SSRF/path, API/logic, infrastructure/CI) plus **language-specific deep scans** for Go
and TypeScript/JS, over ~**1,232 Go files (~284k LOC)** and ~**340 TS/TSX files (~93k LOC)**, with light passes on the
Python and Rust SDKs. Phase 2 produced 31 raw non-informational candidates; verification merged duplicates and
eliminated defense-in-depth/false-positive items down to **13 verified findings**.

**The codebase is genuinely well-hardened.** Whole domains came back verified-clean: no SQL/NoSQL/template/LDAP
injection surface, a disciplined single-choke-point command-execution sandbox (env-scrubbed, edict-gated, array-form
`exec.Command`), a correct SSRF dialer guard (`netguard`) that validates the *resolved* IP on every redirect hop,
AES-256-GCM/PBKDF2-200k vault encryption with fresh CSPRNG nonces, constant-time auth comparisons, a strict CSP with
no CORS exposure, and SHA-pinned least-privilege CI. `gitleaks` came back empty (`[]`).

The findings that remain cluster around three themes: a **privilege-management logic flaw in delegation** (the only
High), a **boot-window data race** that can crash the daemon, and a **resource-exhaustion / budget-DoS** gap from
missing default rate limits on expensive endpoints.

### Key Metrics
| Metric | Value |
|--------|-------|
| Total Verified Findings | 13 |
| Critical | 0 |
| High | 1 |
| Medium | 7 |
| Low | 5 |
| Info | 0 (informational items consolidated under *Eliminated Findings* in `verified-findings.md`) |

### Top Risks
1. **VULN-001 (High):** Trust ceiling is last-write-wins, not min-clamped — a capped run can **delegate to a
   higher-ceiling agent and escape its parent's autonomy cap** (privilege escalation logic flaw). Compounded by
   VULN-003 (uncapped autonomous default) and VULN-004 (injected content reaching an autonomous run) — one chain.
2. **VULN-002 (Medium, Confirmed):** Boot-window **data race on process-global channel-registry maps** → Go fatal
   `concurrent map read and map write` → **unrecoverable daemon crash** (the web UI auto-polls the exact endpoint).
3. **VULN-005 / 006 / 007 (Medium cluster):** **No default per-caller rate limit** on the expensive run endpoints,
   and the daily budget ceiling is a **soft check-then-act cap** — a token holder (or a leaked `/hooks/` secret) can
   exhaust the budget and pin CPU.

---

## Scan Statistics

| Statistic | Value |
|-----------|-------|
| Files Scanned | ~1,232 Go · ~340 TS/TSX · 104 JS · 27 Py · 6 Rs |
| Lines of Code | ~377k (Go ~284k, TS/JS ~93k) |
| Languages Detected | Go (primary), TypeScript/JS (primary), Python (SDK), Rust (SDK) |
| Frameworks Detected | stdlib `net/http` + custom JSON-line RPC control plane; React 19 / Vite 8 / Tailwind 4 / Radix (embedded via `go:embed`) |
| Persistence | Append-only JSONL journal + JSON state buckets + AES-256-GCM vault — **no SQL/NoSQL DB, no ORM** |
| Skills / Scanners Executed | 10 hunt scanners (covering 33 skill domains) + recon + dependency-audit + verifier + report |
| Raw Findings (Phase 2) | 31 non-informational candidates |
| Merged / Eliminated | 18 (duplicates merged + defense-in-depth/FP eliminated) |
| Final Verified Findings | 13 |

### Finding Distribution by Category
| Vulnerability Category | Critical | High | Medium | Low | Info* |
|------------------------|:--------:|:----:|:------:|:---:|:-----:|
| Access Control / PrivEsc | 0 | 1 | 2 | 0 | ✓ |
| Concurrency / DoS (lang-go) | 0 | 0 | 1 | 1 | ✓ |
| API / Rate-limiting / Logic | 0 | 0 | 3 | 0 | ✓ |
| Client-side (XSS/iframe) | 0 | 0 | 1 | 0 | ✓ |
| Infrastructure / CI/CD | 0 | 0 | 1 | 1 | ✓ |
| Dependencies / Supply-chain | 0 | 0 | 0 | 1 | ✓ |
| Data Exposure (CWE-209) | 0 | 0 | 0 | 1 | ✓ |
| Injection · Code-exec · SSRF · Secrets/Crypto | 0 | 0 | 0 | 0 | ✓ (all verified-clean) |

\* Every category produced informational/defense-in-depth confirmations — see *Eliminated Findings* in `verified-findings.md`.

---

## High Findings

### VULN-001: Trust ceiling is last-write-wins — delegation escapes the parent's initiative cap
**Severity:** High · **Confidence:** 90/100 (Confirmed) · **CWE-269** (Improper Privilege Management) · **OWASP A01:2021 — Broken Access Control**
**Location:** `kernel/runtime/runtime.go:2005-2010` (`WithTrustCeiling`), `:2228-2232` (`WithAgentProfile`); `kernel/runtime/subagent.go:561,573-575`; `kernel/edict/edict.go:726,776-779`

**Description.** `WithTrustCeiling` stores the ceiling as a plain context value with no regard for any ceiling already
present (`return context.WithValue(ctx, ctxKeyTrustCeiling, ceiling)` — last-write-wins, no `min`-merge). A run capped
at L0/L1/L2 by a standing-order initiative ceiling holds the default-allow `delegate` tool. When it delegates to a
directly-callable agent whose profile `TrustCeiling` is **higher (L3) or empty (→ L4, uncapped)**,
`WithAgentProfile(childCtx, *prof)` re-applies the **target's looser** ceiling, overwriting the parent's. `DecideWithCeiling`
then clamps against the surviving (looser) value, so capabilities the parent run was forbidden now execute in the child.

**Vulnerable code (conceptual):**
```go
// kernel/runtime/runtime.go:2005
func WithTrustCeiling(ctx context.Context, ceiling Level) context.Context {
    if ceiling >= LevelAllow { return ctx }       // only a no-op short-circuit
    return context.WithValue(ctx, ctxKeyTrustCeiling, ceiling) // overwrites a tighter existing ceiling
}
```

**Proof of concept (conceptual).** An enabled, L2-capped standing-order initiative run invokes `delegate` against an
agent profile with no `TrustCeiling` (or L3). The child context inherits the target profile's looser ceiling; the child
then performs an L3/L4 capability (e.g. an action the parent's L2 cap would have forced to *ask*/deny) with no approval.

**Impact.** Autonomy/privilege escalation: the operator's intended "this initiative may only act up to L2" bound is
silently bypassed for any delegated work, defeating the trust-ceiling control for the most autonomous code paths. The
non-overridable hard-deny floor still holds, but the entire L1/L2/L3-vs-L4 ask/approval surface is bypassed.

**Remediation.** Make the ceiling **monotonically tightening** down the delegation tree:
```go
func WithTrustCeiling(ctx context.Context, ceiling Level) context.Context {
    if existing, ok := ctx.Value(ctxKeyTrustCeiling).(Level); ok && existing < ceiling {
        ceiling = existing                         // a child ceiling may only tighten, never loosen
    }
    return context.WithValue(ctx, ctxKeyTrustCeiling, ceiling)
}
```
Equivalently, have the delegation path explicitly intersect parent ceiling with the target profile ceiling before
applying. Add a regression test asserting a delegated child never runs above its parent's ceiling.

**References:** CWE-269 · OWASP A01:2021.

---

## Medium Findings

### VULN-002: Boot-window data race on channel-registry maps → fatal crash DoS
**Confidence:** 90/100 (Confirmed) · **CWE-362** · **Location:** `kernel/channel/registry.go:46-110`; writers `cmd/agezt/main.go:1827-1828,1933`; listener start `:1368`; readers `kernel/controlplane/channels.go:90-119`

Three package-global maps (`registry`, `live`, `liveInstances`) are read/written with **no mutex**. The control-plane
listener starts serving (`main.go:1368`) **before** the boot-time writes (`:1827-1933`). A `/api/channel/list` request
in that window races a map read against a write → Go's runtime **fatally aborts** (`concurrent map read and map write`),
bypassing `recover()`. The web UI auto-polls this endpoint on load, landing precisely in the window.
**Fix:** add a package `sync.RWMutex` — `RLock` in `Manifests`/`LookupManifest`/`IsLive`/`IsLiveInstance`, `Lock` in
`RegisterManifest`/`SetLive`/`SetLiveInstances`.

### VULN-003: `act_or_ask`/empty initiative mode with no `max_trust` runs uncapped
**Confidence:** 80/100 · **CWE-269** · **Location:** `kernel/standing/standing.go:77-86`; `cmd/agezt/main.go:5133-5148,5271-5273`

`standingTrustCeiling` returns *no cap* for `act_or_ask`/empty mode when `MaxTrust==""`, so the fire path never calls
`WithTrustCeiling` and the run executes at full default-allow (L4). The most-permissive "initiative mode" silently
equals "no trust clamp." This is the **enabling condition** that lets VULN-001's escalation reach an uncapped state.
**Fix:** apply a non-`LevelAllow` default ceiling (mirror the seeded responder's L2) whenever an autonomous mode has no
explicit `max_trust`, so the fire path fails safe; surface the effective ceiling in `standing_list`.

### VULN-004: Untrusted Pulse-observation content reaches an autonomous run via the intent string
**Confidence:** 72/100 · **CWE-1427 / CWE-77** · **Location:** `kernel/standing/runner.go:134-152`; `cmd/agezt/main.go:5229`; `kernel/runtime/runtime.go:1616-1627`

When a standing order fires from `pulse.initiative.act`, the trigger payload (which for web/external Pulse observers can
derive from attacker-influenced ingested content) is serialized **verbatim** into the agent's user intent. The
prompt-injection guard is **taint-based** and fires only on `UntrustedObservationTaint`-marked *tool observations* — the
intent path carries no taint, so the guard never engages. Guarded down by default (the seeded `guardian-initiative`
responder ships **disabled**, L2-capped), but operators do enable initiative.
**Fix:** run the fired order's intent through the same injection screening / `UNTRUSTED OBSERVATION` wrapping used for
tool output, or attach `UntrustedObservationTaint` when the intent is built from an external trigger payload.

### VULN-005: No per-caller rate limit on expensive run endpoints by default
**Confidence:** 78/100 · **CWE-770** · **Location:** `kernel/openaiapi/openaiapi.go:170,188,487`; `kernel/restapi/restapi.go:382`; `kernel/webui/webui.go:644-645`; governor default `cmd/agezt/main.go:6382` (`ratePerMin := 0`)

Every run-submitting endpoint drives the LLM + `code_exec`/tool loop (real money + CPU per request). The governor's
`RateLimitPerMin` **defaults to 0 (unlimited)** unless `AGEZT_RATE_PER_MIN` is set, and there is no global
max-in-flight-runs semaphore. A token holder (or anyone reaching a reverse-proxied instance) can fire unlimited
concurrent expensive runs, exhausting the daily budget in seconds and pinning CPU. The `/v1/audio/transcriptions`
multipart path (`io.ReadAll` of ≤25 MiB) shares the no-throttle property at lower impact.
**Fix:** default `AGEZT_RATE_PER_MIN` to a sane non-zero value for network listeners (or per-token rate-limit the
OpenAI/REST surfaces as agentgw already does); add an optional global max-concurrent-runs semaphore; stream the
transcription upload. At minimum, prominently document that operators fronting these listeners must set a rate cap.

### VULN-006: Daily/task/agent budget ceilings are soft (check-then-act) — concurrent runs overshoot
**Confidence:** 70/100 (documented-as-intended) · **CWE-362** · **Location:** `kernel/governor/governor.go:602-621,648-676,1450-1500,1614-1643`; `kernel/runtime/subagent.go:501`

The budget gate is a check-then-act split across two critical sections with the slow provider call in between: N
concurrent completions can all read headroom, all proceed, and together exceed the ceiling by up to (N-1) calls' worth.
The code **explicitly documents this as an accepted soft-cap design** (reaffirmed 2026-06); negative-token clamping
already prevents ledger-crediting attacks. Meaningful mainly as the **amplifier of VULN-005**.
**Fix (only if a hard cap is required):** reserve estimated cost under the same lock as the pre-check and reconcile the
actual after the call. Otherwise accept as-is and document.

### VULN-007: Workflow webhook (`/hooks/`) has no rate limit — one leaked secret = unbounded paid runs
**Confidence:** 68/100 · **CWE-770 / CWE-799** · **Location:** `kernel/webui/webui.go:730,742`; `kernel/controlplane/workflow.go:215-221`

`/hooks/<workflow>` is the deliberately token-free web path; auth is otherwise sound (empty-secret rejected,
constant-time, uniform 403, 256 KiB body cap). But there is **no rate limit** — an attacker who learns one workflow
secret can POST it in a loop, each fire launching a governed agent run with full spend, bounded only by the same soft
daily cap. `?secret=` in the query string also lands in proxy/access logs and browser history.
**Fix:** add a per-workflow (or per-source-IP) rate limit on `/hooks/`; prefer the header form and strip `?secret=` from
logs; add a per-workflow max-fires-per-minute knob.

### VULN-008: Agent/channel HTML artifacts rendered as live scripts in a sandboxed iframe
**Confidence:** 62/100 · **CWE-79 / CWE-1021** · **Location:** `frontend/src/views/Artifacts.tsx:394-402` (sink), `:346,359-361` (data flow)

The artifact viewer renders an HTML artifact's bytes with `<iframe srcDoc={text} sandbox="allow-scripts">`. Artifact
bytes are attacker-influenceable, and `srcDoc` re-injection bypasses the server's `text/html`→octet-stream content-type
defense. **Isolation is genuinely strong** — `allow-same-origin` is *absent*, so scripts run in a null origin and
cannot read the parent token/cookies/DOM or issue same-origin `/api/*` calls; the page CSP and the server route's
content-type downgrade add two more layers. Residual risk: `allow-scripts` still permits uncredentialed exfil/beacon,
convincing in-console phishing overlays (fake "re-enter password"), and browser-0-day surface — from merely viewing a
malicious artifact (CSP-on-`srcdoc` is not uniform across engine versions).
**Fix:** drop `allow-scripts` and render sanitized static HTML (or route HTML artifacts through the safe Markdown/text
path); if live HTML must run, add an explicit per-frame `csp` attribute, set `referrerpolicy="no-referrer"`, and gate
script execution behind an explicit operator "run scripts" click.

### VULN-009: CI runs on persistent self-hosted runners — fork-PR safety rests on a single `if:` gate
**Confidence:** 60/100 (latent) · **CWE-693 / CWE-829** · **Location:** `.github/workflows/ci.yml:32` (gate replicated on all 14 jobs); `runs-on: [self-hosted, Linux, X64]`

All 14 CI jobs run on three persistent WSL runners sharing one VM. The **only** thing keeping fork-PR code off those
long-lived machines is the per-job `if:` gate (present and correct today). If it is ever edited away or repo Actions
settings relax, the result is arbitrary code execution on the owner's daily-driver VM with **state-bleed** into
subsequent trusted builds (`~/go/bin`, `/dev/shm/goroot-*`, `RUNNER_TOOL_CACHE`) → supply-chain poisoning of release
binaries. Not exploitable today; flagged for blast-radius. (Project history notes `main` has at times been unprotected.)
**Fix:** set Actions → "Require approval for all external collaborators"; never offer self-hosted runners to public-fork
PRs; move toward ephemeral runners (or pre-job wipe of the shared caches); protect `.github/` (see VULN-013).

---

## Low Findings

| ID | Title | CWE | Confidence | Location | Fix summary |
|----|-------|-----|:---------:|----------|-------------|
| **VULN-010** | `undici` security override (`^7.28.0`) not enforced in resolved lock (+ dual lockfile) | CWE-1104 | 65 | `frontend/package.json` vs `pnpm-lock.yaml` (`undici@7.27.2`) | `pnpm install` to re-resolve & commit; delete the non-source-of-truth lockfile (keep pnpm); verify `lucide-react@1.x` provenance. Build-only, no runtime path. |
| **VULN-011** | Unbounded `io.ReadAll` in `retry.ReadBody` (latent OOM) | CWE-770 | 48 | `plugins/providers/internal/retry/retry.go:259,262` | Wrap reads in `io.LimitReader`/`httpread.All`; stop discarding the read error. No current hot-path caller. |
| **VULN-012** | OpenAI-compat API echoes raw upstream provider error to authed client | CWE-209 | 42 | `kernel/openaiapi/openaiapi.go:534,726`; `responses.go:92,313` | Run upstream/STT error strings through `kernel/redact` before the HTTP body, or return generic + log redacted. Audience is the already-privileged operator. |
| **VULN-013** | No `CODEOWNERS` protecting `.github/` | CWE-693 | 40 | repo-wide (0 matches) | Add `.github/CODEOWNERS` requiring trusted review of `/.github/**`; enable branch protection + required checks on `main`. |

---

## Informational / Verified-Clean (highlights)

The following were **verified safe** and are documented in full under *Eliminated Findings* in `verified-findings.md`:

- **Injection (SQLi/NoSQLi/SSTI/XXE/LDAP/CRLF):** no DB/ORM/template-engine/LDAP surface; the two real header sinks
  (email `Subject`, artifact `Content-Disposition` filename) strip CRLF; Go stdlib backstops apply.
- **Code-execution:** single warden choke point, env-scrubbed array-form `exec.Command`, edict-gated spawns, slug-only
  agent selectors; **no** `gob`/YAML/`plugin.Open`/`interface{}` deserialization; JWT alg/typ/iss/aud-pinned.
- **SSRF / path / upload:** `netguard` validates the resolved IP on initial dial **and every redirect hop** (defeats
  DNS-rebinding & redirect-to-internal); `file` tool is `EvalSymlinks`+`O_NOFOLLOW` confined; backup restore is
  zip-slip-safe; self-update download is netguard-wrapped + SHA256+Ed25519-verified on `main`.
- **Secrets / crypto:** `.env` correctly gitignored; vault AES-256-GCM w/ fresh CSPRNG salt+nonce + PBKDF2-200k; all
  comparisons constant-time; zero `InsecureSkipVerify`; redaction wired into bus/journal/plugin-log; `gitleaks` = `[]`.
- **Client-side:** strict CSP (`default-src 'none'`), `frame-ancestors 'none'`/`X-Frame-Options DENY`, SameSite=Strict
  + HttpOnly sessions + Origin/Sec-Fetch CSRF gate, **no `Access-Control-Allow-Origin` anywhere**; no
  `dangerouslySetInnerHTML`; XSS-safe Markdown renderer (`safeHref` blocks `javascript:`/`data:`).
- **CI/CD:** top-level `permissions: contents: read`, full-SHA-pinned actions, `persist-credentials: false`, no
  `pull_request_target`, no `${{ github.event.* }}` in `run:` blocks.
- **Docker / IaC:** no surface (no Dockerfile/compose/Terraform/k8s/helm).

---

## Remediation Roadmap

### Phase 1 — Immediate (1–3 days): the substantive chain + the crash
| # | Finding | Effort | Impact |
|---|---------|--------|--------|
| 1 | **VULN-001** trust-ceiling `min`-clamp (privilege escalation) | Low (a few lines + test) | High |
| 2 | **VULN-003** fail-safe default ceiling for autonomous modes | Low | Medium (closes VULN-001's uncapped reach) |
| 3 | **VULN-002** add `RWMutex` to channel registry (crash DoS) | Low | Medium (daemon stability) |

### Phase 2 — Short-Term (1–2 weeks): DoS hardening + injection-path gap
| # | Finding | Effort | Impact |
|---|---------|--------|--------|
| 4 | **VULN-005** non-zero default rate limit + max-concurrent-runs semaphore | Medium | Medium |
| 5 | **VULN-007** rate-limit `/hooks/`; prefer header secret over `?secret=` | Low | Medium |
| 6 | **VULN-004** screen `pulse.initiative.act` intent as untrusted observation | Medium | Medium |
| 7 | **VULN-008** drop `allow-scripts` / gate HTML-artifact script execution | Medium | Medium |

### Phase 3 — Medium-Term (1–2 months): infra & supply-chain
| # | Finding | Effort | Impact |
|---|---------|--------|--------|
| 8 | **VULN-009** ephemeral runners / Actions external-approval; **VULN-013** add `CODEOWNERS` + branch protection | Medium | Medium (blast-radius) |
| 9 | **VULN-010** re-resolve `pnpm-lock`, drop dual lockfile, verify `lucide-react` | Low | Low |
| 10 | **VULN-006** (optional) hard budget cap with cost reservation — only if a hard ceiling is needed | Medium | Low |

### Phase 4 — Hardening (ongoing): defense-in-depth
| # | Recommendation | Effort | Impact |
|---|----------------|--------|--------|
| 11 | **VULN-011** bound `retry.ReadBody` with `io.LimitReader`; stop discarding read error | Low | Low |
| 12 | **VULN-012** redact upstream/STT error text before HTTP body | Low | Low |
| 13 | Apply `safeHref` to backend-supplied `docs_url`/`authorize_url`; re-assert roster `System` flag at store `Update`; document SDK loopback/TLS posture; confirm `DefaultPublicKeyHex` is set in release builds; run live `govulncheck` + `osv-scanner`/`pnpm audit` | Low | Low |

---

## Methodology

This assessment used **security-check** (AI-powered static analysis), a 4-phase pipeline:

1. **Reconnaissance** — architecture mapping, tech-stack/entry-point/trust-boundary detection (`architecture.md`).
2. **Vulnerability Hunting** — 10 parallel domain scanners covering 33 skill areas + Go/TS deep language scans; each ran
   internal Discovery → Verification and wrote a `*-results.md`.
3. **Verification** — `sc-verifier` applied reachability, sanitization, framework-protection, configuration, and context
   modifiers; merged duplicates; assigned 0–100 confidence and recalculated severity (`verified-findings.md`).
4. **Reporting** — CVSS-aligned severity, risk scoring, and a prioritized remediation roadmap (this file).

**Risk-score derivation:** base = 1×High(1.0) + 7×Medium(0.3) + 5×Low(0.1) = **3.6**; modifiers −1.0 (strong security
controls verified in place) −0.5 (good security-feature test coverage) → **2.1**, presented as **2.6/10 (Low Risk)**
to avoid understating the single High privilege-escalation finding.

### Limitations
- Static analysis only — no runtime/dynamic testing or live exploitation was performed.
- Dependency CVE flags are **version heuristics**; confirm with `govulncheck` + `osv-scanner`/`pnpm audit`.
- AI reasoning may miss deep domain-specific or custom business-logic flaws; confidence scores are estimates.
- `.worktrees/rebased-main` and `.claude/worktrees/` duplicate trees were excluded from findings.

---

## Disclaimer

This security assessment was performed using automated AI-powered static analysis. It does not constitute a comprehensive
penetration test or security audit. Findings represent potential vulnerabilities identified through code-pattern analysis
and LLM reasoning; false positives and false negatives are possible. Use this report as a starting point for remediation,
not as a definitive statement of the application's security posture. A professional audit by qualified security engineers
is recommended for production deployments handling sensitive data.

_Generated by security-check — github.com/ersinkoc/security-check_

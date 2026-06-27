# Verified Security Findings

> Phase 3 verifier (`sc-verifier`) over all `security-report/*-results.md`.
> Repo: `D:\Codebox\PROJECTS\AGEZT` (branch `main`). Threat model: **localhost-first, single-operator,
> token-gated daemon**; network listeners are off-by-default + loopback-bound, but the operator *can*
> reverse-proxy/tunnel them (a documented deployment), so internet-facing reachability is weighted —
> not zeroed — accordingly. Duplicate `.worktrees/rebased-main` / `.claude/worktrees/` trees were ignored.
> Two load-bearing claims were re-verified directly against source: `WithTrustCeiling`
> (`kernel/runtime/runtime.go:2005-2010`, last-write-wins confirmed) and the unguarded channel-registry
> globals (`kernel/channel/registry.go:46-110`, no mutex confirmed).

## Summary
- Total raw findings from Phase 2: 31 non-info items (AC-01..04, XSS-001..003, EXPOSE-002, RATE-001..003, RACE-001, API-001, API-002, CICD-001, CICD-002, 2× lang-go Medium + 3× lang-go Low, TS-001, TS-002, DEP-001..009 carrying DEP-001/002 forward)
- After duplicate merging: 16 distinct findings (RATE-001/RATE-002/RATE-003/RACE-001 grouped into the budget-DoS cluster; API-002 + self-update Ed25519-not-activated merged across api-logic/dependency/ssrf perspectives; lang-go context.Background items merged)
- After false-positive / info elimination: 13 verified findings carried into VULN blocks
- **Final verified findings: 13**

## Confidence Distribution
- Confirmed (90-100): 2  (VULN-001, VULN-002)
- High Probability (70-89): 4  (VULN-003, VULN-004, VULN-005, VULN-006)
- Probable (50-69): 4  (VULN-007, VULN-008, VULN-009, VULN-010)
- Possible (30-49): 3  (VULN-011, VULN-012, VULN-013)
- Low Confidence (0-29): 0

## Severity Roll-up (post-recalculation)
| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 1 |
| Medium | 7 |
| Low | 5 |
| Info | 0 (informational items listed under "Eliminated Findings") |

---

## Verified Findings

### VULN-001: Trust ceiling is last-write-wins, not min-clamped — delegation to a higher-ceiling profile escapes the parent's initiative cap
- **Severity:** High
- **Confidence:** 90/100 (Confirmed)
- **Original Skill:** access-control (AC-01)
- **Vulnerability Type:** CWE-269 (Improper Privilege Management)
- **File:** `kernel/runtime/runtime.go:2005-2010` (`WithTrustCeiling`), `:2228-2232` (`WithAgentProfile` re-applies profile ceiling); `kernel/runtime/subagent.go:561,573-575` (child ctx → profile applied); `kernel/edict/edict.go:726,776-779` (`DecideWithCeiling` clamp)
- **Reachability:** Direct (delegation is a default-allow `CapDelegate` tool on every run; child-context apply path is on the hot delegation flow)
- **Sanitization:** N/A (logic flaw, not input-handling)
- **Framework Protection:** None — the non-overridable hard-deny floor still holds, but the entire L1/L2/L3-vs-L4 ask/approval surface is bypassed for delegated work
- **Description:** `WithTrustCeiling` stores the ceiling as a plain context value with no regard for a ceiling already present (`return context.WithValue(ctx, ctxKeyTrustCeiling, ceiling)` — confirmed in source, no min-merge). A run capped at L0/L1/L2 by a standing-order initiative ceiling holds the default-allow `delegate` tool; when it delegates to any directly-callable agent whose profile `TrustCeiling` is higher (L3) or empty (→ L4, uncapped), `WithAgentProfile(childCtx,*prof)` re-applies the *target's* looser ceiling, overwriting the parent's. `DecideWithCeiling` then clamps against the surviving (looser) value, so capabilities the parent run was forbidden execute in the child.
- **Verification Notes:** Re-read `WithTrustCeiling` at the cited lines — it is unambiguously last-write-wins (the only guard is the `ceiling >= LevelAllow` no-op short-circuit, which does NOT prevent a *looser* non-L4 value overwriting a *tighter* one). The escalation requires (a) an enabled, capped initiative run and (b) the default `delegate` tool reaching a higher-ceiling profile — both default conditions, hence confidence held at 90 (the substantive top finding of this report). Reachability +30 (direct from delegate tool), no sanitization/framework offset; base 90 confirmed.
- **Remediation:** In `WithTrustCeiling`, read any existing `ctxKeyTrustCeiling` and store `min(existing, ceiling)`; or have the delegation path explicitly intersect parent ceiling with profile ceiling before applying. A ceiling must only ever tighten down the delegation tree.

---

### VULN-002: Boot-window data race on process-global channel-registry maps → fatal daemon crash (remote-triggerable DoS)
- **Severity:** Medium
- **Confidence:** 90/100 (Confirmed)
- **Original Skill:** lang-go
- **Vulnerability Type:** CWE-362 (Concurrent Execution using Shared Resource without Proper Synchronization)
- **File:** `kernel/channel/registry.go:46,49,52-58,62-65,70,74-80,83,97,100-106,110`; writers `cmd/agezt/main.go:1827-1828,1933`; listener start `:1368`; readers `kernel/controlplane/channels.go:90-119`, `channel_accounts.go:30,130`
- **Reachability:** Direct (control-plane handlers serve `/api/channel/list` and channel-accounts concurrently; the web UI auto-polls the channel list on load, landing in the boot window)
- **Sanitization:** N/A
- **Framework Protection:** None — Go's runtime *detects* concurrent map read/write and **fatally aborts**, bypassing `recover()`
- **Description:** Three package-global maps (`registry`, `live`, `liveInstances`) are read and written with no `sync.Mutex`/`RWMutex` (confirmed: no lock anywhere in `registry.go`). Writers run during daemon boot (`RegisterAll`→`RegisterManifest`, `SetLive`/`SetLiveInstances`) *after* the control-plane listener has already started serving (`srv.Start` at `main.go:1368` precedes the writes at `:1827-1933`). A channel-list request arriving in that window races a `range`/index read against a map write, triggering an unrecoverable `concurrent map read and map write` fatal abort.
- **Verification Notes:** Read the full `registry.go` — confirmed all three globals are bare `map[...]` with plain function-level read/write and no synchronization; even `SetLive`/`SetLiveInstances` swap the map *variable* (a racy write against a concurrent reader). Boot ordering (listener-before-writes) confirmed from the cited `main.go` lines in the source report. The window is short/timing-dependent (slight reachability discount), but the fault is a hard crash and the web UI auto-polls the exact endpoint at load — net confidence held at 90.
- **Remediation:** Add a package-level `sync.RWMutex` guarding all three maps: `RLock` in `Manifests`/`LookupManifest`/`IsLive`/`IsLiveInstance`, `Lock` in `RegisterManifest`/`SetLive`/`SetLiveInstances`.

---

### VULN-003: `act_or_ask`/empty initiative mode with no `max_trust` runs uncapped — most-permissive dial silently = no clamp
- **Severity:** Medium
- **Confidence:** 80/100 (High Probability)
- **Original Skill:** access-control (AC-03)
- **Vulnerability Type:** CWE-269 (Improper Privilege Management)
- **File:** `kernel/standing/standing.go:77-86` (`MaxAutonomyTrust()` → `("",false)` for `act_or_ask`/empty); `cmd/agezt/main.go:5133-5148` (`standingTrustCeiling` → `(_,false)`), `:5271-5273` (fire path skips `WithTrustCeiling` when no ceiling)
- **Reachability:** Indirect (requires an operator — or an agent holding `CapStanding` — to create/edit an enabled `act_or_ask` order with no `max_trust`)
- **Sanitization:** N/A
- **Framework Protection:** None — this IS the missing safe-default
- **Description:** `standingTrustCeiling` returns no cap for `act_or_ask`/empty mode when `MaxTrust==""`, so the fire path never calls `WithTrustCeiling` and the run executes at full default-allow (L4). The "initiative mode" reads like the safety dial, but its most-permissive setting equals "no trust clamp." If the order also names no `Agent`, there is zero autonomy bound.
- **Verification Notes:** Logic-flaw with a clear control path; fail-open default is the substantive issue. This is the enabling condition that makes VULN-001's escalation reach an uncapped state, and the floor through which VULN-004's injected content can act. Confidence at original 80 (no offsetting mitigation — there is no backstop ceiling). Capped at Medium severity per the original rating; the danger is the compounding chain, recorded in VULN-001/004 cross-refs.
- **Remediation:** Apply a non-`LevelAllow` default ceiling whenever an order has an autonomous mode (`act`/`act_or_ask`) and no explicit `max_trust` (mirror the seeded responder's L2 default) so the fire path fails safe. Surface the effective ceiling in `standing_list`.

---

### VULN-004: Untrusted Pulse-observation content reaches an autonomous run via the intent string — prompt-injection guard (taint-based) does not cover the intent path
- **Severity:** Medium
- **Confidence:** 72/100 (High Probability)
- **Original Skill:** access-control (AC-02)
- **Vulnerability Type:** CWE-1427 / CWE-77 (prompt injection → autonomous action)
- **File:** `kernel/standing/runner.go:134-152` (`TriggeredIntent` embeds trigger payload JSON verbatim); `cmd/agezt/main.go:5229`; `kernel/runtime/runtime.go:1616-1627` (injection guard fires only on `UntrustedObservationTaint`, attached to tool observations); `kernel/pulse/engine.go`
- **Reachability:** Indirect — guarded down by default: the seeded `guardian-initiative` responder ships **disabled** and L2-capped (`plugins/builtinguardians/builtinguardians.go:363`). Becomes live only when an operator/agent enables/edits an `act`-mode order bound to `pulse.initiative.act`.
- **Sanitization:** Partial — the taint-based injection guard wraps/screens *tool observations*, but content entering through the *intent string* carries no taint, so `hasTaint` is false and the guard never engages on this path
- **Framework Protection:** Partial (the existing injection guard exists but structurally misses the intent path)
- **Description:** When a standing order fires from `pulse.initiative.act`, the trigger payload — which for web/external Pulse observers can derive from attacker-controlled scraped/ingested content — is serialized verbatim into the agent's user intent (`b.Write(raw)` inside a fenced JSON block). Directive-like text planted in a source a Pulse observer reads can land in an autonomous run's prompt with the injection guard disengaged. The only backstop on the run is the trust ceiling — which VULN-003 may remove and VULN-001 can escalate away.
- **Verification Notes:** Real gap in guard coverage; the default-disabled + L2-capped seed and the operator-enable precondition pull confidence below AC-01. Original 75 → −3 for the dormant-by-default posture (still reachable once enabled, which operators do, per the initiative product arc). Severity held at Medium (was reported High, but the dormant-by-default + enable-required gating caps real-world standing at Medium under the chain-dependent reachability).
- **Remediation:** Treat the `pulse.initiative.act` trigger payload as untrusted observation: run the fired order's intent through the same injection screening / `UNTRUSTED OBSERVATION` wrapping used for tool output, or attach `UntrustedObservationTaint` to the run context when the intent is built from an external trigger payload.

---

### VULN-005: No per-caller rate limit on expensive run endpoints by default — budget/CPU DoS up to the soft daily ceiling
- **Severity:** Medium
- **Confidence:** 78/100 (High Probability)
- **Original Skill:** api-logic (RATE-001, merged with RATE-003 transcription throttle)
- **Vulnerability Type:** CWE-770 (Allocation of Resources Without Limits or Throttling)
- **File:** `kernel/openaiapi/openaiapi.go:487` (`handleChat`), `:170` (`/v1/responses`), `:188` (`handleTranscription`); `kernel/restapi/restapi.go:382` (`handleRunsRoot`); `kernel/webui/webui.go:645` (`/api/run`), `:644` (`/api/plan/run`); governor default `cmd/agezt/main.go:6382` (`ratePerMin := 0`)
- **Reachability:** Direct, post-auth (a token holder, or anyone reaching a reverse-proxied/tunnelled instance)
- **Sanitization:** N/A (bodies are size-capped; *frequency*/*concurrency* is not)
- **Framework Protection:** Partial — the $20/day global budget ceiling is the only enforced rail at defaults, and it is *soft* (see VULN-006), so concurrent bursts overshoot it
- **Description:** Every run-submitting endpoint maps onto the kernel tool-loop (LLM + `code_exec`/tools = real money + CPU per request). The only frequency control is the governor's `RateLimitPerMin`, which defaults to 0 (unlimited) unless `AGEZT_RATE_PER_MIN` is set; there is no global max-in-flight-runs semaphore (`runWG` tracks for drain only). A token holder can fire unlimited concurrent expensive runs, exhausting the daily budget in seconds and pinning CPU, after which the daemon is denied to legitimate use for the rest of the UTC day. The `/v1/audio/transcriptions` multipart path (RATE-003, `io.ReadAll` of up to 25 MiB) shares the same no-throttle property at lower impact (STT must be configured).
- **Verification Notes:** Confirmed default-unlimited from the cited governor default. Reachability is post-auth only (not an unauth bypass) and the daily ceiling is a genuine backstop — so this stays Medium, not High. RATE-003 merged in as the same root cause (no per-caller throttle) on a secondary endpoint. Confidence 80 original − 2 for the loopback-default audience nuance = 78.
- **Remediation:** Default `AGEZT_RATE_PER_MIN` to a sane non-zero value for the network listeners (or gate the OpenAI/REST surfaces behind a per-token rate limit like agentgw already has); add an optional global max-concurrent-runs semaphore; stream the transcription upload rather than full `ReadAll`. At minimum, document prominently that operators fronting these listeners must set a rate cap.

---

### VULN-006: Daily/task/agent budget ceilings are soft check-then-act pre-checks — concurrent runs overshoot
- **Severity:** Medium
- **Confidence:** 70/100 (High Probability; documented-as-intended)
- **Original Skill:** api-logic (RACE-001, related to the M48 sub-agent spend cap)
- **Vulnerability Type:** CWE-362 (Race Condition / TOCTOU)
- **File:** `kernel/governor/governor.go:602-621` (budget pre-check), `:648-676` (agent cap), `:1614-1643` (`budgetExceeded`/`taskBudgetExceeded`), `:1450-1500` (`recordUsage`); `kernel/runtime/subagent.go:501` (sub-agent spend cap reads journal)
- **Reachability:** Direct (any concurrent run set), amplified by VULN-005 (no frequency cap → larger in-flight set → larger overshoot)
- **Sanitization:** N/A
- **Framework Protection:** Partial — negative-token clamping (`recordUsage:1461-1486`) already prevents a hostile usage response from crediting the ledger
- **Description:** The budget gate is a check-then-act split across two critical sections with the slow provider call in between: N concurrent completions can all read headroom, all proceed, and together exceed the ceiling by up to (N-1) calls' worth. Same applies to per-task, per-agent, and the M48 sub-agent spend cap (which reads the journal; concurrent in-flight spawns aren't yet journaled). The code **explicitly documents this as accepted design** (`governor.go:602-611`, reaffirmed 2026-06).
- **Verification Notes:** The race is real and confidence is high (90 raw) that the overshoot exists — but the team has consciously chosen the soft cap and the bounded overshoot is minor for a $20/day-class cap with bounded per-call cost. Because it is an explicitly-accepted, documented design tradeoff (not an oversight), severity is held at Medium and the *confidence that this is an actionable defect* is recalculated down to 70: it is a "fix only if a hard cap is ever required" item, meaningful mainly as the amplifier of VULN-005.
- **Remediation:** Accept as-is OR, if a hard cap is required, reserve estimated cost under the same lock as the pre-check and reconcile the actual after the call.

---

### VULN-007: Workflow webhook (`/hooks/`) has no rate limit; one leaked secret = unbounded paid runs
- **Severity:** Medium
- **Confidence:** 68/100 (Probable)
- **Original Skill:** api-logic (RATE-002)
- **Vulnerability Type:** CWE-770 / CWE-799 (Improper Control of Interaction Frequency)
- **File:** `kernel/webui/webui.go:730` (`handleWorkflowHook`), `:742` (`?secret=`); `kernel/controlplane/workflow.go:215` (`handleWorkflowWebhook`)
- **Reachability:** Indirect — requires the attacker to learn a per-workflow secret (or an over-shared `?secret=` URL); the endpoint is the one deliberately token-free web path
- **Sanitization:** Auth is sound (empty-secret rejected `workflow.go:221`, constant-time, uniform 403, 256 KiB body cap, no path traversal); the gap is purely the missing throttle
- **Framework Protection:** None for frequency; the per-workflow constant-time secret is the only gate, and it is not rate-limited
- **Description:** `/hooks/<workflow>` has no rate limit. An attacker who learns one workflow secret can POST it in a tight loop, each fire launching a governed agent run with full LLM/tool spend, bounded only by the same soft daily cap as VULN-005. Secret-in-query-string also lands in proxy/access logs and browser history, widening exposure.
- **Verification Notes:** Distinct from VULN-005 (token-free path, secret-scoped reachability) so kept separate rather than merged. Reachability is gated on secret leakage (Indirect, −) and impact is the same daily-cap-bounded spend; confidence 70 raw − 2 for the secret-knowledge precondition = 68. Severity Medium.
- **Remediation:** Add a per-workflow (or per-source-IP) rate limit on `/hooks/`; prefer the header form and discourage/strip `?secret=` from logs; add a per-workflow max-fires-per-minute knob.

---

### VULN-008: Agent/channel HTML artifacts rendered as live scripts in a sandboxed iframe (`srcDoc` + `allow-scripts`)
- **Severity:** Medium
- **Confidence:** 62/100 (Probable)
- **Original Skill:** client (XSS-001)
- **Vulnerability Type:** CWE-79 (XSS) / CWE-1021 (improper restriction of rendered UI layers)
- **File:** `frontend/src/views/Artifacts.tsx:394-402` (sink), data flow `:346,359-361`
- **Reachability:** Indirect — requires a hostile channel/agent to store an HTML artifact AND the operator to open its preview in an authenticated session (localhost-console context)
- **Sanitization:** Partial — sandbox is applied **without `allow-same-origin`** (scripts run in an opaque/null origin, cannot read parent `TOKEN`/cookies/`localStorage`/DOM or issue same-origin credentialed `/api/*` calls)
- **Framework Protection:** Partial (two stacked browser behaviors): page CSP (`default-src 'none'; script-src 'self'`) is applied to the `srcdoc` document by modern engines (blocks inline `<script>`), and the server route `/api/artifact/raw` independently refuses to serve `text/html` (forces `application/octet-stream`, `artifact_route.go:65-76`)
- **Description:** The artifact viewer renders an HTML artifact's bytes with `<iframe srcDoc={text} sandbox="allow-scripts">`. Artifact bytes are attacker-influenceable (channel messages, agent/tool output). The `srcdoc` re-injection bypasses the server-side `text/html`→octet-stream content-type defense. Residual risk even with origin isolation: `allow-scripts` permits arbitrary JS — uncredentialed exfil `fetch`/beacon, convincing in-console phishing UI (fake "session expired, re-enter password" overlay), resource abuse, browser-0-day surface — all from merely viewing a malicious artifact.
- **Verification Notes:** The isolation is genuinely strong (no `allow-same-origin`, CSP-on-srcdoc, server content-type downgrade) — three independent layers — which is why this is Medium/Probable and not High. The residual is real but rests on browser behaviors (CSP-on-srcdoc is not uniform across engine versions) and an operator-interaction precondition. Original confidence 70; configuration mitigation (CSP) −10 → adjusted up to 62 after weighing that the CSP layer is non-uniform (partial, not full). Severity held Medium.
- **Remediation:** (a) Drop `allow-scripts` for the HTML preview and render sanitized static HTML, or route HTML artifacts through the safe `Markdown`/text path; (b) if live HTML must run, add an explicit per-frame `csp` attribute / `Content-Security-Policy: sandbox; default-src 'none'` and keep `allow-scripts` behind an explicit "run scripts" operator click; (c) set `referrerpolicy="no-referrer"`; keep `allow-popups`/`allow-top-navigation` absent.

---

### VULN-009: CI runs on persistent (non-ephemeral) self-hosted runners — fork-PR safety depends on a single `if:` gate with no second layer
- **Severity:** Medium
- **Confidence:** 60/100 (Probable)
- **Original Skill:** infra (CICD-001)
- **Vulnerability Type:** CWE-693 (Protection Mechanism Failure) / CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)
- **File:** `.github/workflows/ci.yml:32` (gate replicated on all 14 jobs: lines 33,50,69,87,112,138,157,187,204,225,251,284,301,379); `runs-on: [self-hosted, Linux, X64]`
- **Reachability:** Latent — NOT reachable by an external contributor today: the `if: github.event_name == 'push' || head.repo.full_name == github.repository` gate is present and correct on every job, so fork-PR jobs are skipped. Materializes only if the gate is removed/edited or repo Actions settings ever permit fork PRs on self-hosted runners.
- **Sanitization:** N/A
- **Framework Protection:** Partial — the gate is the correct GitHub-recommended pattern, but it is a single YAML control with no backstop; runners are persistent and reused across runs (`~/go/bin`, `/dev/shm/goroot-*`, `RUNNER_TOOL_CACHE` writable by job code → state-bleed/cache-poisoning if ever breached)
- **Description:** All 14 CI jobs run on three persistent WSL runners sharing one VM. The only thing preventing fork PR code from executing on those long-lived machines is the per-job `if:` gate. If it ever fails open (edited away, or repo settings relaxed), the result is arbitrary code execution on the owner's daily-driver VM with state-bleed into subsequent trusted builds (supply-chain poisoning of release binaries).
- **Verification Notes:** This is explicitly a defense-in-depth / blast-radius finding, not a live exploit (the reporter confirms fork code does not reach the runner today). Original confidence 80, but because the risk is latent (no current reachability), the *confidence that it is an actionable live defect* is recalculated to 60 — Probable, Medium severity (the blast radius is severe enough to keep it Medium rather than Low). Project memory corroborates `main` has at times been unprotected with no required checks, which raises the realistic probability of the gate being silently dropped.
- **Remediation:** Set repo/org Actions → "Require approval for all external collaborators" and never offer self-hosted runners to public-fork PRs (the authoritative control, second layer to the YAML gate); add a label/environment approval gate for self-hosted PR jobs; move toward ephemeral runners (or a pre-job wipe of `~/go/bin`, `/dev/shm/goroot-*`, `$RUNNER_TOOL_CACHE/go`); protect `.github/` so the gate can't be silently dropped (see VULN-013).

---

### VULN-010: `undici` security override (`^7.28.0`) is not enforced in the resolved lock tree
- **Severity:** Low
- **Confidence:** 65/100 (Probable)
- **Original Skill:** dependency-audit (DEP-002, related DEP-001 dual-lockfile)
- **Vulnerability Type:** CWE-1104 (Use of Unmaintained/Unpinned Third-Party Components) / supply-chain integrity
- **File:** `frontend/package.json` (`overrides.undici: ^7.28.0`); `frontend/pnpm-lock.yaml` (resolved `undici@7.27.2`, no `overrides:` block); plus the dual-lockfile divergence (`pnpm-lock.yaml` + `package-lock.json` both committed for the same `package.json`)
- **Reachability:** Indirect / build-time only — `undici` is a build/dev-tree transitive; the frontend ships as a static `go:embed`-ded `dist/` bundle and Node never runs at AGEZT runtime, so runtime exposure is low
- **Sanitization:** N/A
- **Framework Protection:** None — the lock predates the override, so the security-motivated pin is silently ineffective
- **Description:** `frontend/package.json` declares an `undici ^7.28.0` override (undici <7.x had SSRF/redirect CVEs), but `pnpm-lock.yaml` resolved `undici@7.27.2` and contains no `overrides:` block — the intended pin is not actually applied in the installed tree. Compounded by DEP-001: both `pnpm-lock.yaml` and `package-lock.json` are committed for the same manifest (two diverging sources of truth; devs/CI using different package managers resolve different trees).
- **Verification Notes:** Heuristic manifest/lock analysis (no live advisory call), and the dep is dev/build-only with no runtime path — so kept Low. Confidence 80 raw that the pin is ineffective − 15 for build-only/no-runtime exposure context = 65. DEP-001 merged as the same lock-integrity root cause.
- **Remediation:** Run `pnpm install` to re-resolve so the override applies, commit the refreshed lock; delete whichever lockfile is not the source of truth (keep pnpm). Verify `lucide-react@1.x` provenance (DEP-005) while touching the lock.

---

### VULN-011: Unbounded `io.ReadAll` on response body in `retry.ReadBody` (latent memory-exhaustion DoS)
- **Severity:** Low
- **Confidence:** 48/100 (Possible)
- **Original Skill:** lang-go (related Low: ReadBody discards read error)
- **Vulnerability Type:** CWE-770 (Allocation of Resources Without Limits or Throttling)
- **File:** `plugins/providers/internal/retry/retry.go:259` (error path, also discards err with `_`), `:262` (success path)
- **Reachability:** Unknown/latent — a grep of current provider call paths shows live providers route through the bounded `httpread.All`, not `ReadBody`; this is an edge helper with no observed hot-path caller today
- **Sanitization:** None (no `io.LimitReader` wrapping, unlike every other provider response read in the tree)
- **Framework Protection:** N/A
- **Description:** `ReadBody` reads an entire HTTP response into memory with no cap on both error and success paths. A malicious/compromised upstream provider endpoint (operator-configured, semi-trusted) returning a multi-GB body would OOM the daemon — but only if a future caller wires `ReadBody` onto an attacker-influenceable endpoint. Secondary Low: the read error on the non-2xx path is discarded with `_`, yielding a silently-partial `HTTPError.Body`.
- **Verification Notes:** Unbounded read confirmed (High confidence on the pattern), but reachability is the discriminator: no current hot-path caller, and the trigger requires a semi-trusted operator-configured endpoint. Reachability −20 (no clear call path today) takes confidence from ~70 to 48 → Possible, capped at Low (matching the original Low rating for a latent helper).
- **Remediation:** Wrap both reads in `io.LimitReader(resp.Body, httpread.DefaultMaxResponseBytes+1)` (or call `httpread.All`) and surface a "response too large" error; stop discarding the read error.

---

### VULN-012: OpenAI-compatible API echoes raw upstream provider error text to the authenticated client
- **Severity:** Low
- **Confidence:** 42/100 (Possible)
- **Original Skill:** secrets-crypto (EXPOSE-002)
- **Vulnerability Type:** CWE-209 (Generation of Error Message Containing Sensitive Information)
- **File:** `kernel/openaiapi/openaiapi.go:534` (`upstream_error`, `err.Error()`), `:726` (stream `[error: …]`); `kernel/openaiapi/responses.go:92,313`
- **Reachability:** Indirect — endpoint is off by default, loopback by default, bearer-token-gated; the audience is the single operator who already holds the daemon token and the provider keys
- **Sanitization:** None — these HTTP response bodies are not passed through `kernel/redact` (redact applies to journal/bus payloads, not live HTTP responses)
- **Framework Protection:** None for the live-response path
- **Description:** `upstream_error` / `stt_error` responses return the raw upstream provider error string to the caller. A pathological upstream error echoing a request fragment could surface internal detail. Real exploitability requires an attacker who is already an authenticated operator, and provider errors do not normally contain the API key.
- **Verification Notes:** Genuine info-leak surface but the trust model nearly neutralizes it — the recipient is the operator themselves. Original confidence 55; the off-by-default + loopback + already-privileged-audience configuration mitigation (−~13) lands it at 42 → Possible, Low. Defense-in-depth only.
- **Remediation:** Run upstream/STT error strings through the daemon redactor before placing them in the HTTP body, or return a generic `upstream_error` and log the detail to the (already-redacted) journal.

---

### VULN-013: No `CODEOWNERS` protecting `.github/` — workflow-trust gate has no change-review backstop
- **Severity:** Low
- **Confidence:** 40/100 (Possible)
- **Original Skill:** infra (CICD-002)
- **Vulnerability Type:** CWE-693 (Protection Mechanism Failure)
- **File:** repo-wide — no `CODEOWNERS` file exists (`.github/CODEOWNERS` and `**/CODEOWNERS` → 0 matches)
- **Reachability:** Latent — not an exploit itself; the absence of a guardrail that would catch a regression of VULN-009 or an introduced expression-injection
- **Sanitization:** N/A
- **Framework Protection:** None in-repo (branch-protection rules are server-side and could exist without an in-repo artifact)
- **Description:** The fork-PR safety of VULN-009, the SHA pins, the `permissions: contents: read` cap, and `persist-credentials: false` all live in `.github/`. There is no `CODEOWNERS` requiring a trusted reviewer for workflow/action changes. A future PR (or a self-merge, given memory notes `main` has at times been unprotected) could weaken any of these controls and merge without a forced second pair of eyes on the most security-sensitive directory.
- **Verification Notes:** Concrete, verifiable absence (no CODEOWNERS anywhere), but it is the lack of a backstop, not a live hole — and branch protection could exist server-side. Confidence held at the reported 40 → Possible, Low. This is the cheapest durable backstop for the whole infra section.
- **Remediation:** Add `.github/CODEOWNERS` requiring a trusted owner to review `/.github/**`; enable branch protection on `main` with "Require review from Code Owners" + required status checks.

---

## Eliminated Findings (Informational / Verified-Safe / False Positives)

These were reviewed and **not** carried forward as actionable findings — each is either already mitigated, a defense-in-depth confirmation, out-of-trust-model, or a deliberate documented design choice.

**Access control**
- **AC-04 (Roster `System` flag not re-asserted at store `Update`)** — Low, confidence 60. No working bypass exists today: control-plane add forces `System=false`, the edit mutable-field whitelist omits it, the overseer tool refuses to edit System guardians, and they can't be hard-deleted. Pure defense-in-depth gap (a future caller wiring `dst.System=in.System` would defeat it) — recommend the one-line `p.System = snapshot.System` assertion, but not a current vulnerability. *Eliminated: no reachable bypass.*

**Client-side**
- **XSS-002 (external-doc links not through `safeHref`)** — Low, confidence 40. `docs_url`/`docs` come from server-side channel/ACP catalog config, not free-form channel-message content; exploitation requires control of the catalog. Consistency fix only (apply the existing `safeHref` primitive). *Eliminated: source not attacker-controlled; low confidence.*
- **XSS-003 (`window.open(authorize_url)` without scheme validation)** — Low, confidence 35. `authorize_url` is produced server-side from OAuth config (not user-message-controlled); `noopener,noreferrer` correctly set. *Eliminated: source not attacker-controlled; low confidence.*

**API / logic**
- **API-001 (`/metrics` gated by `s.auth` not `adminAuth`)** — Low, confidence 70. Correctly token-gated (not unauth); exposes spend/activity gauges to a same-tier token holder. Hardening nit (gate behind `adminAuth`), no auth bypass. *Eliminated to info: correctly authenticated; tier-tightening hygiene.*
- **API-002 (self-update bounded only by admin token if Ed25519 key not activated)** — Low, confidence 50. Correctly `adminAuth`-gated; the residual is contingent on `DefaultPublicKeyHex` being unset in a build — a release-engineering activation step, not a code defect. Merged with the cross-scanner self-update note (ssrf "Notable change": the download path is already netguard-wrapped on `main`; dependency-audit DEP-008 confirms update modules are SHA256+Ed25519-verified). *Eliminated to release-eng action item: confirm `DefaultPublicKeyHex` is set in release builds.*

**Lang-go (Low/Info)**
- **context.Background on long-lived daemon goroutines / plugin callback** (`runtime.go:892`, `roster.go runAgentWake`, `plugin/host.go:918`) — Low. Leaning by-design (a wake *should* outlive its triggering request); the plugin callback has a bounded `InvokeTimeout`, so bounded resource use, not a leak. *Eliminated to hygiene: shutdown-gracefulness, not security.*
- **`unsafe.Pointer` confined to OS-syscall bridges; randomness posture; slow-loris timeouts + body caps** — Info, verified-clean. *Eliminated: confirmed safe.*

**Lang-ts (Low/Info)**
- **TS-001 (SDKs are HTTP-only, no TLS)** — Low, confidence 80. Intentional zero-dependency design tied to the loopback model; the Rust client hard-rejects `https://`. Documentation ask only (READMEs must state non-loopback use requires an HTTPS reverse proxy). *Eliminated to doc action: by-design for loopback; no code change.*
- **TS-002 (broad `as any` / `as unknown as T` casts)** — Low, confidence 55. UI-internal conveniences; data sinks are React text (escaped) or already runtime-guarded (`typeof` in `agentrepair.ts`). No concrete unsafe flow. *Eliminated to hygiene: no exploitable type-confusion sink.*

**Dependency-audit (remaining DEP items)**
- **DEP-003/004 (bleeding-edge dev majors; `@types/node` skew), DEP-005 (`lucide-react@1.x` provenance), DEP-006 (`go-imap` beta.8), DEP-007 (`cpuid v2.0.9` outdated), DEP-008 (graph-only x/* not compiled), DEP-009 (Go 1.26.4 toolchain)** — Low/Info. All dev/build-only or graph-only (not compiled into AGEZT binaries), or routine version-hygiene. DEP-005 (`lucide-react@1.x` typosquat check) folded into VULN-010's remediation. DEP-006 (`go-imap` beta is a real compiled IMAP/MIME parser surface) flagged to "treat untrusted mail defensively" but no specific defect identified. *Eliminated to maintenance backlog: no confirmed advisory; verify with live `govulncheck`/`osv-scanner`.*

**Whole-domain verified-safe (no findings)**
- **Injection (SQLi/NoSQLi/SSTI/XXE/LDAP/CRLF)** — INJ-INFO-01..05. No DB/ORM/template-engine/LDAP; the two real header sinks (email Subject, artifact `Content-Disposition` filename) strip CRLF; Go stdlib backstops (`net/http` header validation, `net/smtp` address validation, `encoding/xml` no-entity-resolution). *Eliminated: structurally inhospitable + sinks defended.*
- **Code-execution (cmdi/RCE/sandbox-escape/deserialization)** — INFO-1..5. Single warden choke point, array-form `exec.Command` everywhere, env-scrub before every child, slug-only agent selectors, edict-gated spawns; no gob/YAML/`plugin.Open`/`interface{}` type-confusion; JWT alg/typ/iss/aud-pinned + constant-time. The Windows `cmd /S /C` verbatim builder reviewed safe in current call paths. *Eliminated: intended capabilities, correct trust boundaries.*
- **SSRF / path-traversal / file-upload** — INFO-1 (STT/TTS not netguard-wrapped: operator-config destination, not agent-reachable), INFO-2 (`file` tool `..` prefix check is correct here). netguard validates resolved IP on initial dial + every redirect hop; `file` tool Abs+EvalSymlinks confinement; zip-slip-safe backup restore; content-addressed artifacts. Self-update download now netguard-wrapped on `main`. *Eliminated: strongly hardened, info-only notes.*
- **Secrets / crypto** — SECRET-001, CRYPTO-001..004, EXPOSE-001. `.env` correctly gitignored; vault AES-256-GCM with fresh CSPRNG salt+nonce + PBKDF2-200k; all comparisons constant-time; SHA-1/MD5/`math/rand` usages all non-security; zero `InsecureSkipVerify`; redaction wired into bus/journal/plugin-log. `gitleaks.json` = `[]`. *Eliminated: posture confirmed strong.*
- **Verified-clean access-control controls** — control-plane primary/tenant token, web UI auth/session, REST/OpenAI bearer + tenant IDOR-bound engine, agentgw HMAC-JWT (alg/iss/aud pinned, cap-subset, per-install CSPRNG secret), HITL approval fail-closed. *Eliminated: correctly enforced.*
- **Mass-assignment, sub-agent fan-out bounds, GraphQL/gRPC absence** (api-logic INFO-001/002) — explicit field allowlists + double identity-reset defense; fan-out depth/tree/spend bounded; no GraphQL/gRPC surface. *Eliminated: verified sound / not present.*
- **Docker & IaC** (infra) — no Dockerfile/compose, no Terraform/k8s/helm/CFN. *Eliminated: no scanner surface.*

---

## Verifier note
This codebase is genuinely well-hardened: the overwhelming majority of raw items are defense-in-depth confirmations or out-of-trust-model nits, and the report says so plainly rather than inflating them. The three substantive items the operator should prioritize are **VULN-001** (trust-ceiling last-write-wins delegation escape — a real privilege-escalation logic flaw, the only High), **VULN-002** (channel-registry boot-window data race — a remote-triggerable hard crash), and the **VULN-005/006/007** budget-DoS cluster (no default per-caller rate limit + soft, overshootable spend ceilings). VULN-001's impact is compounded by VULN-003 (uncapped autonomous default) and VULN-004 (injected content reaching an autonomous run), which together form the one chain worth fixing as a unit.

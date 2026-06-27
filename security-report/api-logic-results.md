# API Security & Logic — Findings

Domain: REST/API security, rate limiting & DoS, business-logic flaws, race conditions/TOCTOU, mass assignment, GraphQL.
Repo: `D:\Codebox\PROJECTS\AGEZT`. Scanner: sc-api-security / sc-rate-limiting / sc-business-logic / sc-race-condition / sc-mass-assignment / sc-graphql.

## Summary

This surface is **unusually well-hardened**. Every API listener is token-authed and body-capped; mass-assignment is blocked by explicit field allowlists; the documented agentgw token-mint hole is closed (caps/expiry/rate all clamped to parent); budget overshoot, sub-agent fan-out, and the soft daily cap are all *deliberate, commented* design tradeoffs rather than oversights. **GraphQL: not present** — confirmed no schema/resolver/`graphql` endpoint anywhere (architecture §8 "No `/graphql`"; grep clean). gRPC: not present (stdlib `net/http` only).

The findings below are the residual items. The most serious is the **absence of any per-caller rate limit on the expensive LLM/run endpoints by default** — a token holder (or anyone who reaches a loopback-bound, reverse-proxied instance) can drive spend/CPU up to the $20/day global ceiling with no frequency throttle. This is Medium because the daily budget is a real backstop and the listeners are off-by-default + loopback + token-gated.

Severity counts: **Critical 0 · High 0 · Medium 3 · Low 3 · Informational 2**

---

## MEDIUM

### RATE-001 — No per-request rate limit on expensive run endpoints by default (Medium, CWE-770, confidence 80)
**Files:** `kernel/openaiapi/openaiapi.go:487` (`handleChat`), `:170` (`/v1/responses`); `kernel/restapi/restapi.go:382` (`handleRunsRoot`); `kernel/webui/webui.go:645` (`/api/run`), `:644` (`/api/plan/run`); governor default `cmd/agezt/main.go:6382` (`ratePerMin := 0`).

Every run-submitting endpoint (`POST /v1/chat/completions`, `POST /v1/responses`, `POST /api/v1/runs`, web `/api/run`, `/api/plan/run`) maps onto the kernel tool-loop, which calls the LLM and can invoke `code_exec`/tools — real money + CPU per request. The only frequency control is the governor's `RateLimitPerMin`, which **defaults to 0 (unlimited)** unless the operator sets `AGEZT_RATE_PER_MIN`. The only enforced rail at default settings is the **$20/day global budget ceiling** (`governor.DefaultDailyCeilingMicrocents`), and that ceiling is a *soft* cap (see RACE-001) so concurrent bursts can overshoot it. Bodies are size-capped (16 MiB OpenAI/REST, 1 MiB web) but nothing bounds request *frequency* or *concurrency*: there is no global max-in-flight-runs limit in `kernel/runtime` (`RunWith`/`RunModel` start a run unconditionally; `runWG` only tracks for drain-on-halt, it is not a semaphore).

**Impact:** A holder of the daemon token — or any client reachable if the operator front-ends the loopback listener with a reverse proxy/tunnel (a documented deployment) — can fire unlimited concurrent expensive runs, exhausting the daily budget in seconds and pinning CPU via parallel `code_exec`/tool fan-out, until the daily ceiling trips. After the ceiling trips the daemon is effectively denied to legitimate use for the rest of the UTC day.
**Reachable/authenticated:** Yes, post-auth. Not an unauth bypass.
**Remediation:** Default `AGEZT_RATE_PER_MIN` to a sane non-zero value for the network listeners (or gate the OpenAI/REST surfaces behind a separate per-token rate limit like agentgw already has), and add an optional global max-concurrent-runs semaphore. At minimum, document prominently that operators fronting these listeners must set a rate cap. The infra-level mitigation (gateway/WAF rate limit) is a legitimate but unstated dependency.

### RATE-002 — Workflow webhook (`/hooks/`) has no rate limit; one known secret = unbounded paid runs (Medium, CWE-770/CWE-799, confidence 70)
**Files:** `kernel/webui/webui.go:730` (`handleWorkflowHook`); `kernel/controlplane/workflow.go:215` (`handleWorkflowWebhook`).

`/hooks/<workflow>` is the **one deliberately token-free** web path; auth is a per-workflow constant-time secret. The auth itself is sound (empty-secret rejected `workflow.go:221`, uniform 403 refusals, 256 KiB body cap, no path traversal). But there is **no rate limit on the endpoint**: an attacker who learns a single workflow secret (or an over-shared webhook URL containing `?secret=`) can POST it in a tight loop, each fire launching a governed agent run with full LLM/tool spend. Secret-in-query-string (`:742`) also lands in proxy/access logs and browser history, widening exposure.

**Impact:** Spend/CPU exhaustion up to the daily ceiling triggerable by anyone with one leaked workflow secret, with no per-secret throttle. Bounded only by the same soft daily cap as RATE-001.
**Remediation:** Add a per-workflow (or per-source-IP) rate limit on `/hooks/`. Prefer the header form and discourage `?secret=` (or strip it from logs). Consider a per-workflow max-fires-per-minute knob.

### RACE-001 — Daily/task/agent budget ceilings are soft pre-checks; concurrent runs overshoot (Medium, CWE-362, confidence 90 — but documented as intended)
**Files:** `kernel/governor/governor.go:602-621` (`preflightAndRoute` budget pre-check), `:648-676` (agent cap), `:1614-1643` (`budgetExceeded`/`taskBudgetExceeded`), `:1450-1500` (`recordUsage`).

The budget gate is a check-then-act split across two critical sections with the (slow) provider call in between: N concurrent completions can all read headroom, all proceed, and together exceed the ceiling by up to (N-1) calls' worth. The same applies to per-task (`taskBudgetExceeded`) and per-agent (`spentByAgentToday`) caps, and to the M48 sub-agent spend cap (`runtime/subagent.go:501`, which reads the journal — concurrent in-flight spawns aren't yet journaled). The code **explicitly documents this as the accepted design** (`governor.go:602-611`, reaffirmed 2026-06): a hard cap would need pessimistic pre-call estimation and could reject valid near-ceiling calls.

**Impact:** Bounded overshoot of any spend ceiling by the in-flight set. For a $20/day-class cap with bounded per-call cost this is minor; it becomes more meaningful combined with RATE-001 (no frequency cap → larger in-flight set → larger overshoot). Negative-token clamping (`recordUsage:1461-1486`) already prevents a hostile usage response from crediting the ledger (good).
**Remediation:** Accept as-is OR, if a hard cap is ever required, reserve estimated cost under the same lock as the pre-check and reconcile the actual after the call. Flagged for completeness; the team has consciously chosen the soft cap.

---

## LOW

### API-001 — `/metrics` exposes spend/activity volume to any token holder (Low, CWE-200, confidence 70)
**File:** `kernel/restapi/restapi.go:178`, `:238` (`handleMetrics`).
`/metrics` is token-gated (correctly, not unauth), but it's gated by the *same* daemon/tenant `s.auth` as ordinary read routes, not the stricter `adminAuth`. It exposes financial/operational gauges (spend, active runs). In a multi-tenant deployment a per-tenant token reaches it. Comment at `:175` acknowledges the sensitivity. **Remediation:** Gate `/metrics` behind `adminAuth` (like mailbox/update), or scope per-tenant.

### API-002 — Self-update endpoints code-mutating but bounded only by admin token (Low, CWE-306-adjacent, confidence 50)
**File:** `kernel/restapi/restapi.go:196-197` (`/api/v1/update`, `/api/v1/update/apply`).
Correctly `adminAuth`-gated and per architecture §9 Ed25519 update signature verification "shipped but may require `DefaultPublicKeyHex` activation." If the signing key isn't activated in a given build, `/api/v1/update/apply` stages host-mutating code behind only the bearer token. **Remediation:** Confirm `DefaultPublicKeyHex` is set in release builds so update payloads are signature-verified, not token-only. (Cross-ref the supply-chain/auth scanners.)

### RATE-003 — `/v1/audio/transcriptions` multipart has no per-caller throttle (Low, CWE-770, confidence 55)
**File:** `kernel/openaiapi/openaiapi.go:188` (`handleTranscription`).
Body capped at 25 MiB (`audioMaxBytes`) and reads the whole upload into memory (`io.ReadAll`, `:210`) before handing to the STT backend. No frequency cap; repeated 25 MiB uploads cost memory + paid STT calls. Lower impact than RATE-001 (STT must be configured). **Remediation:** Same per-caller rate cap as RATE-001; consider streaming the upload rather than full `ReadAll`.

---

## INFORMATIONAL (verified-safe — documented for the report)

### INFO-001 — Mass assignment is robustly defended
- `handleAgentAdd` forces `p.System = false` (`controlplane/roster.go:1149`, M961) — kernel-owned guardian flag can't be spoofed.
- `handleAgentEdit` applies an **explicit field allowlist** (`applyAgentMutableProfileFields`, `:1212`) that omits `System`, `ID`, `Slug`, `Enabled`, `Retired`, timestamps; the roster store *also* resets identity/lifecycle fields from a pre-mutation snapshot after the mutator runs (`roster/roster.go:851-852`). Double defense.
- Web write surfaces funnel through `decodeAllowedBody` (`webui/webui.go:1430`) and per-route arg allowlists (`writeRoutes`/`jsonRoutes`), stripping every unknown field before it reaches a control-plane handler.
- REST/OpenAI run requests bind to narrow DTOs (`runRequest`, `chatRequest`) with explicit fields, not arbitrary structs.
No mass-assignment path to a protected field (System/owner-as-escalation/trust-ceiling-bypass) was found. `TrustCeiling`/`ToolAllow`/`OwnerAgent` are operator-editable *by design* under the admin token.

### INFO-002 — Sub-agent fan-out, agentgw, control-plane, and webhook auth verified sound
- **Sub-agent fan-out** (`runtime/subagent.go`): bounded by depth (default 3), tree-total (default 48 when depth>1), mutex-guarded per-correlation fan-out tally, and optional spend cap — all enforced at spawn time identically for sync/async. Recursion can't run away.
- **agentgw token mint** (`gateway.go:353-438`): caps subset-checked (`CapsSubset`, reject-not-drop), expiry clamped to parent, rate/burst clamped, RunID inherited — the documented escalation hole is closed. Per-token sliding-window rate limit with bounded 4096-entry table + idle eviction (CWE-770 bounded).
- **Control plane** (`server.go:444-504`): per-connection panic recovery (no single request DoSes the daemon), bounded pre-auth read (`readBoundedLine`, M188 — no OOM), constant-time token compare, tenant command allowlist (`tenantTokenAllows`) + tenant-arg pinning.
- **Workflow webhook**: constant-time secret, empty-secret rejected, uniform refusals (no name/secret/disabled oracle), body cap, no path traversal. (Only gap = rate limit, see RATE-002.)
- **SSRF/CSRF/CSP/host-allowlist**: out of this domain but observed intact (netguard dialer guard, `sameOriginMutation`, locked CSP) — no CORS allow-all.
- **GraphQL/gRPC**: absent. No applicable findings.

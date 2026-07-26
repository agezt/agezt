# AGEZT Autonomous System Refactor Plan

> Baseline: `main` at 2026-07-26. This is an implementation plan, not a feature
> wish list. Every phase preserves the agent-loop invariants in
> `AGENT-LOOP-INVARIANTS.md` and ends at an executable validation gate.

## Outcome

AGEZT should behave as one governed autonomous system:

`observe → decide → plan → authorize → act → verify → learn → sleep/wake`

Every transition must be journaled, attributable to a durable agent identity,
bounded by policy/budget/time, recoverable after interruption, and visible in
the WebUI without requiring the operator to understand internal packages.

## Verified baseline

- Go daemon/kernel, CLI, plugin providers/tools/channels, four SDK surfaces,
  embedded React WebUI, typed schedules, workflows, durable roster agents,
  delegation, memory/world model, policy, journal, recovery, and operator
  intervention are implemented.
- The source has strong domain packages but excessive composition in
  `cmd/agezt/main.go`, `kernel/runtime`, and `kernel/controlplane`.
- The WebUI has broad capability coverage and a production-daemon Playwright
  smoke flow, but its breadth creates navigation and task-discovery pressure.
- On 2026-07-26, split Go tests, 1,443 Vitest tests, the production frontend
  build, and the embedded-daemon Playwright flow passed.
- Dependency drift was real: dual npm/pnpm lockfiles produced a partial install
  that passed unit tests but failed the production build.

## Non-negotiable architecture boundaries

1. Agents are durable identities; chats, schedules, and workflows are not agents.
2. Schedules and standing orders are typed wake triggers, never hidden prompts.
3. Policy decisions precede effects; denied effects still produce correlated
   results.
4. A run has one correlation chain across parent, delegated, retry, recovery,
   approval, and final verification events.
5. Autonomous actions are bounded by explicit authority, spend, iterations,
   wall time, concurrency, and blast radius.
6. Memory writes need provenance, confidence, expiry/decay, and a reversible
   correction path.
7. WebUI state is derived from backend evidence. UI-only “healthy”, “done”, or
   “autonomous” claims are forbidden.

## Target layers

| Layer | Owns | Must not own |
|---|---|---|
| `kernel/auth` | credentials, authority tiers, tenant authorization contracts | HTTP routing, OAuth UI |
| `kernel/httpserver` | listeners, route policy, body/time limits, auth middleware | domain decisions |
| domain packages | journal, roster, memory, board, schedules, workflows, policy | HTTP envelopes |
| `kernel/runtime` | loop orchestration, retry/recovery, context, delegation | domain-specific tool implementations |
| `cmd/agezt` | configuration composition and process lifecycle | business logic |
| WebUI features | task-oriented operator workflows and live evidence | authoritative state |

## Delivery phases

### Phase 0 — Reproducible and secure baseline

Status: completed in the 2026-07-26 slice.

- Make npm the only frontend package manager and `package-lock.json` the only
  dependency source of truth.
- Patch audited frontend dependencies and require `npm ci`, `npm audit`,
  Vitest, production build, and embedded-daemon Playwright.
- Upgrade directly compiled security-sensitive dependencies and run
  call-graph-aware Go vulnerability scanning.
- Keep generated `kernel/webui/dist` synchronized only after a successful build.

Exit gate: zero npm audit findings, zero called Go vulnerabilities, all build and
browser gates green.

### Phase 1 — Shared auth and HTTP transport

Status: in progress. `kernel/auth`, shared route/body-limit middleware, and the
OpenAI, native REST, and WebUI router migrations are implemented. The WebUI
keeps its shell/session/EventSource credential semantics behind a request-aware
auth adapter while route tiers and body caps live in the shared registry.
The shared streaming-safe server lifecycle is also implemented; address/TLS
binding policy and the remaining route metadata still remain.

1. Add `kernel/httpserver` route metadata:
   `{method, path, tier, body_max, timeout, mutation}`.
2. Migrate OpenAI API, native REST, then WebUI routes without changing wire
   contracts.
3. Move tenant and OAuth domain decisions behind `kernel/auth` interfaces.
4. Migrate agent gateway credentials separately because they are capability
   JWTs, not daemon bearer tokens.
5. Add cross-surface contract tests for empty token, wrong tier, tenant scope,
   body cap, cancellation, and security headers.

Exit gate: no surface implements its own token comparison; route policy is
inspectable from one registry.

### Phase 2 — Decompose control plane, runtime, and bootstrap

1. Move list/fold/pagination logic from `kernel/controlplane` into its owning
   domain package; keep control plane as decode/call/encode glue.
2. Move domain tool implementations out of `kernel/runtime`; runtime retains
   loop, context, delegation, retry, recovery, and lifecycle.
3. Replace the daemon's monolithic bootstrap with explicit service constructors
   and ordered start/drain hooks.
4. Introduce narrow interfaces at runtime-to-workflow, runtime-to-memory, and
   runtime-to-delegation boundaries.

Exit gate: package dependency graph has no domain importing a transport, daemon
startup order is tested, and cancellation drains every resident.

### Phase 3 — Close the autonomous control loop

1. Persist a run objective, success criteria, constraints, and verification
   state independently of provider context.
2. Require a verifier result before autonomous completion; “model said done” is
   not a terminal condition.
3. Make recovery choose among retry, re-plan, alternate provider/tool,
   escalation, pause, and abort using typed failure classes.
4. Record decision evidence: observations used, alternatives rejected, policy
   result, estimated/actual cost, and post-action verification.
5. Evaluate memory candidates before write; attach provenance/confidence/TTL and
   measure whether later retrieval improved outcomes.
6. Add autonomy evaluations for interruption/resume, duplicate delivery,
   partial tool failure, stale world facts, budget exhaustion, and delegated
   child failure.

Exit gate: deterministic scenarios prove that the system can finish, recover,
escalate, or stop safely without an operator babysitting the happy path.

### Phase 4 — WebUI as an operator cockpit

Status: in progress. The 67 stable views are grouped into eight operator jobs
(Talk, Observe, Automate, Govern, Knowledge, Connect, Build, Admin), with at
most ten choices per group; route ids, hashes, help links, and command actions
remain compatible. Global run state is now daemon-seeded before SSE folding and
shared by the header, navigation badges, Now strip, and Activity view, including
reconciliation after reconnect. High-impact confirmations now support explicit
target, impact, and recovery facts; agent retirement and checkpoint rollback
use that contract.

1. Organize navigation by operator jobs: Talk, Observe, Automate, Govern,
   Knowledge, Connect, Build, Admin. Hide expert surfaces behind progressive
   disclosure rather than adding more top-level destinations.
2. Keep a global live-run context across views: objective, current step,
   parent/child topology, elapsed time, spend, policy state, and next safe action.
3. Standardize all pages on loading, empty, stale, partial, denied, retrying,
   failed, and disconnected states.
4. Make destructive or high-impact actions show target, blast radius,
   reversibility, policy decision, and confirmation.
5. Add command-palette actions and contextual deep links from every alert,
   approval, agent, schedule, and run event.
6. Expand Playwright coverage to login/setup, chat/tool trace, agent wake,
   schedule creation/fire, approval, failure/recovery, file flow, and a 390px
   mobile route. Assert accessibility names, focus order, no horizontal
   overflow, no console errors, and backend evidence.

Exit gate: the most common operator tasks need no CLI fallback, and every
autonomous decision can be followed from trigger to verified outcome.

### Phase 5 — Scale, operations, and release

1. Cursor-paginate every unbounded event/log/list surface.
2. Add backpressure and per-tenant quotas for streams, delegates, channels, and
   batch work.
3. Export OpenTelemetry traces/metrics with correlation and tenant-safe labels.
4. Add fault-injection and soak tests for restart, disk full, slow subscriber,
   provider outage, duplicate webhook, and clock skew.
5. Version event and HTTP contracts; generate SDK conformance tests from them.
6. Define SLOs for run acceptance, first action, completion, recovery, SSE
   freshness, and audit durability.

Exit gate: release check is reproducible on a clean machine and operational
failure modes have a tested runbook plus WebUI evidence.

## Continuous quality gates

```powershell
go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen
go vet ./...
go test ./...
go run ./tools/depscheck
go run ./tools/sdkparity -check docs/SDK-PARITY.md
go run ./tools/deadcodecheck
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
Set-Location frontend
npm ci
npm audit
npm test
npm run build
Set-Location ..
.\scripts\webui-e2e.ps1
```

Green unit tests alone are insufficient for WebUI or autonomy claims. A phase
is complete only when its real transport/browser path and failure path are
verified.

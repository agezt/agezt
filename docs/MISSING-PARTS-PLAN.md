# AGEZT Missing Parts Implementation Plan

Generated: 2026-06-21  
Last updated: 2026-06-22  
Input: `docs/MISSING-PARTS-REPORT.md`  
Purpose: decide what to do, what not to do yet, and how to execute the remaining work in scoped, verifiable slices.

## Decision summary

Not every gap in `docs/MISSING-PARTS-REPORT.md` should become immediate code work. Some are documentation reconciliation, some are release hygiene, some are deliberate out-of-scope/security caveats, and a few require owner sign-off before implementation.

### Current open work

The implementation/proof/documentation mismatches from the original report are reconciled or documented as regression bars. Current open work is release hygiene:

1. Resolve or explicitly scope the dirty worktree before commits.
2. Commit validated test files in scoped slices or explicitly discard them by owner decision.
3. Preserve the fresh local validation results, or rerun the gate if the tree changes again before release/merge.
4. Keep SDK parity, schedule targets, authority proof, event compatibility, and mailbox lineage tied to their documented regression bars for future changes.

### Will document / keep bounded, not solve fully now

These should be documented clearly and optionally improved with examples, but not treated as blockers:

1. Web UI APIs remain internal unless promoted to `/api/v1`.
2. Operational automation gaps: Grafana dashboard, alerting, auto-backup, vault rotation.
3. Plugin security residual risks: opt-in pinning, no code signing, process boundary not sandbox.
4. Prompt injection and irreversible tool limits: bounded-risk posture, not total prevention.
5. Platform isolation caveats: Linux stronger than Windows/macOS.

### Needs owner sign-off before implementation

1. Automatic destructive graveyard removal / profile deletion.
2. Any default behavior that auto-removes memory, config, skills, workspaces, or profiles.
3. Stronger sandboxing guarantees that would change platform support or require non-stdlib dependencies.
4. Built-in monitoring/backup/rotation automation if it changes product scope rather than providing examples.

## Execution phases

## Phase 0 — Worktree ownership and safety gate

Goal: prevent accidental mixing of unrelated agent work.

Scope:

- Inspect `git status --short`.
- Partition existing changes into ownership buckets:
  - docs/report artifacts: `docs/SYSTEM-REVIEW.md`, `docs/MISSING-PARTS-REPORT.md`, `docs/MISSING-PARTS-PLAN.md`, `docs/index.md`
  - SDK parity/docs: `docs/API-STABILITY.md`, `docs/SDK-PARITY.md`, `tools/sdkparity/main.go`, `tools/sdkparity/main_test.go`
  - frontend UI/tests: AgentDetail, AgentPage, Dashboard, Activity, Insights, ConfigCenter, missing smoke/import tests
  - backend tests: `kernel/intervention/intervention_test.go`, `kernel/restapi/update_handlers_test.go`
  - misc scripts/build: `Makefile`, `dev.ps1`
  - existing generated/stale architecture report: `docs/ARCHITECTURAL-REPORT.md`

Implementation steps:

1. Run `git status --short` and record current dirty files.
2. For any commit, stage explicit paths only.
3. Prefer separate commits for docs-only, SDK parity, frontend, and backend tests.
4. Do not modify or revert files outside the active bucket.

Validation:

- `git diff --stat -- <bucket paths>` before commit.
- No blind `git add .`.

Exit criteria:

- Every dirty file is either owned by a bucket or explicitly left untouched.
- Any produced commit/patch contains only one concern.

Status note: targeted validation for the previously untracked test bucket passed on 2026-06-22 (frontend missing-smoke/import/API/ACP/agent/market/AgentPage tests, `kernel/intervention`, `kernel/restapi -run Update`, and `tools/sdkparity`). These files still need scoped commit ownership or explicit discard before release/merge.

Current dirty-worktree ownership snapshot from 2026-06-22:

- **Missing-parts docs/status slice:** `docs/MISSING-PARTS-PLAN.md`, `docs/MISSING-PARTS-REPORT.md`.
- **SDK parity generated-report slice:** `docs/SDK-PARITY.md`, `tools/sdkparity/main.go`, `tools/sdkparity/main_test.go`.
- **Frontend AgentDetail/comms UX slice:** `frontend/src/components/AgentDetail.tsx`, `frontend/src/components/AgentDetail.test.tsx`, plus generated Web UI dist asset replacements under `kernel/webui/dist/` if committed with the frontend build.
- **Frontend analytics/UI/test coverage slice:** `frontend/src/lib/insights.ts`, `frontend/src/lib/insights.test.ts`, `frontend/src/views/Activity.tsx`, `frontend/src/views/AgentPage.tsx`, `frontend/src/views/AgentPage.test.tsx`, `frontend/src/views/ConfigCenter.test.tsx`, `frontend/src/views/Dashboard.tsx`, `frontend/src/views/Dashboard.test.tsx`, `frontend/src/views/Insights.tsx`, `frontend/src/components/missing-smoke.test.tsx`, `frontend/src/views/missing-imports.test.tsx`, `frontend/src/lib/acp.test.ts`, `frontend/src/lib/agent.test.ts`, `frontend/src/lib/api.test.ts`, `frontend/src/lib/market.test.ts`.
- **Backend coverage slice:** `kernel/intervention/intervention_test.go`, `kernel/restapi/update_handlers_test.go`.
- **Build/script slice:** `Makefile`, `dev.ps1`.

Commit guidance: commit each slice separately with explicit paths only. If committing frontend source after a production build, include the matching `kernel/webui/dist/index.html` and asset delete/add pair from that same build; otherwise discard regenerated dist assets before committing source-only changes.

## Phase 1 — Canonical status and documentation reconciliation

Goal: make project status consistent across docs without changing behavior.

Covers report items: 1, 16, 17, 18.

Scope:

- `README.md`
- `docs/NEXT.md`
- `docs/COMPARISON.md`
- `docs/API-STABILITY.md`
- `docs/SDK-PARITY.md`
- `docs/ARCHITECTURAL-REPORT.md`
- `docs/SYSTEM-REVIEW.md`
- `docs/MISSING-PARTS-REPORT.md`
- `docs/index.md`

Implementation steps:

1. Define a single status vocabulary:
   - **Implemented**: code exists.
   - **Validated**: tests/demos have passed recently.
   - **Documented**: docs accurately describe current behavior.
   - **Owner-gated**: technically possible but intentionally blocked by policy/decision.
   - **Future roadmap**: not committed to current completion scope.
2. Update `README.md` recent validation wording to avoid stale exact test counts unless freshly rerun.
3. Update `docs/ARCHITECTURAL-REPORT.md` in one of two ways:
   - preferred: regenerate/update current-state sections for post-M781 work;
   - fallback: add a prominent stale-report note pointing to `docs/SYSTEM-REVIEW.md` and `docs/MISSING-PARTS-REPORT.md`.
4. Update `docs/NEXT.md` to distinguish genuinely remaining work from items completed by later commits/messages.
5. Update `docs/COMPARISON.md` so “all complete” claims are backed by linked evidence or downgraded to “implemented; validation pending”.
6. Update `docs/API-STABILITY.md` known gaps: remove or narrow plugin protocol versioning if machine-checkable versioning is truly implemented; keep event/journal schema gap if still true.
7. Ensure `docs/index.md` links all current review/planning artifacts.

Validation:

- Grep for contradictory phrases:
  - `Do not declare the project complete`
  - `all complete`
  - `M781`
  - stale test counts like `121 files / 1052 tests`
  - `protocol versioning needs`
- Read the changed sections manually.

Exit criteria:

- No doc claims contradict the current implementation status.
- Stale architecture report is either updated or clearly marked as historical/stale.

## Phase 2 — SDK parity and API conformance proof

Status: documentation reconciliation applied on 2026-06-22. `tools/sdkparity` now generates `docs/SDK-PARITY.md` with static route-string coverage plus behavioral SDK test evidence for Python sync/async/mailbox, TypeScript run/mailbox, Rust REST, and Go local-control-plane paths. Future SDK-intended endpoints still require behavioral tests before being called SDK-complete.

Goal: resolve whether SDK parity is route-only or behaviorally proven, then align docs/tests.

Covers report item: 5.

Scope:

- `docs/SDK-PARITY.md`
- `docs/API-STABILITY.md`
- `docs/COMPARISON.md`
- `tools/sdkparity/main.go`
- `tools/sdkparity/main_test.go`
- `sdk/python/tests/*`
- `sdk/typescript/test/*`
- `sdk/rust/tests/*`
- Go SDK tests under `sdk/*.go`

Implementation steps:

1. Inventory actual behavioral SDK tests by language:
   - auth header behavior;
   - error shape preservation;
   - typed health/model/run/mailbox structures;
   - run event arc fields;
   - tenant header behavior;
   - unsupported update routes.
2. If “20 behavioral dimensions” exists, list the dimensions in `docs/SDK-PARITY.md` under a new behavioral-conformance section.
3. If the tests do not exist, downgrade `docs/COMPARISON.md` wording and add concrete test tasks.
4. Keep route-string coverage clearly separate from behavioral conformance.
5. Add missing tests only where a claimed behavior has no test.

Validation:

- `go run ./tools/sdkparity -check docs/SDK-PARITY.md`
- Python SDK tests.
- TypeScript SDK tests.
- Rust SDK tests.
- Go SDK tests.

Exit criteria:

- `docs/SDK-PARITY.md`, `docs/API-STABILITY.md`, and `docs/COMPARISON.md` agree on SDK maturity.
- Every claimed behavioral parity dimension has a test reference or is explicitly listed as pending.

## Phase 3 — Schedule typed-target hardening proof

Status: documentation reconciliation applied on 2026-06-21. Current target types are evidence-backed by cadence validation/injection tests, daemon scheduled-target tests, CLI schedule rendering tests, and the typed system-task runnable demo. Future schedule target extensions must preserve this validation bar.

Goal: prove or finish the “typed schedules, not prompts” claim.

Covers report item: 4.

Scope:

- `kernel/controlplane/schedule.go`
- `kernel/controlplane/schedule_fires.go`
- `kernel/scheduler/scheduler.go`
- `plugins/tools/schedule/schedule.go`
- `cmd/agt/schedule.go`
- `frontend/src/views/Schedules.tsx`
- `frontend/src/views/Schedules.test.tsx`
- docs/examples under `examples/autonomous/typed-schedule-system-task/`

Implementation steps:

1. Verify all target types are represented in code and tests:
   - `agent`: wakes durable agent identity;
   - `workflow`: runs workflow chain;
   - `system_task`: runs whitelisted maintenance task;
   - `tool`: runs governed tool payload.
2. Verify system-task names are enum/allowlist based, not free-form prompts.
3. Verify tool target payloads cannot smuggle agent instructions into schedule descriptions.
4. Add missing end-to-end tests only for uncovered target types.
5. Update `docs/NEXT.md` and `docs/COMPARISON.md` to a single status:
   - complete if all target types are tested;
   - partial if some target types are implemented but not proven.

Validation:

- `go test ./kernel/controlplane -run Schedule`
- `go test ./cmd/agt -run Schedule`
- `npm --prefix frontend test -- Schedules.test.tsx`
- Run or syntax-check `examples/autonomous/typed-schedule-system-task/run.sh` where supported.

Exit criteria:

- Each schedule target type has code + test evidence.
- Docs no longer disagree about schedule hardening status.

## Phase 4 — End-to-end authority proof

Status: documentation reconciliation applied on 2026-06-21. Runtime/CLI authority proof is evidence-backed for current tool policy, approvals, denials, direct `RunTool`, in-loop tool calls, and `agt agent authority`. Future displayed authority fields must keep the same runtime/journal proof bar.

Goal: prove displayed authority matches runtime enforcement and audit evidence.

Covers report item: 3.

Scope:

- `kernel/agent/agent.go`
- `kernel/runtime/toolrun.go`
- `kernel/edict/*`
- `kernel/controlplane/tool.go`
- `kernel/controlplane/roster.go`
- `cmd/agt/agent.go`
- `cmd/agt/agent_authority_test.go`
- `frontend/src/components/AgentDetail.tsx`
- `frontend/src/components/AgentDetail.test.tsx`

Implementation steps:

1. Define one authority proof scenario:
   - agent has profile allow/deny/trust settings;
   - live Edict policy overlays or caps behavior;
   - an allowed tool call journals allow;
   - a denied tool call is refused and journals deny;
   - high-risk approval is surfaced;
   - `agt agent authority --explain` renders the effective result;
   - AgentDetail shows the same key facts.
2. Add or verify backend tests for loop tool calls and direct `RunTool` path.
3. Add or verify CLI rendering tests for `--explain`.
4. Add or verify frontend tests for authority/approval/denial display.
5. Update docs to say exactly which authority properties are enforced and which are display-only if any remain.

Validation:

- `go test ./kernel/agent ./kernel/runtime ./kernel/controlplane -run 'Policy|Authority|RunTool|Approval|Denial'`
- `go test ./cmd/agt -run AgentAuthority`
- `npm --prefix frontend test -- AgentDetail.test.tsx`

Exit criteria:

- One documented proof path connects profile/policy -> runtime decision -> journal event -> CLI/UI display.
- Docs stop warning about unproven authority if the proof is complete; otherwise they list the exact remaining unproven fields.

## Phase 5 — Event/journal schema compatibility

Status: documentation completed on 2026-06-21 via `docs/EVENT-SCHEMA.md`. Compatibility is now policy-documented with append-only event kind rules, core field stability, payload/subject migration expectations, and consumer/producer guidance. Numeric schema versioning remains deferred until a concrete breaking migration requires it.

Goal: make event/journal compatibility explicit for consumers and demos.

Covers report item: 6.

Scope:

- `kernel/event/kinds.go`
- `kernel/journal/*`
- `cmd/agt/why.go`
- `docs/API-STABILITY.md`
- potentially new `docs/EVENT-SCHEMA.md`

Implementation steps:

1. Document event compatibility rules:
   - subjects/kinds are append-only where possible;
   - fields are additive by default;
   - renames require migration notes or dual-write period;
   - consumers must tolerate unknown fields;
   - breaking changes require changelog entry.
2. Decide whether to add explicit schema version metadata now or only document compatibility rules first.
3. If adding metadata, start minimally: a version constant and docs, not a broad migration engine.
4. Add tests only if code changes are made.

Validation:

- Docs review for clarity.
- `go test ./kernel/event ./kernel/journal ./cmd/agt -run 'Event|Journal|Why'` if code changes.

Exit criteria:

- API stability docs no longer say event schema migration story is missing.
- Event consumers have clear compatibility guidance.

## Phase 6 — Agent communication and lineage polish

Status: documentation reconciliation applied on 2026-06-22. Current mailbox, delegated, and doctor lineage paths are evidence-backed by control-plane/REST tests and AgentDetail helper/component coverage. Compact inbox priority summary is now implemented in AgentDetail Comms as UX polish, with focused frontend coverage.

Goal: improve traceability beyond already-shipped mailbox wake causality.

Covers report item: 8.

Scope:

- `kernel/controlplane/roster.go`
- `kernel/controlplane/board_write_test.go`
- `kernel/restapi/mailbox_test.go`
- `kernel/runtime/subagent.go`
- `cmd/agezt/auto_repair.go`
- `frontend/src/components/AgentDetail.tsx`
- `frontend/src/lib/agentdetail.ts`

Implementation steps:

1. Verify mailbox message -> wake event -> run correlation is already exposed.
2. Add missing explicit metadata where absent:
   - sender identity;
   - recipient identity;
   - wake intent;
   - response correlation;
   - escalation status.
3. Extend delegated/doctor lineage display if not already present.
4. Add compact inbox priority summary if still absent and useful.

Validation:

- `go test ./kernel/controlplane -run 'Board|Mailbox|Wake|Roster'`
- `go test ./kernel/restapi -run Mailbox`
- `npm --prefix frontend test -- AgentDetail.test.tsx`

Exit criteria:

- Agent-to-agent communication has traceable sender/recipient/wake/run linkage in API and UI.
- Inbox priority summary is implemented or intentionally deferred. Current AgentDetail Comms implements the compact direct/broadcast/help/replied/stale summary.

## Phase 7 — Operational examples and bounded-risk documentation

Status: documentation completed on 2026-06-22. `docs/OPERATIONS.md` now includes example operator wiring for Prometheus/Grafana/backup/vault rotation/platform CI notes; `docs/PLUGIN-SECURITY.md` includes a production plugin hardening checklist; `docs/THREAT-MODEL.md` includes claims guardrails for bounded-risk language. No bundled automation or product-scope expansion was added.

Goal: keep deliberate limitations visible and optionally provide examples without changing core scope.

Covers report items: 10, 11, 12, 14.

Scope:

- `docs/OPERATIONS.md`
- `docs/PLUGIN-SECURITY.md`
- `docs/THREAT-MODEL.md`
- `README.md`
- possibly `examples/operations/*`

Implementation steps:

1. Add example snippets rather than built-in automation:
   - Prometheus scrape notes;
   - Grafana starter panel list;
   - cron/systemd timer for `agt backup`;
   - vault rotation reminder/checklist;
   - CI matrix notes for Linux/Windows/macOS-specific tests.
2. Keep limitations explicit:
   - plugin pinning opt-in;
   - no code signing;
   - process isolation is not sandbox;
   - prompt injection bounded, not solved;
   - irreversible tools rely on gating/audit.
3. If adding scripts/config examples, keep them non-invasive and documented as examples.

Validation:

- Docs review.
- Shell syntax check for any script examples.

Exit criteria:

- Operators have a practical path for monitoring/backup/rotation without AGEZT pretending to bundle those systems.
- Security claims remain honest and non-overstated.

## Phase 8 — Owner-gated destructive graveyard automation

Status: decision record completed on 2026-06-22 via `docs/GRAVEYARD-POLICY.md`. No destructive automation implemented. Current posture is keep-by-default with report-only scan; future automatic removal requires explicit owner approval and must meet the documented design bar.

Goal: decide, not implement by default.

Covers report item: 9.

Scope:

- `docs/NEXT.md`
- `kernel/controlplane/roster.go`
- graveyard scan/system task code if owner approves later

Implementation steps now:

1. Keep current report-only graveyard scan behavior.
2. Add a short owner decision record if needed:
   - default keep forever;
   - report-only scan is safe;
   - auto-delete requires explicit owner approval.
3. If owner approves later, design with:
   - dry-run;
   - approval gate;
   - retention threshold;
   - audit event;
   - tombstone export;
   - restore/rollback story if possible.

Validation now:

- Docs only.

Exit criteria now:

- No destructive automation is added without explicit approval.

## Recommended immediate task order

1. **Worktree ownership** — split or commit dirty files in explicit scoped slices; do not blanket-stage.
2. **Validation freshness** — fresh local gate passed on 2026-06-22; rerun only if the dirty worktree changes before release/merge.
3. **Regression bars** — keep schedule proof, SDK parity, authority proof, event compatibility, and communication lineage tied to tests/docs for future changes.
4. **Owner-gated decisions** — keep destructive graveyard automation deferred until explicit owner sign-off.

## Suggested scoped commits

Use explicit path staging only.

1. `docs: add missing-parts reports and plan`
   - `docs/SYSTEM-REVIEW.md`
   - `docs/MISSING-PARTS-REPORT.md`
   - `docs/MISSING-PARTS-PLAN.md`
   - `docs/index.md`
2. `docs: reconcile project status and API maturity`
   - `README.md`
   - `docs/NEXT.md`
   - `docs/COMPARISON.md`
   - `docs/API-STABILITY.md`
   - `docs/SDK-PARITY.md`
   - `docs/ARCHITECTURAL-REPORT.md` if edited
3. `test(sdk): prove behavioral SDK parity`
   - SDK tests and `tools/sdkparity/*`
4. `test(schedule): prove typed schedule targets`
   - schedule backend/CLI/frontend tests
5. `test(authority): prove effective authority end-to-end`
   - runtime/controlplane/CLI/frontend tests
6. `docs(events): document event schema compatibility`
   - `docs/API-STABILITY.md`
   - optional `docs/EVENT-SCHEMA.md`

## Risk notes

- The shared worktree is dirty. Do not use blanket staging.
- Some gaps may already be fixed in modified/untracked files. Verify before adding new code.
- Avoid changing product scope while reconciling docs.
- Do not implement destructive graveyard automation without owner approval.
- Do not weaken security limitations for marketing language.

## Validation matrix

| Area | Minimum validation |
|---|---|
| Docs-only reconciliation | read changed docs, grep stale/conflicting phrases |
| SDK parity | `go run ./tools/sdkparity -check docs/SDK-PARITY.md`, SDK package tests |
| Schedule hardening | `go test ./kernel/controlplane -run Schedule`, `go test ./cmd/agt -run Schedule`, frontend Schedules test |
| Authority proof | runtime/controlplane/agent policy tests, `go test ./cmd/agt -run AgentAuthority`, AgentDetail test |
| Event schema docs/code | docs review; if code changes, event/journal/why tests |
| Frontend UI changes | focused Vitest + `npm --prefix frontend run typecheck` |
| Release-level confidence | codegen, `go vet ./...`, Go tests, depscheck, SDK parity, frontend tests/typecheck |

## Definition of done for this missing-parts program

Current status against this program:

1. Docs no longer contradict each other about project status and maturity; broad historical reports are marked stale where needed.
2. Dirty worktree changes still need to be committed in scoped slices or intentionally left with ownership notes before a release/merge; the previously untracked test bucket and full local validation gate are green as of 2026-06-22.
3. SDK parity claims have matching generated route coverage and listed behavioral test evidence.
4. Schedule target hardening is evidence-backed for current target types and kept as a regression bar.
5. Effective authority has runtime/CLI/journal proof paths and is kept as a regression bar for future displayed fields.
6. Event/journal schema compatibility is documented in `docs/EVENT-SCHEMA.md`.
7. Operational/security limitations remain explicit, not hidden.
8. Destructive graveyard automation is explicitly deferred behind owner approval and `docs/GRAVEYARD-POLICY.md`.

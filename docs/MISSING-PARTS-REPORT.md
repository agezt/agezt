# AGEZT Missing Parts Report

Generated: 2026-06-21  
Last updated: 2026-06-22  
Scope: current source tree, repository docs, handoff notes, current git status, targeted test validation, and fresh local gate results.

## Summary

AGEZT has a broad implemented surface and the missing-parts review has reconciled the major documentation/proof mismatches found during the audit. Current remaining work is mostly release hygiene: dirty-worktree ownership, scoped commits/discards, and preserving or rerunning the fresh validation gate if the tree changes again.

This report intentionally distinguishes three kinds of gaps:

- **Missing feature / behavior:** code or proof still needs to be built.
- **Documentation gap:** implemented behavior is not accurately reflected in canonical docs.
- **Quality/proof gap:** feature may exist, but tests, demos, or validation evidence are incomplete or stale.

## High-priority status and release hygiene

### 1. Canonical docs now separate current status, regression bars, and release hygiene

Evidence:

- `README.md` uses count-free validation wording and points to `docs/SYSTEM-REVIEW.md` for the latest review artifact.
- `docs/ARCHITECTURAL-REPORT.md` has a current-state note that it is broad but partially stale in phase/test-count sections, and points to the latest review/gap docs.
- `docs/COMPARISON.md` keeps positioning claims tied to implemented demos and explicit overclaim boundaries.
- `docs/MISSING-PARTS-PLAN.md` now lists schedule hardening, SDK parity, authority proof, event compatibility, and mailbox lineage as regression bars rather than unresolved implementation gaps.

Impact: readers can distinguish implemented/evidence-backed behavior from active pre-release completion goals and release hygiene. `docs/NEXT.md` still intentionally says not to declare the broader autonomous-agent goal complete, but that no longer conflicts with the narrower completed proof/documentation slices.

Recommended fix: keep this status vocabulary intact in future docs: implemented/evidence-backed, regression bar, owner-gated, future roadmap, or release hygiene.

### 2. Dirty shared worktree is bucketed but still needs scoped commit ownership

Evidence:

- `git status` on 2026-06-22 still shows modified/untracked files across docs, SDK parity, frontend source/tests, generated Web UI dist assets, backend tests, and build scripts.
- `docs/MISSING-PARTS-PLAN.md` Phase 0 now records an ownership snapshot with commit buckets: missing-parts docs/status, SDK parity generated report, Frontend AgentDetail/comms UX, frontend analytics/UI/test coverage, backend coverage, and build/script changes.
- Fresh local validation is green, but uncommitted/generated assets can still be accidentally mixed into unrelated commits.

Impact: reports and validation are now clearer, but release quality still depends on committing or discarding each bucket intentionally. A blanket commit would still risk mixing independent agent work.

Recommended fix: commit each bucket separately with explicit path lists, or explicitly discard owner-rejected files. If committing frontend source after `npm run build`, include the matching `kernel/webui/dist/index.html` and generated asset delete/add pair; otherwise discard regenerated dist assets before source-only commits.

### 3. Runtime authority is evidence-backed for current tool policy paths; future UI fields need the same proof bar

Evidence:

- `cmd/agt/agent_authority_test.go` tests merged profile + Edict authority rendering.
- `kernel/agent/agent_test.go` verifies denied in-loop tool calls skip invocation and journal policy decisions.
- `kernel/runtime/toolcaps_test.go` verifies direct `RunTool` calls journal allow/deny policy decisions.
- `kernel/runtime/approval_test.go` verifies approval grant/deny behavior and high-risk tool effect metadata.
- `kernel/controlplane/roster_test.go` folds policy denials into agent status.
- `docs/COMPARISON.md` now frames authority as runtime-enforced and evidence-backed for current paths.

Impact: current tool policy, approval, denial, and CLI authority proof paths are tested. The remaining risk is future UI/display fields drifting away from runtime/journal evidence.

Recommended fix: treat authority as a regression bar: every new displayed authority field must cite the runtime source, policy decision, journal event, or control-plane fold that proves it.

### 4. Schedule typed-target hardening is evidence-backed for current target types

Evidence:

- `docs/NEXT.md` Priority #4 section documents schedule target types (agent/workflow/system_task/tool) as evidence-backed with backend tests, CLI rendering tests, and a runnable typed-schedule-system-task demo.
- `docs/COMPARISON.md` frames schedule typed-target execution as implemented with evidence to inspect.
- `kernel/cadence` validation/injection tests, daemon scheduled-target tests, and CLI schedule tests cover typed targets, system-task enum rejection, and suspicious intent warnings.

Impact: current schedule target paths are tested and auditable. Future schedule target extensions must keep the same validation bar.

Recommended fix: keep schedule typed-target hardening as a regression bar for future target types; verify new targets have typed validation, typed ids, audit fields, and no arbitrary agent instructions in system-task payloads.

### 5. SDK parity separates static route coverage from behavioral evidence

Evidence:

- `docs/SDK-PARITY.md` is generated by `tools/sdkparity` and now lists both static route-string coverage and behavioral test evidence.
- Python, TypeScript, and Rust each show 9/9 SDK-intended REST routes covered by source route strings.
- Behavioral coverage is listed by test file for Python sync/async/mailbox, TypeScript run/mailbox, Rust REST, and the native Go SDK path.
- `docs/API-STABILITY.md` still correctly states that new SDK-intended features require behavioral tests before being called SDK-complete.

Impact: SDK route coverage and behavioral evidence are now explicitly separated, reducing the risk that external consumers over-trust static route-string parity alone.

Recommended fix: keep `tools/sdkparity` as the generated source for `docs/SDK-PARITY.md`; when new `/api/v1` SDK-intended features are added, update route coverage and add behavioral tests in the same slice.

## Medium-priority gaps

### 6. Event/journal compatibility is now policy-documented; numeric versioning remains deferred

Evidence:

- `docs/EVENT-SCHEMA.md` defines append-only event kind rules, core event field compatibility, payload/subject migration expectations, and consumer/producer guidance.
- `kernel/event/kinds.go` already states kinds grow append-only and should not be renamed.
- `kernel/event/event.go` already states core `Event` field order is part of the wire/hash contract.

Impact: event consumers now have explicit compatibility guidance. A global numeric schema version is still deferred until a concrete breaking migration needs it.

Recommended fix: treat `docs/EVENT-SCHEMA.md` as the compatibility bar for future event changes; add code-level schema versioning only when a real migration requires it.

### 7. Web UI APIs are broad and internal

Evidence:

- `docs/API-STABILITY.md:20` classifies Web UI private APIs as internal.
- `docs/API-STABILITY.md:135` warns external callers may discover them but should not depend on them unless promoted to `/api/v1`.

Impact: broad internal APIs are useful for the SPA but can become accidental external contracts.

Recommended fix: document which Web UI routes are private, which are candidates for `/api/v1`, and add route-level comments/tests to avoid accidental promotion.

### 8. Agent-to-agent communication lineage is evidence-backed; inbox prioritization UX is implemented

Evidence:

- `kernel/controlplane/board_write_test.go` covers DM, inbox, reply threading, broadcast ack, topic posts, and notifier correlation.
- `kernel/restapi/mailbox_test.go` covers the REST mailbox arc, auth, validation, replies, ack, topics, and correlation echo.
- `kernel/controlplane/roster_test.go` covers mailbox wake causality (`mailbox_wakes`), delegated wake runbook lineage, doctor wake runbook lineage, and escalation/delegation incident ids.
- `frontend/src/lib/agentdetail.test.ts` covers `mailboxWakeFor`, doctor/delegated `wakeLineage`, and escalation causality lineage helpers.
- `frontend/src/components/AgentDetail.tsx` renders mailbox wake badges and run/incident links through those helpers.

Impact: sender/recipient/reply/wake/run lineage is covered for the current mailbox, delegated, and doctor paths. AgentDetail Comms now adds a compact priority summary for direct, broadcast, help/escalation, replied, and stale unanswered buckets.

Recommended fix: keep mailbox/delegated/doctor lineage and the inbox priority buckets as regression bars for future communication changes.

### 9. Removal/graveyard destructive automation is intentionally incomplete, with a decision record

Evidence:

- `docs/NEXT.md` says actual auto-removal needs owner sign-off because it would call irreversible `RemoveProfile` on a timer.
- Current graveyard scan is report-only and removes nothing.
- `docs/GRAVEYARD-POLICY.md` records the keep-by-default posture and the design bar (dry-run, approval gate, retention threshold, tombstone export, audit events, restore/rollback, tenant safety, fail-safe defaults, tests) any future destructive automation must meet.

Impact: safe by default; the retention lifecycle is not fully automated and will not be without explicit owner approval.

Recommended fix: keep the decision record as the gate. Open an owner decision issue referencing `docs/GRAVEYARD-POLICY.md` before implementing automatic removal.

### 10. Operational automation is deliberately external, with example wiring documented

Evidence:

- `docs/OPERATIONS.md` says AGEZT does not ship built-in Grafana dashboards, built-in alerting, auto-rotation of vault passphrases, or auto-backup.
- `docs/OPERATIONS.md` now includes example operator wiring for Prometheus scraping, starter Grafana panels, backup scheduling, vault rotation, and platform-specific CI notes.

Impact: production operators must still integrate monitoring, alerting, backup, and rotation themselves, but they now have documented starting points.

Recommended fix: keep these as examples unless the product scope explicitly changes to bundle monitoring/backup automation.

### 11. Plugin security has explicit residual risks and operator hardening guidance

Evidence:

- `docs/PLUGIN-SECURITY.md` says pinning is opt-in and there is no code-signing model.
- `docs/PLUGIN-SECURITY.md` says process isolation is not a sandbox, plugins run with daemon OS privileges, and hung/excessive CPU/memory plugins are not proactively killed outside invoke timeouts.
- `docs/PLUGIN-SECURITY.md` now includes an operator hardening checklist for pins, allowlists, low-privilege users, binary placement, logs, MCP trust domains, and re-hashing.
- `docs/THREAT-MODEL.md` says Windows/macOS warden falls back to timeout/output/env/workdir only, with no process-level isolation.

Impact: plugin/tool execution security still depends on operator policy and platform, but deployment guidance is explicit.

Recommended fix: keep warnings prominent; treat code-signing and proactive external-plugin resource monitoring as future roadmap unless owner scope changes.

### 12. Prompt injection and irreversible tools remain bounded-risk, not solved

Evidence:

- `docs/THREAT-MODEL.md` says prompt injection is not solved; AGEZT contains blast radius.
- `docs/THREAT-MODEL.md` says irreversible tools have no generic rollback and auto-approve weakens controls.
- `docs/THREAT-MODEL.md` now includes claims guardrails for prompt injection, irreversible tools, platform isolation, and plugin pinning.

Impact: appropriate security posture remains visible and harder to overstate in future docs/marketing.

Recommended fix: keep demos focused on denial/approval/audit and continue avoiding language implying total prevention or rollback.

## Test and quality gaps

### 13. Fresh validation gate was run locally on 2026-06-22

Evidence:

- `go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen` — passed, with no generated contract drift observed in git status.
- `go vet ./...` — passed.
- `go run ./tools/depscheck` — passed (`OK: 24 core dependencies, all justified`).
- `go run ./tools/sdkparity -check docs/SDK-PARITY.md` — passed.
- `npm test` in `frontend/` — passed, 138 files / 1194 tests.
- Frontend typecheck — passed.
- `npm run build` in `frontend/` — passed via shell after the restricted npm exec wrapper blocked `npm run build`; Vite emitted only the existing large-chunk warning and refreshed embedded Web UI dist assets.
- `go test ./...` — passed on Windows within a 10-minute timeout.

Impact: the previous validation status no longer depends only on mailbox coordination evidence. The remaining release hygiene is ownership of dirty files/generated Web UI assets and scoped commit/discard decisions.

Recommended fix: before final release/merge, preserve these gate results in the release note or rerun if the dirty worktree changes again; stage generated Web UI assets only with the corresponding frontend source changes.

### 14. Environment-conditional tests are documented as CI matrix requirements

Evidence:

- Grep found `t.Skip` in tests for Windows permission bits, timezone data, missing `python`/`go`/`git`/code-exec runtimes, symlink/nofollow availability, noisy timing, Linux-only warden namespace/resource-limit paths, and other environmental constraints.
- `docs/OPERATIONS.md` now documents platform-specific validation notes for Linux, Windows, macOS, optional tool-runtime images, and timing-sensitive stable CI lanes.

Impact: local Windows green is useful but still does not prove every Linux-only isolation or Unix filesystem path. The documentation now makes that matrix expectation explicit.

Recommended fix: keep the CI matrix aligned with `docs/OPERATIONS.md`; when adding environment-conditional tests, update the platform validation notes if they introduce a new required lane or runtime.

### 15. Previously untracked tests are targeted-validated but still need commit ownership

Evidence:

Current untracked files include:

- `frontend/src/components/missing-smoke.test.tsx`
- `frontend/src/lib/acp.test.ts`
- `frontend/src/lib/agent.test.ts`
- `frontend/src/lib/api.test.ts`
- `frontend/src/lib/market.test.ts`
- `frontend/src/views/AgentPage.test.tsx`
- `frontend/src/views/missing-imports.test.tsx`
- `kernel/intervention/intervention_test.go`
- `kernel/restapi/update_handlers_test.go`
- `tools/sdkparity/main_test.go`

Targeted validation on 2026-06-22:

- `npm test -- src/components/missing-smoke.test.tsx src/lib/acp.test.ts src/lib/agent.test.ts src/lib/api.test.ts src/lib/market.test.ts src/views/AgentPage.test.tsx src/views/missing-imports.test.tsx` — passed, 7 files / 16 tests.
- `go test ./kernel/intervention` — passed.
- `go test ./kernel/restapi -run Update` — passed.
- `go test ./tools/sdkparity` — passed.

Impact: these tests close coverage gaps and have targeted green evidence, but while untracked they are still not part of committed quality proof.

Recommended fix: commit these files in scoped test slices or explicitly discard them by owner/agent; do not leave them untracked before release/merge.

## Documentation gaps

### 16. `docs/ARCHITECTURAL-REPORT.md` is downgraded from latest canonical status

Evidence:

- `docs/ARCHITECTURAL-REPORT.md` now carries a prominent current-state note that it is broad but partially stale in phase/test-count sections.
- The note points readers to `docs/SYSTEM-REVIEW.md`, `docs/MISSING-PARTS-REPORT.md`, and `docs/MISSING-PARTS-PLAN.md` for the latest review and gap plan.

Impact: users can still use the broad architecture report for background without mistaking older phase/test-count sections for the latest review artifact.

Recommended fix: regenerate `docs/ARCHITECTURAL-REPORT.md` only when a fresh comprehensive architecture sweep is desired; otherwise keep the current-state note prominent.

### 17. README validation wording avoids stale exact counts

Evidence:

- `README.md` now says recent local gates included `go test ./...`, frontend `npm test`, `npm run build`, and Playwright E2E without hard-coding stale file/test counts.
- It links `docs/SYSTEM-REVIEW.md` for latest review artifact and validation notes.

Impact: README no longer advertises obsolete frontend test counts while preserving useful validation context.

Recommended fix: keep exact validation counts out of README unless they are freshly rerun and intentionally maintained.

### 18. API stability known gaps now separate plugin protocol and event/journal compatibility

Evidence:

- `docs/API-STABILITY.md` states plugin protocol versioning is explicit and machine-checkable, while compatibility policy should stay documented for plugin authors.
- `docs/API-STABILITY.md` separately states event/journal compatibility is policy-documented in `docs/EVENT-SCHEMA.md`, with numeric schema versioning deferred until a concrete breaking migration requires it.

Impact: API stability docs no longer contradict the completed plugin-protocol-versioning claim, and remaining compatibility caveats are scoped to future evolution.

Recommended fix: keep plugin protocol compatibility expectations documented and add code-level event schema versioning only when a real migration needs it.

## Recommended priority order

1. Split/commit or otherwise resolve the dirty worktree in scoped slices (see `docs/MISSING-PARTS-PLAN.md` Phase 0 ownership snapshot).
2. Fresh release-level validation passed on 2026-06-22; rerun only if the dirty worktree changes before release/merge.
3. Keep schedule typed-target, SDK behavioral parity, authority proof, event compatibility, and mailbox lineage as regression bars for future changes.
4. Keep owner-gated destructive graveyard automation deferred unless the owner explicitly approves a safe design.
5. Keep plugin/warden/prompt-injection limitations visible and tested.

## Files most relevant for follow-up

- `docs/NEXT.md`
- `docs/ARCHITECTURAL-REPORT.md`
- `docs/SYSTEM-REVIEW.md`
- `docs/COMPARISON.md`
- `docs/API-STABILITY.md`
- `docs/SDK-PARITY.md`
- `docs/PLUGIN-SECURITY.md`
- `docs/THREAT-MODEL.md`
- `docs/OPERATIONS.md`
- `README.md`
- `Makefile`
- `tools/sdkparity/main.go`
- `kernel/controlplane/schedule.go`
- `kernel/controlplane/schedule_fires.go`
- `kernel/runtime/toolrun.go`
- `kernel/agent/agent.go`
- `frontend/src/components/AgentDetail.tsx`

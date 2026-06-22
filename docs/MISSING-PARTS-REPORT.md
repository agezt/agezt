# AGEZT Missing Parts Report

Generated: 2026-06-21  
Scope: current source tree, repository docs, handoff notes, current git status, and previously reported validation status.

## Summary

AGEZT has a broad implemented surface, but the project is not cleanly complete. The strongest missing work clusters are: stale/colliding documentation, incomplete or not-yet-proven end-to-end behavioral guarantees, SDK/API conformance depth, schedule/system-task hardening, explicit event/schema versioning, and shared-worktree hygiene.

This report intentionally distinguishes three kinds of gaps:

- **Missing feature / behavior:** code or proof still needs to be built.
- **Documentation gap:** implemented behavior is not accurately reflected in canonical docs.
- **Quality/proof gap:** feature may exist, but tests, demos, or validation evidence are incomplete or stale.

## Critical / high-priority gaps

### 1. Canonical docs disagree about project completion and current phase

Evidence:

- `NEXT.md:7` says: "Do not declare the project complete. Continue making concrete progress."
- `NEXT.md:491-505` lists completion criteria and ends with: "The project is not there yet."
- `docs/COMPARISON.md:255-266` marks all listed comparison/platform maturity priorities complete.
- `ARCHITECTURAL-REPORT.md:1108-1126` still describes current state as 2026-06-10 / M781 / PR #224 and owner-gated release items.
- `docs/SYSTEM-REVIEW.md:97-99` records that `ARCHITECTURAL-REPORT.md` is partially stale.

Impact: readers and future agents get conflicting signals about whether the product is complete, pre-release, or still missing major autonomous-agent behavior.

Recommended fix: choose a single canonical status source. Update `ARCHITECTURAL-REPORT.md`, `NEXT.md`, `docs/COMPARISON.md`, and `README.md` so they consistently separate "implemented", "validated", "owner-gated", and "future roadmap".

### 2. Dirty shared worktree makes completion/quality status ambiguous

Evidence:

- `git status --short` shows many modified and untracked files, including docs, Makefile, SDK parity checker, frontend UI/test files, and new backend tests.
- `docs/SYSTEM-REVIEW.md:101-103` already flags the shared worktree risk.
- `NEXT.md:471-481` warns that the repository has many modified files and commits must be scoped.

Impact: reports, validation claims, and commits can accidentally mix unrelated agent work. This is a quality and release-management risk.

Recommended fix: split into scoped commits or patches: docs/report artifacts, SDK parity/docs, frontend UI/test changes, backend tests, and Makefile/dev script changes. Avoid blanket staging.

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

### 4. Schedule typed-target hardening needs reconciliation across docs and tests

Evidence:

- `NEXT.md:209-238` lists schedule target execution checks and system-task examples as next work.
- `docs/COMPARISON.md:125-131` says schedule target hardening remains to prove each target type end-to-end.
- `docs/COMPARISON.md:257-260` later marks schedule typed-target execution hardening done.
- `NEXT.md:415-418` still recommends schedule typed-target hardening as a good next slice.

Impact: the implementation may have advanced, but docs disagree about whether this is complete. That weakens the "typed schedules, not prompts" positioning claim.

Recommended fix: verify current schedule tests and examples, then update `NEXT.md` and `docs/COMPARISON.md` with one consistent status. If gaps remain, add end-to-end tests for agent/workflow/system_task/tool targets and payload smuggling resistance.

### 5. SDK parity is not fully behavioral despite route coverage

Evidence:

- `docs/SDK-PARITY.md:5` says the report is route-string coverage, not behavioral conformance.
- `docs/SDK-PARITY.md:36-38` says typed requests/responses, auth behavior, error behavior, and tests remain conformance work.
- `docs/API-STABILITY.md:128-134` repeats that route coverage does not replace behavioral SDK tests.
- `docs/COMPARISON.md:264-266` says behavioral SDK parity tests are done across 20 dimensions, creating a documentation mismatch.

Impact: SDK completeness is not clearly established. External consumers may over-trust static route-string parity.

Recommended fix: reconcile `docs/SDK-PARITY.md`, `docs/API-STABILITY.md`, and `docs/COMPARISON.md`. If 20 behavioral dimensions are truly implemented, list them in `docs/SDK-PARITY.md`; otherwise keep comparison wording conservative.

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

### 8. Agent-to-agent communication lineage is mostly evidence-backed; inbox prioritization remains optional UX work

Evidence:

- `kernel/controlplane/board_write_test.go` covers DM, inbox, reply threading, broadcast ack, topic posts, and notifier correlation.
- `kernel/restapi/mailbox_test.go` covers the REST mailbox arc, auth, validation, replies, ack, topics, and correlation echo.
- `kernel/controlplane/roster_test.go` covers mailbox wake causality (`mailbox_wakes`), delegated wake runbook lineage, doctor wake runbook lineage, and escalation/delegation incident ids.
- `frontend/src/lib/agentdetail.test.ts` covers `mailboxWakeFor`, doctor/delegated `wakeLineage`, and escalation causality lineage helpers.
- `frontend/src/components/AgentDetail.tsx` renders mailbox wake badges and run/incident links through those helpers.

Impact: sender/recipient/reply/wake/run lineage is covered for the current mailbox, delegated, and doctor paths. The remaining item is a compact inbox priority summary, which is useful UX but not a blocker for core traceability.

Recommended fix: keep mailbox/delegated/doctor lineage as a regression bar. Add compact inbox priority summary only if product UX needs it, with tests for direct, broadcast, help/escalation, replied, and stale unanswered buckets.

### 9. Removal/graveyard destructive automation is intentionally incomplete, with a decision record

Evidence:

- `NEXT.md` says actual auto-removal needs owner sign-off because it would call irreversible `RemoveProfile` on a timer.
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

### 13. Fresh validation was not run during this missing-parts review

Evidence:

- `docs/SYSTEM-REVIEW.md:76-89` records validation status from mailbox reports, not fresh local execution.
- This review performed reads/greps/status checks only.

Impact: reported green status depends on prior agent coordination messages. It is useful evidence but not a fresh gate.

Recommended fix: before release or commit, rerun the project gate: codegen, `go vet`, Go tests, depscheck, SDK parity, frontend tests, and frontend typecheck. On Windows, split Go tests if full `go test ./...` times out.

### 14. Some tests are environment-conditional or skipped

Evidence:

- Grep found `t.Skip` in tests for Windows permission bits, timezone data, Python/go availability, noisy timing, and other environmental constraints.

Impact: normal for cross-platform projects, but it means CI matrix coverage matters. Local Windows green does not prove Linux-only isolation paths or permission semantics.

Recommended fix: document which checks require Linux/macOS/Windows CI and ensure the CI matrix covers platform-specific behavior.

### 15. Untracked tests exist and need ownership

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

Impact: these likely close coverage gaps, but while untracked they are not part of committed quality proof.

Recommended fix: review, run targeted tests, then commit or discard intentionally by owner/agent.

## Documentation gaps

### 16. `ARCHITECTURAL-REPORT.md` should be regenerated or downgraded from canonical status

Evidence:

- It still references M781 and older current state/test data.
- It is linked from `docs/index.md` as the broader generated architecture report.

Impact: users may treat stale data as authoritative.

Recommended fix: regenerate it from current source or add a prominent note that `docs/SYSTEM-REVIEW.md` is the latest current-state artifact.

### 17. README validation counts are stale relative to mailbox reports

Evidence:

- `README.md:17-18` says frontend `npm test` was 121 files / 1052 tests.
- Mailbox reported later frontend runs with 132/1177 and 138/1193 tests.

Impact: validation claims look stale.

Recommended fix: update README recent gates only after rerunning or choosing a stable wording that avoids exact counts.

### 18. API stability known-gaps section conflicts with later completed-work claims

Evidence:

- `docs/API-STABILITY.md:130-138` still lists plugin protocol versioning as needing explicit machine-checkable compatibility.
- Mailbox and `docs/COMPARISON.md:264-266` say protocol versioning is done.

Impact: docs disagree on maturity.

Recommended fix: update `docs/API-STABILITY.md` to reflect implemented protocol versioning, or narrow the remaining gap to event/journal schema compatibility.

## Recommended priority order

1. Reconcile docs status: `NEXT.md`, `docs/COMPARISON.md`, `docs/API-STABILITY.md`, `docs/SDK-PARITY.md`, `ARCHITECTURAL-REPORT.md`, `README.md`.
2. Split/commit or otherwise resolve the dirty worktree in scoped slices.
3. Verify schedule typed-target and SDK behavioral parity claims with tests and docs.
4. Add end-to-end authority proof from displayed config/policy to runtime/journal/UI evidence.
5. Document event/journal schema versioning and migration rules.
6. Decide whether operational automation gaps are deliberately out-of-scope or need examples.
7. Keep plugin/warden/prompt-injection limitations visible and tested.

## Files most relevant for follow-up

- `NEXT.md`
- `ARCHITECTURAL-REPORT.md`
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

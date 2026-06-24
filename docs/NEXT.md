# AGEZT Next Plan And Handoff Notes

This is the continuation plan for the next agent working on AGEZT. For the current missing-parts audit and execution plan, see `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md`.

The user goal is not "make a nicer UI". The goal is a real autonomous multi-agent system: durable agents with identity, memory, skills, authority, lifecycle, mailbox, self-repair, schedules, workflows, and visible runtime behavior.

Do not declare the project complete. Continue making concrete progress.

## Immediate Context

The most recent phase focused on making agent wake behavior auditable and visible:

1. `agent.wake` events now carry an `autonomy_runbook` payload.
2. The latest wake runbook is folded into agent status as `last_autonomy_runbook`.
3. Agent detail reads that status through `AgentCardRuntimeSummary.lastAutonomyRunbook`.
4. Agent detail shows an `Autonomy runbook` card with trigger, route, mailbox, execution, recovery, and sleep behavior.
5. Agent activity timeline wake rows include a readable contract suffix.
6. Schedule-triggered agent wakes now add `autonomy_runbook` to `schedule.fired` payloads when an agent profile is resolved.
7. Schedule fire history exposes `autonomy_runbook` so UI/CLI can show which contract woke the agent.
8. Agent status can fold schedule fire runbooks into `last_autonomy_runbook` with `phase: schedule_fired`, `source: schedule`, and `schedule_id`.

The point of this work is to make the agent's operational contract traceable:

event payload -> status -> detail UI -> activity timeline

Keep extending that pattern.

## Top Priorities

### 1. Enforce lifecycle semantics everywhere

Current state (audited — see findings below):

- `kernel/runtime.RunWith` calls `completeAgentLifecycle` on its TWO success paths
  (heuristic bypass + normal), each only after the `err != nil` early returns, and
  returns immediately after — so a failed run never advances. VERIFIED.
- `RunWithRetry` retries `RunWith` only on failure and returns on first success →
  advances exactly once. VERIFIED.
- Delegated sub-agents advance via `subagent.go` `executeSubAgent` →
  `completeAgentLifecycle(childCtx, childCorr)` on success. VERIFIED.
- Doctor escalation/delegation wakes, manual wake, schedule-agent, standing, and
  mailbox wakes all run through `RunWith`/`RunWithRetry`/`RunAssured` → self-complete.
- There are NO external `CompleteAgentLifecycle` callers; schedule
  workflow/system_task/tool targets do NOT advance agent lifecycle (correct — those
  are workflow/tool/maintenance executions, not the agent completing a cycle).

FIXED this phase — `RunAssured` over-increment: `RunAssured` re-invokes `RunWith`
under the SAME correlation until completion verifies, so each successful-but-
incomplete inner run was calling `completeAgentLifecycle` and double-counting the
cycle. `completeAgentLifecycle` is now idempotent per correlation via a durable
`roster.AgentLifecycle.LastCompletedRun` marker checked inside the atomic
`UpdateProfile` (race-free, survives restart). Test:
`TestCompleteAgentLifecycle_IdempotentPerCorrelation`.

Next work:

- DONE: the helper matrix (cycle/one-shot/failed/retry/assure idempotency) AND the
  end-to-end control-plane manual wake path are covered:
  `TestAgentWake_AdvancesCycleLifecycleOnce` (one increment per operator wake,
  `LastCompletedRun==corr`) and `TestAgentWake_FailedWakeDoesNotAdvanceLifecycle`
  (exhausted provider → 0, not retired).
- Remaining (lower priority): an end-to-end standing/schedule fire lifecycle test
  lives in `cmd/agezt` (needs the standing runner / schedule engine), not the
  control plane — add there if you want belt-and-suspenders, but the shared
  `completeAgentLifecycle` is already proven idempotent and once-only.
- Cancel contract: an operator-cancelled (context-cancelled) run returns an error
  before the success path, so it does not advance lifecycle. This is the intended
  contract; the failed-wake test pins the error-path behavior it shares.

Important files:

- `kernel/runtime/runtime.go`
- `kernel/runtime/subagent.go`
- `kernel/runtime/agentprofile_ctx_test.go`
- `kernel/runtime/subagent_roster_test.go`
- `kernel/controlplane/roster.go`
- `kernel/controlplane/schedule.go`
- `kernel/controlplane/schedule_fires.go`
- `kernel/controlplane/standing.go`

Suggested tests:

- `go test ./kernel/runtime -run Lifecycle`
- `go test ./kernel/runtime -run AgentProfile`
- `go test ./kernel/runtime -run Subagent`
- `go test ./kernel/controlplane -run AgentWake`
- `go test ./kernel/controlplane -run Schedule`

Do not double-increment `completed_cycles`.

### 2. Make autonomy runbook broader than manual wake

Current state:

- Manual/operator `agent.wake` has `autonomy_runbook`.
- Latest manual wake runbook appears in status.
- Activity timeline includes manual wake contract summary.
- Schedule-triggered agent wakes have `autonomy_runbook` on `schedule.fired`.
- Schedule fire history exposes `autonomy_runbook`.
- Latest schedule wake runbook can appear in agent status and last activity.
- Standing-order agent wakes now carry `autonomy_runbook` on `standing.fired`
  (when a roster profile is resolved). It folds into `last_autonomy_runbook` with
  `phase: standing_fired`, `source: standing`, `standing_id`, `standing_name`,
  and `trigger_subject`; activity timeline renders `standing wake fired: <id> ·
  contract ...`; AgentDetail execution detail shows `via standing <name|id>`.
- Mailbox wakes are the standing route matched on a `board.*` subject. When a
  standing fire's `trigger_subject` is a board subject the fold adds
  `wake_via: mailbox`, `mailbox_message_id`, `mailbox_from`, `mailbox_to`,
  `mailbox_reply_to`, `mailbox_help` (from the standing.fired `trigger_payload`);
  activity renders `mailbox wake fired: from <sender>`; AgentDetail shows
  `via mailbox from <sender>`. Linkage = message id + sender + run correlation
  (the standing.fired CorrelationID) all on one folded runbook.
- Delegated sub-agent wakes now carry the runbook on the `KindSubAgentSpawned`
  event (`kernel/runtime/subagent.go`, when delegating AS a named profile) with
  `wake_source: delegated`, `delegated_by` (caller), `parent_correlation_id`. The
  fold sets `phase: delegated_wake`, `source: delegated`, and points
  `correlation_id` at the child run (`child_correlation`); activity renders
  `delegated wake fired: by <leader>`; AgentDetail shows `via delegation by
  <leader>`. NOTE: the runbook builder is now centralized as
  `roster.AutonomyRunbook(p)` — the controlplane and cmd/agezt wrappers delegate to
  it. Use it for any future wake-evidence emitter; do not re-derive the shape.

- Doctor/repair-triggered wakes now carry the runbook. `cmd/agezt/auto_repair.go`
  attaches `autonomy_runbook` (the woken target's contract, via
  `autoRepairWakeResult.Runbook`) + `wake_source: doctor` to the `escalation_woke`
  and `delegation_woke` `doctor.auto_repair` events. The fold attributes to
  `target_agent` (NOT `agent`), sets `source: doctor`, `doctor_for` (the repaired
  agent), `doctor_mode`, `incident_id`; the woken agent's activity rows
  (`accepted escalation/delegated wake for <agent>`) now carry the contract suffix;
  AgentDetail shows `via doctor for <agent>`.

**Status: COMPLETE.** All wake families now carry identically-shaped runbook
evidence through event -> status -> AgentDetail -> activity: manual · schedule ·
standing · mailbox · delegated · doctor. The only remaining candidate is workflow
agent-node wake (low priority — workflow nodes are not yet a first-class agent wake
route; revisit when/if the workflow engine wakes named agents directly).

Follow-ups (Priority #3 territory, not #2):

- DONE (mailbox): "this message woke the agent" now shows in the AgentDetail comms
  tab via `status.mailbox_wakes` + the `⚡ woke` badge (`agentMailboxWakeViews` /
  `mailboxWakeFor`).
- DONE (delegated/doctor): the AgentDetail overview shows a "last wake" lineage line
  under the Autonomy Runbook (`wakeLineage` helper) — a doctor wake links to its
  incident via `openIncident`; a delegated wake shows the parent/lead run
  correlation. Uses the already-folded `incident_id` / `parent_correlation_id` (no
  new journal scan).
- DONE (autonomy feed): named sub-agent delegations now appear in the Autonomy feed
  (`autonomyKinds[KindSubAgentSpawned]` = category `delegation`), filtered to
  identity-level (named) delegations so anonymous fan-out stays out of the curated
  timeline; frontend `catMeta.delegation` gives it a GitBranch icon.
- Keep field names consistent (every new emitter must call `roster.AutonomyRunbook`).
- Fold the latest runbook from all relevant wake subjects, not only `agent.wake`, if new subjects are introduced.

Possible field additions:

- `wake_source`
- `wake_subject`
- `wake_target_type`
- `workflow_id`
- `schedule_id`
- `standing_id`
- `mailbox_message_id`
- `delegated_by`
- `parent_correlation_id`

Important files:

- `kernel/controlplane/roster.go`
- `kernel/controlplane/schedule_fires.go`
- `kernel/controlplane/standing.go`
- `kernel/runtime/subagent.go`
- `kernel/board/board.go`
- `frontend/src/lib/agentdetail.ts`
- `frontend/src/components/AgentDetail.tsx`

### 3. Strengthen mailbox wake and agent-to-agent communication

Current state:

- Agent detail shows mailbox subjects and can arm mailbox wake.
- Board messages can be sent, acknowledged, replied to, and shown in agent detail.
- Managed sub-agents are blocked from direct mailbox arming and should route through owner/parent.
- Mailbox message -> wake event -> run correlation is exposed via `status.mailbox_wakes` and AgentDetail wake badges.
- Delegated and doctor wake lineage have run/incident helper coverage and AgentDetail navigation hooks.
- Sender/recipient/reply/correlation metadata is covered in control-plane and REST mailbox tests.

Completed optional UX work:

- DONE: AgentDetail Comms now includes a compact inbox priority summary:
  - waiting direct messages
  - waiting broadcast messages
  - escalation/help messages
  - messages with replies
  - stale unanswered messages

Important tests:

- `frontend/src/components/AgentDetail.test.tsx`
- `kernel/controlplane/board_write_test.go`
- `kernel/restapi/mailbox_test.go`
- `kernel/controlplane/standing_test.go`

### 4. Complete schedule as typed cron infrastructure

Current state:

- Schedule UI has target manifest.
- Schedule target types exist for agent/workflow/system_task/tool.
- User explicitly does not want schedules to look like agent prompt storage.
- Schedule-fired agent wakes now carry explicit autonomy runbook evidence.
- Backend tests cover typed target validation, typed fire metadata, system-task enum rejection, suspicious intent warning behavior, and structured action text.
- The runnable `examples/autonomous/typed-schedule-system-task/` demo proves system-task schedules are not cron-wrapped prompts.

Status: implemented/evidence-backed for current target types. Future schedule target extensions must keep the same validation bar: typed target, typed id, schedule id, result/correlation audit fields, and no arbitrary agent instructions in system-task payloads.

Important files:

- `kernel/controlplane/schedule.go`
- `kernel/controlplane/schedule_fires.go`
- `kernel/scheduler/scheduler.go`
- `plugins/tools/schedule/schedule.go`
- `frontend/src/views/Schedules.tsx`
- `frontend/src/views/Schedules.test.tsx`
- `cmd/agt/schedule.go`

Suggested tests:

- `go test ./kernel/controlplane -run Schedule`
- `go test ./cmd/agt -run Schedule`
- `npm --prefix frontend test -- Schedules.test.tsx`

### 5. Extend agent permissions into enforceable authority

Current state:

- UI shows authority/control center concepts.
- Tool allow/deny, trust ceiling, config access, memory scope, and noise policy appear in UI.
- Some runtime enforcement exists, but the next agent must verify coverage before claiming it is complete.
- VERIFIED: tool denials ARE enforced and journaled at runtime — the agent loop
  publishes a `policy.decision` event (allow/reason/capability/hard_denied/
  effect_class) for every tool call, and a denied verdict short-circuits the
  invocation (`kernel/agent/agent.go`). The `RunTool` direct path also refuses on a
  deny verdict (but does not journal — see below).

Done this phase — denials are now surfaced per agent:

- `agentPolicyDenialViews` folds `policy.decision` allow=false events by run
  correlation (a `task.received` names the agent for a corr) into agent status:
  `policy_denied_count` + last tool/reason/capability/hard/ts.
- AgentDetail shows a "tool denials" passport (count + last-refusal tooltip) when
  > 0, linking to the diagnostics tab. Pure summarizer `summarizeAgentPolicyDenials`.
- Tests: `TestAgentList_FoldsPolicyDenialsByRunCorrelation` (incl. unrelated-corr
  isolation) + frontend summarizer cases.

Next work:

- Trace effective permission computation from UI to API to runtime (the displayed
  capability passport vs what the runtime actually enforces).
- DONE: `RunTool` direct (operator/CLI) path now journals a `policy.decision` for
  both allowed and refused calls (`kernel/runtime/toolrun.go`), so direct tool runs
  are audited like loop calls. Test:
  `TestRunTool_JournalsPolicyDecisionOnDirectPath`.
- DONE (CLI surface): `agt agent show <slug>` now prints the latest "wake contract"
  (trigger/route/recovery/sleep · source · phase, from `last_autonomy_runbook`) and
  a "tool denials" line (count + last refusal, from `policy_denied_*`); `agt agent
  list` shows a compact `denied=N` flag per agent for at-a-glance fleet scans. NOTE:
  cmd/agt has no command-level server test harness (these commands are
  untested-by-design; full `go test ./cmd/agt` deadlocks on a signal test — run
  targeted, trust linux CI). The rendered fields are covered by the control-plane
  fold tests.
- Surface high-risk tool APPROVALS (not just denials) per agent the same way.
- Make config center access explicit per agent:
  - owned entries
  - allowlisted shared entries
  - hidden secrets
  - excluded entries
- Ensure `tool_allow` and `tool_deny` behavior is consistent for system guardians.

Important files:

- `kernel/controlplane/tool.go`
- `kernel/runtime/toolrun.go`
- `kernel/runtime/toolcaps_test.go`
- `kernel/controlplane/configcenter_handler.go`
- `frontend/src/components/AgentDetail.tsx`
- `frontend/src/components/AgentDetail.test.tsx`

### 6. Make system guardians quiet by default

Current state (audited — largely DONE, deeper than the plan assumed):

- Roster detects noisy guardians (frontend `systemGuardianSafetyIssues`).
- Quiet action exists; doctor quiet patch removes memory from allow and adds to deny.
- VERIFIED: quiet-by-default is ENFORCED at the roster layer, not just the seeder.
  `roster.applySystemGuardianDefaults` runs inside `normalizeProfile`, which
  `Store.Add` AND `Store.Update` both call on every write. For any `System` profile
  it forces: memory scope `system/<slug>`, run+daily cost caps ($0.05),
  `TrustCeiling` capped to L2 (a higher level is pulled back down), `NoisePolicy`
  with `SilentOnSuccess`+`DisableMemoryWrites`, `MinNotifySeverity>=warning`,
  cooldown >= 8h. `enforceNoiseToolDeny` additionally moves `memory` from allow to
  deny when memory writes are disabled.
- CONSEQUENCE: a guardian CANNOT drift noisy through any persisted path — every
  Add/Update re-applies the defaults. A server-side "noisy guardian" warning is
  therefore unreachable for persisted profiles (I prototyped and removed it as dead
  code — it could never fire). The frontend summary remains a useful display.
- `plugins/builtinguardians.SeedAll` + `reconcileExistingGuardian` apply the same
  quiet posture at boot (belt-and-suspenders over the roster enforcement).

Next work (only if a real gap appears):

- If you ever ADD a code path that persists a profile WITHOUT `normalizeProfile`
  (e.g. a raw store write), re-introduce a guardian-safety assertion there.
- "why quieted" audit event when the doctor quiet patch fires (small, optional).
- Ensure any frequent guardian schedule is disabled or explicitly quiet (the seeder
  uses 8h cooldowns; verify no guardian schedule is added at a higher frequency).

Important files:

- `plugins/builtinguardians/*`
- `cmd/agt/doctor.go`
- `cmd/agt/doctor_test.go`
- `frontend/src/views/Roster.tsx`
- `frontend/src/views/Roster.test.tsx`

### 7. Improve removal, retirement, and graveyard operations

Current state:

- Retire/revive/remove exist.
- Remove impact includes schedules, standing orders, memory, skills, config, workspace, workflow refs, mailbox/audit messages, sub-agent tree.
- UI has death certificate.

Next work:

- DONE (read-only tombstone export): `CmdAgentTombstone` (`agent_tombstone`) +
  `agt agent tombstone <slug> [--json]` return a death-certificate snapshot —
  identity, kind, manager, retirement record, lifecycle cycles, and the durable
  resource footprint (reuses `agentImpactResult` counts) plus the retained-by-design
  refs (mailbox/audit + workflow). Read-only: removes/mutates nothing. Tests:
  `TestAgentTombstone_ReturnsIdentityAndFootprint` / `_UnknownAgentErrors`.
- DONE (read-only graveyard inspection): `CmdAgentGraveyard` (`agent_graveyard`) +
  `agt agent graveyard [--older-than DAYS] [--json]` list retired agents oldest-first
  with retirement age — the retention-ELIGIBILITY view. Reports only; no archiving or
  deletion. Tests: `TestAgentGraveyard_ListsOnlyRetired`.
- Existing "dry run removal plan" is effectively `CmdAgentImpact` (counts before
  retire/remove) — confirm that satisfies the requirement before adding another.
- DONE (notify-only retention): `graveyard_scan` system task
  (`cadence.SystemTaskGraveyardScan`, executor `runScheduledGraveyardScan` in
  cmd/agezt) — an operator can schedule it; it journals
  `schedule.system_task.graveyard_scan` with `graveyard_count` / `eligible_count` /
  `eligible` (slugs past the `AGEZT_GRAVEYARD_RETENTION_DAYS` window, default 0 =
  keep-forever) and `action: report_only`. It REMOVES NOTHING (test asserts the
  retired agent survives the scan). Env var registered in `configEnvVars`; surfaced
  in the Schedules UI fallback/presets. Test:
  `TestRunScheduledGraveyardScanReportsOnly`.
- STILL NEEDS OWNER SIGN-OFF (the only destructive EXECUTION path left, do not build
  unprompted): turning the eligibility report into actual auto-removal. With no
  non-destructive archive state, that means irreversible `RemoveProfile` on a timer.
  The safe building blocks (eligibility view + notify-only scan) are done; wiring
  auto-deletion needs explicit policy + defaults. Same for remove-cascade defaults.
  See `docs/GRAVEYARD-POLICY.md` for the decision record and the design bar any
  future destructive automation must meet.

Important files:

- `kernel/controlplane/roster.go`
- `frontend/src/views/Roster.tsx`
- `frontend/src/components/AgentDetail.tsx`

## Recommended Next Slice

The autonomy-runbook pipeline (Priority #2) is COMPLETE: manual · schedule ·
standing · mailbox · delegated · doctor all carry identically-shaped runbook
evidence through event -> status -> AgentDetail -> activity, via the centralized
`roster.AutonomyRunbook(p)` builder and the one fold in
`agentLastAutonomyRunbookViews`.

Lifecycle idempotency (Priority #1 core gap) shipped: `completeAgentLifecycle` is
idempotent per correlation, fixing `RunAssured` cycle over-increment (Priority #1).

Mailbox wake CAUSALITY shipped (Priority #3): `agentMailboxWakeViews` exposes
`status.mailbox_wakes` (board message id -> {correlation_id, ts_unix_ms,
trigger_subject}); the AgentDetail comms tab badges the waking message ("⚡ woke
<agent>", `mailboxWakeFor` helper) and titles it with the run correlation.

Compact inbox priority summary shipped (Priority #3 UX follow-up):
`agentInboxPrioritySummary` folds waiting direct, broadcast, help/escalation,
replied, and stale unanswered buckets; AgentDetail Comms renders the rollup;
focused helper test covers all five buckets.

Schedule typed-target hardening (Priority #4) and effective tool/permission
authority (Priority #5) are evidence-backed for current target/runtime/CLI paths
and kept as regression bars for future extensions and displayed authority fields.

Given those slices, the best next work is release hygiene, not new code:

A) Commit or discard dirty-worktree buckets using explicit path staging only.
   See `docs/MISSING-PARTS-PLAN.md` Phase 0 for the current ownership snapshot
   (missing-parts docs, SDK parity generated report, frontend AgentDetail/comms
   UX, frontend analytics/UI/test coverage, backend coverage, build/script).

B) If committing frontend source after `npm run build`, include the matching
   `kernel/webui/dist/index.html` and generated asset delete/add pair; otherwise
   discard regenerated dist assets before source-only commits.

C) Preserve the fresh validation results in the release note, or rerun the
   Makefile `check` equivalent (codegen, vet, Go tests, depscheck, SDK parity,
   frontend tests/typecheck) if the tree changes again before release/merge.

Do not add new product code unless the owner explicitly opens a new feature
request; the current audit backlog is documentation/proof/regression-bar only.

## Guardrails For The Next Agent

Do not:

- Turn schedule descriptions into prompts.
- Put agent identity instructions into schedule payloads.
- Treat a workflow as an agent.
- Allow managed sub-agents to be directly woken unless deliberately changing the hierarchy model.
- Delete shared memory/config/skills as a side effect of agent removal unless explicit.
- Re-enable memory writes for noisy system guardians casually.
- Add frequent system guardian schedules without quiet policy and test coverage.
- Use destructive git cleanup commands in this dirty worktree.

Do:

- Read the current code before editing.
- Use `rg` first.
- Keep edits scoped.
- Add tests around behavior, not only helper text.
- Preserve current helper exports used by tests.
- Prefer backend event evidence plus UI surface over UI-only summaries.
- Make every new autonomous behavior inspectable in journal/status/activity/detail.

## Current Validation Commands

Fresh local gate was green on 2026-06-22. Makefile `check` equivalent (Windows-safe):

```powershell
go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen
go vet ./...
go run ./tools/depscheck
go run ./tools/sdkparity -check docs/SDK-PARITY.md
cd frontend; npm test
cd frontend; npm run typecheck
cd frontend; npm run build
go test ./...
```

Run focused tests during small changes, then broaden when changing shared runtime/control-plane behavior. If `go test ./...` times out on Windows, rerun after cache warm-up or split by package group.

## Notes About Dirty Worktree

The repository currently has many modified files, including generated Web UI artifacts and large frontend/control-plane files.

Before any future commit or PR:

1. Inspect `git status --short`.
2. Separate intentional changes from unrelated existing dirty state.
3. Do not revert user/other-agent changes.
4. Consider generating a scoped patch summary instead of relying on full diff stats.
5. Rebuild Web UI only if the project convention requires committed dist assets.

## Suggested Handoff Prompt For Another Agent

Use this with the next agent:

> Continue AGEZT toward durable autonomous agents. Read `docs/ARCHITECTURE.md`, `docs/NEXT.md`, and `docs/MISSING-PARTS-PLAN.md` first. Preserve the core boundaries: agents are durable identities; schedules are typed cron/event triggers; workflows are reusable chains; chat sessions are not agents. The recent work reconciled missing-parts documentation/proof claims and ran a fresh local validation gate; the remaining backlog is release hygiene (dirty-worktree ownership, scoped commits/discards, and rerunning validation if the tree changes again). Do not add new product code unless the owner explicitly opens a feature request, and do not revert unrelated dirty worktree changes.

## Completion Standard

Do not mark the main goal complete until all of these are true and verified:

- Durable agents have complete identity, lifecycle, memory, skills, settings, model/fallbacks, provider selection, permissions, mailbox, workspace, retry/doctor/self-repair, logs, and activity.
- Agents can sleep and wake by schedule, event, mailbox, workflow, operator, and other agents.
- Managed sub-agents are controlled through owners/leaders unless explicitly configured otherwise.
- Schedule system is typed cron infrastructure, not prompt storage.
- Workflows are reusable chains runnable by users, agents, and schedules.
- Agent-to-agent communication is traceable.
- Removal/retirement/graveyard behavior is safe and inspectable.
- System guardians are quiet and governed by default.
- Runtime behavior is enforced, not just displayed.
- Web UI shows live state without misleading card clutter.
- Tests prove the behavior across backend and frontend.

The broad autonomous-agent acceptance bar is not fully met yet. Until then, keep moving in small, verified slices, prioritizing release hygiene over new product code while the current audit backlog is documentation/proof/regression-bar only.

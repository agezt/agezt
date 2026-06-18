# AGEZT Autonomous Agent Architecture

This document is a handoff-grade architecture map for the current AGEZT direction: durable autonomous agents, schedule/workflow/tool execution as infrastructure, and a Web UI that exposes real agent identity and runtime state instead of prompt-shaped automation.

It is intentionally detailed. The next agent should treat this as operational context, not as a finished-state claim.

## Product Direction

AGEZT is being moved toward a "real autonomous agent fleet" model.

The core distinction is:

- Agents are durable entities with identity, lifecycle, memory, skills, settings, permissions, logs, mailbox, workspace, retry/doctor/self-repair policy, provider/model routing, and possibly parent/sub-agent ownership.
- Schedules are typed cron/event triggers. They should wake or invoke a target. They are not places to hide agent identity, prompt instructions, or long-lived behavior.
- Workflows are reusable chains, closer to n8n-style multi-step flows. Users, agents, and schedules can run workflows. Workflows are not agent identities.
- Chat sessions are conversations. A chat may say "create an agent", but the chat history itself is not the agent.
- System agents and user-created agents both live in the same conceptual roster, but system agents must be quieter and more governed by default.

The user's repeated design constraint is that agents must feel and behave like independently inspectable, sleeping/waking entities rather than one-off LLM prompts.

## Current High-Level Architecture

The system currently has these major layers:

- `kernel/roster`: durable profile store for agent identity.
- `kernel/runtime`: agent execution runtime, tool execution, retry handling, lifecycle completion, delegation, and run journal emission.
- `kernel/controlplane`: API/control-plane command handlers for roster, schedule, standing orders, workflows, memory, board/mailbox, tools, status, and Web UI support.
- `kernel/scheduler`, `kernel/standing`, `kernel/cadence`: scheduled/standing trigger infrastructure.
- `kernel/board`: mailbox/message board used for agent-to-agent and operator-agent communication.
- `kernel/memory`, `kernel/skill`, `kernel/configcenter`: agent-scoped and shared resources.
- `frontend/src/views/Roster.tsx`: fleet/roster management surface.
- `frontend/src/components/AgentDetail.tsx`: detailed identity page for one agent.
- `frontend/src/views/Schedules.tsx`: schedule management, now moving toward typed targets instead of prompt-like schedules.

The current worktree is heavily modified. Do not assume a clean diff. Inspect current files before editing, and do not revert unrelated changes.

## Agent Identity Model

Durable agent identity is represented primarily by `kernel/roster.Profile`.

Important identity fields include:

- `Slug`: durable address. Must not be casually renamed because schedules, standing orders, memory, skills, config, and logs reference it.
- `System`: marks shipped/internal guardian agents.
- `OwnerAgent` and `ParentAgent`: hierarchy ownership and direct leader/delegation relationship.
- `DirectCallable`: whether operators/schedules/channels may wake the agent directly. If false, it is a managed sub-agent and must be woken through its owner/parent.
- `RetryPolicy`: whole-run retry behavior.
- `HealthPolicy`: doctor/stale/failure policy.
- `SelfRepairPolicy`: self-repair attempts and escalation owner.
- `NoisePolicy`: controls notification and memory-write noise, especially for system guardians.
- `Lifecycle`: persistent/cycle/retire-on-complete behavior.
- `TaskList`: durable cycle and total tasks.
- `Model`, `Fallbacks`, `TaskType`: provider/model routing.
- `MemoryScope`, `ToolAllow`, `ToolDeny`, `TrustCeiling`, `ConfigOverrides`, `Workdir`: authority and resource boundaries.

Derived identity classes:

- System agent: `System == true` or kind system.
- Sub-agent: non-direct-callable or explicit subagent kind.
- Custom/user agent: direct-callable non-system profile.

Frontend helpers mirror this model in `frontend/src/views/Roster.tsx` and `frontend/src/components/AgentDetail.tsx`.

## Runtime Execution And Lifecycle

`kernel/runtime.RunWith` executes an agent run. It already calls lifecycle completion logic after successful runs:

- `completeAgentLifecycle`
- `CompleteAgentLifecycle`
- `shouldRetireAgentAfterComplete`
- `resetCompletedCycleTasks`

Current lifecycle behavior:

- `retire_on_complete` retires the identity after a successful run.
- `cycle` or `max_cycles > 0` increments `completed_cycles`.
- Completed cycle tasks are reset from `done` to `todo`.
- If `completed_cycles >= max_cycles`, the agent is retired.
- Lifecycle completion publishes `roster.<slug>` events with action `lifecycle_cycle_completed`.

Important caution:

- Before adding new lifecycle behavior, verify whether the path uses `RunWith`, `RunWithRetry`, delegation, schedule tool targets, workflow targets, or direct external execution.
- `RunWith` handles lifecycle internally. External non-RunWith paths should call `CompleteAgentLifecycle` only after real success.
- Do not double-increment cycle counts. `completeAgentLifecycle` is idempotent per correlation (durable `roster.AgentLifecycle.LastCompletedRun` marker), so `RunAssured`/`RunWithRetry` re-running `RunWith` under one correlation advances the cycle exactly once. A genuinely new wake uses a new correlation and advances again.

## Wake Contract And Autonomy Runbook

Recent work added an autonomy runbook concept across backend and frontend.

Backend:

- `kernel/controlplane/roster.go`
- `agentAutonomyRunbookPayload(p roster.Profile)` builds a machine-readable contract for wake events.
- `handleAgentWake` adds `autonomy_runbook` to `agent.wake` requested events.
- `runAgentWake` adds the same runbook to completed and failed events.
- `cmd/agezt` schedule firing payloads add `autonomy_runbook` when the schedule is bound to a concrete agent profile.
- `cmd/agezt` standing-order firings (`buildStandingRunner.fire`) add `autonomy_runbook` to `standing.fired` when the order resolves a roster profile, via the shared builder.
- `kernel/runtime/subagent.go` `prepareSubAgent` adds `autonomy_runbook` + `wake_source: delegated` + `delegated_by` + `parent_correlation_id` to the `KindSubAgentSpawned` event when delegating AS a named profile.
- The runbook builder is centralized as `roster.AutonomyRunbook(p Profile) map[string]any` — the single source of truth; the controlplane and `cmd/agezt` `agentAutonomyRunbookPayload` wrappers delegate to it. Any new wake-evidence emitter should call it, not re-derive the shape.
- `kernel/controlplane/schedule_fires.go` exposes that runbook on schedule firing history rows.
- `agentLastAutonomyRunbookViews` folds the latest manual wake, schedule fire, or standing fire runbook into agent status as `last_autonomy_runbook` (schedule fires set `phase: schedule_fired`/`source: schedule`; standing fires set `phase: standing_fired`/`source: standing` with `standing_id`/`standing_name`/`trigger_subject`). A standing fire whose `trigger_subject` is a `board.*` subject is the mailbox-wake route: the fold adds `wake_via: mailbox` plus `mailbox_message_id`/`mailbox_from`/`mailbox_to`/`mailbox_reply_to` from the standing.fired `trigger_payload` (`isMailboxWakeSubject`). Delegated spawns (`KindSubAgentSpawned`) fold with `phase: delegated_wake`/`source: delegated`, `delegated_by`, `parent_correlation_id`, and `correlation_id` pointing at the child run. Doctor escalation/delegation wakes (`doctor.auto_repair` `escalation_woke`/`delegation_woke`, attached in `cmd/agezt/auto_repair.go`) fold by `target_agent` with `source: doctor`, `doctor_for`, `doctor_mode`, `incident_id`. All six wake families share the `roster.AutonomyRunbook` builder.
- `agentActivitySummary` adds a readable timeline suffix such as:
  `contract operator_schedule_channel/self_owned/manual/persistent`

Current runbook fields:

- `identity_kind`
- `trigger_contract`
- `route_contract`
- `recovery_contract`
- `sleep_contract`
- `direct_callable`
- `delegation_manager`
- `retry_attempts`
- optional `self_repair_enabled`
- optional `self_repair_attempts`
- optional `doctor_agent`
- status fold adds `phase`, `correlation_id`, `ts_unix_ms`
- schedule folds additionally add `source: schedule` and `schedule_id`

Frontend:

- `frontend/src/lib/agentdetail.ts`
- `AgentRuntimeStatus.last_autonomy_runbook`
- `AgentCardRuntimeSummary.lastAutonomyRunbook`
- `frontend/src/components/AgentDetail.tsx`
- `agentAutonomyRunbook(...)`
- `AutonomyRunbook` UI component on the agent detail overview.

The detail page now exposes:

- trigger
- route
- mailbox
- execution
- recovery
- sleep
- last journal contract phase/correlation where available

This is a step toward making the agent's operational contract inspectable rather than implicit.

## Roster And Agent Detail Web UI

`Roster.tsx` has been expanded substantially. The intent is for roster cards to work like identity cards, not loose lists.

Important concepts now surfaced in the roster:

- identity kind
- lifecycle rail
- live presence
- mailbox pressure
- wake route
- command strip
- model route
- skill/config/resource/authority summaries
- repair governance
- graveyard / retired identities
- system guardian noise summary
- removal/retirement impact

`AgentDetail.tsx` is the deeper identity control surface. It includes:

- identity card
- command strip
- lifecycle intervention
- autonomy runbook
- operations passport
- runtime doctor ledger
- mailbox/comms tab
- trigger tab and mailbox wake arming
- model/fallback display
- memory and skills
- diagnostic/capability control
- config authority
- lifecycle/tasklist editing

Watch for UI duplication. Many concepts appear in both Roster and AgentDetail. That is intentional, but future edits should avoid creating conflicting explanations.

## Schedule Architecture

The schedule direction is:

- Schedule target types are explicit: agent, workflow, system task, tool.
- Schedules should store target type, target identifier, interval/cron/event timing, and typed payload.
- Schedules should not contain long agent instructions.
- Agent behavior belongs to the agent identity.
- Workflow behavior belongs to workflow definitions.
- System maintenance belongs to system-task targets.

Recent work in `frontend/src/views/Schedules.tsx` added target manifest UI and invalid payload visibility. Backend schedule support already has typed target validation in the control plane and schedule tests.

Recent backend work also makes schedule-triggered agent wake evidence explicit:

- `schedule.fired` events include the agent's `autonomy_runbook` when an agent profile is resolved.
- `agt schedule fires` / control-plane schedule fires output includes `autonomy_runbook`.
- Agent roster/detail status can show a schedule fire as the latest autonomy contract with `phase: schedule_fired`.
- Activity summaries can read as `schedule wake fired: <schedule-id> · contract ...`.

Continue to defend this boundary.

## Workflows

Workflows are reusable execution chains. They may use LLM steps, tools, and agent calls, but they are not agents.

Expected model:

- User can run workflow.
- Agent can run workflow.
- Schedule can run workflow.
- Workflow may call agents.
- Workflow definitions should be inspectable and reusable.

Do not mix workflow ownership with agent identity. An agent may author or use a workflow, but the workflow is a tool/chain, not a soul-bearing entity.

## Mailbox And Agent Communication

The board/mailbox system is central to multi-agent behavior.

Current patterns:

- Board messages can be addressed to an agent, broadcast, acknowledged, replied to, and shown in agent detail.
- Agent detail has mailbox wake subjects (`DM`, `Help`, `Broadcast`) and can arm standing orders for mailbox wake.
- Managed sub-agents should not be armed/woken directly; mailbox wake should route through owner/parent where required.

Needed future direction:

- More explicit agent-to-agent protocols.
- Better inbox prioritization.
- Stronger linkage between mailbox messages, wake events, run correlations, and repair/escalation events.
- Clearer distinction between "message waiting" and "message caused wake".

## System Guardians And Noise

The user called out system agents creating too much memory, notification, and LLM usage.

Recent related work:

- Guardian quieting logic exists in `cmd/agt/doctor.go`.
- Quiet policy removes `memory` from tool allow and adds it to deny.
- Roster highlights noisy guardians and offers quiet action.
- System guardian safety summaries check noise policy, memory scope, cost caps, trust ceiling, and memory tool denial.

Expected baseline for system guardians:

- `silent_on_success`
- `disable_memory_writes`
- notify severity at least warning
- notify cooldown around 8h or more
- memory isolated under `system/<slug>`
- memory tool denied unless explicitly needed
- cost/run and daily caps
- trust ceiling at or below L2 unless justified

Do not add frequent system-agent schedules without strong reason and explicit quiet policy.

## Remove, Retire, Graveyard, And Death Certificate

Agents can live, sleep, retire, be revived, and be removed.

Current semantics:

- Retire moves identity to the graveyard.
- Retired identities remain inspectable.
- Revive returns them in paused state.
- Remove deletes the identity and can cascade owned/private resources.
- System agents are protected from hard removal.
- Sub-agent removal requires dependent tree handling to avoid orphaned identities.

Backend impact data is in `kernel/controlplane/roster.go`.

Frontend removal UX in `Roster.tsx` includes:

- removal plan
- lifecycle summary
- custody summary
- removal ledger
- death certificate
- cleanup preset
- cascade toggles
- sub-agent impact
- workflow/mailbox/audit retention notes

Important design rule:

- Mailbox/audit and workflow references should generally be retained or explicitly marked as retained by design, not silently deleted.
- Private/owned memory, skills, config, workspace, standing orders, and schedules can be cascade-cleaned.

## Permissions And Capability Control

Agent capability is not just tools. It includes:

- tool allow/deny
- trust ceiling
- spend caps
- memory scope and memory writes
- config center access
- workspace
- data lake access
- schedule/channel/operator/delegation wake routes
- repair/doctor authority

AgentDetail has capability control UI. Roster has authority summaries.

Future work should make capability enforcement auditable end-to-end:

- UI shows effective policy.
- API returns effective policy.
- Runtime enforces effective policy.
- Journal records denials and high-risk tool use.

## Tests That Currently Matter

Frequently used target tests:

- `go test ./kernel/controlplane -run TestAgentWake_AcceptsAndRuns`
- `go test ./cmd/agt -run "TestGuardianQuietPatch|TestGuardianNoise"`
- `npm --prefix frontend test -- AgentDetail.test.tsx`
- `npm --prefix frontend test -- Roster.test.tsx`
- `npm --prefix frontend test -- Schedules.test.tsx`
- `npm --prefix frontend test -- agentdetail.test.ts`
- `npm --prefix frontend run typecheck`

When changing lifecycle or runtime:

- Also inspect `kernel/runtime/agentprofile_ctx_test.go`
- Also inspect `kernel/runtime/subagent_roster_test.go`
- Run focused runtime tests before broad control-plane tests.

When changing schedule target behavior:

- Inspect `kernel/controlplane/schedule_test.go`
- Inspect `cmd/agt/schedule_fires_test.go`
- Inspect `frontend/src/views/Schedules.test.tsx`

## Current Known Risk Areas

- The worktree is very dirty. Do not use destructive git commands.
- Large frontend files now contain many exported helpers. Be careful when moving functions because tests import them directly.
- Roster and AgentDetail share concepts but not always shared implementations. Keep UI language consistent.
- Backend lifecycle already exists. Avoid double-calling lifecycle completion.
- System guardian schedules can accidentally create noise and memory pressure.
- `memory` tool allow/deny is sensitive. Quieting guardians should deny memory.
- Sub-agent direct wake must stay blocked unless intentionally changed.
- Schedule must remain typed target infrastructure, not prompt storage.
- Workflow must remain reusable chain infrastructure, not agent identity.

## Handoff Summary

The project is not done. It is roughly in the 37-42% range toward the user's stated "full autonomous Jarvis" goal.

Completed in this phase:

- Stronger roster identity cards.
- Agent detail autonomy runbook.
- Backend wake autonomy runbook payload.
- Latest manual and schedule wake runbooks folded into agent status.
- Activity timeline wake contract summaries.
- Schedule-triggered agent wake runbook evidence in `schedule.fired` and schedule fires output.
- Removal/death certificate UX.
- Schedule target manifest UI.
- Guardian quieting hardening.

The next agent should continue by turning more of these visible contracts into enforced runtime behavior and durable event evidence.

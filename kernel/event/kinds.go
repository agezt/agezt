// SPDX-License-Identifier: MIT

package event

// Kind is the canonical event-kind discriminator (e.g. "agent.spawned").
// Kinds are pinned by the wire contract (.project/agezt-contract.jsonc) and
// grow append-only as new layers land (DECISIONS B0b). Never renumber or
// rename an existing kind; only add new ones.
type Kind string

// Base set — Milestone 0.5 ("core-core"). The remainder of the kinds enum
// is added as their layers come online (Pulse, Memory/Forge, Channels,
// Operability, …). See INDEX.md §2 for the full destination list.
const (
	// Agent lifecycle (P0-LIFE-04).
	KindAgentSpawned   Kind = "agent.spawned"
	KindAgentSuspended Kind = "agent.suspended"
	KindAgentResumed   Kind = "agent.resumed"
	KindAgentDied      Kind = "agent.died"
	KindAgentCrashed   Kind = "agent.crashed"

	// Task / orchestration (tool-loop core; DAG layer adds plan/node kinds
	// later per DECISIONS B0d).
	KindTaskReceived  Kind = "task.received"
	KindTaskCompleted Kind = "task.completed"
	// KindTaskAbandoned marks a run that was received but never completed
	// in a prior daemon session (a crash mid-run, or a run that errored and
	// emitted no completion). Published once at boot during orphan
	// reconciliation, so `agt runs` shows it as "abandoned" instead of
	// "running" forever (M28).
	KindTaskAbandoned Kind = "task.abandoned"
	// KindTaskFailed is the terminal event for a run that started
	// (task.received) but errored out instead of completing — a provider
	// error, an exhausted iteration budget, or a cancelled/timed-out
	// context. Emitted live by the agent loop on any error return after
	// task.received (best-effort), so `agt runs` can tell a real failure
	// apart from a true orphan (M28) and `agt runs stats` (M29) can split
	// the success rate. The payload carries {error, reason} where reason ∈
	// {error, max_iters, canceled, timeout}. The boot-time abandon
	// reconciliation treats task.failed as terminal (a failed run is not
	// abandoned) (M30).
	KindTaskFailed Kind = "task.failed"

	// Multi-agent orchestration (P6-MULTI-01). A lead agent delegates a
	// bounded task to a sub-agent via the `delegate` tool; the spawn is
	// journaled under the PARENT correlation (carrying the child correlation)
	// so `agt why <parent>` shows the delegation, and the child correlation is
	// the drill-down into the sub-agent's own run.
	KindSubAgentSpawned Kind = "subagent.spawned"

	// An ASYNCHRONOUSLY delegated sub-agent finished (M881). Published under
	// the PARENT correlation (carrying the child correlation and outcome) the
	// moment the child's run returns — the push-based completion signal that
	// pairs with delegate(async=true) + delegate_await. Synchronous
	// delegations don't emit this: their completion is the tool result itself.
	KindSubAgentCompleted Kind = "subagent.completed"

	// Tool calls (the in-process Tool interface, DECISIONS B0a).
	KindToolInvoked Kind = "tool.invoked"
	KindToolResult  Kind = "tool.result"

	// LLM/provider traffic (canonical, dialect-free; SPEC-15).
	KindLLMRequest   Kind = "llm.request"
	KindLLMResponse  Kind = "llm.response"
	KindLLMToken     Kind = "llm.token"
	KindLLMReasoning Kind = "llm.reasoning" // ephemeral: a reasoning-model chain-of-thought delta (M317)

	// Control plane (P0-CTRL-*).
	KindHalt   Kind = "halt"
	KindResume Kind = "resume"

	// KindAnomalyDetected records the anomaly auto-halt circuit breaker tripping
	// (SPEC-06 §5): a runaway signal (e.g. tool-call rate exceeding the ceiling
	// within a window) that auto-engages a halt. Journaled so `agt why`/`agt
	// journal` explain WHY the daemon halted itself.
	KindAnomalyDetected Kind = "system.anomaly"

	// KindInfo is a generic informational event for daemon lifecycle notices
	// that don't warrant their own kind (first use: the self-update checker's
	// update.available / update.applied notices, M860).
	KindInfo Kind = "info"

	// Policy / Edict (P1-EDICT-*).
	KindPolicyDecision Kind = "policy.decision"
	// KindPolicyChanged records a runtime mutation of the policy engine's
	// hard-deny rules (operator added/removed a deny rule over the control
	// plane). The deny floor is security-critical, so every change is an
	// auditable event in the same journal as the decisions it governs.
	KindPolicyChanged Kind = "policy.changed"

	// Governor (P1-CONDUIT-*).
	KindRoutingDecision  Kind = "routing.decision"
	KindBudgetConsumed   Kind = "budget.consumed"
	KindProviderFallback Kind = "provider.fallback"
	// One provider being retried IN PLACE with backoff on a transient error
	// (rate limit / 5xx / network blip) before the chain falls back (M882).
	KindProviderRetry  Kind = "provider.retry"
	KindBudgetExceeded Kind = "budget.exceeded"
	KindRateLimited    Kind = "rate.limited"
	// KindBudgetCapInert records that a per-run cost cap (--max-cost / M166) was
	// set on a run whose effective model has no known pricing, so the cap can never
	// trip (spend computes as $0). An advisory at run submission (M169) — the
	// run-time counterpart to the dry-run "will not bind" warning. Payload:
	// {model, cap_microcents}.
	KindBudgetCapInert Kind = "budget.cap_inert"
	// KindBudgetUnpriced records that strict-pricing mode (M193) refused a request
	// because its model has no known price (catalog + fallback table miss). Without
	// strict pricing such a model is charged $0, silently bypassing the budget; this
	// event makes the refusal auditable. Payload: {model}.
	KindBudgetUnpriced Kind = "budget.unpriced"
	// KindBudgetCeilingSet records that an operator adjusted the global daily
	// spend ceiling at runtime (M607) via the control plane / Web UI cockpit —
	// the audit trail for the "ayarla" knob. Payload: {ceiling_mc,
	// prev_ceiling_mc, spent_today_mc}. Distinct from budget.exceeded (the cap
	// tripping) — this is the cap itself being moved.
	KindBudgetCeilingSet Kind = "budget.ceiling_set"
	// Live run steering (M608) — the operator "flies" a running agent from the
	// cockpit. KindRunPaused/Resumed/Stepped record the control actions (emitted
	// by the kernel the instant the operator acts); KindRunSteered records the
	// loop actually folding an injected directive into the next prompt (emitted
	// by the agent loop at the iteration boundary where it takes effect).
	// Payloads carry {correlation_id} and, for steered, {directive, iter}.
	KindRunPaused  Kind = "run.paused"
	KindRunResumed Kind = "run.resumed"
	KindRunStepped Kind = "run.stepped"
	KindRunSteered Kind = "run.steered"
	// Council of Elders (M837) — a panel of differently-modelled advisors debates a
	// question and converges to a consensus. These events are the audit trail an
	// operator (and `agt why`) reads to see who said what and how the council
	// decided. KindCouncilConvened opens it {question, members, rounds};
	// KindCouncilOpinion records one member's position {seat, model, round};
	// KindCouncilConsensus records the synthesized verdict {chars}.
	KindCouncilConvened  Kind = "council.convened"
	KindCouncilOpinion   Kind = "council.opinion"
	KindCouncilConsensus Kind = "council.consensus"
	// KindTaskContinued records that a run exhausted its tool-round budget
	// (MaxIter) without a final answer and was AUTOMATICALLY continued (M833) —
	// the loop injected a "keep going" turn and granted another batch of rounds
	// instead of failing with max_iters. Payload: {attempt, of, iters_so_far}.
	// The audit trail for "why did this run keep going past the cap"; bounded by
	// MaxAutoContinue, so a run can't continue forever silently.
	KindTaskContinued Kind = "task.continued"
	// KindPolicyCompacted records that the durable policy overlay was compacted to
	// a snapshot (M176). Its payload {through_seq, content_hash} binds the on-disk
	// snapshot to the tamper-evident journal: at boot the snapshot is trusted only
	// if its content hash equals the latest journaled value, so a snapshot file
	// edited to loosen policy (e.g. shell→L4) is rejected and the journal — the
	// immutable source of truth — is folded instead.
	KindPolicyCompacted Kind = "policy.compacted"
	// KindNetguardBlocked records an outbound connection the egress guard
	// refused at dial time (M109) — a tool (http/browser) tried to reach an
	// internal/metadata address. An audit signal for SSRF / prompt-injection /
	// exfiltration attempts.
	KindNetguardBlocked Kind = "netguard.blocked"
	// KindCapabilityRejected records a pre-flight rejection of a request
	// because the target model lacks a required capability (M25 strict
	// mode: a tools-bearing request to a non-tool model).
	KindCapabilityRejected Kind = "capability.rejected"
	// KindCapabilityRerouted records a pre-flight model REMAP: a
	// tools-bearing request to a tool-incapable model was down-routed to a
	// tool-capable alternative instead of being rejected (M37 down-routing).
	// Payload: {from_model, to_model, capability, tools_requested}.
	KindCapabilityRerouted Kind = "capability.rerouted"
	// KindCapabilityDegraded records that a requested capability was SILENTLY
	// downgraded rather than rejected or rerouted: the request proceeds, but on
	// a model that can't honour the feature natively (M381: a JSON-mode request
	// to a provider family with no native structured-output switch, which falls
	// back to prompt-instructed JSON). Makes the otherwise-invisible degradation
	// auditable. Payload: {model, capability, reason}.
	KindCapabilityDegraded Kind = "capability.degraded"
	// KindMeshLoopRefused records that a cross-node delegation (M8 mesh) was
	// refused because its hop count exceeded the limit — a federation loop was
	// stopped. Payload: {hop, max_hops}. (M210)
	KindMeshLoopRefused Kind = "mesh.loop_refused"
	// KindContextCompacted records that the agent loop trimmed its own context to
	// stay under a budget before a provider call (SPEC-10 §3): the oldest tool
	// OUTPUTS were elided to stubs while system + recent turns were protected.
	// Makes the otherwise-invisible drop auditable. Payload: {elided, reclaimed_chars,
	// context_chars_before, context_chars_after, budget}. (M393)
	KindContextCompacted Kind = "context.compacted"

	// Warden (P1-WARD-*).
	KindWardenExecuted          Kind = "warden.executed"
	KindWardenProfileDowngraded Kind = "warden.profile_downgraded"
	KindWardenLimitExceeded     Kind = "warden.limit_exceeded"

	// Approval / HITL (SPEC-06 §3.4).
	KindApprovalRequested Kind = "approval.requested"
	KindApprovalGranted   Kind = "approval.granted"
	KindApprovalDenied    Kind = "approval.denied"
	KindApprovalTimeout   Kind = "approval.timeout"

	// Config Center (config.access, rating-based access control).
	KindConfigAccess Kind = "config.access"

	// Scheduler / DAG (SPEC-02 §4; TASKS P1-SCHED-*).
	KindPlanStarted   Kind = "plan.started"
	KindPlanCompleted Kind = "plan.completed"
	KindPlanFailed    Kind = "plan.failed"
	KindNodeStarted   Kind = "node.started"
	KindNodeCompleted Kind = "node.completed"
	KindNodeFailed    Kind = "node.failed"

	// Catalog / provider ecosystem (SPEC-15 §1; TASKS P1-CONDUIT-04).
	KindCatalogSynced             Kind = "catalog.synced"
	KindCatalogSyncFailed         Kind = "catalog.sync_failed"
	KindCatalogDiscoveryCompleted Kind = "catalog.discovery_completed"
	KindCatalogDiscoveryFailed    Kind = "catalog.discovery_failed"

	// Pulse — the proactive heart (SPEC-03). Every stage emits its own
	// event so `agt why` reconstructs tick→delta→score→initiative→brief.
	KindPulseTick       Kind = "pulse.tick"
	KindObserverDelta   Kind = "observer.delta"
	KindSalienceScored  Kind = "salience.scored"
	KindInitiativeTaken Kind = "initiative.taken"
	KindBriefingSent    Kind = "briefing.sent"
	KindPulsePaused     Kind = "pulse.paused"
	KindPulseResumed    Kind = "pulse.resumed"

	// Channels (SPEC-04 §1). Inbound/outbound messages normalized to
	// UnifiedMessage; the Unified Inbox folds these by correlation.
	KindChannelInbound  Kind = "channel.inbound"
	KindChannelOutbound Kind = "channel.outbound"
	// KindChannelError records a recovered panic in inbound-message handling, so a
	// malformed message that trips a handler bug stays diagnosable (`agt journal`)
	// rather than vanishing into a silent recover.
	KindChannelError Kind = "channel.error"

	// Memory-lite (SPEC-05 §2; ROADMAP §2.3). The store is content-
	// addressed and journaled so `agt why` can explain every belief.
	KindMemoryWritten    Kind = "memory.written"    // a record created/reinforced/revived
	KindMemoryRetrieved  Kind = "memory.retrieved"  // records surfaced into a run's context
	KindMemoryForgotten  Kind = "memory.forgotten"  // a record tombstoned (soft delete)
	KindMemoryPruned     Kind = "memory.pruned"     // soft-deleted records hard-removed (M857)
	KindMemorySuperseded Kind = "memory.superseded" // a record replaced by a newer version
	// KindMemoryConsolidated (M804): one brain-distillation pass merged
	// clusters of related records into consolidated summaries.
	KindMemoryConsolidated Kind = "memory.consolidated"
	// KindMemoryPromoted (M915): a private (scoped) record was shared — its
	// scope tag cleared so it joins the brain every agent recalls.
	KindMemoryPromoted Kind = "memory.promoted"

	// World model (SPEC-05 §3). The entity/relation graph is content-
	// addressed and journaled so `agt why` can explain why "the portfolio"
	// resolves to those repos, and so the graph is diffable/revertible.
	KindWorldEntityUpserted   Kind = "worldmodel.entity.upserted"   // a node created/reinforced
	KindWorldRelationUpserted Kind = "worldmodel.relation.upserted" // an edge created/reinforced
	KindWorldRetrieved        Kind = "worldmodel.retrieved"         // entities resolved into a run's context
	KindWorldForgotten        Kind = "worldmodel.forgotten"         // a node/edge tombstoned (soft delete)
	KindWorldSuperseded       Kind = "worldmodel.superseded"        // a node replaced by a newer version

	// Forge — auditable self-improvement (SPEC-05 §5). Skill lifecycle is a
	// journaled state machine so `agt skill history` and `agt why` explain
	// every create/promote/quarantine, and revert is non-destructive.
	KindSkillCreated     Kind = "skill.created"          // a draft skill authored (by Forge or operator)
	KindSkillPromoted    Kind = "skill.promoted"         // draft→shadow→active (or un-quarantine)
	KindSkillQuarantined Kind = "skill.quarantined"      // pulled from production
	KindSkillReverted    Kind = "skill.reverted"         // a reversal appended (never an edit)
	KindSkillActivated   Kind = "skill.activated"        // active skills injected into a run's context
	KindSkillShadowEval  Kind = "skill.shadow_evaluated" // a shadow skill judged against a completed run (M400)
	KindSkillShared      Kind = "skill.shared"           // a private (per-agent) skill promoted to the shared pool (M942)
	KindSkillReassigned  Kind = "skill.reassigned"       // a skill's owning agent changed (M942)

	// Chronos standing orders (SPEC-16 §4) — persistent goals; their lifecycle
	// is journaled so the changelog / `agt standing` can explain them.
	KindStandingCreated Kind = "standing.created"
	KindStandingUpdated Kind = "standing.updated" // paused/resumed/edited
	KindStandingRemoved Kind = "standing.removed"
	KindStandingFired   Kind = "standing.fired" // a trigger matched → the order's plan was launched
	// KindStandingError records a recovered panic while running a fired order, so a
	// crash in one order's plan (a buggy tool/plugin reached via the run) stays
	// diagnosable (`agt journal`) instead of taking down the whole daemon.
	KindStandingError Kind = "standing.error"

	// Agent roster (M783) — durable named agent profiles (slug + soul + model
	// + budget + memory scope). Lifecycle is journaled so `agt why` and the
	// changelog can explain how an agent came to exist.
	KindRosterCreated Kind = "roster.created"
	KindRosterUpdated Kind = "roster.updated" // edited/paused/resumed
	KindRosterRemoved Kind = "roster.removed"

	// Reflection — meta-cognition (SPEC-05 §6). The system reviews its own
	// behaviour from the journal and recalibrates; the report (observations,
	// adjustments applied, advisory proposals) is itself journaled.
	KindReflectionCompleted Kind = "reflection.completed"

	// Journal self-events (used for snapshot/verify boundaries).
	KindJournalSegmentRotated Kind = "journal.segment_rotated"

	// Outbound webhooks (P7-API-02). The webhook dispatcher POSTs journal
	// events to operator-configured endpoints; each delivery attempt's outcome
	// is itself journaled so `agt journal grep webhook` audits what left the
	// system. The dispatcher never re-delivers webhook.* events (no feedback
	// loop).
	KindWebhookDelivered Kind = "webhook.delivered" // a 2xx delivery
	KindWebhookFailed    Kind = "webhook.failed"    // exhausted retries (error or non-2xx)

	// Scheduled intents (autonomy). The cadence resident fires operator-
	// configured intents on a timer through the normal governed loop; each
	// firing is journaled so `agt journal grep schedule` shows what the system
	// did on its own and `agt why` links the firing to the resulting run.
	KindScheduleFired Kind = "schedule.fired"

	// KindAssureVerdict is journaled by the "do-it-for-sure" loop after each
	// attempt's completion check (M651): it carries the attempt number, whether
	// the verifier judged the task complete, and the remaining gap if not — so
	// `agt why` can show why an assured run retried (or stopped).
	KindAssureVerdict Kind = "assure.verdict"

	// KindBoardPosted is published when an agent posts to the shared message board
	// (M656). Its subject is "board.<topic>", so a standing order can trigger on a
	// specific topic (e.g. "board.acil-mudahale") or all of them ("board.*") — the
	// primitive that lets one agent's post WAKE another agent. Payload carries the
	// topic, the optional from-role, and the message length.
	KindBoardPosted Kind = "board.posted"

	// KindCodeExecuted is published once per `code_exec` run (M683): the agent
	// wrote and ran a program. Payload carries {language, project?, code_bytes,
	// exit_code, timed_out, net, profile_effective, duration_ms} so `agt why` and
	// the run timeline show what code ran and how it ended. The warden additionally
	// emits its own warden.exec for the underlying process.
	KindCodeExecuted Kind = "code.executed"

	// Script-tool forge (M794) — agent-authored code promoted into durable,
	// callable tools (forge_<name>), executed through the code-exec sandbox.
	// Lifecycle is journaled so `agt why` can explain how a tool came to be,
	// who tested it, and when it went live (or was pulled).
	KindScriptToolCreated     Kind = "scripttool.created"
	KindScriptToolUpdated     Kind = "scripttool.updated" // edit (code changes demote to draft)
	KindScriptToolTested      Kind = "scripttool.tested"  // a sandbox test of the current code (ok true/false)
	KindScriptToolPromoted    Kind = "scripttool.promoted"
	KindScriptToolQuarantined Kind = "scripttool.quarantined"
	KindScriptToolRemoved     Kind = "scripttool.removed"

	// MCP self-install (M796) — runtime-attached Model Context Protocol
	// servers. Registering/attaching is the governed self-install path
	// (`mcp.install` Edict capability); every lifecycle transition is
	// journaled so `agt why` explains how a server's tools became callable.
	KindMCPAdded    Kind = "mcp.added"
	KindMCPUpdated  Kind = "mcp.updated" // enabled/disabled (auto-attach flag)
	KindMCPAttached Kind = "mcp.attached"
	KindMCPDetached Kind = "mcp.detached"
	KindMCPRemoved  Kind = "mcp.removed"

	// Workflow engine (M798) — durable typed-node graphs. CRUD plus one
	// started/node.../completed|failed arc per run, all under subject
	// "workflow.<name>", so the console canvas can replay a run live and
	// `agt why` explains what each node did.
	KindWorkflowSaved     Kind = "workflow.saved"
	KindWorkflowUpdated   Kind = "workflow.updated" // enabled/disabled
	KindWorkflowRemoved   Kind = "workflow.removed"
	KindWorkflowStarted   Kind = "workflow.started"
	KindWorkflowNode      Kind = "workflow.node" // one node executed (ok/error, fired port)
	KindWorkflowCompleted Kind = "workflow.completed"
	KindWorkflowFailed    Kind = "workflow.failed"
	// KindWorkflowDrafted (M802): the copilot turned a plain-language
	// description into a validated (but UNSAVED) workflow graph.
	KindWorkflowDrafted Kind = "workflow.drafted"
)

// IsKnown reports whether k is one of the kinds defined in this file. Useful
// for guarding against typos in tests; the journal accepts any Kind so that
// future layers can add kinds without changing this file at the same moment.
func IsKnown(k Kind) bool {
	_, ok := knownKinds[k]
	return ok
}

var knownKinds = map[Kind]struct{}{
	KindInfo:                      {},
	KindAgentSpawned:              {},
	KindAgentSuspended:            {},
	KindAgentResumed:              {},
	KindAgentDied:                 {},
	KindAgentCrashed:              {},
	KindTaskReceived:              {},
	KindTaskCompleted:             {},
	KindTaskAbandoned:             {},
	KindTaskFailed:                {},
	KindSubAgentSpawned:           {},
	KindSubAgentCompleted:         {},
	KindToolInvoked:               {},
	KindToolResult:                {},
	KindLLMRequest:                {},
	KindLLMResponse:               {},
	KindLLMToken:                  {},
	KindLLMReasoning:              {},
	KindHalt:                      {},
	KindAnomalyDetected:           {},
	KindResume:                    {},
	KindPolicyDecision:            {},
	KindPolicyChanged:             {},
	KindRoutingDecision:           {},
	KindBudgetConsumed:            {},
	KindProviderFallback:          {},
	KindProviderRetry:             {},
	KindBudgetExceeded:            {},
	KindRateLimited:               {},
	KindBudgetCapInert:            {},
	KindBudgetUnpriced:            {},
	KindBudgetCeilingSet:          {},
	KindRunPaused:                 {},
	KindRunResumed:                {},
	KindRunStepped:                {},
	KindRunSteered:                {},
	KindTaskContinued:             {},
	KindCouncilConvened:           {},
	KindCouncilOpinion:            {},
	KindCouncilConsensus:          {},
	KindPolicyCompacted:           {},
	KindNetguardBlocked:           {},
	KindCapabilityRejected:        {},
	KindCapabilityRerouted:        {},
	KindCapabilityDegraded:        {},
	KindContextCompacted:          {},
	KindWardenExecuted:            {},
	KindWardenProfileDowngraded:   {},
	KindWardenLimitExceeded:       {},
	KindApprovalRequested:         {},
	KindApprovalGranted:           {},
	KindApprovalDenied:            {},
	KindApprovalTimeout:           {},
	KindPlanStarted:               {},
	KindPlanCompleted:             {},
	KindPlanFailed:                {},
	KindNodeStarted:               {},
	KindNodeCompleted:             {},
	KindNodeFailed:                {},
	KindCatalogSynced:             {},
	KindCatalogSyncFailed:         {},
	KindCatalogDiscoveryCompleted: {},
	KindCatalogDiscoveryFailed:    {},
	KindChannelInbound:            {},
	KindChannelOutbound:           {},
	KindChannelError:              {},
	KindPulseTick:                 {},
	KindObserverDelta:             {},
	KindSalienceScored:            {},
	KindInitiativeTaken:           {},
	KindBriefingSent:              {},
	KindPulsePaused:               {},
	KindPulseResumed:              {},
	KindMemoryWritten:             {},
	KindMemoryRetrieved:           {},
	KindMemoryForgotten:           {},
	KindMemoryPruned:              {},
	KindMemorySuperseded:          {},
	KindMemoryConsolidated:        {},
	KindMemoryPromoted:            {},
	KindWorldEntityUpserted:       {},
	KindWorldRelationUpserted:     {},
	KindWorldRetrieved:            {},
	KindWorldForgotten:            {},
	KindWorldSuperseded:           {},
	KindSkillCreated:              {},
	KindSkillPromoted:             {},
	KindSkillQuarantined:          {},
	KindSkillReverted:             {},
	KindSkillActivated:            {},
	KindSkillShadowEval:           {},
	KindSkillShared:               {},
	KindSkillReassigned:           {},
	KindStandingCreated:           {},
	KindStandingUpdated:           {},
	KindStandingRemoved:           {},
	KindStandingFired:             {},
	KindStandingError:             {},
	KindRosterCreated:             {},
	KindRosterUpdated:             {},
	KindRosterRemoved:             {},
	KindReflectionCompleted:       {},
	KindJournalSegmentRotated:     {},
	KindWebhookDelivered:          {},
	KindWebhookFailed:             {},
	KindScheduleFired:             {},
	KindAssureVerdict:             {},
	KindBoardPosted:               {},
	KindCodeExecuted:              {},
	KindScriptToolCreated:         {},
	KindScriptToolUpdated:         {},
	KindScriptToolTested:          {},
	KindScriptToolPromoted:        {},
	KindScriptToolQuarantined:     {},
	KindScriptToolRemoved:         {},
	KindMCPAdded:                  {},
	KindMCPUpdated:                {},
	KindMCPAttached:               {},
	KindMCPDetached:               {},
	KindMCPRemoved:                {},
	KindWorkflowSaved:             {},
	KindWorkflowUpdated:           {},
	KindWorkflowRemoved:           {},
	KindWorkflowStarted:           {},
	KindWorkflowNode:              {},
	KindWorkflowCompleted:         {},
	KindWorkflowFailed:            {},
	KindWorkflowDrafted:           {},
}

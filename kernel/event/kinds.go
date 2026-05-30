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

	// Multi-agent orchestration (P6-MULTI-01). A lead agent delegates a
	// bounded task to a sub-agent via the `delegate` tool; the spawn is
	// journaled under the PARENT correlation (carrying the child correlation)
	// so `agt why <parent>` shows the delegation, and the child correlation is
	// the drill-down into the sub-agent's own run.
	KindSubAgentSpawned Kind = "subagent.spawned"

	// Tool calls (the in-process Tool interface, DECISIONS B0a).
	KindToolInvoked Kind = "tool.invoked"
	KindToolResult  Kind = "tool.result"

	// LLM/provider traffic (canonical, dialect-free; SPEC-15).
	KindLLMRequest  Kind = "llm.request"
	KindLLMResponse Kind = "llm.response"
	KindLLMToken    Kind = "llm.token"

	// Control plane (P0-CTRL-*).
	KindHalt   Kind = "halt"
	KindResume Kind = "resume"

	// Policy / Edict (P1-EDICT-*).
	KindPolicyDecision Kind = "policy.decision"

	// Governor (P1-CONDUIT-*).
	KindRoutingDecision  Kind = "routing.decision"
	KindBudgetConsumed   Kind = "budget.consumed"
	KindProviderFallback Kind = "provider.fallback"
	KindBudgetExceeded   Kind = "budget.exceeded"

	// Warden (P1-WARD-*).
	KindWardenExecuted          Kind = "warden.executed"
	KindWardenProfileDowngraded Kind = "warden.profile_downgraded"
	KindWardenLimitExceeded     Kind = "warden.limit_exceeded"

	// Approval / HITL (SPEC-06 §3.4).
	KindApprovalRequested Kind = "approval.requested"
	KindApprovalGranted   Kind = "approval.granted"
	KindApprovalDenied    Kind = "approval.denied"
	KindApprovalTimeout   Kind = "approval.timeout"

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

	// Memory-lite (SPEC-05 §2; ROADMAP §2.3). The store is content-
	// addressed and journaled so `agt why` can explain every belief.
	KindMemoryWritten    Kind = "memory.written"    // a record created/reinforced/revived
	KindMemoryRetrieved  Kind = "memory.retrieved"  // records surfaced into a run's context
	KindMemoryForgotten  Kind = "memory.forgotten"  // a record tombstoned (soft delete)
	KindMemorySuperseded Kind = "memory.superseded" // a record replaced by a newer version

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
	KindSkillCreated     Kind = "skill.created"     // a draft skill authored (by Forge or operator)
	KindSkillPromoted    Kind = "skill.promoted"    // draft→shadow→active (or un-quarantine)
	KindSkillQuarantined Kind = "skill.quarantined" // pulled from production
	KindSkillReverted    Kind = "skill.reverted"    // a reversal appended (never an edit)
	KindSkillActivated   Kind = "skill.activated"   // active skills injected into a run's context

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
)

// IsKnown reports whether k is one of the kinds defined in this file. Useful
// for guarding against typos in tests; the journal accepts any Kind so that
// future layers can add kinds without changing this file at the same moment.
func IsKnown(k Kind) bool {
	_, ok := knownKinds[k]
	return ok
}

var knownKinds = map[Kind]struct{}{
	KindAgentSpawned:              {},
	KindAgentSuspended:            {},
	KindAgentResumed:              {},
	KindAgentDied:                 {},
	KindAgentCrashed:              {},
	KindTaskReceived:              {},
	KindTaskCompleted:             {},
	KindSubAgentSpawned:           {},
	KindToolInvoked:               {},
	KindToolResult:                {},
	KindLLMRequest:                {},
	KindLLMResponse:               {},
	KindLLMToken:                  {},
	KindHalt:                      {},
	KindResume:                    {},
	KindPolicyDecision:            {},
	KindRoutingDecision:           {},
	KindBudgetConsumed:            {},
	KindProviderFallback:          {},
	KindBudgetExceeded:            {},
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
	KindMemorySuperseded:          {},
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
	KindReflectionCompleted:       {},
	KindJournalSegmentRotated:     {},
	KindWebhookDelivered:          {},
	KindWebhookFailed:             {},
	KindScheduleFired:             {},
}

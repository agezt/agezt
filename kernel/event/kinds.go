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
	KindCatalogSynced              Kind = "catalog.synced"
	KindCatalogSyncFailed          Kind = "catalog.sync_failed"
	KindCatalogDiscoveryCompleted  Kind = "catalog.discovery_completed"
	KindCatalogDiscoveryFailed     Kind = "catalog.discovery_failed"

	// Journal self-events (used for snapshot/verify boundaries).
	KindJournalSegmentRotated Kind = "journal.segment_rotated"
)

// IsKnown reports whether k is one of the kinds defined in this file. Useful
// for guarding against typos in tests; the journal accepts any Kind so that
// future layers can add kinds without changing this file at the same moment.
func IsKnown(k Kind) bool {
	_, ok := knownKinds[k]
	return ok
}

var knownKinds = map[Kind]struct{}{
	KindAgentSpawned:          {},
	KindAgentSuspended:        {},
	KindAgentResumed:          {},
	KindAgentDied:             {},
	KindAgentCrashed:          {},
	KindTaskReceived:          {},
	KindTaskCompleted:         {},
	KindToolInvoked:           {},
	KindToolResult:            {},
	KindLLMRequest:            {},
	KindLLMResponse:           {},
	KindLLMToken:              {},
	KindHalt:                  {},
	KindResume:                {},
	KindPolicyDecision:        {},
	KindRoutingDecision:         {},
	KindBudgetConsumed:          {},
	KindProviderFallback:        {},
	KindBudgetExceeded:          {},
	KindWardenExecuted:          {},
	KindWardenProfileDowngraded: {},
	KindWardenLimitExceeded:     {},
	KindApprovalRequested:       {},
	KindApprovalGranted:         {},
	KindApprovalDenied:          {},
	KindApprovalTimeout:         {},
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
	KindJournalSegmentRotated:     {},
}

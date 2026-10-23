// SPDX-License-Identifier: MIT

// Package api is the agents module's public vocabulary: the rows the module
// publishes, the ports it needs from the host and the surface the host talks
// to.
//
// It exists because another module may import a module's api package and
// nothing else (archcheck's principle P5). The runtime keeps its own names as
// aliases of these rows, so the host's public API is unchanged while the
// implementation lives in the module.
//
// Rules of the road:
//
//   - Types, ports and the surface only. The scanning itself lives in the
//     module's implementation package (kernel/modules/agents/reaper); what
//     arrives here is what both sides must name.
//   - One type, one home. A row both the host and the module name is declared
//     here exactly once; the other side aliases it.
package api

import (
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// The finding rows below are the reaper's rows: one declaration per finding,
// published as they are. Every field is always present — the empty optional
// fields are explicit rather than omitted — so the operator's scan (the
// application layer) and the module never drift apart. The rows are read-only
// values: nothing here is marshalled on its own.

// ReaperAgent is a dead-agent candidate: an enabled, non-retired roster agent,
// old enough to judge, with no task activity since the idle cutoff.
type ReaperAgent struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	LastActiveMS int64  `json:"last_active_ms"` // 0 = never ran a task
}

// DegradedAgent is an enabled roster agent whose recent terminal runs crossed
// its profile health threshold. Detection only: repair/escalation remains a
// separate action by a guardian/doctor or operator.
type DegradedAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Failures          int    `json:"failures"`
	Window            int    `json:"window"`
	Threshold         int    `json:"threshold"`
	DoctorAgent       string `json:"doctor_agent"`
	SelfRepairEnabled bool   `json:"self_repair_enabled"`
	EscalateTo        string `json:"escalate_to"`
	LastFailureMS     int64  `json:"last_failure_ms"`
	LastReason        string `json:"last_reason"`
}

// MisconfiguredAgent is an enabled, non-retired agent whose runtime override
// config or hierarchy references are invalid, so one or more intended autonomy
// knobs will not actually apply until repaired.
type MisconfiguredAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Issues            []string `json:"issues"`
	DoctorAgent       string   `json:"doctor_agent"`
	SelfRepairEnabled bool     `json:"self_repair_enabled"`
	EscalateTo        string   `json:"escalate_to"`
}

// RoutingPressureAgent is an enabled, non-retired agent whose recent per-task
// model-chain fallbacks crossed the routing pressure threshold. This catches
// agents that are limping along by repeatedly dropping to backup models even if
// they have not yet crossed the failed-run health threshold.
type RoutingPressureAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent"`
	SelfRepairEnabled bool   `json:"self_repair_enabled"`
	EscalateTo        string `json:"escalate_to"`
	LastFallbackMS    int64  `json:"last_fallback_ms"`
	LastReason        string `json:"last_reason"`
	LastFailedModel   string `json:"last_failed_model"`
	LastNextModel     string `json:"last_next_model"`
	TaskType          string `json:"task_type"`
}

// RetryPressureAgent is an enabled, non-retired agent whose recent runs hit
// transient provider failures often enough to cross the retry-pressure
// threshold: it catches agents that are still trying to recover before they
// fully fail the health window.
type RetryPressureAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent"`
	SelfRepairEnabled bool   `json:"self_repair_enabled"`
	EscalateTo        string `json:"escalate_to"`
	LastRetryMS       int64  `json:"last_retry_ms"`
	LastReason        string `json:"last_reason"`
	NextAttempt       int    `json:"next_attempt"`
	MaxAttempts       int    `json:"max_attempts"`
}

// RoutingForcedAgent is an enabled, non-retired agent whose model chain was
// forced by a manager or owner. The three forced findings — still in probation,
// failed after it, or exhausted by repeated forced generations — are one row;
// the report keeps one list per phase.
//
// Provenance: the probation, failed and exhausted findings were three
// identical declarations of the same row; they are one declaration now.
type RoutingForcedAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent"`
	SelfRepairEnabled bool     `json:"self_repair_enabled"`
	EscalateTo        string   `json:"escalate_to"`
	LastFallbackMS    int64    `json:"last_fallback_ms"`
	LastForcedMS      int64    `json:"last_forced_ms"`
	LastReason        string   `json:"last_reason"`
	TaskType          string   `json:"task_type"`
	ForcedChain       []string `json:"forced_chain"`
	ForceGeneration   int      `json:"routing_force_generation"`
}

// The three forced phases keep their names — the host's public API is
// unchanged — but they are one row.
type RoutingForcedProbationAgent = RoutingForcedAgent
type RoutingForcedFailedAgent = RoutingForcedAgent
type RoutingForcedExhaustedAgent = RoutingForcedAgent

// RoutingUnstableAgent is an enabled, non-retired agent whose routing repair
// loop already rolled back recently and is still under model-chain fallback
// pressure for the same task type. This is a stronger signal than plain
// routing pressure: self-repair already tried to retune the chain and the
// chain still destabilized again.
type RoutingUnstableAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent"`
	SelfRepairEnabled bool     `json:"self_repair_enabled"`
	EscalateTo        string   `json:"escalate_to"`
	LastRollbackMS    int64    `json:"last_rollback_ms"`
	TaskType          string   `json:"task_type"`
	CurrentChain      []string `json:"current_chain"`
	PreviousChain     []string `json:"previous_chain"`
	LastReason        string   `json:"last_reason"`
}

// ReaperReport is the read-only result of a scan: dead-agent candidates plus
// stale-artifact totals. Detection only.
type ReaperReport struct {
	DeadAgents             []ReaperAgent                 `json:"dead_agents"`
	DegradedAgents         []DegradedAgent               `json:"degraded_agents,omitempty"`
	MisconfiguredAgents    []MisconfiguredAgent          `json:"misconfigured_agents,omitempty"`
	RetryPressure          []RetryPressureAgent          `json:"retry_pressure_agents,omitempty"`
	RoutingPressure        []RoutingPressureAgent        `json:"routing_pressure_agents,omitempty"`
	RoutingForced          []RoutingForcedProbationAgent `json:"routing_forced_probation_agents,omitempty"`
	RoutingForcedFailed    []RoutingForcedFailedAgent    `json:"routing_forced_failed_agents,omitempty"`
	RoutingForcedExhausted []RoutingForcedExhaustedAgent `json:"routing_forced_exhausted_agents,omitempty"`
	RoutingUnstable        []RoutingUnstableAgent        `json:"routing_unstable_agents,omitempty"`
	StaleArtifacts         int                           `json:"stale_artifacts"`
	StaleBytes             int64                         `json:"stale_bytes"`
}

// Empty reports whether the scan found nothing to reap.
func (r ReaperReport) Empty() bool {
	return len(r.DeadAgents) == 0 && len(r.DegradedAgents) == 0 && len(r.MisconfiguredAgents) == 0 && len(r.RetryPressure) == 0 && len(r.RoutingPressure) == 0 && len(r.RoutingForced) == 0 && len(r.RoutingForcedFailed) == 0 && len(r.RoutingForcedExhausted) == 0 && len(r.RoutingUnstable) == 0 && r.StaleArtifacts == 0
}

// ConfigIssue is one malformed runtime override on an agent profile. The host
// validates the overrides it actually consumes (the daemon's own table of
// knobs), so the module never has to know that table.
type ConfigIssue struct {
	Key   string
	Value string
	Issue string
}

// Binding is one trigger that names an agent: a schedule or a standing order.
// The module only needs to know WHICH agent a trigger is bound to and how to
// address it in an operator-facing message.
type Binding struct {
	// ID is the trigger's identifier ("schedule:<id>", "standing:<id>").
	ID string
	// Agent is the roster slug the trigger runs as (empty = none).
	Agent string
}

// ReaperPorts are the module's ports: everything the reaper needs from the
// host. A module may not reach into the host's kernel, so the host supplies
// each capability explicitly.
//
// Every port is optional: a host that cannot supply one gets a nil port and
// the module skips that finding (a fleet with no artifact index has no stale
// artifacts to report).
type ReaperPorts struct {
	// Journal walks the durable journal once. The scan reads the task,
	// retry, provider-fallback and doctor events it needs from it.
	Journal func(func(*event.Event) error) error
	// Profiles lists the fleet the scan judges.
	Profiles func() []roster.Profile
	// RuntimeConfigIssues reports the malformed runtime overrides on a
	// profile (the doctor calls it too, so the two never disagree).
	RuntimeConfigIssues func(overrides map[string]string) []ConfigIssue
	// StaleArtifacts counts the artifacts older than the cutoff and their
	// total size.
	StaleArtifacts func(cutoffMS int64) (count int, bytes int64)
	// TaskModelChain returns the live model chain for a task type (the
	// governor's current routing), if the host has one.
	TaskModelChain func(taskType string) []string
	// Schedules lists the schedule bindings.
	Schedules func() []Binding
	// Standing lists the standing-order bindings.
	Standing func() []Binding
}

// ReaperScanner is the module's surface: the read-only scan the host asks
// for. The cutoffs are absolute wall-clock milliseconds — the caller passes
// `now - grace`.
type ReaperScanner interface {
	// Scan finds dead-agent candidates (enabled, non-retired, created before
	// the agent idle cutoff, and with no task activity at/after it) and counts
	// stale artifacts (created before the artifact stale cutoff). Read-only.
	Scan(agentIdleCutoffMs, artifactStaleCutoffMs int64) ReaperReport
}

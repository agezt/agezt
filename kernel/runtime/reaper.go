// SPDX-License-Identifier: MIT

package runtime

// The reaper (#53) finds what's gone stale — agents that look abandoned and
// artifacts past their useful life — so they can be retired to the graveyard
// (roster.SetRetired, M846) or collected (artifact Collect, M845). This file is
// the DETECTION half: read-only scans. It mutates nothing — retire/collect stay
// operator-gated. The pulse ReaperObserver (kernel/pulse) runs ReaperScan on a
// cadence to surface candidates autonomously; the control plane exposes it
// on-demand for `agt reaper` and the UI.

import (
)

// ReaperAgent is a dead-agent candidate: an enabled, non-retired roster agent,
// old enough to judge, with no task activity since the idle cutoff.
type ReaperAgent struct {
	Slug         string `json:"slug"`
	Name         string `json:"name,omitempty"`
	LastActiveMS int64  `json:"last_active_ms"` // 0 = never ran a task
}

// DegradedAgent is an enabled roster agent whose recent terminal runs crossed
// its profile health threshold. Detection only: repair/escalation remains a
// separate action by a guardian/doctor or operator.
type DegradedAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name,omitempty"`
	Failures          int    `json:"failures"`
	Window            int    `json:"window"`
	Threshold         int    `json:"threshold"`
	DoctorAgent       string `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool   `json:"self_repair_enabled,omitempty"`
	EscalateTo        string `json:"escalate_to,omitempty"`
	LastFailureMS     int64  `json:"last_failure_ms,omitempty"`
	LastReason        string `json:"last_reason,omitempty"`
}

// MisconfiguredAgent is an enabled, non-retired agent whose runtime override
// config or hierarchy references are invalid, so one or more intended autonomy
// knobs will not actually apply until repaired.
type MisconfiguredAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Issues            []string `json:"issues,omitempty"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
}

// RoutingPressureAgent is an enabled, non-retired agent whose recent per-task
// model-chain fallbacks crossed the routing pressure threshold. This catches
// agents that are limping along by repeatedly dropping to backup models even if
// they have not yet crossed the failed-run health threshold.
type RoutingPressureAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name,omitempty"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool   `json:"self_repair_enabled,omitempty"`
	EscalateTo        string `json:"escalate_to,omitempty"`
	LastFallbackMS    int64  `json:"last_fallback_ms,omitempty"`
	LastReason        string `json:"last_reason,omitempty"`
	LastFailedModel   string `json:"last_failed_model,omitempty"`
	LastNextModel     string `json:"last_next_model,omitempty"`
	TaskType          string `json:"task_type,omitempty"`
}

// RetryPressureAgent is an enabled, non-retired agent whose whole-run retry
// policy is being exercised repeatedly. This is separate from failed terminal
// runs: it catches agents that are still trying to recover before they fully
// fail the health window.
type RetryPressureAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name,omitempty"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool   `json:"self_repair_enabled,omitempty"`
	EscalateTo        string `json:"escalate_to,omitempty"`
	LastRetryMS       int64  `json:"last_retry_ms,omitempty"`
	LastReason        string `json:"last_reason,omitempty"`
	NextAttempt       int    `json:"next_attempt,omitempty"`
	MaxAttempts       int    `json:"max_attempts,omitempty"`
}

// RoutingForcedProbationAgent is an enabled, non-retired agent that is still
// under model-chain fallback pressure, but a manager/owner recently forced a
// specific routing chain and that same chain is still active. The doctor layer
// should observe the probation window before retuning it again.
type RoutingForcedProbationAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastFallbackMS    int64    `json:"last_fallback_ms,omitempty"`
	LastForcedMS      int64    `json:"last_forced_ms,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	ForcedChain       []string `json:"forced_chain,omitempty"`
	ForceGeneration   int      `json:"routing_force_generation,omitempty"`
}

type RoutingForcedFailedAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastFallbackMS    int64    `json:"last_fallback_ms,omitempty"`
	LastForcedMS      int64    `json:"last_forced_ms,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	ForcedChain       []string `json:"forced_chain,omitempty"`
	ForceGeneration   int      `json:"routing_force_generation,omitempty"`
}

type RoutingForcedExhaustedAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastFallbackMS    int64    `json:"last_fallback_ms,omitempty"`
	LastForcedMS      int64    `json:"last_forced_ms,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	ForcedChain       []string `json:"forced_chain,omitempty"`
	ForceGeneration   int      `json:"routing_force_generation,omitempty"`
}

// RoutingUnstableAgent is an enabled, non-retired agent whose routing repair
// loop already rolled back recently and is still under model-chain fallback
// pressure for the same task type. This is a stronger signal than plain
// routing pressure: self-repair already tried to retune the chain and the
// chain still destabilized again.
type RoutingUnstableAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastRollbackMS    int64    `json:"last_rollback_ms,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	CurrentChain      []string `json:"current_chain,omitempty"`
	PreviousChain     []string `json:"previous_chain,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
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

// ReaperScan finds dead-agent candidates (enabled, non-retired, created before
// agentIdleCutoffMs, and with no task activity at/after it) and counts stale
// artifacts (created before artifactStaleCutoffMs). Both cutoffs are absolute
// wall-clock ms — the caller passes `now - grace`. Read-only.
//
// "Created before the cutoff" is a grace window: a freshly-added agent that
// hasn't run yet is not reaped until it's been idle past the threshold, so the
// scan never flags an agent the operator just set up.
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
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// The finding rows below are the reaper's rows: one declaration per finding,
// published as they are. Every field is always present — the empty optional
// fields are explicit rather than omitted — so the operator's scan (the
// application layer) and the module never drift apart. The rows are read-only
// values: nothing here is marshalled on its own.
//
// The ROWS live in the contract layer (agentapi): they cross the boundary —
// the module produces them, the application layer reads them, and the
// adapter hands them on. The module's api package re-exports them as aliases,
// so there is one row and one spelling: every importer compiles unchanged
// while the implementation lives in the module.
type (
	// ReaperAgent is a dead-agent candidate: an enabled, non-retired roster
	// agent, old enough to judge, with no task activity since the idle cutoff.
	ReaperAgent = agentapi.ReaperAgent
	// DegradedAgent is an enabled roster agent whose recent terminal runs
	// crossed its profile health threshold.
	DegradedAgent = agentapi.DegradedAgent
	// MisconfiguredAgent is an enabled, non-retired agent whose runtime
	// override config or hierarchy references are invalid.
	MisconfiguredAgent = agentapi.MisconfiguredAgent
	// RoutingPressureAgent is an enabled, non-retired agent whose recent
	// per-task model-chain fallbacks crossed the routing pressure threshold.
	RoutingPressureAgent = agentapi.RoutingPressureAgent
	// RetryPressureAgent is an enabled, non-retired agent whose recent runs
	// hit transient provider failures often enough to cross the threshold.
	RetryPressureAgent = agentapi.RetryPressureAgent
	// RoutingForcedAgent is an enabled, non-retired agent whose model chain
	// was forced by a manager or owner. The three forced phases are one row.
	RoutingForcedAgent = agentapi.RoutingForcedAgent
	// RoutingForcedProbationAgent is the forced finding still in probation.
	RoutingForcedProbationAgent = agentapi.RoutingForcedProbationAgent
	// RoutingForcedFailedAgent is the forced finding that failed after it.
	RoutingForcedFailedAgent = agentapi.RoutingForcedFailedAgent
	// RoutingForcedExhaustedAgent is the forced finding exhausted by
	// repeated forced generations.
	RoutingForcedExhaustedAgent = agentapi.RoutingForcedExhaustedAgent
	// RoutingUnstableAgent is an enabled, non-retired agent whose routing
	// repair loop already rolled back recently.
	RoutingUnstableAgent = agentapi.RoutingUnstableAgent
	// ReaperReport is the read-only result of a scan: dead-agent candidates
	// plus stale-artifact totals. Detection only.
	ReaperReport = agentapi.ReaperReport
)

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

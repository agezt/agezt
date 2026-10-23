// SPDX-License-Identifier: MIT

// Package introspectapi is the contract layer's home for the introspector's
// rows: the daemon's own at-a-glance health snapshot and the reaper's fleet
// scan, as the agent-facing tool publishes them.
//
// The rows live here because the tool that publishes them is a PLUGIN, and a
// plugin may reach the contract and the platform layers and nothing else. The
// implementation that ASSEMBLES them from the live kernel is the app layer's
// (kernel/app/introspect) — the app layer is the one that can see both the
// kernel and the contract, and the plugin may not name the kernel at all.
package introspectapi

import (
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/types"
)

// Delegation mirrors the active sub-agent governance ceilings (M46–M48) for the
// overview. 0 fan-out / spend / total = unbounded. It is the contract type
// itself, so the overview carries the runtime's ceilings unchanged.
type Delegation = types.SubAgentLimits

// ReaperOverview is the daemon's stale/degraded/misconfigured fleet snapshot.
type ReaperOverview struct {
	DeadAgents          int      `json:"dead_agents"`
	DegradedAgents      int      `json:"degraded_agents"`
	MisconfiguredAgents int      `json:"misconfigured_agents"`
	DeadSlugs           []string `json:"dead_slugs,omitempty"`
	DegradedSlugs       []string `json:"degraded_slugs,omitempty"`
	MisconfiguredSlugs  []string `json:"misconfigured_slugs,omitempty"`
}

type ReaperAgent struct {
	Slug         string `json:"slug"`
	Name         string `json:"name,omitempty"`
	LastActiveMS int64  `json:"last_active_ms,omitempty"`
}

type ReaperDegradedAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name,omitempty"`
	Failures          int    `json:"failures,omitempty"`
	Window            int    `json:"window,omitempty"`
	Threshold         int    `json:"threshold,omitempty"`
	DoctorAgent       string `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool   `json:"self_repair_enabled,omitempty"`
	EscalateTo        string `json:"escalate_to,omitempty"`
	LastFailureMS     int64  `json:"last_failure_ms,omitempty"`
	LastReason        string `json:"last_reason,omitempty"`
}

type ReaperMisconfiguredAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Issues            []string `json:"issues,omitempty"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
}

type ReaperReport struct {
	DeadAgents          []ReaperAgent              `json:"dead_agents,omitempty"`
	DegradedAgents      []ReaperDegradedAgent      `json:"degraded_agents,omitempty"`
	MisconfiguredAgents []ReaperMisconfiguredAgent `json:"misconfigured_agents,omitempty"`
	StaleArtifacts      int                        `json:"stale_artifacts"`
	StaleBytes          int64                      `json:"stale_bytes"`
}

// Overview is the daemon's at-a-glance health snapshot — the same shape `agt
// status` assembles, minus the server-level (HTTP/channel) extras the kernel
// doesn't own. Assembled by the Source (the kernel adapter); the tool just
// formats it.
type Overview struct {
	Daemon                 string         `json:"daemon"`
	Protocol               int            `json:"protocol"`
	Model                  string         `json:"model"`
	UptimeSeconds          int64          `json:"uptime_seconds"`
	Halted                 bool           `json:"halted"`
	ActiveRuns             int            `json:"active_runs"`
	Tools                  []string       `json:"tools"`
	MemoryRecords          int            `json:"memory_records"`
	WorldEntities          int            `json:"world_entities"`
	ActiveSkills           int            `json:"active_skills"`
	JournalHead            int64          `json:"journal_head"`
	SchedulesTotal         int            `json:"schedules_total"`
	SchedulesEnabled       int            `json:"schedules_enabled"`
	PendingApprovals       int            `json:"pending_approvals"`
	ProviderFallbacks      int            `json:"provider_fallbacks"`
	ProviderFallbackReason string         `json:"provider_fallback_reason,omitempty"`
	Delegation             Delegation     `json:"delegation"`
	Reaper                 ReaperOverview `json:"reaper"`
}

// Source is the narrow live-state surface the tool reads. The app layer's
// kernel adapter (kernel/app/introspect) implements it against the real
// *runtime.Kernel; tests inject a fake without standing up a daemon.
type Source interface {
	// Overview returns the daemon's at-a-glance health snapshot.
	Overview() Overview
	// Reaper returns the current dead/degraded/misconfigured fleet scan.
	Reaper() ReaperReport
	// Schedules returns every cadence entry (operator- and agent-created).
	Schedules() []cadenceapi.Entry
	// Standing returns every standing order on this daemon.
	Standing() []standingapi.Order
}

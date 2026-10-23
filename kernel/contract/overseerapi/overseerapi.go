// SPDX-License-Identifier: MIT

// Package overseerapi is the contract the supervisory surface and the
// implementations that answer it agree on.
//
// The overseer lets a privileged agent supervise and intervene on the fleet:
// see who is running, cancel a runaway run, halt or resume the daemon, pause
// / retire / revive agents, triage the board's open help requests. The tool
// that exposes it to agents is a PLUGIN, and a plugin may only reach the
// contract and the platform layers (archcheck's plugin rule) — so the port
// through which the live kernel answers the tool has to live here.
//
// The implementation does NOT live here: answering the port needs the live
// kernel (the runs module) and the board store (the board module), and the
// app layer is the only layer that may see both.
//
// The rules of the road:
//
//   - Ports and the rows they carry only. No store, no kernel, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package overseerapi

import (
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// Source is the narrow slice of the live kernel the overseer steers — an
// interface so the tool is testable without a real daemon (a fake Source + an
// in-memory board is enough).
type Source interface {
	// Read.
	IsHalted() bool
	ActiveRunIDs() []string
	Agents() []agentapi.Profile
	OpenHelp(limit int) []boardapi.Message
	AgentImpact(slug string) []string
	// Intervene. Each mirrors an operator control and journals through the kernel.
	CancelRun(corr string) bool
	Halt(reason string)
	ResumeAll(reason string)
	SetAgentEnabled(ref string, enabled bool) (agentapi.Profile, error)
	SetAgentRetired(ref string, retired bool, reason string) (agentapi.Profile, error)
	// EditAgent applies the mutable fields of `in` to the agent named by ref
	// (the same set the webui's agent-edit allows, including config_overrides
	// and governance knobs). The System flag is never touched. CreateAgent adds
	// a brand-new agent. Both let a guardian "treat" or build other agents —
	// the agent-facing half of the webui's roster admin (M961).
	EditAgent(ref string, in agentapi.Profile) (agentapi.Profile, error)
	CreateAgent(in agentapi.Profile) (agentapi.Profile, error)
	// DeleteAgent permanently removes a non-System agent by slug or id.
	// Returns true when the agent existed. System agents are refused — pause
	// or retire them instead.
	DeleteAgent(ref string) (bool, error)
	// GetAgent returns the full profile for a single agent by slug or id.
	// Returns false when the agent does not exist.
	GetAgent(ref string) (agentapi.Profile, bool, error)
	// CloneAgent creates a new agent by copying an existing profile's fields
	// and applying the caller's overrides on top. The slug in `overrides` is
	// required. Source must be an existing agent. System flag is never copied.
	CloneAgent(source string, overrides agentapi.Profile) (agentapi.Profile, error)
	// SearchAgents returns agents matching the given filter criteria. An empty
	// filter returns all non-retired agents (same as Agents()). All filter fields
	// are AND-ed together; empty/nil fields are ignored.
	SearchAgents(filter SearchFilter) []agentapi.Profile
	// BulkSetEnabled pauses (false) or resumes (true) multiple agents at once.
	// Returns one result per slug, continuing past individual errors.
	BulkSetEnabled(slugs []string, enabled bool) []BulkResult
	// BulkSetRetired retires (true) or revives (false) multiple agents at once.
	// Returns one result per slug, continuing past individual errors.
	BulkSetRetired(slugs []string, retired bool, reason string) []BulkResult
	// BulkDelete permanently removes multiple non-System agents at once.
	// Returns one result per slug, continuing past individual errors.
	BulkDelete(slugs []string) []BulkResult
	// WakeAgent explicitly wakes a named agent now, asynchronously, with an
	// intent or reason. Returns the run correlation id immediately.
	WakeAgent(ref, intent, reason string) (string, error)
	// RepairAgent runs a governed self-repair pass AS the target agent, optionally
	// auto-applying a closing profile proposal from the run's final answer.
	RepairAgent(ref, reason string) (agentapi.RepairResult, error)
}

// BulkResult is one entry in a batch operation response.
type BulkResult struct {
	Slug    string `json:"slug"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// SearchFilter is the set of optional criteria for op=search. All fields are
// AND-ed together; empty/nil fields are ignored (no filter on that axis).
type SearchFilter struct {
	Query       string   `json:"query,omitempty"`        // substring match on slug, name, description
	State       string   `json:"state,omitempty"`        // enabled | paused | retired
	Model       string   `json:"model,omitempty"`        // exact model name
	TaskType    string   `json:"task_type,omitempty"`    // exact task type
	System      *bool    `json:"system,omitempty"`       // nil = all, true = only system, false = only non-system
	HasOwner    *bool    `json:"has_owner,omitempty"`    // nil = all, true = has owner_agent, false = no owner_agent
	HasParent   *bool    `json:"has_parent,omitempty"`   // nil = all, true = has parent_agent
	ToolAllowed string   `json:"tool_allowed,omitempty"` // agent has this tool in its tool_allow list
	Limit       int      `json:"limit,omitempty"`        // max results (default 100)
	Tags        []string `json:"tags,omitempty"`         // descriptive labels (future)
}

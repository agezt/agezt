// SPDX-License-Identifier: MIT

// Package agentapi is the contract the agent roster and the tools that steer
// it agree on.
//
// The roster's profile row is a contract between the module that owns the
// roster and every consumer that names an agent — the overseer tool (a
// plugin), the control plane, the Web UI. A plugin may only reach the
// contract and the platform layers, so a row a plugin has to name cannot live
// in a module's implementation package.
//
// The rules of the road:
//
//   - Rows and the pure rules over them only. No store, no validation of the
//     operator's input, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
//
// The roster's own package re-exports these types under its names, so the
// module, the host and the tools all speak the same type.
package agentapi

import (
	"strings"
)

// Profile is one named agent identity. Slug is the address — unique,
// immutable, what operators and (future) other agents refer to it by.
type Profile struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`           // unique handle, e.g. "researcher"
	Name string `json:"name,omitempty"` // human label; defaults to the slug

	// Soul is the agent's system prompt — who it IS. Applied as the run's
	// system override; memory/world/skill injection still layers on top.
	Soul string `json:"soul,omitempty"`

	// Instructions are durable operating rules for this identity. Soul says who
	// the agent is; instructions say how it should work across every wake.
	Instructions []string `json:"instructions,omitempty"`

	// Model is the primary model for this agent's runs (empty = kernel
	// default); Fallbacks is its ordered per-agent fallback chain (reserved
	// for the routing integration arc — stored and validated now so profiles
	// are forward-complete).
	Model     string   `json:"model,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`

	// TaskType is the governor task type this agent's runs default to
	// (e.g. "coding", "research"); empty = unclassified.
	TaskType string `json:"task_type,omitempty"`

	// MaxCostMc is the per-run spend ceiling in USD-microcents (0 = none).
	// Applied as the run's max_cost default; an explicit per-run cap wins.
	MaxCostMc int64 `json:"max_cost_mc,omitempty"`

	// MaxDailyMc is the per-DAY spend ceiling in USD-microcents (0 = none):
	// the Governor meters every completion this agent makes (runs, delegate
	// children, standing firings) against an identity ledger and refuses
	// once today's total reaches the ceiling (M793).
	MaxDailyMc int64 `json:"max_daily_mc,omitempty"`

	// MemoryScope is the agent's private memory scope (M652); empty = the
	// slug, so every named agent gets its own notes by default.
	MemoryScope string `json:"memory_scope,omitempty"`

	// Workdir is a workspace-relative subdirectory this agent works in
	// (reserved for the per-agent sandbox arc). Must be relative and must
	// not escape the workspace.
	Workdir string `json:"workdir,omitempty"`

	// OwnerAgent and ParentAgent model the durable agent hierarchy. OwnerAgent is
	// the supervising/owning agent (or owner's brain) responsible for this
	// profile. ParentAgent is the leader that may delegate to this profile when
	// it is a managed worker/sub-agent. They are slugs, validated syntactically
	// here and resolved by control-plane/runtime call sites when needed.
	OwnerAgent  string `json:"owner_agent,omitempty"`
	ParentAgent string `json:"parent_agent,omitempty"`

	// DirectCallable controls whether operators/schedules/channels may wake this
	// agent directly. nil or true = directly callable (default for old profiles);
	// false = managed sub-agent, callable only through delegation.
	DirectCallable *bool `json:"direct_callable,omitempty"`

	RetryPolicy      *RetryPolicy      `json:"retry_policy,omitempty"`
	HealthPolicy     *HealthPolicy     `json:"health_policy,omitempty"`
	SelfRepairPolicy *SelfRepairPolicy `json:"self_repair,omitempty"`
	NoisePolicy      *NoisePolicy      `json:"noise_policy,omitempty"`
	ToolAllow        []string          `json:"tool_allow,omitempty"`
	ToolDeny         []string          `json:"tool_deny,omitempty"`
	TrustCeiling     string            `json:"trust_ceiling,omitempty"`
	// ExecutionProfile is the agent's default isolation surface (a warden-family
	// execution profile id: local|warden|container) for its dispatched work. A
	// per-task seat that sets its own isolation overrides this; empty = tool
	// defaults. Applied on the workboard dispatch path.
	ExecutionProfile string            `json:"execution_profile,omitempty"`
	ConfigOverrides  map[string]string `json:"config_overrides,omitempty"`
	Lifecycle        AgentLifecycle    `json:"lifecycle,omitempty"`
	TaskList         []AgentTask       `json:"tasklist,omitempty"`

	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`

	// Retired moves a no-longer-needed agent to the GRAVEYARD (M846): it is kept
	// (recoverable via Revive) but excluded from delegation and marked in the
	// roster — distinct from a temporary pause (Enabled=false) and from a hard
	// Remove. RetiredMS is when it was retired; RetiredReason is the optional
	// operator/doctor/reaper note explaining why the identity was buried.
	Retired       bool   `json:"retired,omitempty"`
	RetiredMS     int64  `json:"retired_ms,omitempty"`
	RetiredReason string `json:"retired_reason,omitempty"`

	// System marks a SHIPPED internal agent (a guardian seeded at boot, M961):
	// part of the daemon's own self-healing fleet, not a user creation. System
	// agents are protected — Remove refuses them and the reaper never flags them
	// — but they can still be paused, retired, and edited like any agent. The
	// flag is kernel-owned: it is set only at seed time and is never accepted
	// from an edit/add payload, so it cannot be spoofed or cleared by a profile
	// write.
	System bool `json:"system,omitempty"`

	CreatedMS int64 `json:"created_ms"`
	UpdatedMS int64 `json:"updated_ms"`
}

type RetryPolicy struct {
	MaxAttempts  int      `json:"max_attempts,omitempty"`
	Backoff      string   `json:"backoff,omitempty"` // fixed | exponential
	BaseDelaySec int      `json:"base_delay_sec,omitempty"`
	MaxDelaySec  int      `json:"max_delay_sec,omitempty"`
	RetryOn      []string `json:"retry_on,omitempty"` // error | timeout | canceled | halted
}

type HealthPolicy struct {
	StaleAfterSec    int    `json:"stale_after_sec,omitempty"`
	FailureWindow    int    `json:"failure_window,omitempty"`
	FailureThreshold int    `json:"failure_threshold,omitempty"`
	DoctorAgent      string `json:"doctor_agent,omitempty"`
}

type SelfRepairPolicy struct {
	Enabled     bool   `json:"enabled,omitempty"`
	MaxAttempts int    `json:"max_attempts,omitempty"`
	EscalateTo  string `json:"escalate_to,omitempty"`
}

type NoisePolicy struct {
	// SilentOnSuccess is an explicit operating contract for UIs/prompts: routine
	// green runs should finish without operator interruption.
	SilentOnSuccess bool `json:"silent_on_success,omitempty"`
	// DisableMemoryWrites removes the memory tool from this agent's effective
	// tool set, making "don't write sweep logs" deterministic instead of prompt-only.
	DisableMemoryWrites bool `json:"disable_memory_writes,omitempty"`
	// MinNotifySeverity gates notify tool calls. Empty means no extra gate;
	// accepted values are info, warning, critical.
	MinNotifySeverity string `json:"min_notify_severity,omitempty"`
	// MinNotifyIntervalSec is a per-agent durable cooldown for notify calls.
	MinNotifyIntervalSec int `json:"min_notify_interval_sec,omitempty"`
}

const (
	LifecyclePersistent       = "persistent"
	LifecycleCycle            = "cycle"
	LifecycleRetireOnComplete = "retire_on_complete"
)

type AgentLifecycle struct {
	// Mode is the agent identity lifecycle. persistent is the default; cycle
	// means the agent expects repeated wakes; retire_on_complete buries it after
	// a successful run.
	Mode string `json:"mode,omitempty"`
	// RetireOnComplete is kept as an explicit flag so older/newer clients can
	// express the behavior without depending only on Mode.
	RetireOnComplete bool `json:"retire_on_complete,omitempty"`
	MaxCycles        int  `json:"max_cycles,omitempty"`
	CompletedCycles  int  `json:"completed_cycles,omitempty"`
	// LastCompletedRun is the correlation of the run whose success last advanced
	// the cycle count. It makes lifecycle completion idempotent per logical run:
	// RunAssured / RunWithRetry can invoke the agent multiple times under ONE
	// correlation (re-running until the work verifies complete), and without this
	// marker each successful inner run would double-count the cycle.
	LastCompletedRun string `json:"last_completed_run,omitempty"`
}

type AgentTask struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope,omitempty"`  // cycle | total
	Status      string `json:"status,omitempty"` // todo | doing | done | blocked | retired
	CreatedMS   int64  `json:"created_ms,omitempty"`
	UpdatedMS   int64  `json:"updated_ms,omitempty"`
}

// Kind returns the profile's durable identity class for UI/API consumers. It is
// derived from behavior-owning fields so the persisted model cannot drift:
// System guardians are system agents; non-direct-callable profiles are managed
// sub-agents; everything else is a user/custom agent.
func (p Profile) Kind() string {
	if p.System {
		return "system"
	}
	if !p.AllowsDirectCall() {
		return "subagent"
	}
	return "custom"
}

// AllowsDirectCall returns the direct-call policy, defaulting old profiles to
// true so existing roster files don't become inaccessible when the field is
// introduced.
func (p Profile) AllowsDirectCall() bool {
	return p.DirectCallable == nil || *p.DirectCallable
}

// AllowsDelegationFrom reports whether caller may run this profile as a
// delegated worker. Direct-callable agents can always be delegated to. Managed
// workers (DirectCallable=false) require a named caller; if parent/owner is set,
// the caller must match one of those slugs.
func (p Profile) AllowsDelegationFrom(caller string) bool {
	if p.AllowsDirectCall() {
		return true
	}
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return false
	}
	parent := strings.TrimSpace(p.ParentAgent)
	owner := strings.TrimSpace(p.OwnerAgent)
	if parent == "" && owner == "" {
		return false
	}
	return caller == parent || caller == owner
}

// RepairResult is the operator/guardian-facing outcome of one self-repair pass.
type RepairResult struct {
	Agent                         string   `json:"agent"`
	Correlation                   string   `json:"correlation"`
	Applied                       []string `json:"applied,omitempty"`
	RoutingTaskType               string   `json:"routing_task_type,omitempty"`
	RoutingTaskModelChain         []string `json:"routing_task_model_chain,omitempty"`
	PreviousRoutingTaskModelChain []string `json:"previous_routing_task_model_chain,omitempty"`
	Answer                        string   `json:"answer,omitempty"`
}

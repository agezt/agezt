// SPDX-License-Identifier: MIT

// Package overseer owns the implementation behind the overseer's supervisory
// port — the half of the surface that reads and steers the LIVE system.
//
// The port itself is a contract (kernel/contract/overseerapi): the tool that
// exposes supervision to agents is a plugin, and a plugin may only reach the
// contract and the platform layers. The implementation needs the live kernel
// (the runs module) and the board store (the board module), and the app layer
// is the only layer that may see both.
//
// The daemon builds the source and injects it: the control plane takes it as
// the agents module's repair source, and the overseer tool takes it as the
// contract's port.
package overseer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/overseerapi"
)

// sharing the board tool's in-process instance.
type kernelSource struct {
	ports    Ports
	baseDir  string
	boardDir string
	// fleetLock, when set, makes the AGENT-reachable EditAgent/CreateAgent paths
	// refuse (V-012). It is an OPT-IN guardrail off by default — the project's
	// default-allow posture is preserved unless an operator sets
	// AGEZT_OVERSEER_FLEET_LOCK. Operator control-plane edits and the auto-repair
	// daemon use other Source methods (RepairAgent/routing), so they are
	// unaffected; only an agent self-administering the fleet via the `overseer`
	// tool is gated.
	fleetLock bool
}

// NewKernelSource builds the source from the overseer's ports. baseDir is the
// daemon's base directory; the board lives at <baseDir>/board.
//
// The source holds the PORTS and not the handle they were filled from: a file
// that names the runtime is the port's business, and the adapter that answers
// the contract is not.
func NewKernelSource(ports Ports, baseDir string) overseerapi.Source {
	return &kernelSource{
		ports:     ports,
		baseDir:   baseDir,
		boardDir:  filepath.Join(baseDir, "board"),
		fleetLock: fleetLockEnabled(),
	}
}

// fleetLockEnabled reports whether agent-initiated fleet administration (editing
// or creating agents via the overseer tool) is locked. Off by default — only an
// explicit truthy AGEZT_OVERSEER_FLEET_LOCK turns it on, so the default-allow
// posture is the default and the restriction is strictly opt-out.
func fleetLockEnabled() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv(brand.EnvPrefix + "OVERSEER_FLEET_LOCK"))) {
	case "1", "on", "true", "yes":
		return true
	default:
		return false
	}
}

func (s *kernelSource) IsHalted() bool         { return s.ports.Stopped() }
func (s *kernelSource) ActiveRunIDs() []string { return s.ports.RunningIDs() }
func (s *kernelSource) Agents() []agentapi.Profile {
	return s.ports.Profiles()
}
func (s *kernelSource) AgentImpact(slug string) []string { return s.ports.ImpactOf(slug) }
func (s *kernelSource) CancelRun(corr string) bool       { return s.ports.StopRun(corr) }
func (s *kernelSource) Halt(reason string)               { s.ports.HaltDaemon(reason) }
func (s *kernelSource) ResumeAll(reason string)          { s.ports.ResumeDaemon(reason) }

func (s *kernelSource) SetAgentEnabled(ref string, enabled bool) (agentapi.Profile, error) {
	return s.ports.EnableAgent(ref, enabled)
}
func (s *kernelSource) SetAgentRetired(ref string, retired bool, reason string) (agentapi.Profile, error) {
	r := strings.TrimSpace(reason)
	if r == "" {
		return s.ports.RetireAgent(ref, retired, "")
	}
	return s.ports.RetireAgent(ref, retired, r)
}

// EditAgent applies `in`'s mutable fields to the agent named by ref, using a
// PATCH semantic: only fields explicitly present in the input JSON payload are
// applied; all other fields remain unchanged. This prevents a partial profile
// (e.g. `{"model":"gpt-5"}` from clearing soul, budget, policy fields, etc.
//
// Identity/lifecycle fields (id/slug/enabled/retired) and the System flag are
// NOT touched — a guardian can retune another agent but can't resurrect,
// rename, or promote it to a protected guardian.
//
// A System-protected guardian (the daemon's own self-healing fleet) cannot be
// edited through this tool at all. The overseer tool is agent-reachable —
// CapOversee is default-allow — so without this guard an arbitrary agent could
// rewrite a guardian's Soul/ToolAllow/ConfigOverrides and behaviorally
// "defang" it even though the System flag itself is preserved (and RemoveProfile
// already refuses to delete it). Operators can still edit guardians through the
// admin control-plane path; only the agent-reachable tool path is restricted.
func (s *kernelSource) EditAgent(ref string, in agentapi.Profile) (agentapi.Profile, error) {
	if s.fleetLock {
		return agentapi.Profile{}, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must make agent edits through the console/CLI")
	}
	cur, ok := s.ports.Profile(ref)
	if !ok {
		return agentapi.Profile{}, fmt.Errorf("unknown agent: %s", ref)
	}
	if cur.System {
		return agentapi.Profile{}, fmt.Errorf("agent %q is a protected system guardian — it can be retuned only by an operator, not via the overseer tool", cur.Slug)
	}
	// Discover which fields the caller explicitly provided. JSON unmarshal into
	// a flat map tells us the set of top-level keys — nil pointers and zero-value
	// ints/strings come from omission, not from the user intentionally clearing.
	provided := map[string]bool{}
	if raw, err := json.Marshal(in); err == nil {
		var flat map[string]any
		if json.Unmarshal(raw, &flat) == nil {
			for k := range flat {
				provided[k] = true
			}
		}
	}
	p, found, err := s.ports.EditProfile(ref, func(dst *agentapi.Profile) {
		applyProfilePatchField(provided, "name", &dst.Name, in.Name)
		applyProfilePatchField(provided, "soul", &dst.Soul, in.Soul)
		applyProfilePatchField(provided, "model", &dst.Model, in.Model)
		applyProfilePatchField(provided, "task_type", &dst.TaskType, in.TaskType)
		applyProfilePatchField(provided, "memory_scope", &dst.MemoryScope, in.MemoryScope)
		applyProfilePatchField(provided, "workdir", &dst.Workdir, in.Workdir)
		applyProfilePatchField(provided, "owner_agent", &dst.OwnerAgent, in.OwnerAgent)
		applyProfilePatchField(provided, "parent_agent", &dst.ParentAgent, in.ParentAgent)
		applyProfilePatchField(provided, "trust_ceiling", &dst.TrustCeiling, in.TrustCeiling)
		applyProfilePatchField(provided, "description", &dst.Description, in.Description)
		if provided["fallbacks"] {
			dst.Fallbacks = in.Fallbacks
		}
		if provided["max_cost_mc"] {
			dst.MaxCostMc = in.MaxCostMc
		}
		if provided["max_daily_mc"] {
			dst.MaxDailyMc = in.MaxDailyMc
		}
		if provided["direct_callable"] {
			dst.DirectCallable = in.DirectCallable
		}
		if provided["retry_policy"] {
			dst.RetryPolicy = in.RetryPolicy
		}
		if provided["health_policy"] {
			dst.HealthPolicy = in.HealthPolicy
		}
		if provided["self_repair"] {
			dst.SelfRepairPolicy = in.SelfRepairPolicy
		}
		if provided["noise_policy"] {
			dst.NoisePolicy = in.NoisePolicy
		}
		if provided["tool_allow"] {
			dst.ToolAllow = in.ToolAllow
		}
		if provided["tool_deny"] {
			dst.ToolDeny = in.ToolDeny
		}
		if provided["config_overrides"] {
			dst.ConfigOverrides = in.ConfigOverrides
		}
		if provided["instructions"] {
			dst.Instructions = in.Instructions
		}
		if provided["lifecycle"] {
			dst.Lifecycle = in.Lifecycle
		}
		if provided["tasklist"] {
			dst.TaskList = in.TaskList
		}
	})
	if err != nil {
		return agentapi.Profile{}, err
	}
	if !found {
		return agentapi.Profile{}, fmt.Errorf("unknown agent: %s", ref)
	}
	return p, nil
}

// applyProfilePatchField sets *dst = val only when key is present in provided.
func applyProfilePatchField[T comparable](provided map[string]bool, key string, dst *T, val T) {
	if provided[key] {
		*dst = val
	}
}

// CreateAgent adds a brand-new agent. System is forced off — only boot-time
// guardian seeding may mint a protected agent.
func (s *kernelSource) CreateAgent(in agentapi.Profile) (agentapi.Profile, error) {
	if s.fleetLock {
		return agentapi.Profile{}, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must create agents through the console/CLI")
	}
	in.System = false
	return s.ports.CreateProfile(in)
}

// DeleteAgent permanently removes a non-System agent by slug or id. System
// agents are protected — pause or retire them instead. Fleet lock, when set,
// also refuses agent-initiated hard deletes through this tool.
func (s *kernelSource) DeleteAgent(ref string) (bool, error) {
	if s.fleetLock {
		return false, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must delete agents through the console/CLI")
	}
	ok, err := s.ports.DeleteProfile(ref)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// GetAgent returns the full profile for a single agent by slug or id.
func (s *kernelSource) GetAgent(ref string) (agentapi.Profile, bool, error) {
	p, ok := s.ports.Profile(ref)
	if !ok {
		return agentapi.Profile{}, false, nil
	}
	return p, true, nil
}

// CloneAgent creates a new agent from a source profile with caller overrides.
// The source must exist. Override fields (soul/model/fallbacks/task_type/etc.)
// are applied on top. The System flag from the source is never copied. Slug in
// overrides is required. Fleet lock, when set, also refuses cloning through
// this tool.
func (s *kernelSource) CloneAgent(source string, overrides agentapi.Profile) (agentapi.Profile, error) {
	if s.fleetLock {
		return agentapi.Profile{}, errors.New("fleet administration via the overseer tool is locked (AGEZT_OVERSEER_FLEET_LOCK): an operator must create agents through the console/CLI")
	}
	src, ok := s.ports.Profile(source)
	if !ok {
		return agentapi.Profile{}, fmt.Errorf("unknown source agent: %s", source)
	}
	// Copy mutable fields from source, then apply overrides on top.
	built := src
	built.System = false // never copy the system flag
	built.ID = ""
	built.CreatedMS = 0
	built.UpdatedMS = 0
	built.Enabled = false // caller must explicitly resume
	built.Retired = false
	built.RetiredMS = 0
	built.RetiredReason = ""

	// Apply overrides
	if v := strings.TrimSpace(overrides.Slug); v != "" {
		built.Slug = v
	}
	if v := strings.TrimSpace(overrides.Name); v != "" {
		built.Name = v
	}
	if overrides.Soul != "" {
		built.Soul = overrides.Soul
	}
	if overrides.Model != "" {
		built.Model = overrides.Model
	}
	if len(overrides.Fallbacks) > 0 {
		built.Fallbacks = append([]string(nil), overrides.Fallbacks...)
	}
	if v := strings.TrimSpace(overrides.TaskType); v != "" {
		built.TaskType = v
	}
	if overrides.MaxCostMc > 0 {
		built.MaxCostMc = overrides.MaxCostMc
	}
	if overrides.MaxDailyMc > 0 {
		built.MaxDailyMc = overrides.MaxDailyMc
	}
	if v := strings.TrimSpace(overrides.MemoryScope); v != "" {
		built.MemoryScope = v
	}
	if v := strings.TrimSpace(overrides.Workdir); v != "" {
		built.Workdir = v
	}
	if v := strings.TrimSpace(overrides.Description); v != "" {
		built.Description = v
	}
	if len(overrides.Instructions) > 0 {
		built.Instructions = append([]string(nil), overrides.Instructions...)
	}
	if v := strings.TrimSpace(overrides.OwnerAgent); v != "" {
		built.OwnerAgent = v
	}
	if v := strings.TrimSpace(overrides.ParentAgent); v != "" {
		built.ParentAgent = v
	}
	if overrides.DirectCallable != nil {
		built.DirectCallable = overrides.DirectCallable
	}
	if len(overrides.ToolAllow) > 0 {
		built.ToolAllow = append([]string(nil), overrides.ToolAllow...)
	}
	if len(overrides.ToolDeny) > 0 {
		built.ToolDeny = append([]string(nil), overrides.ToolDeny...)
	}
	if v := strings.TrimSpace(overrides.TrustCeiling); v != "" {
		built.TrustCeiling = v
	}
	if len(overrides.ConfigOverrides) > 0 {
		if built.ConfigOverrides == nil {
			built.ConfigOverrides = make(map[string]string, len(overrides.ConfigOverrides))
		}
		for k, v := range overrides.ConfigOverrides {
			built.ConfigOverrides[k] = v
		}
	}
	if overrides.RetryPolicy != nil {
		built.RetryPolicy = overrides.RetryPolicy
	}
	if overrides.HealthPolicy != nil {
		built.HealthPolicy = overrides.HealthPolicy
	}
	if overrides.SelfRepairPolicy != nil {
		built.SelfRepairPolicy = overrides.SelfRepairPolicy
	}
	if overrides.NoisePolicy != nil {
		built.NoisePolicy = overrides.NoisePolicy
	}
	if len(overrides.Lifecycle.Mode) > 0 {
		built.Lifecycle = overrides.Lifecycle
	}
	if len(overrides.TaskList) > 0 {
		built.TaskList = append([]agentapi.AgentTask(nil), overrides.TaskList...)
	}

	return s.ports.CreateProfile(built)
}

// SearchAgents returns agents matching the filter criteria. Empty filter returns
// all non-retired agents (same as Agents()). Filters are AND-ed.

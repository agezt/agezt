// SPDX-License-Identifier: MIT

// roster_crud_internal.go owns the four pure helpers used
// by the Add / Edit / SetEnabled HTTP handlers:
// applyAgentMutableProfilePatch (the field-by-field merge
// that respects a `provided` map so the daemon can keep
// fields the client didn't intend to change — staggered
// PUTs won't lose data),
// normalizeAgentProfileKind (coerces raw JSON `kind`
// strings into the roster enum + flags defaults),
// validateAgentHierarchyRefs (catches cycles /
// dangling parent / owner-agent refs), and
// managedSubagentDirectCallError (the operator-facing
// error string when someone tries to send an action to a
// managed sub-agent instead of its manager). The HTTP
// handlers live in roster_crud.go.
package controlplane

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/roster"
)

func applyAgentMutableProfilePatch(dst *roster.Profile, in roster.Profile, provided map[string]bool) {
	if provided["name"] {
		dst.Name = in.Name
	}
	if provided["soul"] {
		dst.Soul = in.Soul
	}
	if provided["instructions"] {
		dst.Instructions = in.Instructions
	}
	if provided["model"] {
		dst.Model = in.Model
	}
	if provided["fallbacks"] {
		dst.Fallbacks = in.Fallbacks
	}
	if provided["task_type"] {
		dst.TaskType = in.TaskType
	}
	if provided["max_cost_mc"] {
		dst.MaxCostMc = in.MaxCostMc
	}
	if provided["max_daily_mc"] {
		dst.MaxDailyMc = in.MaxDailyMc
	}
	if provided["memory_scope"] {
		dst.MemoryScope = in.MemoryScope
	}
	if provided["workdir"] {
		dst.Workdir = in.Workdir
	}
	if provided["owner_agent"] {
		dst.OwnerAgent = in.OwnerAgent
	}
	if provided["parent_agent"] {
		dst.ParentAgent = in.ParentAgent
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
	if provided["trust_ceiling"] {
		dst.TrustCeiling = in.TrustCeiling
	}
	if provided["execution_profile"] {
		dst.ExecutionProfile = strings.TrimSpace(in.ExecutionProfile)
	}
	if provided["config_overrides"] {
		dst.ConfigOverrides = in.ConfigOverrides
	}
	if provided["lifecycle"] {
		dst.Lifecycle = in.Lifecycle
	}
	if provided["tasklist"] {
		dst.TaskList = in.TaskList
	}
	if provided["description"] {
		dst.Description = in.Description
	}
}

func normalizeAgentProfileKind(raw []byte, p *roster.Profile) {
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(meta.Kind), "subagent") {
		no := false
		p.DirectCallable = &no
	}
}

func (s *Server) validateAgentHierarchyRefs(p roster.Profile) error {
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.EqualFold(ref, strings.TrimSpace(p.Slug)) {
			return fmt.Errorf("roster: %s cannot point to the same agent", label)
		}
		target, ok := s.k.Roster().Get(ref)
		if !ok {
			return fmt.Errorf("roster: %s %q does not exist", label, ref)
		}
		if target.Retired {
			return fmt.Errorf("roster: %s %q is retired", label, ref)
		}
	}
	return nil
}

func managedSubagentDirectCallError(p roster.Profile, action string) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot be " + action + " directly; " + hint
}

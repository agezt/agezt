// SPDX-License-Identifier: MIT

// Control-plane tool views: decodeControlplaneArg + agentCapabilityPatch + wake/governance/noise/permission/config view helpers + stringSet.
// Code extracted from tool_views.go during the Day-138 god-file split.
// Public API unchanged.
package controlplane



import (
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/roster"
)



func decodeControlplaneArg(raw any, out any) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func applyAgentCapabilityPatch(dst *roster.Profile, patch agentCapabilityPatch) {
	if patch.TrustCeiling != nil {
		dst.TrustCeiling = *patch.TrustCeiling
	}
	if patch.ToolAllow != nil {
		dst.ToolAllow = append([]string(nil), (*patch.ToolAllow)...)
	}
	if patch.ToolDeny != nil {
		dst.ToolDeny = append([]string(nil), (*patch.ToolDeny)...)
	}
	if patch.NoisePolicy != nil {
		v := *patch.NoisePolicy
		dst.NoisePolicy = &v
	}
	if patch.ConfigOverrides != nil {
		if len(*patch.ConfigOverrides) == 0 {
			dst.ConfigOverrides = nil
		} else {
			dst.ConfigOverrides = make(map[string]string, len(*patch.ConfigOverrides))
			for key, value := range *patch.ConfigOverrides {
				dst.ConfigOverrides[key] = value
			}
		}
	}
	if patch.MemoryScope != nil {
		dst.MemoryScope = *patch.MemoryScope
	}
	if patch.Workdir != nil {
		dst.Workdir = *patch.Workdir
	}
	if patch.MaxCostMc != nil {
		dst.MaxCostMc = *patch.MaxCostMc
	}
	if patch.MaxDailyMc != nil {
		dst.MaxDailyMc = *patch.MaxDailyMc
	}
}

func agentWakeAccessView(p roster.Profile) map[string]any {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	direct := p.Enabled && !p.Retired && p.AllowsDirectCall()
	reason := "directly callable"
	status := "direct"
	if p.Retired {
		status = "retired"
		reason = "agent is retired"
	} else if !p.Enabled {
		status = "paused"
		reason = "agent is paused"
	} else if !p.AllowsDirectCall() {
		status = "managed"
		if manager != "" {
			reason = "managed by " + manager
		} else {
			reason = "managed sub-agent requires parent/owner delegation"
		}
	}
	delegators := []string{}
	if owner := strings.TrimSpace(p.OwnerAgent); owner != "" {
		delegators = append(delegators, owner)
	}
	if parent := strings.TrimSpace(p.ParentAgent); parent != "" && !stringSet(delegators)[strings.ToLower(parent)] {
		delegators = append(delegators, parent)
	}
	delegationAllowed := p.Enabled && !p.Retired && (p.AllowsDirectCall() || len(delegators) > 0)
	delegationScope := "any"
	if !p.AllowsDirectCall() {
		delegationScope = "manager"
	}
	return map[string]any{
		"status":             status,
		"reason":             reason,
		"direct_callable":    p.AllowsDirectCall(),
		"direct_allowed":     direct,
		"schedule_allowed":   direct,
		"channel_allowed":    direct,
		"operator_allowed":   direct,
		"delegation_allowed": delegationAllowed,
		"delegation_scope":   delegationScope,
		"delegation_sources": delegators,
		"manager":            manager,
		"owner_agent":        strings.TrimSpace(p.OwnerAgent),
		"parent_agent":       strings.TrimSpace(p.ParentAgent),
		"retired":            p.Retired,
		"enabled":            p.Enabled,
		"system":             p.System,
		"kind":               p.Kind(),
	}
}


// SPDX-License-Identifier: MIT

// Agent governance views: applyAgentCapabilityPatch + agentWakeAccessView + agentGovernanceView + effectiveGovernanceNoisePolicy + noiseSeverityRank + agentPermissionRows + permissionStatus + agentConfigPermissionRows + agentConfigVisibility + stringSet.
// Code extracted from tool.go during the Day-59 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/roster"
	"sort"
	"strconv"
	"strings"
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

func agentGovernanceView(p roster.Profile, permissions, configRows []map[string]any) map[string]any {
	allowed := 0
	ask := 0
	blocked := 0
	directTools := []string{}
	askTools := []string{}
	blockedTools := []string{}
	for _, row := range permissions {
		rowAllowed, _ := row["allowed"].(bool)
		rowAsk, _ := row["ask"].(bool)
		name, _ := row["name"].(string)
		switch {
		case rowAllowed && rowAsk:
			ask++
			if name != "" {
				askTools = append(askTools, name)
			}
		case rowAllowed:
			allowed++
			if name != "" {
				directTools = append(directTools, name)
			}
		default:
			blocked++
			if name != "" {
				blockedTools = append(blockedTools, name)
			}
		}
	}
	visibleConfigs := 0
	ownedConfigs := 0
	hiddenConfigs := 0
	visibleConfigKeys := []string{}
	hiddenConfigKeys := []string{}
	for _, row := range configRows {
		key, _ := row["key"].(string)
		if visible, _ := row["visible"].(bool); visible {
			visibleConfigs++
			if key != "" {
				visibleConfigKeys = append(visibleConfigKeys, key)
			}
		} else {
			hiddenConfigs++
			if key != "" {
				hiddenConfigKeys = append(hiddenConfigKeys, key)
			}
		}
		if owned, _ := row["owned"].(bool); owned {
			ownedConfigs++
		}
	}
	trust := strings.TrimSpace(p.TrustCeiling)
	if trust == "" {
		trust = "L4"
	}
	policy := effectiveGovernanceNoisePolicy(p)
	summary := []string{
		"tools " + strconv.Itoa(allowed) + "/" + strconv.Itoa(len(permissions)) + " allowed",
		strconv.Itoa(ask) + " ask",
		strconv.Itoa(blocked) + " blocked",
		"config " + strconv.Itoa(visibleConfigs) + "/" + strconv.Itoa(len(configRows)) + " visible",
		"trust " + trust,
	}
	if p.System {
		summary = append(summary, "system enforced")
	}
	risk := "governed"
	if len(permissions) > 0 && blocked == 0 && ask == 0 && len(p.ToolAllow) == 0 && len(p.ToolDeny) == 0 && trust == "L4" {
		risk = "open"
	}
	if blocked > 0 || ask > 0 || len(p.ToolAllow) > 0 || len(p.ToolDeny) > 0 || trust != "L4" {
		risk = "restricted"
	}
	if p.System {
		risk = "system_guardian"
	}
	toolPolicy := "default"
	switch {
	case len(p.ToolAllow) > 0 && len(p.ToolDeny) > 0:
		toolPolicy = "allowlist+denylist"
	case len(p.ToolAllow) > 0:
		toolPolicy = "allowlist"
	case len(p.ToolDeny) > 0:
		toolPolicy = "denylist"
	}
	memoryScope := strings.TrimSpace(p.MemoryScope)
	memoryPolicy := "default:" + strings.TrimSpace(p.Slug)
	if memoryScope != "" {
		memoryPolicy = "scoped:" + memoryScope
	}
	memoryWrites := "enabled"
	if policy.DisableMemoryWrites {
		memoryWrites = "disabled"
	}
	authorityBoundary := "direct agent · owns soul, memory scope, tool policy, trust ceiling, and config overrides"
	if p.System {
		authorityBoundary = "system guardian · kernel-owned defaults enforce quiet, capped permissions"
	} else if !p.AllowsDirectCall() {
		authorityBoundary = "managed sub-agent · manager controls wake access; delegated work still runs under this agent policy"
	}
	permissionPassport := strings.Join([]string{
		"trust " + trust,
		"tools " + toolPolicy,
		strconv.Itoa(allowed) + " direct",
		strconv.Itoa(ask) + " ask",
		strconv.Itoa(blocked) + " blocked",
		"memory " + memoryPolicy,
		"memory_writes " + memoryWrites,
	}, ", ")
	return map[string]any{
		"summary":                       strings.Join(summary, ", "),
		"risk":                          risk,
		"system_enforced":               p.System,
		"authority_boundary":            authorityBoundary,
		"execution_boundary":            "agent identity owns tools, memory, model route, retry, and repair; schedules/workflows invoke through this policy",
		"permission_passport":           permissionPassport,
		"tool_policy":                   toolPolicy,
		"memory_policy":                 memoryPolicy,
		"memory_writes":                 memoryWrites,
		"trust_ceiling":                 trust,
		"tool_count":                    len(permissions),
		"allowed_count":                 allowed,
		"ask_count":                     ask,
		"blocked_count":                 blocked,
		"direct_tools":                  directTools,
		"ask_tools":                     askTools,
		"blocked_tools":                 blockedTools,
		"tool_allow_count":              len(p.ToolAllow),
		"tool_deny_count":               len(p.ToolDeny),
		"config_count":                  len(configRows),
		"config_visible_count":          visibleConfigs,
		"config_owned_count":            ownedConfigs,
		"config_hidden_count":           hiddenConfigs,
		"visible_configs":               visibleConfigKeys,
		"hidden_configs":                hiddenConfigKeys,
		"memory_scope":                  strings.TrimSpace(p.MemoryScope),
		"max_cost_mc":                   p.MaxCostMc,
		"max_daily_mc":                  p.MaxDailyMc,
		"noise_silent_on_success":       policy.SilentOnSuccess,
		"noise_disable_memory_writes":   policy.DisableMemoryWrites,
		"noise_min_notify_severity":     strings.TrimSpace(policy.MinNotifySeverity),
		"noise_min_notify_interval_sec": policy.MinNotifyIntervalSec,
	}
}

func effectiveGovernanceNoisePolicy(p roster.Profile) roster.NoisePolicy {
	var policy roster.NoisePolicy
	if p.NoisePolicy != nil {
		policy = *p.NoisePolicy
	}
	if !p.System {
		return policy
	}
	policy.SilentOnSuccess = true
	policy.DisableMemoryWrites = true
	if noiseSeverityRank(policy.MinNotifySeverity) < noiseSeverityRank("warning") {
		policy.MinNotifySeverity = "warning"
	}
	if policy.MinNotifyIntervalSec < 8*3600 {
		policy.MinNotifyIntervalSec = 8 * 3600
	}
	return policy
}

func noiseSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info", "":
		return 1
	default:
		return 0
	}
}

func (s *Server) agentPermissionRows(p roster.Profile) []map[string]any {
	tools := s.k.Tools()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	allow := stringSet(p.ToolAllow)
	deny := stringSet(p.ToolDeny)
	ceiling := edict.LevelAllow
	if raw := strings.TrimSpace(p.TrustCeiling); raw != "" {
		if lvl, err := edict.ParseTrustLevel(raw); err == nil {
			ceiling = lvl
		}
	}
	rows := make([]map[string]any, 0, len(names))
	for _, name := range names {
		def := tools[name].Definition()
		cap := edict.CapabilityForToolCall(name, catalogProbe[name])
		row := map[string]any{
			"name":        def.Name,
			"description": def.Description,
			"capability":  string(cap),
		}
		switch {
		case deny[strings.ToLower(name)]:
			row["allowed"] = false
			row["ask"] = false
			row["status"] = "denied"
			row["source"] = "agent_deny"
			row["reason"] = "agent tool denylist"
			row["level"] = ""
		case len(allow) > 0 && !allow[strings.ToLower(name)]:
			row["allowed"] = false
			row["ask"] = false
			row["status"] = "hidden"
			row["source"] = "agent_allow"
			row["reason"] = "not in agent tool allowlist"
			row["level"] = ""
		default:
			out := s.k.Edict().DecideWithCeiling(cap, "", ceiling)
			row["allowed"] = out.Decision == edict.DecisionAllow
			row["ask"] = out.WouldAsk || out.RequiresApproval
			row["status"] = permissionStatus(out)
			row["source"] = "edict"
			row["reason"] = out.Reason
			row["level"] = out.Level.String()
			row["hard_denied"] = out.HardDenied
			if out.RequiresApproval {
				row["requires_approval"] = true
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func permissionStatus(out edict.Outcome) string {
	if out.Decision == edict.DecisionDeny {
		return "denied"
	}
	if out.WouldAsk || out.RequiresApproval {
		return out.Level.String()
	}
	return "allowed"
}

func (s *Server) agentConfigPermissionRows(p roster.Profile) []map[string]any {
	if s.k.ConfigCenter() == nil {
		return nil
	}
	entries := s.k.ConfigCenter().ListEntries()
	sort.Slice(entries, func(i, j int) bool {
		return strings.Compare(entries[i].Key, entries[j].Key) < 0
	})
	rows := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		allowedAgents := append([]string(nil), entry.AllowedAgents...)
		excludedAgents := append([]string(nil), entry.ExcludedAgents...)
		visible, source, reason := agentConfigVisibility(p.Slug, allowedAgents, excludedAgents)
		row := map[string]any{
			"key":             entry.Key,
			"rating":          string(entry.Rating),
			"visible":         visible,
			"source":          source,
			"reason":          reason,
			"allowed_agents":  allowedAgents,
			"excluded_agents": excludedAgents,
		}
		if configEntryBelongsToAgent(entry, p.Slug) {
			row["owned"] = true
		}
		if entry.Description != "" {
			row["description"] = entry.Description
		}
		rows = append(rows, row)
	}
	return rows
}

func agentConfigVisibility(slug string, allowedAgents, excludedAgents []string) (bool, string, string) {
	slug = strings.TrimSpace(slug)
	for _, denied := range excludedAgents {
		if strings.EqualFold(strings.TrimSpace(denied), slug) {
			return false, "config_excluded", "agent is in config excluded_agents"
		}
	}
	if len(allowedAgents) == 0 {
		return true, "config_global", "visible to all eligible agents"
	}
	for _, allowed := range allowedAgents {
		if strings.EqualFold(strings.TrimSpace(allowed), slug) {
			return true, "config_allowed", "agent is in config allowed_agents"
		}
	}
	return false, "config_allowed", "not in config allowed_agents"
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		if item = strings.ToLower(strings.TrimSpace(item)); item != "" {
			out[item] = true
		}
	}
	return out
}

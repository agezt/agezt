// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// PermissionPorts binds the permission picture to the primary kernel: the
// roster, the registered tools, the policy engine and the config center.
type PermissionPorts struct {
	Get    func(string) (core.Profile, bool)
	Update func(string, func(*core.Profile)) (core.Profile, bool, error)
	Tools  func() map[string]toolapi.Tool
	Decide func(edict.Capability, edict.TrustLevel) edict.Outcome
	// Config lists the config center's entries; false when there is none.
	Config func() ([]*configcenter.ConfigEntry, bool)
}

// PermissionService explains which tools and config entries an agent may use,
// and patches the agent-local capability surface.
type PermissionService struct{ ports PermissionPorts }

func NewPermissions(ports PermissionPorts) *PermissionService {
	return &PermissionService{ports: ports}
}

type PermissionsRequest struct {
	Ref json.RawMessage `json:"ref,omitempty"`
}

type CapabilitiesRequest struct {
	Ref             json.RawMessage `json:"ref,omitempty"`
	TrustCeiling    json.RawMessage `json:"trust_ceiling,omitempty"`
	ToolAllow       json.RawMessage `json:"tool_allow,omitempty"`
	ToolDeny        json.RawMessage `json:"tool_deny,omitempty"`
	NoisePolicy     json.RawMessage `json:"noise_policy,omitempty"`
	ConfigOverrides json.RawMessage `json:"config_overrides,omitempty"`
	MemoryScope     json.RawMessage `json:"memory_scope,omitempty"`
	Workdir         json.RawMessage `json:"workdir,omitempty"`
	MaxCostMc       json.RawMessage `json:"max_cost_mc,omitempty"`
	MaxDailyMc      json.RawMessage `json:"max_daily_mc,omitempty"`
}

// PermissionRow is one registered tool as the agent sees it. hard_denied is
// reported only for tools the policy engine decided.
type PermissionRow struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Capability       string `json:"capability"`
	Allowed          bool   `json:"allowed"`
	Ask              bool   `json:"ask"`
	Status           string `json:"status"`
	Source           string `json:"source"`
	Reason           string `json:"reason"`
	Level            string `json:"level"`
	HardDenied       *bool  `json:"hard_denied,omitempty"`
	RequiresApproval bool   `json:"requires_approval,omitempty"`
}

type ConfigPermissionRow struct {
	Key            string   `json:"key"`
	Rating         string   `json:"rating"`
	Visible        bool     `json:"visible"`
	Source         string   `json:"source"`
	Reason         string   `json:"reason"`
	AllowedAgents  []string `json:"allowed_agents"`
	ExcludedAgents []string `json:"excluded_agents"`
	Owned          bool     `json:"owned,omitempty"`
	Description    string   `json:"description,omitempty"`
}

type WakeAccess struct {
	Status            string   `json:"status"`
	Reason            string   `json:"reason"`
	DirectCallable    bool     `json:"direct_callable"`
	DirectAllowed     bool     `json:"direct_allowed"`
	ScheduleAllowed   bool     `json:"schedule_allowed"`
	ChannelAllowed    bool     `json:"channel_allowed"`
	OperatorAllowed   bool     `json:"operator_allowed"`
	DelegationAllowed bool     `json:"delegation_allowed"`
	DelegationScope   string   `json:"delegation_scope"`
	DelegationSources []string `json:"delegation_sources"`
	Manager           string   `json:"manager"`
	OwnerAgent        string   `json:"owner_agent"`
	ParentAgent       string   `json:"parent_agent"`
	Retired           bool     `json:"retired"`
	Enabled           bool     `json:"enabled"`
	System            bool     `json:"system"`
	Kind              string   `json:"kind"`
}

type Governance struct {
	Summary                   string   `json:"summary"`
	Risk                      string   `json:"risk"`
	SystemEnforced            bool     `json:"system_enforced"`
	AuthorityBoundary         string   `json:"authority_boundary"`
	ExecutionBoundary         string   `json:"execution_boundary"`
	PermissionPassport        string   `json:"permission_passport"`
	ToolPolicy                string   `json:"tool_policy"`
	MemoryPolicy              string   `json:"memory_policy"`
	MemoryWrites              string   `json:"memory_writes"`
	TrustCeiling              string   `json:"trust_ceiling"`
	ToolCount                 int      `json:"tool_count"`
	AllowedCount              int      `json:"allowed_count"`
	AskCount                  int      `json:"ask_count"`
	BlockedCount              int      `json:"blocked_count"`
	DirectTools               []string `json:"direct_tools"`
	AskTools                  []string `json:"ask_tools"`
	BlockedTools              []string `json:"blocked_tools"`
	ToolAllowCount            int      `json:"tool_allow_count"`
	ToolDenyCount             int      `json:"tool_deny_count"`
	ConfigCount               int      `json:"config_count"`
	ConfigVisibleCount        int      `json:"config_visible_count"`
	ConfigOwnedCount          int      `json:"config_owned_count"`
	ConfigHiddenCount         int      `json:"config_hidden_count"`
	VisibleConfigs            []string `json:"visible_configs"`
	HiddenConfigs             []string `json:"hidden_configs"`
	MemoryScope               string   `json:"memory_scope"`
	MaxCostMc                 int64    `json:"max_cost_mc"`
	MaxDailyMc                int64    `json:"max_daily_mc"`
	NoiseSilentOnSuccess      bool     `json:"noise_silent_on_success"`
	NoiseDisableMemoryWrites  bool     `json:"noise_disable_memory_writes"`
	NoiseMinNotifySeverity    string   `json:"noise_min_notify_severity"`
	NoiseMinNotifyIntervalSec int      `json:"noise_min_notify_interval_sec"`
}

// PermissionsOutput is the agent's full permission picture. config_entries is
// null when the daemon has no config center.
type PermissionsOutput struct {
	Slug          string                `json:"slug"`
	TrustCeiling  string                `json:"trust_ceiling"`
	ToolAllow     []string              `json:"tool_allow"`
	ToolDeny      []string              `json:"tool_deny"`
	Permissions   []PermissionRow       `json:"permissions"`
	ConfigEntries []ConfigPermissionRow `json:"config_entries"`
	WakeAccess    WakeAccess            `json:"wake_access"`
	Governance    Governance            `json:"governance"`
	Count         int                   `json:"count"`
	AllowedCount  int                   `json:"allowed_count"`
}

// CapabilitiesOutput is the patched profile beside its new permission picture.
type CapabilitiesOutput struct {
	Profile ProfileOutput `json:"profile"`
	PermissionsOutput
}

// Permissions reports one agent's permission picture.
func (s *PermissionService) Permissions(_ context.Context, in PermissionsRequest) (PermissionsOutput, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return PermissionsOutput{}, err
	}
	p, ok := s.ports.Get(ref)
	if !ok {
		return PermissionsOutput{}, errors.New("unknown agent: " + ref)
	}
	return s.picture(p), nil
}

// Capabilities patches only the fields the caller sent. The patch is validated
// against the live roster's hierarchy before the journaled update.
func (s *PermissionService) Capabilities(_ context.Context, in CapabilitiesRequest) (CapabilitiesOutput, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return CapabilitiesOutput{}, err
	}
	current, ok := s.ports.Get(ref)
	if !ok {
		return CapabilitiesOutput{}, errors.New("unknown agent: " + ref)
	}
	patch, touched, err := decodeCapabilityPatch(in)
	if err != nil {
		return CapabilitiesOutput{}, err
	}
	if !touched {
		return CapabilitiesOutput{}, errors.New("args capability field required")
	}
	candidate := current
	patch.apply(&candidate)
	if err := ValidateHierarchyRefs(candidate, s.ports.Get); err != nil {
		return CapabilitiesOutput{}, err
	}
	updated, found, err := s.ports.Update(ref, patch.apply)
	if err != nil {
		return CapabilitiesOutput{}, err
	}
	if !found {
		return CapabilitiesOutput{}, errors.New("unknown agent: " + ref)
	}
	return CapabilitiesOutput{Profile: profileWriteOutput(updated).Profile, PermissionsOutput: s.picture(updated)}, nil
}

func (s *PermissionService) picture(p core.Profile) PermissionsOutput {
	rows := s.permissionRows(p)
	configRows := s.configRows(p)
	allowed := 0
	for _, row := range rows {
		if row.Allowed {
			allowed++
		}
	}
	return PermissionsOutput{
		Slug:          p.Slug,
		TrustCeiling:  strings.TrimSpace(p.TrustCeiling),
		ToolAllow:     append([]string(nil), p.ToolAllow...),
		ToolDeny:      append([]string(nil), p.ToolDeny...),
		Permissions:   rows,
		ConfigEntries: configRows,
		WakeAccess:    wakeAccess(p),
		Governance:    governance(p, rows, configRows),
		Count:         len(rows),
		AllowedCount:  allowed,
	}
}

type capabilityPatch struct {
	trustCeiling    *string
	toolAllow       *[]string
	toolDeny        *[]string
	noisePolicy     *core.NoisePolicy
	configOverrides *map[string]string
	memoryScope     *string
	workdir         *string
	maxCostMc       *int64
	maxDailyMc      *int64
}

// decodeField decodes one present field into a fresh value. JSON null decodes
// to the zero value and still counts as sent.
func decodeField[T any](raw json.RawMessage, dst **T, touched *bool) error {
	if len(raw) == 0 {
		return nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*dst = &v
	*touched = true
	return nil
}

func decodeCapabilityPatch(in CapabilitiesRequest) (capabilityPatch, bool, error) {
	var patch capabilityPatch
	touched := false
	for _, decode := range []func() error{
		func() error { return decodeField(in.TrustCeiling, &patch.trustCeiling, &touched) },
		func() error { return decodeField(in.ToolAllow, &patch.toolAllow, &touched) },
		func() error { return decodeField(in.ToolDeny, &patch.toolDeny, &touched) },
		func() error { return decodeField(in.NoisePolicy, &patch.noisePolicy, &touched) },
		func() error { return decodeField(in.ConfigOverrides, &patch.configOverrides, &touched) },
		func() error { return decodeField(in.MemoryScope, &patch.memoryScope, &touched) },
		func() error { return decodeField(in.Workdir, &patch.workdir, &touched) },
		func() error { return decodeField(in.MaxCostMc, &patch.maxCostMc, &touched) },
		func() error { return decodeField(in.MaxDailyMc, &patch.maxDailyMc, &touched) },
	} {
		if err := decode(); err != nil {
			return capabilityPatch{}, false, err
		}
	}
	return patch, touched, nil
}

func (patch capabilityPatch) apply(dst *core.Profile) {
	if patch.trustCeiling != nil {
		dst.TrustCeiling = *patch.trustCeiling
	}
	if patch.toolAllow != nil {
		dst.ToolAllow = append([]string(nil), (*patch.toolAllow)...)
	}
	if patch.toolDeny != nil {
		dst.ToolDeny = append([]string(nil), (*patch.toolDeny)...)
	}
	if patch.noisePolicy != nil {
		v := *patch.noisePolicy
		dst.NoisePolicy = &v
	}
	if patch.configOverrides != nil {
		if len(*patch.configOverrides) == 0 {
			dst.ConfigOverrides = nil
		} else {
			dst.ConfigOverrides = make(map[string]string, len(*patch.configOverrides))
			for key, value := range *patch.configOverrides {
				dst.ConfigOverrides[key] = value
			}
		}
	}
	if patch.memoryScope != nil {
		dst.MemoryScope = *patch.memoryScope
	}
	if patch.workdir != nil {
		dst.Workdir = *patch.workdir
	}
	if patch.maxCostMc != nil {
		dst.MaxCostMc = *patch.maxCostMc
	}
	if patch.maxDailyMc != nil {
		dst.MaxDailyMc = *patch.maxDailyMc
	}
}

// permissionRows walks the registered tools by registration name: the agent
// denylist wins, then a non-empty allowlist hides the rest, then the policy
// engine decides under the agent's trust ceiling (an unparsable ceiling is
// ignored).
func (s *PermissionService) permissionRows(p core.Profile) []PermissionRow {
	tools := s.ports.Tools()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	allow := lowerSet(p.ToolAllow)
	deny := lowerSet(p.ToolDeny)
	ceiling := edict.LevelAllow
	if raw := strings.TrimSpace(p.TrustCeiling); raw != "" {
		if lvl, err := edict.ParseTrustLevel(raw); err == nil {
			ceiling = lvl
		}
	}
	rows := make([]PermissionRow, 0, len(names))
	for _, name := range names {
		def := tools[name].Definition()
		capability := apptools.PrimaryCapability(name)
		row := PermissionRow{Name: def.Name, Description: def.Description, Capability: string(capability)}
		switch {
		case deny[strings.ToLower(name)]:
			row.Status, row.Source, row.Reason = "denied", "agent_deny", "agent tool denylist"
		case len(allow) > 0 && !allow[strings.ToLower(name)]:
			row.Status, row.Source, row.Reason = "hidden", "agent_allow", "not in agent tool allowlist"
		default:
			out := s.ports.Decide(capability, ceiling)
			hard := out.HardDenied
			row.Allowed = out.Decision == edict.DecisionAllow
			row.Ask = out.WouldAsk || out.RequiresApproval
			row.Status = permissionStatus(out)
			row.Source = "edict"
			row.Reason = out.Reason
			row.Level = out.Level.String()
			row.HardDenied = &hard
			row.RequiresApproval = out.RequiresApproval
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

// configRows lists config entries by key with the agent's visibility; nil when
// the daemon has no config center.
func (s *PermissionService) configRows(p core.Profile) []ConfigPermissionRow {
	entries, ok := s.ports.Config()
	if !ok {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.Compare(entries[i].Key, entries[j].Key) < 0
	})
	rows := make([]ConfigPermissionRow, 0, len(entries))
	for _, entry := range entries {
		allowedAgents := append([]string(nil), entry.AllowedAgents...)
		excludedAgents := append([]string(nil), entry.ExcludedAgents...)
		visible, source, reason := configVisibility(p.Slug, allowedAgents, excludedAgents)
		rows = append(rows, ConfigPermissionRow{
			Key:            entry.Key,
			Rating:         string(entry.Rating),
			Visible:        visible,
			Source:         source,
			Reason:         reason,
			AllowedAgents:  allowedAgents,
			ExcludedAgents: excludedAgents,
			Owned:          ConfigEntryBelongsToAgent(entry, p.Slug),
			Description:    entry.Description,
		})
	}
	return rows
}

func configVisibility(slug string, allowedAgents, excludedAgents []string) (bool, string, string) {
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

// ConfigEntryBelongsToAgent reports whether a config entry is the agent's own:
// its key sits under the agent's namespace, or the agent created, tagged or is
// named in its ownership metadata. Agent teardown prunes the same entries.
func ConfigEntryBelongsToAgent(e *configcenter.ConfigEntry, slug string) bool {
	if e == nil {
		return false
	}
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return false
	}
	key := strings.TrimSpace(strings.ToLower(e.Key))
	for _, prefix := range []string{
		"agent/" + slug + "/",
		"agents/" + slug + "/",
		"agent." + slug + ".",
		"agents." + slug + ".",
	} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(e.CreatedBy), slug) {
		return true
	}
	for _, tag := range e.Tags {
		t := strings.TrimSpace(strings.ToLower(tag))
		if t == "agent:"+slug || t == "agent/"+slug || t == "owner:"+slug || t == "owner/"+slug {
			return true
		}
	}
	for _, k := range []string{"agent", "agent_slug", "owner_agent", "parent_agent"} {
		if strings.EqualFold(strings.TrimSpace(e.Metadata[k]), slug) {
			return true
		}
	}
	return false
}

func lowerSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		if item = strings.ToLower(strings.TrimSpace(item)); item != "" {
			out[item] = true
		}
	}
	return out
}

func wakeAccess(p core.Profile) WakeAccess {
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
	if parent := strings.TrimSpace(p.ParentAgent); parent != "" && !lowerSet(delegators)[strings.ToLower(parent)] {
		delegators = append(delegators, parent)
	}
	delegationScope := "any"
	if !p.AllowsDirectCall() {
		delegationScope = "manager"
	}
	return WakeAccess{
		Status:            status,
		Reason:            reason,
		DirectCallable:    p.AllowsDirectCall(),
		DirectAllowed:     direct,
		ScheduleAllowed:   direct,
		ChannelAllowed:    direct,
		OperatorAllowed:   direct,
		DelegationAllowed: p.Enabled && !p.Retired && (p.AllowsDirectCall() || len(delegators) > 0),
		DelegationScope:   delegationScope,
		DelegationSources: delegators,
		Manager:           manager,
		OwnerAgent:        strings.TrimSpace(p.OwnerAgent),
		ParentAgent:       strings.TrimSpace(p.ParentAgent),
		Retired:           p.Retired,
		Enabled:           p.Enabled,
		System:            p.System,
		Kind:              p.Kind(),
	}
}

func governance(p core.Profile, permissions []PermissionRow, configRows []ConfigPermissionRow) Governance {
	allowed, ask, blocked := 0, 0, 0
	directTools, askTools, blockedTools := []string{}, []string{}, []string{}
	for _, row := range permissions {
		switch {
		case row.Allowed && row.Ask:
			ask++
			if row.Name != "" {
				askTools = append(askTools, row.Name)
			}
		case row.Allowed:
			allowed++
			if row.Name != "" {
				directTools = append(directTools, row.Name)
			}
		default:
			blocked++
			if row.Name != "" {
				blockedTools = append(blockedTools, row.Name)
			}
		}
	}
	visibleConfigs, ownedConfigs, hiddenConfigs := 0, 0, 0
	visibleConfigKeys, hiddenConfigKeys := []string{}, []string{}
	for _, row := range configRows {
		if row.Visible {
			visibleConfigs++
			if row.Key != "" {
				visibleConfigKeys = append(visibleConfigKeys, row.Key)
			}
		} else {
			hiddenConfigs++
			if row.Key != "" {
				hiddenConfigKeys = append(hiddenConfigKeys, row.Key)
			}
		}
		if row.Owned {
			ownedConfigs++
		}
	}
	trust := strings.TrimSpace(p.TrustCeiling)
	if trust == "" {
		trust = "L4"
	}
	policy := effectiveNoisePolicy(p)
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
	return Governance{
		Summary:                   strings.Join(summary, ", "),
		Risk:                      risk,
		SystemEnforced:            p.System,
		AuthorityBoundary:         authorityBoundary,
		ExecutionBoundary:         "agent identity owns tools, memory, model route, retry, and repair; schedules/workflows invoke through this policy",
		PermissionPassport:        permissionPassport,
		ToolPolicy:                toolPolicy,
		MemoryPolicy:              memoryPolicy,
		MemoryWrites:              memoryWrites,
		TrustCeiling:              trust,
		ToolCount:                 len(permissions),
		AllowedCount:              allowed,
		AskCount:                  ask,
		BlockedCount:              blocked,
		DirectTools:               directTools,
		AskTools:                  askTools,
		BlockedTools:              blockedTools,
		ToolAllowCount:            len(p.ToolAllow),
		ToolDenyCount:             len(p.ToolDeny),
		ConfigCount:               len(configRows),
		ConfigVisibleCount:        visibleConfigs,
		ConfigOwnedCount:          ownedConfigs,
		ConfigHiddenCount:         hiddenConfigs,
		VisibleConfigs:            visibleConfigKeys,
		HiddenConfigs:             hiddenConfigKeys,
		MemoryScope:               memoryScope,
		MaxCostMc:                 p.MaxCostMc,
		MaxDailyMc:                p.MaxDailyMc,
		NoiseSilentOnSuccess:      policy.SilentOnSuccess,
		NoiseDisableMemoryWrites:  policy.DisableMemoryWrites,
		NoiseMinNotifySeverity:    strings.TrimSpace(policy.MinNotifySeverity),
		NoiseMinNotifyIntervalSec: policy.MinNotifyIntervalSec,
	}
}

// effectiveNoisePolicy is the profile's noise policy; a system guardian is held
// quiet regardless: silent on success, no memory writes, at least warning
// severity and at most one notification per eight hours.
func effectiveNoisePolicy(p core.Profile) core.NoisePolicy {
	var policy core.NoisePolicy
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

// permissionsSchema and capabilitiesSchema mirror the outputs with the
// marshaler-free profile view.
type capabilitiesSchema struct {
	Profile profileSchema `json:"profile"`
	PermissionsOutput
}

// PermissionOperations declares the read-only permission picture and the
// audited capability patch, both operator-only on the primary roster.
func PermissionOperations(provider func(context.Context) *PermissionService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster permission provider required")
	}
	readOut, err := schema.FromType(reflect.TypeFor[PermissionsOutput](), false)
	if err != nil {
		return nil, err
	}
	writeOut, err := schema.FromType(reflect.TypeFor[capabilitiesSchema](), false)
	if err != nil {
		return nil, err
	}
	read, err := app.NewOperation(opapi.Spec{Name: "agent_permissions", ReadOnly: true, OutputSchema: readOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/agents/permissions"}}, func(ctx context.Context, in PermissionsRequest) (PermissionsOutput, error) {
		return provider(ctx).Permissions(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	write, err := app.NewOperation(opapi.Spec{Name: "agent_capabilities", OutputSchema: writeOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"trust_ceiling":{},"tool_allow":{},"tool_deny":{},"noise_policy":{},"config_overrides":{},"memory_scope":{},"workdir":{},"max_cost_mc":{},"max_daily_mc":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/capabilities"}}, func(ctx context.Context, in CapabilitiesRequest) (CapabilitiesOutput, error) {
		return provider(ctx).Capabilities(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{read, write}, nil
}

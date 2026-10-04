// SPDX-License-Identifier: MIT

package system

import (
	"encoding/json"

	"github.com/agezt/agezt/internal/brand"
)

type StatusOutput struct {
	Daemon            string           `json:"daemon"`
	Protocol          int              `json:"protocol"`
	Model             string           `json:"model"`
	UptimeSeconds     int64            `json:"uptime_seconds"`
	Halted            bool             `json:"halted"`
	ActiveRuns        int              `json:"active_runs"`
	Tools             int              `json:"tools"`
	MemoryRecords     int              `json:"memory_records"`
	WorldEntities     int              `json:"world_entities"`
	ActiveSkills      int              `json:"active_skills"`
	JournalHead       int64            `json:"journal_head"`
	Schedules         ScheduleStatus   `json:"schedules"`
	PendingApprovals  int              `json:"pending_approvals"`
	ProviderFallbacks FallbackStatus   `json:"provider_fallbacks"`
	ModelFallbacks    FallbackStatus   `json:"model_fallbacks"`
	Delegation        DelegationStatus `json:"delegation"`
	Tenants           *int             `json:"tenants,omitempty"`
	HTTPServers       []HTTPBinding    `json:"http_servers,omitempty"`
	Channels          []ChannelInfo    `json:"channels,omitempty"`
	CredChain         string           `json:"cred_chain,omitempty"`
}

type ScheduleStatus struct {
	Total    int  `json:"total"`
	Enabled  int  `json:"enabled"`
	Running  int  `json:"running"`
	Resident bool `json:"resident"`
}

type FallbackStatus struct {
	Count      int    `json:"count"`
	LastReason string `json:"last_reason"`
	LastMS     int64  `json:"last_ms"`
}

type DelegationStatus struct {
	Enabled            bool  `json:"enabled"`
	MaxDepth           int   `json:"max_depth"`
	MaxFanout          int   `json:"max_fanout"`
	MaxSpendMicrocents int64 `json:"max_spend_microcents"`
	MaxTotal           int   `json:"max_total"`
}

// VersionOutput retains brand.Binary as the wire key through custom encoding,
// rather than hardcoding the product name in a struct tag.
type VersionOutput struct {
	Version         string
	ProtocolVersion int
	Revision        string
	Built           string
	BuildModified   bool
}

func (v VersionOutput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		brand.Binary: v.Version, "protocol_version": v.ProtocolVersion,
		"revision": v.Revision, "built": v.Built, "build_modified": v.BuildModified,
	})
}

func versionOutputSchema() json.RawMessage {
	properties := map[string]any{
		brand.Binary: map[string]any{"type": "string"}, "protocol_version": map[string]any{"type": "integer"},
		"revision": map[string]any{"type": "string"}, "built": map[string]any{"type": "string"},
		"build_modified": map[string]any{"type": "boolean"},
	}
	raw, _ := json.Marshal(map[string]any{
		"type": "object", "properties": properties,
		"required":             []string{brand.Binary, "protocol_version", "revision", "built", "build_modified"},
		"additionalProperties": false,
	})
	return raw
}

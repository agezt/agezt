// SPDX-License-Identifier: MIT

// Package configcenter data types: ConfigEntry, ConfigAccessRequest,
// ConfigAccessResponse, AccessDecision + AccessDecision constants, AuditEntry,
// Store. Split from types.go during Day 211 god-file refactor (#55).
// Public API unchanged.
package configcenter

import (
	"sync"
	"time"
)

type ConfigEntry struct {
	// Key is the unique identifier for this config value.
	Key string `json:"key"`

	// Value is the actual configuration value.
	Value string `json:"value"`

	// Rating indicates the sensitivity level.
	Rating Rating `json:"rating,omitempty"`

	// Tags are arbitrary labels for grouping.
	Tags []string `json:"tags,omitempty"`

	// Description explains what this config is for.
	Description string `json:"description,omitempty"`

	// AccessPolicy overrides the default policy for this entry.
	AccessPolicy Policy `json:"access_policy,omitempty"`

	// AllowedAgents restricts access to specific agent IDs.
	AllowedAgents []string `json:"allowed_agents,omitempty"`

	// ExcludedAgents excludes specific agent IDs from access.
	ExcludedAgents []string `json:"excluded_agents,omitempty"`

	// VaultBacked indicates this value comes from a vault.
	VaultBacked bool `json:"vault_backed,omitempty"`

	// VaultPath is the path in vault (if VaultBacked is true).
	VaultPath string `json:"vault_path,omitempty"`

	// ValueHash is the SHA256 hash of the value for change detection.
	ValueHash string `json:"value_hash,omitempty"`

	// Version for optimistic locking.
	Version int `json:"version"`

	// Metadata is arbitrary additional data.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Created info.
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}
type ConfigAccessRequest struct {
	// AgentID is the subprocess ID requesting access.
	AgentID string

	// RunID is the parent run ID for correlation.
	RunID string

	// Key is the config key to access.
	Key string

	// Reason is why the agent needs this value (shown to operator for HITL).
	Reason string

	// CachedValueHash is the hash of a previously cached value.
	CachedValueHash string

	// Timestamp of the request.
	Timestamp time.Time
}
type ConfigAccessResponse struct {
	// Decision is the access decision.
	Decision AccessDecision

	// Value is the config value (only if allowed).
	Value string

	// Reason explains why access was denied or pending.
	Reason string

	// Rating is the rating of the accessed key.
	Rating Rating

	// ApprovalID is the approval request ID (if pending HITL).
	ApprovalID string

	// Approver is who granted the access (if applicable).
	Approver string

	// Extra contains additional context.
	Extra map[string]string
}
// AccessDecision represents the outcome of an access request.
type AccessDecision string
const (
	AccessAllowed AccessDecision = "allowed"
	AccessDenied  AccessDecision = "denied"
	AccessPending AccessDecision = "pending"
)
// AuditEntry represents an audit log entry.
type AuditEntry struct {
	// ID is the unique identifier for this audit entry.
	ID string `json:"id"`

	// Event is always "config.access".
	Event string `json:"event"`

	// Timestamp of the access attempt.
	Timestamp int64 `json:"timestamp"`

	// Access details.
	AgentID string `json:"agent_id"`
	RunID   string `json:"run_id"`
	Key     string `json:"key"`
	Rating  Rating `json:"rating"`
	Reason  string `json:"reason,omitempty"`

	// Decision details.
	Decision   AccessDecision `json:"decision"`
	Policy     string         `json:"policy"`
	ReasonCode string         `json:"reason_code"`

	// Audit metadata.
	ValueLog   string            `json:"value_log,omitempty"` // "REDACTED", "ghp_xxxx...xxxx", or hash
	Approver   string            `json:"approver,omitempty"`
	ApprovalID string            `json:"approval_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
// Store provides persistent storage for config entries.
type Store struct {
	mu      sync.RWMutex
	entries map[string]*ConfigEntry
	audit   []*AuditEntry
}

// SPDX-License-Identifier: MIT

package configcenter

import (
	"sync"
	"time"
)

// ConfigEntry represents a single configuration entry.
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

// NewConfigEntry creates a new config entry with defaults.
func NewConfigEntry(key, value string) *ConfigEntry {
	now := time.Now().Unix()
	return &ConfigEntry{
		Key:       key,
		Value:     value,
		Rating:    RatingInternal, // Default rating
		Tags:      []string{},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]string),
	}
}

// SetRating sets the rating and returns self for chaining.
func (e *ConfigEntry) SetRating(r Rating) *ConfigEntry {
	e.Rating = r
	return e
}

// SetTags sets the tags and returns self for chaining.
func (e *ConfigEntry) SetTags(tags ...string) *ConfigEntry {
	e.Tags = tags
	return e
}

// SetDescription sets the description and returns self for chaining.
func (e *ConfigEntry) SetDescription(desc string) *ConfigEntry {
	e.Description = desc
	return e
}

// SetAccessPolicy sets a custom access policy.
func (e *ConfigEntry) SetAccessPolicy(p Policy) *ConfigEntry {
	e.AccessPolicy = p
	return e
}

// AllowAgent allows a specific agent ID to access this config.
func (e *ConfigEntry) AllowAgent(agentID string) *ConfigEntry {
	for _, a := range e.AllowedAgents {
		if a == agentID {
			return e
		}
	}
	e.AllowedAgents = append(e.AllowedAgents, agentID)
	return e
}

// DenyAgent denies a specific agent ID from accessing this config.
func (e *ConfigEntry) DenyAgent(agentID string) *ConfigEntry {
	for _, a := range e.ExcludedAgents {
		if a == agentID {
			return e
		}
	}
	e.ExcludedAgents = append(e.ExcludedAgents, agentID)
	return e
}

// ConfigAccessRequest represents a request to access a config value.
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

// ConfigAccessResponse represents the response to a config access request.
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
	path    string
	entries map[string]*ConfigEntry
	audit   []*AuditEntry
}

// NewStore creates a new in-memory store.
func NewStore() *Store {
	return &Store{
		entries: make(map[string]*ConfigEntry),
		audit:   make([]*AuditEntry, 0),
	}
}

// Get retrieves a config entry by key.
func (s *Store) Get(key string) (*ConfigEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[key]
	if !ok {
		return nil, NewConfigError(ErrKeyNotFound, "config key not found: "+key)
	}
	return entry, nil
}

// Set creates or updates a config entry.
func (s *Store) Set(entry *ConfigEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	entry.UpdatedAt = now

	if existing, ok := s.entries[entry.Key]; ok {
		entry.Version = existing.Version + 1
		entry.CreatedAt = existing.CreatedAt
		entry.CreatedBy = existing.CreatedBy
	} else {
		entry.Version = 1
		entry.CreatedAt = now
	}

	s.entries[entry.Key] = entry
	return nil
}

// Delete removes a config entry.
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[key]; !ok {
		return NewConfigError(ErrKeyNotFound, "config key not found: "+key)
	}
	delete(s.entries, key)
	return nil
}

// List returns all config entries.
func (s *Store) List() []*ConfigEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ConfigEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		result = append(result, entry)
	}
	return result
}

// ListByRating returns entries filtered by rating.
func (s *Store) ListByRating(rating Rating) []*ConfigEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ConfigEntry, 0)
	for _, entry := range s.entries {
		if entry.Rating == rating {
			result = append(result, entry)
		}
	}
	return result
}

// ListAccessible returns entries that are accessible without HITL (public/internal).
func (s *Store) ListAccessible() []*ConfigEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ConfigEntry, 0)
	for _, entry := range s.entries {
		if entry.Rating == RatingPublic || entry.Rating == RatingInternal {
			result = append(result, entry)
		}
	}
	return result
}

// Search finds entries by key prefix or tag.
func (s *Store) Search(query string, limit int) []*ConfigEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ConfigEntry, 0)
	for _, entry := range s.entries {
		// Skip secrets and restricted from search results
		if entry.Rating == RatingSecret {
			continue
		}

		// Check key prefix match
		if len(entry.Key) >= len(query) && entry.Key[:len(query)] == query {
			result = append(result, entry)
			if limit > 0 && len(result) >= limit {
				break
			}
			continue
		}

		// Check tag match
		for _, tag := range entry.Tags {
			if len(tag) >= len(query) && tag[:len(query)] == query {
				result = append(result, entry)
				if limit > 0 && len(result) >= limit {
					break
				}
				break
			}
		}
	}
	return result
}

// AddAuditEntry adds an audit entry.
func (s *Store) AddAuditEntry(entry *AuditEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = append(s.audit, entry)
}

// GetAuditLog returns audit entries, optionally filtered.
func (s *Store) GetAuditLog(limit int) []*AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.audit) {
		limit = len(s.audit)
	}
	return s.audit[len(s.audit)-limit:]
}

// UpdateRating updates only the rating of an entry.
func (s *Store) UpdateRating(key string, rating Rating) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[key]
	if !ok {
		return NewConfigError(ErrKeyNotFound, "config key not found: "+key)
	}
	entry.Rating = rating
	entry.UpdatedAt = time.Now().Unix()
	return nil
}

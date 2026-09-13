// SPDX-License-Identifier: MIT
//
// ConfigCenter: the Center struct + the lifecycle (New + Open +
// SetApprovalRegistry + Close) + the Get accessor + the Config + Classifier
// accessors + the SearchOptions struct + GetEntry.
// Everything else (Set/Delete/UpdateRating/SetOverride + the persistence
// helpers + List/Search + audit + stats + ParseRating) lives in
// center_ops.go.
// Extracted from center.go during the Day-205 god-file split.
// Public API unchanged.
package configcenter

import (
	"log/slog"
	"sync"

	"github.com/agezt/agezt/kernel/approval"
)

// Center is the main Config Center implementation.
type Center struct {
	config     *Config
	store      *Store
	classifier *SecretClassifier
	policy     *AccessPolicy
	auditLog   *AuditLogger

	mu sync.RWMutex
}

// New creates a new Config Center with the given configuration.
func New(cfg *Config) (*Center, error) {
	cfg.EnsureDefaults("")

	// Create classifier with overrides
	classifier := NewSecretClassifier()
	for key, rating := range cfg.Classifier.Overrides {
		classifier.SetOverride(key, rating)
	}

	// Create store
	store := NewStore()

	// Load existing entries from disk
	if err := loadStoreFromDisk(store, cfg.Dir); err != nil {
		// Non-fatal, just log
		slog.Warn("config center: failed to load existing entries", "error", err)
	}

	// Create audit logger
	auditLog := NewAuditLogger(cfg.Dir, cfg)

	// Create access policy
	policy := NewAccessPolicy(cfg, store, auditLog)

	c := &Center{
		config:     cfg,
		store:      store,
		classifier: classifier,
		policy:     policy,
		auditLog:   auditLog,
	}

	return c, nil
}

// Open creates and opens a Config Center.
func Open(cfg *Config) (*Center, error) {
	if cfg == nil {
		cfg = DefaultConfig("")
	}
	return New(cfg)
}

// SetApprovalRegistry sets the approval registry for HITL support.
func (c *Center) SetApprovalRegistry(registry *approval.Registry) {
	c.policy.SetRegistry(registry)
}

// Close closes the Config Center.
func (c *Center) Close() error {
	if c.auditLog != nil {
		c.auditLog.Close()
	}
	return nil
}

// SearchOptions contains search options.
type SearchOptions struct {
	Rating Rating
	Limit  int
}

// GetEntry returns a config entry without access control (for admin use).
func (c *Center) GetEntry(key string) (*ConfigEntry, error) {
	return c.store.Get(key)
}
// Config returns the current config.
func (c *Center) Config() *Config {
	return c.config
}
// Classifier returns the secret classifier for rating detection.
func (c *Center) Classifier() *SecretClassifier {
	return c.classifier
}

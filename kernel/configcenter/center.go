// SPDX-License-Identifier: MIT

package configcenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
		fmt.Printf("config center: failed to load existing entries: %v\n", err)
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

// Get retrieves a config value with access control.
func (c *Center) Get(ctx context.Context, req ConfigAccessRequest) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Use access policy to evaluate
	resp, err := c.policy.Evaluate(ctx, &req)
	if err != nil {
		return "", err
	}

	if resp.Decision != AccessAllowed {
		return "", NewConfigError(ErrAccessDenied, resp.Reason)
	}

	return resp.Value, nil
}

// Set creates or updates a config entry.
func (c *Center) Set(entry *ConfigEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Auto-classify if rating not set
	if entry.Rating == "" {
		entry.Rating = c.classifier.Classify(entry.Key, entry.Value)
	}

	// Compute value hash
	hash := sha256.Sum256([]byte(entry.Value))
	entry.ValueHash = hex.EncodeToString(hash[:])

	// Save to store
	if err := c.store.Set(entry); err != nil {
		return err
	}

	// Persist to disk
	return c.persistEntry(entry)
}

// Delete removes a config entry.
func (c *Center) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove from store
	if err := c.store.Delete(key); err != nil {
		return err
	}

	// Remove from disk
	entryFile := c.entryFile(key)
	os.Remove(entryFile)

	return nil
}

// List returns all config entries.
func (c *Center) List() []*ConfigEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.store.List()
}

// ListAccessible returns entries that don't require special approval.
func (c *Center) ListAccessible() []*ConfigEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.store.ListAccessible()
}

// Search finds entries by key prefix or tag.
func (c *Center) Search(query string, opts SearchOptions) []*ConfigEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if opts.Limit == 0 {
		opts.Limit = 50
	}

	return c.store.Search(query, opts.Limit)
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

// UpdateRating updates the rating for a key.
func (c *Center) UpdateRating(key string, rating Rating) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.store.UpdateRating(key, rating); err != nil {
		return err
	}

	entry, _ := c.store.Get(key)
	if entry != nil {
		return c.persistEntry(entry)
	}

	return nil
}

// SetOverride sets a manual rating override for a key.
func (c *Center) SetOverride(key string, rating Rating) {
	c.classifier.SetOverride(key, rating)

	// Also update the store
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, err := c.store.Get(key); err == nil {
		entry.Rating = rating
		c.store.Set(entry)
		c.persistEntry(entry)
	}
}

// GetAuditLog returns audit entries with optional filtering.
func (c *Center) GetAuditLog(opts AuditQuery) []*AuditEntry {
	return c.auditLog.Query(opts)
}

// Config returns the current config.
func (c *Center) Config() *Config {
	return c.config
}

// ListEntries returns all config entries (admin use).
func (c *Center) ListEntries() []*ConfigEntry {
	return c.store.List()
}

// ListByRating returns config entries filtered by rating.
func (c *Center) ListByRating(rating Rating) []*ConfigEntry {
	var result []*ConfigEntry
	for _, e := range c.store.List() {
		if e.Rating == rating {
			result = append(result, e)
		}
	}
	return result
}

// AccessLog returns access log entries with optional filtering.
// This is a convenience method that wraps GetAuditLog with AccessLogQuery.
func (c *Center) AccessLog(key, agentID string, since time.Duration) []*AccessLogEntry {
	query := AuditQuery{}
	if since > 0 {
		query.Since = time.Now().Add(-since).Unix()
	}
	if key != "" {
		query.Key = &key
	}
	if agentID != "" {
		query.AgentID = &agentID
	}
	return c.auditLog.QueryAccessLog(query)
}

// AuditLog returns audit log entries with optional filtering.
func (c *Center) AuditLog(since time.Duration) []*AuditEntry {
	query := AuditQuery{}
	if since > 0 {
		query.Since = time.Now().Add(-since).Unix()
	}
	return c.auditLog.Query(query)
}

// Stats returns statistics about the config center.
func (c *Center) Stats() map[string]any {
	entries := c.store.List()
	total := len(entries)
	byRating := make(map[string]int)
	for _, e := range entries {
		byRating[string(e.Rating)]++
	}
	return map[string]any{
		"total_entries": total,
		"by_rating":     byRating,
	}
}

// GetAutoRating returns the auto-detected rating for a key/value pair
// without modifying any stored value.
func (c *Center) GetAutoRating(key, value string) Rating {
	return c.classifier.Classify(key, value)
}

// Classifier returns the secret classifier for rating detection.
func (c *Center) Classifier() *SecretClassifier {
	return c.classifier
}

// ParseRating parses a rating string and returns the Rating or an error.
func ParseRating(s string) (Rating, error) {
	switch strings.ToLower(s) {
	case "public":
		return RatingPublic, nil
	case "internal":
		return RatingInternal, nil
	case "restricted":
		return RatingRestricted, nil
	case "secret":
		return RatingSecret, nil
	default:
		return "", fmt.Errorf("invalid rating: %s (expected: public, internal, restricted, secret)", s)
	}
}

// entryFile returns the path to the entry file for a key.
func (c *Center) entryFile(key string) string {
	// Create a safe filename from the key
	safeName := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))[:16]
	return filepath.Join(c.config.Dir, fmt.Sprintf("entry_%s.json", safeName))
}

// persistEntry writes an entry to disk.
func (c *Center) persistEntry(entry *ConfigEntry) error {
	if c.config.Dir == "" {
		return nil
	}

	os.MkdirAll(c.config.Dir, 0755)

	filename := c.entryFile(entry.Key)

	// Marshal to JSON
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}

// loadStoreFromDisk loads all entries from disk into the store.
func loadStoreFromDisk(store *Store, dir string) error {
	if dir == "" {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < 7 || entry.Name()[:7] != "entry_" {
			continue
		}

		filepath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filepath)
		if err != nil {
			continue
		}

		var e ConfigEntry
		if err := json.Unmarshal(data, &e); err != nil {
			continue
		}

		store.Set(&e)
	}

	return nil
}

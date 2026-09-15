// SPDX-License-Identifier: MIT

// configcenter: public CRUD/search API (Get, Set, Delete, List,
// ListAccessible, ListAccessibleForAgent, Search, SearchForAgent).
// Split from center_ops.go during Day 211 god-file refactor (#45).
// Public API unchanged.
package configcenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
)

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

// ListAccessibleForAgent returns public/internal entries visible to one agent.
// It mirrors value access restrictions for allowed/excluded agent lists without
// triggering value-level audit/HITL/rate-limit side effects.
func (c *Center) ListAccessibleForAgent(agentID string) []*ConfigEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return filterEntriesForAgent(c.store.ListAccessible(), agentID)
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

// SearchForAgent returns metadata search results visible to one agent. It keeps
// the store's rating/search rules, then applies the same per-agent visibility
// lists used by Get.
func (c *Center) SearchForAgent(agentID, query string, opts SearchOptions) []*ConfigEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if opts.Limit == 0 {
		opts.Limit = 50
	}

	return filterEntriesForAgent(c.store.Search(query, opts.Limit), agentID)
}

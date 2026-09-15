// SPDX-License-Identifier: MIT

// configcenter: rating + audit/log + filter helpers (filterEntriesForAgent,
// entryVisibleToAgent, UpdateRating, SetOverride, GetAuditLog, ListEntries,
// ListByRating, AccessLog, AuditLog, Stats, GetAutoRating, ParseRating).
// Split from center_ops.go during Day 211 god-file refactor (#45).
// Public API unchanged.
package configcenter

import (
	"fmt"
	"strings"
	"time"
)

func filterEntriesForAgent(entries []*ConfigEntry, agentID string) []*ConfigEntry {
	agentID = strings.TrimSpace(agentID)
	out := make([]*ConfigEntry, 0, len(entries))
	for _, entry := range entries {
		if entryVisibleToAgent(entry, agentID) {
			out = append(out, entry)
		}
	}
	return out
}

func entryVisibleToAgent(entry *ConfigEntry, agentID string) bool {
	if entry == nil {
		return false
	}
	for _, denied := range entry.ExcludedAgents {
		if strings.TrimSpace(denied) == agentID {
			return false
		}
	}
	if len(entry.AllowedAgents) == 0 {
		return true
	}
	for _, allowed := range entry.AllowedAgents {
		if strings.TrimSpace(allowed) == agentID {
			return true
		}
	}
	return false
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

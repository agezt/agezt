// SPDX-License-Identifier: MIT

// ConfigEntry constructor + fluent builder methods (SetRating, SetTags,
// SetDescription, SetAccessPolicy, AllowAgent, DenyAgent). Extracted from
// types.go during Day 211 god-file refactor (#55). Public API unchanged.
package configcenter

import "time"

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
func (e *ConfigEntry) SetRating(r Rating) *ConfigEntry {
	e.Rating = r
	return e
}
func (e *ConfigEntry) SetTags(tags ...string) *ConfigEntry {
	e.Tags = tags
	return e
}
func (e *ConfigEntry) SetDescription(desc string) *ConfigEntry {
	e.Description = desc
	return e
}
func (e *ConfigEntry) SetAccessPolicy(p Policy) *ConfigEntry {
	e.AccessPolicy = p
	return e
}
func (e *ConfigEntry) AllowAgent(agentID string) *ConfigEntry {
	for _, a := range e.AllowedAgents {
		if a == agentID {
			return e
		}
	}
	e.AllowedAgents = append(e.AllowedAgents, agentID)
	return e
}
func (e *ConfigEntry) DenyAgent(agentID string) *ConfigEntry {
	for _, a := range e.ExcludedAgents {
		if a == agentID {
			return e
		}
	}
	e.ExcludedAgents = append(e.ExcludedAgents, agentID)
	return e
}

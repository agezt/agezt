// SPDX-License-Identifier: MIT

// Store constructor + access methods (Get/Set/Delete/List/ListByRating/
// ListAccessible/Search) + audit (AddAuditEntry/GetAuditLog) +
// UpdateRating. Extracted from types.go during Day 211 god-file refactor (#55).
// Public API unchanged.
package configcenter

import "time"

func NewStore() *Store {
	return &Store{
		entries: make(map[string]*ConfigEntry),
		audit:   make([]*AuditEntry, 0),
	}
}
func (s *Store) Get(key string) (*ConfigEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[key]
	if !ok {
		return nil, NewConfigError(ErrKeyNotFound, "config key not found: "+key)
	}
	return entry, nil
}
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
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[key]; !ok {
		return NewConfigError(ErrKeyNotFound, "config key not found: "+key)
	}
	delete(s.entries, key)
	return nil
}
func (s *Store) List() []*ConfigEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ConfigEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		result = append(result, entry)
	}
	return result
}
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
func (s *Store) AddAuditEntry(entry *AuditEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = append(s.audit, entry)
}
func (s *Store) GetAuditLog(limit int) []*AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.audit) {
		limit = len(s.audit)
	}
	return s.audit[len(s.audit)-limit:]
}
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

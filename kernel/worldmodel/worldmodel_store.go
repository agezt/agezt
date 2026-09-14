// SPDX-License-Identifier: MIT

// Package worldmodel: FileStore method implementations — PutEntity +
// GetEntity + AllEntities + PutRelation + GetRelation + AllRelations +
// Count + Close + snapshotLocked. The content-addressing + soft-delete
// (Tombstoned + SupersededBy) + per-write snapshot lives here. Extracted
// from worldmodel.go during the Day-211 god-file split. Public API
// unchanged.
package worldmodel


import (
	"errors"
	"strings"
)
func (s *FileStore) PutEntity(e Entity) error {
	if e.ID == "" {
		return errors.New("worldmodel: entity id required")
	}
	if strings.TrimSpace(e.Name) == "" {
		return ErrEmptyName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entities[e.ID] = e
	return s.snapshotLocked()
}

// GetEntity implements Store.
func (s *FileStore) GetEntity(id string) (Entity, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entities[id]
	return e, ok, nil
}

// AllEntities implements Store. Sorted by CreatedMS then ID so two consecutive
// calls produce identical output (deterministic CLI + snapshot tests).
func (s *FileStore) AllEntities() ([]Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entity, 0, len(s.entities))
	for _, e := range s.entities {
		out = append(out, e)
	}
	sortEntities(out)
	return out, nil
}

// PutRelation implements Store.
func (s *FileStore) PutRelation(r Relation) error {
	if r.ID == "" {
		return errors.New("worldmodel: relation id required")
	}
	if r.From == "" || r.To == "" {
		return errors.New("worldmodel: relation needs from and to")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.relations[r.ID] = r
	return s.snapshotLocked()
}

// GetRelation implements Store.
func (s *FileStore) GetRelation(id string) (Relation, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.relations[id]
	return r, ok, nil
}

// AllRelations implements Store. Sorted by CreatedMS then ID.
func (s *FileStore) AllRelations() ([]Relation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Relation, 0, len(s.relations))
	for _, r := range s.relations {
		out = append(out, r)
	}
	sortRelations(out)
	return out, nil
}

// Count implements Store — the number of entities (all states).
func (s *FileStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entities)
}

// Close implements Store. Mutations persist synchronously, so this is a no-op.
func (s *FileStore) Close() error { return nil }

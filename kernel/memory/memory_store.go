// SPDX-License-Identifier: MIT

package memory

// Memory store: Store interface + FileStore + all CRUD methods
// (Open + Put + Get + Delete + All + Count + Close + snapshotLocked
// + sortRecords). Carved out of memory.go during the Day 192
// god-file split so the main file can stay focused on the Record
// type + accessors + content addressing, and the search file can
// stay focused on the Scored scoring + Search ranking + helpers.
// Public API unchanged.

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/jsonstore"
)

// Store is the pure record store. Implementations persist records by id and
// must be safe for concurrent use.
type Store interface {
	// Put upserts r by its ID (overwrites any existing record with that id).
	Put(r Record) error
	// Get returns the record at id; the bool is false if absent.
	Get(id string) (Record, bool, error)
	// Delete hard-removes the record at id (returns false if absent). Unlike
	// Forget (which tombstones), this reclaims the row — used only by the prune
	// pass on already-soft-deleted records (M857).
	Delete(id string) (bool, error)
	// All returns every record (including tombstoned/superseded ones),
	// sorted deterministically by CreatedMS then ID.
	All() ([]Record, error)
	// Count returns the number of records currently stored (all states).
	Count() int
	// Close releases any held resources.
	Close() error
}

// ErrEmptyContent is returned by callers that try to store a record with no
// content; the Manager validates before reaching the store, but the store
// guards too.
var ErrEmptyContent = errors.New("memory: empty content")

// FileStore is the file-backed Store. All records live in a single
// <dir>/memory.json object keyed by id, snapshotted atomically on every
// mutation (write-temp + rename). Simple and crash-safe; not optimized for
// high write volume — adequate for memory-lite.
type FileStore struct {
	path string

	mu   sync.RWMutex
	data map[string]Record
}

// Open opens (or creates) a FileStore under dir, loading <dir>/memory.json
// if present. The directory is created if absent.
func Open(dir string) (*FileStore, error) {
	s := &FileStore{data: make(map[string]Record)}
	path, err := jsonstore.LoadFrom(dir, "memory.json", &s.data)
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}
	s.path = path
	if s.data == nil {
		s.data = make(map[string]Record)
	}
	return s, nil
}

// Put implements Store.
func (s *FileStore) Put(r Record) error {
	if r.ID == "" {
		return errors.New("memory: record id required")
	}
	if strings.TrimSpace(r.Content) == "" {
		return ErrEmptyContent
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[r.ID] = r
	return s.snapshotLocked()
}

// Get implements Store.
func (s *FileStore) Get(id string) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data[id]
	return r, ok, nil
}

// Delete implements Store — a hard removal (M857). Returns false if id is absent.
func (s *FileStore) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return false, nil
	}
	delete(s.data, id)
	return true, s.snapshotLocked()
}

// All implements Store. The returned slice is sorted by CreatedMS then ID so
// two consecutive calls produce identical output (load-bearing for snapshot
// tests and deterministic CLI rendering).
func (s *FileStore) All() ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.data))
	for _, r := range s.data {
		out = append(out, r)
	}
	sortRecords(out)
	return out, nil
}

// Count implements Store.
func (s *FileStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

// Close implements Store. Mutations persist synchronously, so this is a
// no-op.
func (s *FileStore) Close() error { return nil }

// snapshotLocked writes the whole record map atomically. Caller holds s.mu.
// MarshalIndent over a map sorts keys alphabetically (Go guarantee), giving
// deterministic on-disk diffs.
func (s *FileStore) snapshotLocked() error {
	if err := jsonstore.Save(s.path, s.data); err != nil {
		return fmt.Errorf("memory: %w", err)
	}
	return nil
}

// sortRecords orders records deterministically: oldest first, ties broken by
// id. Shared by All and tests.
func sortRecords(rs []Record) {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].CreatedMS != rs[j].CreatedMS {
			return rs[i].CreatedMS < rs[j].CreatedMS
		}
		return rs[i].ID < rs[j].ID
	})
}


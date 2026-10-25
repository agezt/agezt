// SPDX-License-Identifier: MIT

package skill

// Provenance: skill_store.go owns the Skill record type + the persistent Store
//             interface + FileStore impl. Carved out of skill.go during the Day-211
//             god-file split so skill.go can stay focused on the lifecycle state
//             machine (Status + transitions). Public API unchanged.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/platform/filestore"
	"lukechampine.com/blake3"
)

// Metrics tracks how a skill has performed (SPEC-05 §4.1).
//
// The row itself lives in the contract layer (kernel/contract/skillapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Metrics = skillapi.Metrics

// Skill is one reusable, named procedure. JSON tags are stable so the on-disk
// file and the CLI/`--json` shape stay compatible across releases.
//
// The row itself lives in the contract layer (kernel/contract/skillapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Skill = skillapi.Skill

// ContentID computes the content-addressed id for a (name, body) pair. The
// "skill" domain prefix + NUL separators keep ids disjoint from other content
// addresses and avoid concatenation collisions. Name is normalized (lower/trim)
// so the same skill under cosmetic name casing addresses identically; the body
// is hashed verbatim (whitespace is meaningful in instructions).
func ContentID(name, body string) string {
	h := blake3.New(32, nil)
	h.Write([]byte("skill"))
	h.Write([]byte{0})
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	h.Write([]byte{0})
	h.Write([]byte(body))
	return hex.EncodeToString(h.Sum(nil))
}

// Store is the pure skill store. Implementations persist skills by id and must
// be safe for concurrent use.
type Store interface {
	// Put upserts s by its ID.
	Put(s Skill) error
	// Get returns the skill at id; the bool is false if absent.
	Get(id string) (Skill, bool, error)
	// All returns every skill, sorted deterministically by CreatedMS then ID.
	All() ([]Skill, error)
	// Count returns the number of active skills (the retrieval-pool size).
	Count() int
	// Close releases any held resources.
	Close() error
}

// ErrEmptyBody is returned when a skill is written with no body.
//
// The sentinel itself lives in the contract layer (kernel/contract/skillapi);
// the name stays here as an alias, so `errors.Is` still matches it.
var ErrEmptyBody = skillapi.ErrEmptyBody

// FileStore is the file-backed Store. All skills live in a single
// <dir>/skills.json object keyed by id, snapshotted atomically on every
// mutation (write-temp + rename). Mirrors kernel/modules/knowledge/worldmodel's FileStore.
type FileStore struct {
	path string

	mu   sync.RWMutex
	data map[string]Skill
}

// Open opens (or creates) a FileStore under dir, loading <dir>/skills.json if
// present. The directory is created if absent.
func Open(dir string) (*FileStore, error) {
	s := &FileStore{data: make(map[string]Skill)}
	path, err := filestore.LoadFrom(dir, "skills.json", &s.data)
	if err != nil {
		return nil, fmt.Errorf("skill: %w", err)
	}
	s.path = path
	if s.data == nil {
		s.data = make(map[string]Skill)
	}
	return s, nil
}

// Put implements Store.
func (s *FileStore) Put(sk Skill) error {
	if sk.ID == "" {
		return errors.New("skill: record id required")
	}
	if strings.TrimSpace(sk.Body) == "" {
		return ErrEmptyBody
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sk.ID] = sk
	return s.snapshotLocked()
}

// Get implements Store.
func (s *FileStore) Get(id string) (Skill, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, ok := s.data[id]
	return sk, ok, nil
}

// All implements Store. Sorted by CreatedMS then ID so two consecutive calls
// produce identical output (deterministic CLI + snapshot tests).
func (s *FileStore) All() ([]Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Skill, 0, len(s.data))
	for _, sk := range s.data {
		out = append(out, sk)
	}
	sortSkills(out)
	return out, nil
}

// Count implements Store — the number of active skills.
func (s *FileStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, sk := range s.data {
		if sk.Active() {
			n++
		}
	}
	return n
}

// Close implements Store. Mutations persist synchronously, so this is a no-op.
func (s *FileStore) Close() error { return nil }

// snapshotLocked writes the whole skill map atomically. Caller holds s.mu.
func (s *FileStore) snapshotLocked() error {
	if err := filestore.Save(s.path, s.data); err != nil {
		return fmt.Errorf("skill: %w", err)
	}
	return nil
}

func sortSkills(sk []Skill) {
	sort.Slice(sk, func(i, j int) bool {
		if sk[i].CreatedMS != sk[j].CreatedMS {
			return sk[i].CreatedMS < sk[j].CreatedMS
		}
		return sk[i].ID < sk[j].ID
	})
}

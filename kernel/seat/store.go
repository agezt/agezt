// SPDX-License-Identifier: MIT

package seat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	stdruntime "runtime"
	"strings"
	"sync"
)

const (
	storeVersion = 1
	maxCustom    = 500
)

var (
	ErrNotFound   = errors.New("seat: not found")
	ErrBuiltin    = errors.New("seat: built-in seats cannot be modified")
	ErrExists     = errors.New("seat: id already in use")
	ErrInvalidID  = errors.New("seat: id must be lowercase letters, digits, and dashes")
	ErrInvalidIso = errors.New("seat: execution profile must be empty, local, warden, or container")
)

var idRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)

// Store overlays operator-defined custom seats on top of the seeded built-in
// catalog. Only custom seats are persisted; built-ins live in code and are
// non-deletable. Resolution is custom-first, then built-in.
type Store struct {
	path   string
	mu     sync.Mutex
	custom []*Seat
}

type diskState struct {
	Version int     `json:"version"`
	Seats   []*Seat `json:"seats"`
}

// OpenStore opens or creates the custom-seat store under dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("seat: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "seats.json")}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("seat: read %s: %w", s.path, err)
		}
		// The main file is missing. A crash between Remove(path) and the retry
		// Rename in saveLocked's Windows atomic-write fallback can leave the only
		// surviving copy in the temp file — recover it rather than starting
		// empty. A corrupt temp is ignored (start empty; never fail boot).
		tb, terr := os.ReadFile(s.path + ".tmp")
		if terr != nil || len(tb) == 0 {
			return s, nil
		}
		if err := s.load(tb); err != nil {
			s.custom = nil
			return s, nil
		}
		_ = os.Rename(s.path+".tmp", s.path)
		return s, nil
	}
	if len(b) == 0 {
		return s, nil
	}
	if err := s.load(b); err != nil {
		return nil, fmt.Errorf("seat: parse %s: %w", s.path, err)
	}
	return s, nil
}

// load parses a diskState blob and appends its custom seats to the store.
func (s *Store) load(b []byte) error {
	var st diskState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	for _, seat := range st.Seats {
		if seat == nil || IsBuiltin(seat.ID) {
			continue
		}
		cp := cloneSeat(*seat)
		cp.Builtin = false
		s.custom = append(s.custom, &cp)
	}
	return nil
}

// List returns the full catalog: built-ins first (in seeded order), then custom
// seats.
func (s *Store) List() []Seat {
	out := Builtins()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, seat := range s.custom {
		out = append(out, cloneSeat(*seat))
	}
	return out
}

// Get resolves a seat id: empty/"default" → the default built-in; a custom seat
// wins over a built-in of the same id (which cannot happen at create time).
func (s *Store) Get(id string) (Seat, bool) {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		id = "default"
	}
	s.mu.Lock()
	for _, seat := range s.custom {
		if seat.ID == id {
			cp := cloneSeat(*seat)
			s.mu.Unlock()
			return cp, true
		}
	}
	s.mu.Unlock()
	return Get(id)
}

// Valid reports whether id names a known seat (empty = default, always valid).
func (s *Store) Valid(id string) bool {
	if strings.TrimSpace(id) == "" {
		return true
	}
	_, ok := s.Get(id)
	return ok
}

// Create adds a custom seat. The id must be a fresh slug (not a built-in, not an
// existing custom seat) and the execution profile must be warden-family.
func (s *Store) Create(spec Seat) (Seat, error) {
	spec.ID = strings.TrimSpace(strings.ToLower(spec.ID))
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Description = strings.TrimSpace(spec.Description)
	spec.ExecutionProfile = strings.TrimSpace(strings.ToLower(spec.ExecutionProfile))
	if !idRe.MatchString(spec.ID) {
		return Seat{}, ErrInvalidID
	}
	if IsBuiltin(spec.ID) {
		return Seat{}, ErrExists
	}
	if !ValidExecutionProfile(spec.ExecutionProfile) {
		return Seat{}, ErrInvalidIso
	}
	if spec.Name == "" {
		spec.Name = spec.ID
	}
	spec.Builtin = false
	spec.Tools = cleanStrings(spec.Tools)
	spec.ModelChain = cleanStrings(spec.ModelChain)
	spec.RestrictTools = spec.RestrictTools || len(spec.Tools) > 0
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, seat := range s.custom {
		if seat.ID == spec.ID {
			return Seat{}, ErrExists
		}
	}
	if len(s.custom) >= maxCustom {
		return Seat{}, fmt.Errorf("seat: at most %d custom seats", maxCustom)
	}
	cp := cloneSeat(spec)
	s.custom = append(s.custom, &cp)
	if err := s.saveLocked(); err != nil {
		s.custom = s.custom[:len(s.custom)-1]
		return Seat{}, err
	}
	return cloneSeat(cp), nil
}

// Delete removes a custom seat. Built-in seats are refused.
func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(strings.ToLower(id))
	if IsBuiltin(id) {
		return ErrBuiltin
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, seat := range s.custom {
		if seat.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	removed := s.custom[idx]
	s.custom = append(s.custom[:idx], s.custom[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		s.custom = append(s.custom, nil)
		copy(s.custom[idx+1:], s.custom[idx:])
		s.custom[idx] = removed
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(diskState{Version: storeVersion, Seats: s.custom}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if stdruntime.GOOS == "windows" {
			if removeErr := os.Remove(s.path); removeErr == nil || os.IsNotExist(removeErr) {
				if retryErr := os.Rename(tmp, s.path); retryErr == nil {
					return nil
				} else {
					// The target is already gone but the retry rename failed, so
					// tmp now holds the only copy. Preserve it (do NOT remove) so
					// the next OpenStore can recover the custom seats.
					return retryErr
				}
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

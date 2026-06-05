// SPDX-License-Identifier: MIT

// Package state implements the first-class mutable state store
// (DECISIONS B0c). Frequently-read kernel/plugin state is read here directly
// rather than by folding the event log every time; the log remains the
// audit/replay/revert truth.
//
// M0.5 implementation: a per-namespace JSON file under <dir>, snapshotted
// atomically (write-temp + rename) on every mutation. Simple, correct,
// crash-safe, but not optimized for high-write loads. The contract behind
// the Store interface is stable; a CobaltDB-class engine (DECISIONS D2) can
// replace this implementation without changing callers.
//
// Concurrency: a single Store instance is safe for concurrent use.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Store is the namespaced key/value interface. The contract exposes this as
// kernel.stateGet / kernel.stateSet to plugins (agezt-contract.jsonc §3).
type Store interface {
	// Get returns the raw JSON value at (ns, key). The second return is
	// false if the key is absent.
	Get(ns, key string) (json.RawMessage, bool, error)
	// Set stores value at (ns, key). value is JSON-marshaled; if it is
	// already a json.RawMessage it is stored as-is.
	Set(ns, key string, value any) error
	// Delete removes (ns, key). Absent keys are not an error.
	Delete(ns, key string) error
	// Keys lists the keys present in ns, sorted. Absent namespace → empty.
	Keys(ns string) ([]string, error)
	// Close releases any held resources.
	Close() error
}

// ErrInvalidNamespace is returned when a namespace contains characters
// unsafe for use as a filename (`/`, `\`, `:`, `..`, etc.).
var ErrInvalidNamespace = errors.New("state: invalid namespace")

// FileStore is the file-backed Store. Each namespace lives in <dir>/<ns>.json.
type FileStore struct {
	dir string

	mu   sync.RWMutex
	data map[string]map[string]json.RawMessage // ns → key → value
}

// Open opens (or creates) a FileStore at dir, loading every *.json file as
// a namespace. The directory is created if absent.
func Open(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("state: mkdir %s: %w", dir, err)
	}
	s := &FileStore{dir: dir, data: make(map[string]map[string]json.RawMessage)}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("state: readdir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ns := strings.TrimSuffix(e.Name(), ".json")
		if err := validateNamespace(ns); err != nil {
			// Skip foreign files; don't error.
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("state: read %s: %w", e.Name(), err)
		}
		if len(raw) == 0 {
			s.data[ns] = make(map[string]json.RawMessage)
			continue
		}
		var bucket map[string]json.RawMessage
		if err := json.Unmarshal(raw, &bucket); err != nil {
			return nil, fmt.Errorf("state: parse %s: %w", e.Name(), err)
		}
		s.data[ns] = bucket
	}
	return s, nil
}

// Get implements Store.
func (s *FileStore) Get(ns, key string) (json.RawMessage, bool, error) {
	if err := validateNamespace(ns); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket, ok := s.data[ns]
	if !ok {
		return nil, false, nil
	}
	v, ok := bucket[key]
	if !ok {
		return nil, false, nil
	}
	// Return a copy so callers cannot mutate the in-memory store.
	out := make(json.RawMessage, len(v))
	copy(out, v)
	return out, true, nil
}

// Set implements Store. The whole namespace file is rewritten atomically.
func (s *FileStore) Set(ns, key string, value any) error {
	if err := validateNamespace(ns); err != nil {
		return err
	}
	raw, err := toRawMessage(value)
	if err != nil {
		return fmt.Errorf("state: marshal value: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.data[ns]
	if !ok {
		bucket = make(map[string]json.RawMessage)
		s.data[ns] = bucket
	}
	bucket[key] = raw
	return s.snapshotLocked(ns)
}

// Delete implements Store.
func (s *FileStore) Delete(ns, key string) error {
	if err := validateNamespace(ns); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.data[ns]
	if !ok {
		return nil
	}
	if _, present := bucket[key]; !present {
		return nil
	}
	delete(bucket, key)
	return s.snapshotLocked(ns)
}

// Namespaces returns a sorted list of every namespace currently
// loaded in the store. Used by the control plane to power
// `agt state list` — operators frequently need to discover what
// the agent loop / scheduler / planner have been writing without
// shelling into the data dir.
func (s *FileStore) Namespaces() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for ns := range s.data {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}

// Keys implements Store.
func (s *FileStore) Keys(ns string) ([]string, error) {
	if err := validateNamespace(ns); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket, ok := s.data[ns]
	if !ok {
		return nil, nil
	}
	out := make([]string, 0, len(bucket))
	for k := range bucket {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// Close implements Store. The on-disk representation is up to date already
// (Set/Delete persist synchronously); this is a no-op.
func (s *FileStore) Close() error { return nil }

// snapshotLocked writes the namespace bucket atomically. Caller holds s.mu.
func (s *FileStore) snapshotLocked(ns string) error {
	bucket := s.data[ns]
	if len(bucket) == 0 {
		// Remove the file if the namespace is now empty.
		path := s.pathFor(ns)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("state: remove %s: %w", path, err)
		}
		return nil
	}
	// Marshal with sorted keys for deterministic diffs on disk. Go's
	// encoding/json already sorts map keys alphabetically.
	body, err := json.MarshalIndent(bucket, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal ns %s: %w", ns, err)
	}
	return atomicWrite(s.pathFor(ns), body)
}

func (s *FileStore) pathFor(ns string) string {
	return filepath.Join(s.dir, ns+".json")
}

// atomicWrite writes data to a temp file and renames it over the target.
// os.Rename replaces atomically on POSIX and on Windows (MoveFileEx).
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("state: open temp %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("state: write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("state: fsync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("state: close temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("state: rename %s: %w", path, err)
	}
	return nil
}

func toRawMessage(v any) (json.RawMessage, error) {
	if rm, ok := v.(json.RawMessage); ok {
		// Validate a pre-serialized value up front (M426): without this, an invalid
		// RawMessage (e.g. a malformed plugin/tool result handed in via the passthrough
		// path) is written into the in-memory map before snapshotLocked re-marshals the
		// whole namespace and fails — leaving the bad entry resident, which wedges every
		// subsequent Set to that namespace and makes Get return invalid JSON that
		// diverges from disk. Rejecting it here keeps the map consistent with disk.
		if !json.Valid(rm) {
			return nil, fmt.Errorf("invalid json.RawMessage")
		}
		return rm, nil
	}
	return json.Marshal(v)
}

// validateNamespace rejects characters that would let a namespace escape
// the store directory.
func validateNamespace(ns string) error {
	if ns == "" {
		return fmt.Errorf("%w: empty", ErrInvalidNamespace)
	}
	if ns == "." || ns == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidNamespace, ns)
	}
	for _, c := range ns {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-' || c == '.':
		default:
			return fmt.Errorf("%w: %q contains forbidden char %q", ErrInvalidNamespace, ns, c)
		}
	}
	return nil
}

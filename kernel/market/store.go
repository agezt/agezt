// SPDX-License-Identifier: MIT

package market

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Store is the marketplace's durable state under <base>/market/:
//   - installed.json   — provenance of every installed pack (what it materialized)
//   - marketplaces/<n>/index.json — cached remote marketplace indexes (Phase 2)
//
// It mirrors kernel/catalog.Store: atomic writes, crash-safe, mutex-guarded.
type Store struct {
	dir string
	mu  sync.RWMutex
}

// NewStore returns a Store rooted at <baseDir>/market (created on first write).
func NewStore(baseDir string) *Store {
	return &Store{dir: filepath.Join(baseDir, "market")}
}

func (s *Store) installedPath() string { return filepath.Join(s.dir, "installed.json") }

// Installed returns the recorded installed packs (empty slice if none yet).
func (s *Store) Installed() ([]InstalledPack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadInstalled()
}

func (s *Store) loadInstalled() ([]InstalledPack, error) {
	data, err := os.ReadFile(s.installedPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []InstalledPack
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("market: parse installed.json: %w", err)
	}
	return out, nil
}

// InstalledByName returns the install record for a pack, if present.
func (s *Store) InstalledByName(name string) (InstalledPack, bool, error) {
	list, err := s.Installed()
	if err != nil {
		return InstalledPack{}, false, err
	}
	for _, p := range list {
		if p.Name == name {
			return p, true, nil
		}
	}
	return InstalledPack{}, false, nil
}

// RecordInstall upserts an install record (keyed by pack name) atomically.
func (s *Store) RecordInstall(rec InstalledPack) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadInstalled()
	if err != nil {
		return err
	}
	replaced := false
	for i := range list {
		if list[i].Name == rec.Name {
			list[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, rec)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return s.saveInstalled(list)
}

// RemoveInstall drops an install record by name. Returns whether it existed.
func (s *Store) RemoveInstall(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadInstalled()
	if err != nil {
		return false, err
	}
	out := list[:0]
	found := false
	for _, p := range list {
		if p.Name == name {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return false, nil
	}
	return found, s.saveInstalled(out)
}

func (s *Store) saveInstalled(list []InstalledPack) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.installedPath(), data, 0o644)
}

// atomicWrite writes data to a unique temp file in the same dir and renames it
// over the target, so a crash mid-write leaves the prior file intact and
// concurrent writers never race on a shared temp name (mirrors catalog.Store).
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".market-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// Flush to disk before rename: without it a crash right after the rename
	// can leave a zero-length/stale file on some filesystems.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

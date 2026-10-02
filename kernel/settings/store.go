// SPDX-License-Identifier: MIT

package settings

// Package documentation lives in doc.go.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/kernel/platform/filestore"
)

// utf8BOM is the byte-order mark some Windows editors (and PowerShell's
// Set-Content/Out-File) prepend to UTF-8 files. Go's JSON parser rejects it
// ("invalid character 'ï'"), so we strip it on read.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// FileName is the canonical filename under <baseDir>.
const FileName = "config.json"

// DefaultAccount is the single account exposed by the account-less accessors
// until multi-account lands.
const DefaultAccount = "_default"

// Store is a file-backed, account-keyed config store. Safe for concurrent use.
type Store struct {
	Path string

	mu       sync.RWMutex
	accounts map[string]map[string]string // account -> {AGEZT_X: value}
	// pending holds this Store's unsaved default-account changes (nil value =
	// removal). Save applies exactly these onto config.json as it is on disk at
	// save time: every control-plane handler, `agt config` and the daemon open
	// their own Store, and saving a whole stale map let the later writer
	// silently revert the other's setting.
	pending map[string]*string
}

// NewStore returns a Store at <baseDir>/config.json. Touches no files until Load.
func NewStore(baseDir string) *Store {
	return &Store{
		Path:     filepath.Join(baseDir, FileName),
		accounts: map[string]map[string]string{},
	}
}

// Load reads config.json. A missing file is an empty store (the first-run state),
// not an error. The on-disk form is the nested {account: {k:v}} map; a legacy
// flat {k:v} file is accepted and folded into the default account.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	accounts, err := s.readFile()
	if err != nil {
		return err
	}
	s.accounts, s.pending = accounts, nil
	return nil
}

// readFile decodes config.json as it is on disk now.
func (s *Store) readFile() (map[string]map[string]string, error) {
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]map[string]string{}, nil
		}
		return nil, fmt.Errorf("settings: read %s: %w", s.Path, err)
	}
	raw = bytes.TrimPrefix(raw, utf8BOM)
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]map[string]string{}, nil
	}

	var nested map[string]map[string]string
	if err := json.Unmarshal(raw, &nested); err == nil {
		if nested == nil {
			nested = map[string]map[string]string{}
		}
		return nested, nil
	}
	// Fall back to a flat {k:v} file (hand-written or legacy) → default account.
	var flat map[string]string
	if err := json.Unmarshal(raw, &flat); err != nil {
		return nil, fmt.Errorf("settings: parse %s: %w", s.Path, err)
	}
	return map[string]map[string]string{DefaultAccount: flat}, nil
}

// account returns the (mutable) map for acct, creating it if absent. Caller holds
// the write lock.
func (s *Store) account(acct string) map[string]string {
	m := s.accounts[acct]
	if m == nil {
		m = map[string]string{}
		s.accounts[acct] = m
	}
	return m
}

// Get returns the value for name in the default account, and whether it was set.
func (s *Store) Get(name string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.accounts[DefaultAccount][name]
	return v, ok
}

// Set stores name=value in the default account (in memory; call Save to persist).
func (s *Store) Set(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.account(DefaultAccount)[name] = value
	s.markLocked(name, &value)
}

// Remove deletes name from the default account; reports whether it was present.
func (s *Store) Remove(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.accounts[DefaultAccount]
	if _, ok := m[name]; !ok {
		return false
	}
	delete(m, name)
	s.markLocked(name, nil)
	return true
}

// markLocked records an unsaved change for Save to merge (nil = removal).
// Caller holds s.mu for writing.
func (s *Store) markLocked(name string, value *string) {
	if s.pending == nil {
		s.pending = map[string]*string{}
	}
	s.pending[name] = value
}

// All returns a copy of the default account's settings.
func (s *Store) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.accounts[DefaultAccount]))
	for k, v := range s.accounts[DefaultAccount] {
		out[k] = v
	}
	return out
}

// Names returns the default account's keys, sorted.
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.accounts[DefaultAccount]))
	for k := range s.accounts[DefaultAccount] {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Save atomically writes the store to disk (0600), always in the nested
// account-keyed form.
//
// It writes this Store's changes (Set/Remove since the last Load or Save) onto
// config.json as it is NOW, under a cross-process lock — not this Store's whole
// map — so a setting another writer saved in the meantime survives. The merged
// result becomes this Store's view.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := filestore.Lock(s.Path)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	defer unlock()
	out, err := s.readFile()
	if err != nil {
		return fmt.Errorf("settings: re-read before save: %w", err)
	}
	def := out[DefaultAccount]
	if def == nil {
		def = map[string]string{}
		out[DefaultAccount] = def
	}
	for name, v := range s.pending {
		if v == nil {
			delete(def, name)
		} else {
			def[name] = *v
		}
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	if err := atomicWrite(s.Path, raw); err != nil {
		return err
	}
	s.accounts, s.pending = out, nil
	return nil
}

// atomicWrite writes data to path via a unique temp file + rename, forcing 0600
// (rename can widen perms; Windows ignores Unix mode bits). Mirrors the vault's
// atomic write so two concurrent Saves can't corrupt each other.
func atomicWrite(path string, data []byte) error {
	if err := atomicfile.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

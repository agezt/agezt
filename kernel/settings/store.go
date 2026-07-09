// SPDX-License-Identifier: MIT

// Package settings is the file-backed config store behind the Config Center
// (M693): the NON-SECRET, operator-editable settings the daemon would otherwise
// only read from environment variables. It complements the credentials vault
// (kernel/creds) — secrets go there; everything else lives here.
//
// Shape: `<baseDir>/config.json`, 0600, atomically written. Internally the data
// is keyed by ACCOUNT so a future multi-account dimension nests cleanly; today a
// single "_default" account is exposed through the account-less accessors. The
// keys are the exact `AGEZT_*` env-var names, so the daemon can inject these into
// the process environment at startup and the existing ~170 `os.Getenv` consumers
// read them unchanged — no rewrite of config plumbing.
package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/agezt/agezt/internal/atomicfile"
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

	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.accounts = map[string]map[string]string{}
			return nil
		}
		return fmt.Errorf("settings: read %s: %w", s.Path, err)
	}
	raw = bytes.TrimPrefix(raw, utf8BOM)
	if len(bytes.TrimSpace(raw)) == 0 {
		s.accounts = map[string]map[string]string{}
		return nil
	}

	var nested map[string]map[string]string
	if err := json.Unmarshal(raw, &nested); err == nil {
		s.accounts = nested
		return nil
	}
	// Fall back to a flat {k:v} file (hand-written or legacy) → default account.
	var flat map[string]string
	if err := json.Unmarshal(raw, &flat); err != nil {
		return fmt.Errorf("settings: parse %s: %w", s.Path, err)
	}
	s.accounts = map[string]map[string]string{DefaultAccount: flat}
	return nil
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
	return true
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
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return fmt.Errorf("settings: ensure dir: %w", err)
	}
	// Always persist nested, ensuring the default account key exists.
	out := s.accounts
	if out == nil {
		out = map[string]map[string]string{}
	}
	if _, ok := out[DefaultAccount]; !ok {
		out[DefaultAccount] = map[string]string{}
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	return atomicWrite(s.Path, raw)
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

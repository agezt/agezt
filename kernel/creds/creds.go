// SPDX-License-Identifier: MIT

// Package creds is the local credentials vault for provider env vars.
//
// Motivation (M1.o): the catalog pivot (M1.f–M1.n) made *providers* a
// catalog refresh away. Credentials were still shell env vars, which
// doesn't scale past a couple of providers — operators with 10+ keys
// don't want them all in rc files. The vault is a flat JSON file
// (`~/.agezt/creds.json`, 0600 perms) holding env-var-name → value
// pairs that the daemon's cred resolver chains with `os.Getenv`.
//
// Scope (M1.o):
//
//   - Plain-JSON storage. No encryption, no OS-keychain integration.
//     The vault file inherits the same 0600 perms as the rest of
//     `~/.agezt/` and lives next to the journal.
//   - agt writes; daemon reads on startup. Re-export to pick up
//     vault changes (matches catalog-reload UX from M1.f).
//   - Lookup precedence (in `ChainLookup`): vault first, then env.
//     This lets operators temporarily override a vaulted key by
//     `export`-ing in a session without rewriting the vault.
//
// Out of scope (deferred):
//
//   - At-rest encryption (M1.o.x — likely via the OS keychain on
//     macOS/Linux/Windows once we add a small platform-specific
//     dep).
//   - Hot reload by the daemon. Today the daemon snapshot is
//     captured on Open; SIGHUP-driven reload lands when the
//     credentials-rotation UX is fleshed out.
//   - Per-provider scoping. Env-var names are global (OPENAI_API_KEY
//     means the same thing wherever); a flat map is the right shape.
package creds

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

// FileName is the canonical filename under <BaseDir>.
const FileName = "creds.json"

// PassphraseEnvVar is the env var name the vault reads for at-rest
// encryption (M1.w). Empty value disables encryption (plaintext
// vault, backwards-compatible with M1.o behaviour).
const PassphraseEnvVar = "AGEZT_VAULT_PASSPHRASE"

// NewPassphraseEnvVar carries the *target* passphrase during a
// `agt vault rotate` (M1.ee). Distinct from PassphraseEnvVar so the
// operator can hold both simultaneously without the daemon reading
// the wrong one mid-rotation.
const NewPassphraseEnvVar = "AGEZT_VAULT_PASSPHRASE_NEW"

// Store is a file-backed credential vault. Safe for concurrent use.
type Store struct {
	Path string

	mu   sync.RWMutex
	data map[string]string

	// passphraseFn returns the passphrase for at-rest encryption.
	// Defaults to reading PassphraseEnvVar from the process env;
	// tests override to inject a known passphrase without mutating
	// the global environment.
	passphraseFn func() string
	// wasEncrypted remembers what the file looked like on Load. Save
	// uses this to surface "you're about to silently downgrade an
	// encrypted vault to plaintext" rather than just doing it.
	wasEncrypted bool
}

// NewStore returns a Store at <baseDir>/creds.json. Doesn't touch the
// filesystem until Load is called.
func NewStore(baseDir string) *Store {
	return &Store{
		Path:         filepath.Join(baseDir, FileName),
		data:         map[string]string{},
		passphraseFn: func() string { return os.Getenv(PassphraseEnvVar) },
	}
}

// SetPassphraseFn overrides the passphrase source. Used by tests to
// inject a known passphrase without mutating AGEZT_VAULT_PASSPHRASE.
// Pass nil to restore the default env-var lookup.
func (s *Store) SetPassphraseFn(fn func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn == nil {
		fn = func() string { return os.Getenv(PassphraseEnvVar) }
	}
	s.passphraseFn = fn
}

// IsEncrypted reports whether the most recently loaded vault file
// was in encrypted-envelope form. False before Load, false for
// fresh / missing vaults.
func (s *Store) IsEncrypted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.wasEncrypted
}

// Load reads the vault file. A missing file is treated as an empty
// vault (no error) — the canonical "first-run" state. A malformed
// file is an error so operators notice corruption.
//
// M1.w: detects the encrypted-envelope format. Encrypted vaults
// require AGEZT_VAULT_PASSPHRASE in the environment; returns
// ErrPassphraseRequired (when unset) or ErrWrongPassphrase
// (when set but doesn't decrypt) so the caller can produce a
// specific operator-facing message.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = map[string]string{}
			s.wasEncrypted = false
			return nil
		}
		return fmt.Errorf("creds: read %q: %w", s.Path, err)
	}
	if len(raw) == 0 {
		s.data = map[string]string{}
		s.wasEncrypted = false
		return nil
	}
	if isEncryptedVault(raw) {
		passphrase := s.passphraseFn()
		if passphrase == "" {
			return ErrPassphraseRequired
		}
		m, err := decryptVault(raw, passphrase)
		if err != nil {
			return err
		}
		s.data = m
		s.wasEncrypted = true
		return nil
	}
	// Legacy plaintext path (M1.o-compatible).
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("creds: parse %q: %w", s.Path, err)
	}
	s.data = m
	s.wasEncrypted = false
	return nil
}

// Save writes the vault file atomically (write-temp-then-rename) so a
// crashed agt invocation can't leave the file half-written. Sets the
// file's permissions to 0600 even on repeat writes — guards against
// an operator chmod-ing the file world-readable.
//
// M1.w: encrypts the file when AGEZT_VAULT_PASSPHRASE is set.
// When the passphrase is set, Save always writes encrypted; when
// unset, Save writes plaintext. Operators "upgrade" a plaintext
// vault by setting the env var and calling Save (e.g. via any
// `agt provider creds set`).
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return fmt.Errorf("creds: ensure dir: %w", err)
	}

	var raw []byte
	if passphrase := s.passphraseFn(); passphrase != "" {
		out, err := encryptVault(s.data, passphrase)
		if err != nil {
			return fmt.Errorf("creds: encrypt: %w", err)
		}
		raw = out
	} else {
		out, err := json.MarshalIndent(s.data, "", "  ")
		if err != nil {
			return fmt.Errorf("creds: marshal: %w", err)
		}
		raw = out
	}

	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return fmt.Errorf("creds: write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("creds: rename: %w", err)
	}
	// Re-apply 0600 in case umask or platform-default opened it wider
	// on rename (Windows in particular ignores Unix mode bits).
	_ = os.Chmod(s.Path, 0600)
	return nil
}

// Rotate re-encrypts the vault under a new passphrase, atomically
// replacing the on-disk file (M1.ee). Caller MUST have already
// called Load successfully — Rotate reads the in-memory plaintext,
// so a fresh Store with no Load would silently write an empty
// vault under the new passphrase.
//
// Algorithm:
//   1. Validate the new passphrase is non-empty (rejecting "" here
//      prevents accidentally turning the vault plaintext without
//      using `agt vault decrypt`).
//   2. Re-encrypt the in-memory data under newPassphrase using the
//      standard encrypt path (fresh salt + nonce per save — see
//      encrypt.go's encryptVault).
//   3. Atomic write (write-temp + rename) so a crash mid-rotation
//      leaves either the old vault intact OR the new vault intact —
//      never a half-written file. The temp file is removed on
//      rename failure.
//   4. Update the in-memory passphrase function so future Save calls
//      use the new passphrase. This means once Rotate returns the
//      Store is fully consistent — the caller doesn't need to update
//      AGEZT_VAULT_PASSPHRASE in the process env for subsequent
//      Saves to work.
//
// Errors leave the on-disk file unchanged (the temp file is removed
// on rename failure). The in-memory passphrase function is only
// updated AFTER the atomic rename succeeds, so a failed rotation
// also leaves the in-memory Store usable under the old passphrase.
//
// Why a dedicated method rather than "swap passphraseFn + Save":
// callers doing that incur a small race window where a concurrent
// Get / Has / Names hits an inconsistent state (passphraseFn updated
// but file not yet written, or vice versa). Rotate holds the write
// lock for the full operation.
func (s *Store) Rotate(newPassphrase string) error {
	if newPassphrase == "" {
		return errors.New("creds: rotate: new passphrase must be non-empty (use `agt vault decrypt` to switch to plaintext)")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return fmt.Errorf("creds: ensure dir: %w", err)
	}
	raw, err := encryptVault(s.data, newPassphrase)
	if err != nil {
		return fmt.Errorf("creds: rotate encrypt: %w", err)
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return fmt.Errorf("creds: rotate write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("creds: rotate rename: %w", err)
	}
	_ = os.Chmod(s.Path, 0600)
	// In-memory passphrase function now points at the new value so
	// subsequent Save() calls don't need the env var updated.
	s.passphraseFn = func() string { return newPassphrase }
	s.wasEncrypted = true
	return nil
}

// Set assigns a value to an env-var-style name. Empty value removes
// the entry — same convention as `unset` in a shell. Caller is
// responsible for Save.
func (s *Store) Set(name, value string) error {
	if err := validateName(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if value == "" {
		delete(s.data, name)
	} else {
		s.data[name] = value
	}
	return nil
}

// Get returns the value for name, or "" if absent. The empty-means-
// absent convention matches both os.Getenv and compat.CredLookup so a
// Store can be used as a CredLookup directly via the Lookup method.
func (s *Store) Get(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[name]
}

// Has reports whether name has a non-empty value in the vault.
func (s *Store) Has(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[name]
	return ok && v != ""
}

// Remove deletes the entry. Returns true if there was something to
// delete. Caller is responsible for Save.
func (s *Store) Remove(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existed := s.data[name]
	delete(s.data, name)
	return existed
}

// Names returns the sorted list of all stored env-var names.
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Lookup is the CredLookup-compatible signature. Equivalent to Get
// but expresses intent at call sites.
func (s *Store) Lookup(name string) string { return s.Get(name) }

// ChainLookup composes multiple credential sources into a single
// CredLookup-compatible function. The first source that returns a
// non-empty string wins. Typical use: vault first, env second, so
// an operator can `export FOO=...` to override a vaulted value for a
// shell session without rewriting the vault.
//
// Nil sources are skipped, so passing `(nil, os.Getenv)` works.
func ChainLookup(sources ...func(string) string) func(string) string {
	return func(name string) string {
		for _, src := range sources {
			if src == nil {
				continue
			}
			if v := src(name); v != "" {
				return v
			}
		}
		return ""
	}
}

// MaskValue redacts a credential for display: keeps the first 4 and
// last 4 chars (or fewer for short values), with the middle as dots.
// 8-character values and shorter are fully masked.
func MaskValue(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 8 {
		return strings.Repeat("•", len(v))
	}
	return v[:4] + strings.Repeat("•", 6) + v[len(v)-4:]
}

// validateName rejects empty or whitespace-only names so the vault
// can't accumulate unreachable junk entries.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("creds: env var name must be non-empty")
	}
	if name != strings.TrimSpace(name) {
		return errors.New("creds: env var name must not have leading/trailing whitespace")
	}
	return nil
}

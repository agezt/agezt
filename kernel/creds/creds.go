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
//
// The I/O operations (Load + Save + atomicWriteVault + Rotate) live in
// creds_io.go; the entry operations (Set + Get + Has + Remove + Names +
// Lookup + ChainLookup + MaskValue + validateName) live in creds_ops.go.
// Extracted from creds.go during the Day-203 god-file split.
// Public API unchanged.
package creds

import (
	"path/filepath"
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
// filesystem until Load is called. The default passphrase source is the M934
// chain: AGEZT_VAULT_PASSPHRASE wins, else the machine-bound key (so vaults
// encrypt at rest by default), else plaintext (opt-out / no identity source).
func NewStore(baseDir string) *Store {
	return &Store{
		Path:         filepath.Join(baseDir, FileName),
		data:         map[string]string{},
		passphraseFn: defaultPassphraseChain,
	}
}

// SetPassphraseFn overrides the passphrase source. Used by tests to
// inject a known passphrase without mutating AGEZT_VAULT_PASSPHRASE.
// Pass nil to restore the default chain (env passphrase → machine key).
func (s *Store) SetPassphraseFn(fn func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn == nil {
		fn = defaultPassphraseChain
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

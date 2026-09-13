// SPDX-License-Identifier: MIT
//
// creds I/O operations: Load + Save + atomicWriteVault + Rotate
// (the persistence glue + passphrase rotation).
// Extracted from creds.go during the Day-203 god-file split.
// Public API unchanged.
package creds

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/atomicfile"
)

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
			// A machine-derived key (M934) that fails usually means the vault
			// file came from ANOTHER machine/user, or was encrypted with an
			// explicit passphrase that is no longer in the env — say so, the
			// bare "wrong passphrase" reads like corruption to an operator who
			// never set one. (Only when the machine key was the one tried.)
			if errors.Is(err, ErrWrongPassphrase) && strings.HasPrefix(passphrase, "machine-v1:") {
				return fmt.Errorf("%w (the machine-bound key did not open it: the vault was likely encrypted on another machine/user, or with an explicit %s — set that env var to unlock)", err, PassphraseEnvVar)
			}
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

	return atomicWriteVault(s.Path, raw)
}

// atomicWriteVault writes data to path atomically: a UNIQUE temp file in the same
// directory (os.CreateTemp), fsynced, then renamed over path and forced to 0600.
// A unique temp name — rather than a fixed "<path>.tmp" — so two concurrent Save()
// calls (both holding only the read lock) can't race on the same temp file and
// corrupt each other's write (M471). 0600 is re-applied because rename can widen
// perms (Windows ignores Unix mode bits).
func atomicWriteVault(path string, data []byte) error {
	if err := atomicfile.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("creds: %w", err)
	}
	return nil
}

// Rotate re-encrypts the vault under a new passphrase, atomically
// replacing the on-disk file (M1.ee). Caller MUST have already
// called Load successfully — Rotate reads the in-memory plaintext,
// so a fresh Store with no Load would silently write an empty
// vault under the new passphrase.
//
// Algorithm:
//  1. Validate the new passphrase is non-empty (rejecting "" here
//     prevents accidentally turning the vault plaintext without
//     using `agt vault decrypt`).
//  2. Re-encrypt the in-memory data under newPassphrase using the
//     standard encrypt path (fresh salt + nonce per save — see
//     encrypt.go's encryptVault).
//  3. Atomic write (write-temp + rename) so a crash mid-rotation
//     leaves either the old vault intact OR the new vault intact —
//     never a half-written file. The temp file is removed on
//     rename failure.
//  4. Update the in-memory passphrase function so future Save calls
//     use the new passphrase. This means once Rotate returns the
//     Store is fully consistent — the caller doesn't need to update
//     AGEZT_VAULT_PASSPHRASE in the process env for subsequent
//     Saves to work.
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
	if err := atomicWriteVault(s.Path, raw); err != nil {
		return fmt.Errorf("creds: rotate: %w", err)
	}
	// In-memory passphrase function now points at the new value so
	// subsequent Save() calls don't need the env var updated.
	s.passphraseFn = func() string { return newPassphrase }
	s.wasEncrypted = true
	return nil
}

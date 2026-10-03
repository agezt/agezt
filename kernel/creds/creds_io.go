package creds

// Provenance: SPDX-License-Identifier: MIT creds I/O operations: Load + Save +
//             atomicWriteVault + Rotate (the persistence glue + passphrase
//             rotation). Extracted from creds.go during the Day-203 god-file split.
//             Public API unchanged.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/kernel/platform/filestore"
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
	m, encrypted, openedWith, err := s.readFile(s.passphraseFn())
	if err != nil {
		return err
	}
	s.data, s.wasEncrypted, s.openedWith, s.pending = m, encrypted, openedWith, nil
	return nil
}

// readFile decodes the vault file as it is on disk now. A missing or empty file
// is an empty vault. An encrypted file is opened with the first passphrase that
// works (empty candidates skipped) and reports which one did.
func (s *Store) readFile(passphrases ...string) (data map[string]string, encrypted bool, openedWith string, err error) {
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, false, "", nil
		}
		return nil, false, "", fmt.Errorf("creds: read %q: %w", s.Path, err)
	}
	if len(raw) == 0 {
		return map[string]string{}, false, "", nil
	}
	if !isEncryptedVault(raw) {
		// Legacy plaintext path (M1.o-compatible).
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, false, "", fmt.Errorf("creds: parse %q: %w", s.Path, err)
		}
		if m == nil {
			m = map[string]string{}
		}
		return m, false, "", nil
	}
	var lastErr error = ErrPassphraseRequired
	triedMachine := false
	for i, p := range passphrases {
		if p == "" || slices.Contains(passphrases[:i], p) {
			continue
		}
		m, err := decryptVault(raw, p)
		if err == nil {
			return m, true, p, nil
		}
		lastErr = err
		triedMachine = triedMachine || strings.HasPrefix(p, "machine-v1:")
	}
	// A machine-derived key (M934) that fails usually means the vault file came
	// from ANOTHER machine/user, or was encrypted with an explicit passphrase
	// that is no longer in the env — say so, the bare "wrong passphrase" reads
	// like corruption to an operator who never set one.
	if errors.Is(lastErr, ErrWrongPassphrase) && triedMachine {
		return nil, false, "", fmt.Errorf("%w (the machine-bound key did not open it: the vault was likely encrypted on another machine/user, or with an explicit %s — set that env var to unlock)", lastErr, PassphraseEnvVar)
	}
	return nil, false, "", lastErr
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
//
// Save writes this Store's changes (Set/Remove since the last Load or Save)
// onto the file as it is NOW, under a cross-process lock — not this Store's
// whole map — so a key another process saved in the meantime survives. See
// commitLocked.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitLocked(s.passphraseFn())
}

// commitLocked is the vault's only write path. Under the cross-process file
// lock it re-reads the file, applies exactly this Store's pending changes, and
// writes the result encrypted with writePass ("" writes plaintext). The merged
// result becomes the in-memory view, so this Store also picks up whatever other
// writers saved since it loaded.
//
// Re-reading is what makes concurrent writers safe: the daemon and `agt` each
// hold a Store over the same file, and saving a whole in-memory map would let
// the later save delete the keys the earlier one added. A file this Store
// cannot open is an error — overwriting what it could not read is how a vault
// gets clobbered.
//
// Caller holds s.mu for writing.
func (s *Store) commitLocked(writePass string) error {
	unlock, err := filestore.Lock(s.Path)
	if err != nil {
		return fmt.Errorf("creds: %w", err)
	}
	defer unlock()
	merged, _, _, err := s.readFile(s.openedWith, s.passphraseFn(), writePass)
	if err != nil {
		return fmt.Errorf("creds: re-read before save: %w", err)
	}
	for name, v := range s.pending {
		if v == nil {
			delete(merged, name)
		} else {
			merged[name] = *v
		}
	}
	var raw []byte
	if writePass != "" {
		if raw, err = encryptVault(merged, writePass); err != nil {
			return fmt.Errorf("creds: encrypt: %w", err)
		}
	} else if raw, err = json.MarshalIndent(merged, "", "  "); err != nil {
		return fmt.Errorf("creds: marshal: %w", err)
	}
	if err := atomicWriteVault(s.Path, raw); err != nil {
		return err
	}
	s.data, s.pending, s.openedWith, s.wasEncrypted = merged, nil, writePass, writePass != ""
	return nil
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
// replacing the on-disk file (M1.ee). Like Save it re-reads the file under
// the cross-process lock and re-encrypts THAT (plus this Store's pending
// changes), so a key another process saved since this Store loaded is carried
// over rather than dropped, and a Store that never loaded cannot write an
// empty vault.
//
// Algorithm:
//  1. Validate the new passphrase is non-empty (rejecting "" here
//     prevents accidentally turning the vault plaintext without
//     using `agt vault decrypt`).
//  2. Re-encrypt the current file contents under newPassphrase using the
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
	if err := s.commitLocked(newPassphrase); err != nil {
		return fmt.Errorf("creds: rotate: %w", err)
	}
	// In-memory passphrase function now points at the new value so
	// subsequent Save() calls don't need the env var updated.
	s.passphraseFn = func() string { return newPassphrase }
	return nil
}

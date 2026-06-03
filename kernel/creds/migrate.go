// SPDX-License-Identifier: MIT

package creds

import (
	"encoding/json"
	"fmt"
	"os"
)

// VaultStatus describes an on-disk vault's key-derivation parameters and whether
// they meet the current policy. It is the input to a migration decision: an
// encrypted vault written before the PBKDF2 switch (M172), or one whose stored
// iteration count is below the current policy, is readable but weaker than a
// freshly-saved vault and should be re-encrypted.
type VaultStatus struct {
	// Encrypted is false for a plaintext (or absent/empty) vault.
	Encrypted bool
	// KDF is the envelope's key-derivation id ("" when not encrypted).
	KDF string
	// Iterations is the envelope's stored iteration count (0 when not encrypted).
	Iterations int
	// UpToDate is true when the vault is encrypted with the current KDF (PBKDF2)
	// at or above the current iteration policy — i.e. nothing to migrate.
	UpToDate bool
}

// InspectVault reads a vault file's envelope WITHOUT decrypting it and reports
// its encryption parameters. A missing, empty, or plaintext vault returns
// Encrypted=false (and UpToDate=false — there is nothing encrypted to be out of
// date). It never needs the passphrase, so an operator can check migration
// status without unlocking the vault.
func InspectVault(path string) (VaultStatus, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return VaultStatus{}, nil
		}
		return VaultStatus{}, fmt.Errorf("creds: read %q: %w", path, err)
	}
	if len(raw) == 0 || !isEncryptedVault(raw) {
		return VaultStatus{}, nil
	}
	var env encryptedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return VaultStatus{}, fmt.Errorf("creds: parse vault envelope: %w", err)
	}
	return VaultStatus{
		Encrypted:  true,
		KDF:        env.KDF,
		Iterations: env.KDFIter,
		UpToDate:   env.KDF == KDFPBKDF2 && env.KDFIter >= KDFIterations,
	}, nil
}

// MigrateEncryption upgrades an encrypted vault to the current key-derivation
// policy — PBKDF2 at KDFIterations — by decrypting and re-encrypting it in
// place. The passphrase is unchanged; only the KDF and iteration parameters
// improve. It is a no-op (migrated=false) for a plaintext vault or one already
// at the current policy.
//
// The Store's passphrase function must be set (the same passphrase the vault was
// written with); a legacy vault decrypts with its stored KDF and Save re-writes
// it with the current one. before reports the vault's parameters prior to the
// migration so the caller can show what changed.
func (s *Store) MigrateEncryption() (migrated bool, before VaultStatus, err error) {
	before, err = InspectVault(s.Path)
	if err != nil {
		return false, before, err
	}
	if !before.Encrypted || before.UpToDate {
		return false, before, nil // plaintext, or already current — nothing to do
	}
	// Decrypt with the (legacy/low-iteration) KDF, then re-encrypt with the
	// current one. Save writes encrypted whenever the passphrase is set.
	if err := s.Load(); err != nil {
		return false, before, err
	}
	if err := s.Save(); err != nil {
		return false, before, err
	}
	return true, before, nil
}

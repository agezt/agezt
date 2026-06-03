// SPDX-License-Identifier: MIT

package creds

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectVault_LegacyPlaintextAbsent(t *testing.T) {
	dir := t.TempDir()

	// Absent vault → not encrypted.
	if st, err := InspectVault(filepath.Join(dir, "nope.json")); err != nil || st.Encrypted {
		t.Errorf("absent vault: status=%+v err=%v", st, err)
	}

	// Plaintext vault (flat map, no schema) → not encrypted.
	pt := filepath.Join(dir, "plain.json")
	if err := os.WriteFile(pt, []byte(`{"K":"v"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := InspectVault(pt); err != nil || st.Encrypted {
		t.Errorf("plaintext vault: status=%+v err=%v", st, err)
	}

	// Legacy-KDF encrypted vault → encrypted, not up to date.
	leg := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(leg, buildLegacyEnvelope(t, map[string]string{"K": "secret"}, "pw"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := InspectVault(leg)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Encrypted || st.KDF != KDFIteratedHMAC || st.UpToDate {
		t.Errorf("legacy vault status = %+v, want encrypted legacy KDF, not up to date", st)
	}
}

func TestMigrateEncryption_UpgradesLegacyAndPreservesData(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := os.WriteFile(s.Path, buildLegacyEnvelope(t, map[string]string{"API_KEY": "secret-123"}, "pw"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.SetPassphraseFn(func() string { return "pw" })

	migrated, before, err := s.MigrateEncryption()
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !migrated {
		t.Fatal("expected a legacy vault to migrate")
	}
	if before.KDF != KDFIteratedHMAC {
		t.Errorf("before KDF = %q, want legacy", before.KDF)
	}

	// The vault is now the current KDF at the current iteration policy.
	after, err := InspectVault(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if after.KDF != KDFPBKDF2 || after.Iterations != KDFIterations || !after.UpToDate {
		t.Errorf("after migration status = %+v, want current PBKDF2 up to date", after)
	}

	// The secret is preserved and still decrypts with the same passphrase.
	s2 := NewStore(dir)
	s2.SetPassphraseFn(func() string { return "pw" })
	if err := s2.Load(); err != nil {
		t.Fatalf("load migrated vault: %v", err)
	}
	if got := s2.Get("API_KEY"); got != "secret-123" {
		t.Errorf("migrated secret = %q, want secret-123", got)
	}

	// Migrating again is a no-op (already current).
	again, _, err := s2.MigrateEncryption()
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if again {
		t.Error("an already-current vault should not migrate again")
	}
}

func TestMigrateEncryption_NoOpForPlaintextAndAbsent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.SetPassphraseFn(func() string { return "pw" })

	// Absent vault.
	if migrated, _, err := s.MigrateEncryption(); err != nil || migrated {
		t.Errorf("absent vault: migrated=%v err=%v", migrated, err)
	}

	// Plaintext vault.
	if err := os.WriteFile(s.Path, []byte(`{"K":"v"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if migrated, _, err := s.MigrateEncryption(); err != nil || migrated {
		t.Errorf("plaintext vault: migrated=%v err=%v", migrated, err)
	}
}

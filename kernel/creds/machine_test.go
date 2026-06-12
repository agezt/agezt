// SPDX-License-Identifier: MIT

package creds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// requireMachineKey skips on hosts with no stable machine identity (rare:
// containers without /etc/machine-id) — there the chain correctly degrades to
// plaintext and these behaviours don't apply.
func requireMachineKey(t *testing.T) {
	t.Helper()
	if MachinePassphrase() == "" {
		t.Skip("no machine identity source on this host")
	}
}

func TestMachinePassphrase_StableAndTagged(t *testing.T) {
	requireMachineKey(t)
	a, b := MachinePassphrase(), MachinePassphrase()
	if a != b {
		t.Fatalf("machine passphrase not stable: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "machine-v1:") {
		t.Fatalf("machine passphrase %q missing the version tag", a)
	}
}

// TestStore_EncryptsAtRestByDefault (M934): with no explicit passphrase and no
// opt-out, Save writes an encrypted envelope keyed to this machine, and a fresh
// default-chain Store reads it back.
func TestStore_EncryptsAtRestByDefault(t *testing.T) {
	requireMachineKey(t)
	t.Setenv(PassphraseEnvVar, "")
	t.Setenv(AutoEncryptEnvVar, "")
	dir := t.TempDir()

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("FAKE_API_KEY", "sk-very-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !isEncryptedVault(raw) {
		t.Fatalf("vault saved PLAINTEXT despite the machine-bound default:\n%s", raw)
	}
	if strings.Contains(string(raw), "sk-very-secret") {
		t.Fatal("secret value visible in the on-disk vault")
	}

	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("fresh store Load: %v", err)
	}
	if got := s2.Get("FAKE_API_KEY"); got != "sk-very-secret" {
		t.Fatalf("roundtrip Get = %q", got)
	}
	if !s2.IsEncrypted() {
		t.Error("IsEncrypted() = false after loading the encrypted vault")
	}
}

// TestStore_AutoEncryptOptOut: AGEZT_VAULT_AUTOENCRYPT=off restores
// plaintext-at-rest (the pre-M934 default).
func TestStore_AutoEncryptOptOut(t *testing.T) {
	t.Setenv(PassphraseEnvVar, "")
	t.Setenv(AutoEncryptEnvVar, "off")
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Set("FAKE_API_KEY", "sk-plain"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil || m["FAKE_API_KEY"] != "sk-plain" {
		t.Fatalf("opt-out should save plaintext JSON, got err=%v raw=%s", err, raw)
	}
}

// TestStore_ExplicitPassphraseWinsOverMachineKey: AGEZT_VAULT_PASSPHRASE keeps
// its M1.w semantics — it always supersedes the machine-bound key.
func TestStore_ExplicitPassphraseWinsOverMachineKey(t *testing.T) {
	requireMachineKey(t)
	t.Setenv(PassphraseEnvVar, "operator-secret")
	t.Setenv(AutoEncryptEnvVar, "")
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Set("FAKE_API_KEY", "sk-x"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	// The machine key alone (env passphrase gone) must NOT open it — and the
	// error should point the operator at the passphrase env var.
	t.Setenv(PassphraseEnvVar, "")
	s2 := NewStore(dir)
	err := s2.Load()
	if err == nil {
		t.Fatal("machine key opened a vault encrypted with an explicit passphrase")
	}
	if !strings.Contains(err.Error(), PassphraseEnvVar) {
		t.Errorf("error should mention %s, got: %v", PassphraseEnvVar, err)
	}

	// With the passphrase back, it opens.
	t.Setenv(PassphraseEnvVar, "operator-secret")
	s3 := NewStore(dir)
	if err := s3.Load(); err != nil {
		t.Fatalf("Load with explicit passphrase: %v", err)
	}
	if got := s3.Get("FAKE_API_KEY"); got != "sk-x" {
		t.Fatalf("Get = %q", got)
	}
}

// TestStore_EncryptInPlace: the boot-time migration upgrades a legacy plaintext
// vault to encrypted; already-encrypted and empty vaults are no-ops.
func TestStore_EncryptInPlace(t *testing.T) {
	requireMachineKey(t)
	t.Setenv(PassphraseEnvVar, "")
	t.Setenv(AutoEncryptEnvVar, "")
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(`{"FAKE_API_KEY":"sk-legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	up, err := s.EncryptInPlace()
	if err != nil || !up {
		t.Fatalf("EncryptInPlace = (%v, %v), want (true, nil)", up, err)
	}
	raw, _ := os.ReadFile(path)
	if !isEncryptedVault(raw) {
		t.Fatalf("file still plaintext after EncryptInPlace:\n%s", raw)
	}
	if !s.IsEncrypted() {
		t.Error("IsEncrypted() = false after upgrade")
	}

	// Second call is a no-op.
	if up, err := s.EncryptInPlace(); err != nil || up {
		t.Fatalf("second EncryptInPlace = (%v, %v), want (false, nil)", up, err)
	}

	// Empty vault: nothing to do.
	s2 := NewStore(t.TempDir())
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if up, err := s2.EncryptInPlace(); err != nil || up {
		t.Fatalf("empty-vault EncryptInPlace = (%v, %v), want (false, nil)", up, err)
	}
}

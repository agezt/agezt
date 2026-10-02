// SPDX-License-Identifier: MIT

package configcenter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type mapVault struct {
	m     map[string]string
	saves int
}

func newMapVault() *mapVault                       { return &mapVault{m: map[string]string{}} }
func (v *mapVault) Get(n string) string            { return v.m[n] }
func (v *mapVault) Set(n, val string) error        { v.m[n] = val; return nil }
func (v *mapVault) Remove(n string) bool           { _, ok := v.m[n]; delete(v.m, n); return ok }
func (v *mapVault) Save() error                    { v.saves++; return nil }
func readFileString(t *testing.T, p string) string { b, _ := os.ReadFile(p); return string(b) }

const secretValue = "s3cr3t-db-password-value"

// TestSecretValuesLiveInTheVault: entry files used to hold every value in
// plaintext (0644, non-atomic), secrets included. With the vault attached a
// secret-rated value is stored there; its file carries neither the value nor
// its hash, and a reopened center loads it back from the vault.
func TestSecretValuesLiveInTheVault(t *testing.T) {
	dir := t.TempDir()
	v := newMapVault()
	c, err := New(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.UseVault(v); err != nil {
		t.Fatal(err)
	}
	if err := c.Set(&ConfigEntry{Key: "db.password", Value: secretValue, Rating: RatingSecret}); err != nil {
		t.Fatal(err)
	}
	file := readFileString(t, c.entryFile("db.password"))
	if strings.Contains(file, secretValue) || strings.Contains(file, hashValue(secretValue)) {
		t.Fatalf("entry file still carries the secret or its hash:\n%s", file)
	}
	if v.m[VaultPrefix+"db.password"] != secretValue {
		t.Fatalf("vault = %v, want the secret under %q", v.m, VaultPrefix+"db.password")
	}

	reopened, err := New(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.UseVault(v); err != nil {
		t.Fatal(err)
	}
	e, err := reopened.GetEntry("db.password")
	if err != nil || e.Value != secretValue || e.ValueHash != hashValue(secretValue) {
		t.Fatalf("reopened entry = %+v (err %v), want the value and hash restored from the vault", e, err)
	}
}

// TestUseVaultMigratesPlaintextSecrets: files written before the vault held the
// secret in plaintext; attaching the vault moves them and rewrites the file.
func TestUseVaultMigratesPlaintextSecrets(t *testing.T) {
	dir := t.TempDir()
	legacy, err := New(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Set(&ConfigEntry{Key: "api.token", Value: secretValue, Rating: RatingSecret}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFileString(t, legacy.entryFile("api.token")), secretValue) {
		t.Fatal("setup: without a vault the value is expected in the file")
	}

	v := newMapVault()
	c, err := New(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	n, err := c.UseVault(v)
	if err != nil || n != 1 {
		t.Fatalf("UseVault migrated %d (err %v), want 1", n, err)
	}
	if strings.Contains(readFileString(t, c.entryFile("api.token")), secretValue) {
		t.Fatal("plaintext secret left in the entry file after migration")
	}
	if e, _ := c.GetEntry("api.token"); e == nil || e.Value != secretValue {
		t.Fatal("migrated entry lost its value in memory")
	}
	if n, _ := c.UseVault(v); n != 0 {
		t.Fatalf("second UseVault migrated %d, want 0 (idempotent)", n)
	}
}

// TestReRatingAndDeletingVaultBackedEntries: an entry that stops being secret
// takes its value back into its file and leaves the vault; deleting a
// vault-backed entry removes its vault value too.
func TestReRatingAndDeletingVaultBackedEntries(t *testing.T) {
	v := newMapVault()
	c, err := New(DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.UseVault(v); err != nil {
		t.Fatal(err)
	}
	if err := c.Set(&ConfigEntry{Key: "k", Value: secretValue, Rating: RatingSecret}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateRating("k", RatingInternal); err != nil {
		t.Fatal(err)
	}
	var onDisk ConfigEntry
	if err := json.Unmarshal([]byte(readFileString(t, c.entryFile("k"))), &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Value != secretValue || onDisk.VaultBacked || len(v.m) != 0 {
		t.Fatalf("re-rated entry: file=%+v vault=%v", onDisk, v.m)
	}

	if err := c.Set(&ConfigEntry{Key: "gone", Value: secretValue, Rating: RatingSecret}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("gone"); err != nil {
		t.Fatal(err)
	}
	if len(v.m) != 0 {
		t.Fatalf("Delete left the value in the vault: %v", v.m)
	}
	if _, err := os.Stat(c.entryFile("gone")); !os.IsNotExist(err) {
		t.Fatalf("Delete left the entry file: %v", err)
	}
}

// TestEntryAndAuditFilesArePrivate: entry files and the audit log (value
// previews, full public values) are 0600 in a 0700 directory.
func TestEntryAndAuditFilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits")
	}
	dir := t.TempDir()
	cfg := DefaultConfig(dir)
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set(&ConfigEntry{Key: "feature.flag", Value: "on", Rating: RatingPublic}); err != nil {
		t.Fatal(err)
	}
	c.auditLog.Log(&ConfigAccessRequest{AgentID: "a", Key: "feature.flag"}, AccessAllowed, "auto", "ok", "on")
	audits, _ := filepath.Glob(filepath.Join(cfg.Dir, "audit_*.jsonl"))
	if len(audits) == 0 {
		t.Fatal("no audit file written")
	}
	for p, want := range map[string]os.FileMode{cfg.Dir: 0o700, c.entryFile("feature.flag"): 0o600, audits[0]: 0o600} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", p, got, want)
		}
	}
}

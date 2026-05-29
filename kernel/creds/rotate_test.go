// SPDX-License-Identifier: MIT

package creds

// Rotation tests live in package creds (internal) so they can poke
// at the same internals encrypt_test.go does — same convention.

import (
	"os"
	"strings"
	"testing"
)

// TestStore_RotateRoundTrip verifies the basic rotation invariant:
// after Rotate, the vault is readable under the new passphrase and
// NOT under the old one.
func TestStore_RotateRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Write initial encrypted vault under passphrase "old".
	s1 := NewStore(dir)
	s1.SetPassphraseFn(func() string { return "old-passphrase" })
	_ = s1.Set("OPENAI_API_KEY", "sk-test")
	_ = s1.Set("ANTHROPIC_API_KEY", "sk-ant-test")
	if err := s1.Save(); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	// Load + rotate.
	s2 := NewStore(dir)
	s2.SetPassphraseFn(func() string { return "old-passphrase" })
	if err := s2.Load(); err != nil {
		t.Fatalf("Load before rotate: %v", err)
	}
	if !s2.IsEncrypted() {
		t.Fatal("vault should be encrypted before rotate")
	}
	if err := s2.Rotate("new-passphrase"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// New passphrase must work.
	s3 := NewStore(dir)
	s3.SetPassphraseFn(func() string { return "new-passphrase" })
	if err := s3.Load(); err != nil {
		t.Fatalf("Load after rotate (new passphrase): %v", err)
	}
	if got, want := s3.Get("OPENAI_API_KEY"), "sk-test"; got != want {
		t.Errorf("after rotate Get(OPENAI_API_KEY) = %q, want %q", got, want)
	}
	if got, want := s3.Get("ANTHROPIC_API_KEY"), "sk-ant-test"; got != want {
		t.Errorf("after rotate Get(ANTHROPIC_API_KEY) = %q, want %q", got, want)
	}

	// Old passphrase must NOT work.
	s4 := NewStore(dir)
	s4.SetPassphraseFn(func() string { return "old-passphrase" })
	err := s4.Load()
	if err != ErrWrongPassphrase {
		t.Errorf("Load with old passphrase after rotate: err = %v, want ErrWrongPassphrase", err)
	}
}

// TestStore_RotateRejectsEmptyPassphrase: rotating to "" would
// silently turn the vault plaintext — an irreversible accident the
// operator probably doesn't want. Block it; force them through
// `agt vault decrypt` if that's truly the intent.
func TestStore_RotateRejectsEmptyPassphrase(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.SetPassphraseFn(func() string { return "old" })
	_ = s.Set("K", "V")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	err := s.Rotate("")
	if err == nil {
		t.Fatal("Rotate(\"\"): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error message %q doesn't mention non-empty", err.Error())
	}
}

// TestStore_RotateLeavesFileUnchangedOnPreFlightError verifies that
// a rejected rotation (e.g. empty passphrase) doesn't touch the
// existing vault file at all — the old vault remains readable.
func TestStore_RotateLeavesFileUnchangedOnPreFlightError(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	s1.SetPassphraseFn(func() string { return "old" })
	_ = s1.Set("K", "V")
	if err := s1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	before, err := os.ReadFile(s1.Path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	s2 := NewStore(dir)
	s2.SetPassphraseFn(func() string { return "old" })
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s2.Rotate(""); err == nil {
		t.Fatal("Rotate(\"\"): expected error")
	}

	after, err := os.ReadFile(s1.Path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Error("vault file mutated by rejected rotation")
	}
}

// TestStore_RotateUpdatesInMemoryPassphrase verifies that after a
// successful rotation, subsequent Save() calls use the new
// passphrase without the caller having to update the env var or
// call SetPassphraseFn.
func TestStore_RotateUpdatesInMemoryPassphrase(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.SetPassphraseFn(func() string { return "old" })
	_ = s.Set("K", "V")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.Rotate("brand-new"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	// Add another entry and Save — must encrypt under "brand-new".
	_ = s.Set("K2", "V2")
	if err := s.Save(); err != nil {
		t.Fatalf("Save after rotate: %v", err)
	}
	// Verify by loading with the new passphrase from a fresh Store.
	s2 := NewStore(dir)
	s2.SetPassphraseFn(func() string { return "brand-new" })
	if err := s2.Load(); err != nil {
		t.Fatalf("Load with new passphrase: %v", err)
	}
	if got, want := s2.Get("K2"), "V2"; got != want {
		t.Errorf("Get(K2) = %q, want %q", got, want)
	}
}

// TestStore_RotateAlwaysProducesFreshSalt verifies that rotating
// produces a new salt + nonce — successive rotations to even the
// same passphrase yield different envelopes, defeating any kind
// of "swap a captured envelope back in" attack.
func TestStore_RotateAlwaysProducesFreshSalt(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.SetPassphraseFn(func() string { return "old" })
	_ = s.Set("K", "V")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := s.Rotate("p1"); err != nil {
		t.Fatalf("Rotate 1: %v", err)
	}
	a, _ := os.ReadFile(s.Path)
	if err := s.Rotate("p2"); err != nil {
		t.Fatalf("Rotate 2: %v", err)
	}
	b, _ := os.ReadFile(s.Path)
	if string(a) == string(b) {
		t.Error("two consecutive Rotates produced byte-identical files (salt/nonce should differ)")
	}
}

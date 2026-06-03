// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

// A plaintext vault reports "not encrypted" and exits 0 without needing a
// passphrase (M264).
func TestVaultMigrate_NotEncrypted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	if err := os.WriteFile(creds.NewStore(dir).Path, []byte(`{"K":"v"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := cmdVaultMigrate(&out, &errb); code != 0 {
		t.Fatalf("code=%d, err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "not encrypted") {
		t.Errorf("output = %q, want a 'not encrypted' notice", out.String())
	}
}

// An encrypted vault already at the current KDF reports "already…current" and
// does not require the passphrase (the inspect path is passphrase-free).
func TestVaultMigrate_AlreadyCurrent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	t.Setenv(creds.PassphraseEnvVar, "pw")

	s := creds.NewStore(dir)
	if err := s.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil { // writes encrypted with the current KDF
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := cmdVaultMigrate(&out, &errb); code != 0 {
		t.Fatalf("code=%d, err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "already") {
		t.Errorf("output = %q, want an 'already current' notice", out.String())
	}
}

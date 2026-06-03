// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

// An encrypted vault at the current KDF shows its key-derivation policy and an
// "up to date" migration line — without needing the passphrase (M265).
func TestVaultStatus_EncryptedShowsKDF(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	t.Setenv(creds.PassphraseEnvVar, "pw")

	s := creds.NewStore(dir)
	if err := s.Set("API_KEY", "x"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := cmdVaultStatus(&out, &errb); code != 0 {
		t.Fatalf("code=%d, err=%s", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "key deriv:") || !strings.Contains(got, creds.KDFPBKDF2) {
		t.Errorf("output = %q, want a key-derivation line", got)
	}
	if !strings.Contains(got, "up to date") {
		t.Errorf("output = %q, want an 'up to date' migration line", got)
	}
}

// A plaintext vault shows no key-derivation line (there is no KDF to report).
func TestVaultStatus_PlaintextNoKDF(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	if err := os.WriteFile(creds.NewStore(dir).Path, []byte(`{"K":"v"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := cmdVaultStatus(&out, &errb); code != 0 {
		t.Fatalf("code=%d, err=%s", code, errb.String())
	}
	if got := out.String(); strings.Contains(got, "key deriv:") {
		t.Errorf("output = %q, want no key-derivation line for a plaintext vault", got)
	}
}

// SPDX-License-Identifier: MIT

package creds

// Encryption tests live in package creds (not creds_test) so they
// can reach encryptVault / decryptVault / isEncryptedVault / etc.
// directly. Same convention as the bedrock SigV4 tests.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	plain := map[string]string{
		"OPENAI_API_KEY":     "sk-test-1",
		"ANTHROPIC_API_KEY":  "sk-ant-2",
		"AWS_ACCESS_KEY_ID":  "AKID3",
	}
	const passphrase = "correct horse battery staple"

	envelope, err := encryptVault(plain, passphrase)
	if err != nil {
		t.Fatalf("encryptVault: %v", err)
	}
	if !isEncryptedVault(envelope) {
		t.Errorf("isEncryptedVault returned false for fresh envelope")
	}

	got, err := decryptVault(envelope, passphrase)
	if err != nil {
		t.Fatalf("decryptVault: %v", err)
	}
	if len(got) != len(plain) {
		t.Fatalf("decrypted len=%d, want %d", len(got), len(plain))
	}
	for k, v := range plain {
		if got[k] != v {
			t.Errorf("decrypted[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestEncrypt_FreshSaltAndNonce(t *testing.T) {
	// Two encryptions with the same passphrase and plaintext MUST
	// produce different ciphertexts (fresh salt + nonce per call).
	// Otherwise an attacker comparing two snapshots could detect
	// "vault didn't change" or run a replay.
	plain := map[string]string{"K": "V"}
	const passphrase = "x"

	a, err := encryptVault(plain, passphrase)
	if err != nil {
		t.Fatalf("encrypt #1: %v", err)
	}
	b, err := encryptVault(plain, passphrase)
	if err != nil {
		t.Fatalf("encrypt #2: %v", err)
	}
	if string(a) == string(b) {
		t.Error("two encryptions with same inputs produced identical output (salt/nonce not fresh)")
	}
	// Both must still decrypt to the same plaintext.
	for _, env := range [][]byte{a, b} {
		got, err := decryptVault(env, passphrase)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if got["K"] != "V" {
			t.Errorf("got %v", got)
		}
	}
}

func TestDecrypt_WrongPassphraseReturnsSentinel(t *testing.T) {
	env, _ := encryptVault(map[string]string{"K": "V"}, "right")
	_, err := decryptVault(env, "wrong")
	if err != ErrWrongPassphrase {
		t.Errorf("err = %v, want ErrWrongPassphrase", err)
	}
}

func TestDecrypt_TamperedCiphertextRejected(t *testing.T) {
	env, _ := encryptVault(map[string]string{"K": "V"}, "p")
	// Flip a byte in the ciphertext (find the field and mutate
	// one base64 char). GCM auth tag should reject.
	var e encryptedEnvelope
	if err := json.Unmarshal(env, &e); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if len(e.Ciphertext) < 5 {
		t.Fatal("ciphertext too short to tamper")
	}
	// Flip a char. Replace 'A' with 'B' (or vice-versa) somewhere
	// in the middle so the base64 still decodes to a different byte.
	idx := len(e.Ciphertext) / 2
	original := e.Ciphertext[idx]
	replacement := byte('A')
	if original == 'A' {
		replacement = 'B'
	}
	e.Ciphertext = e.Ciphertext[:idx] + string(replacement) + e.Ciphertext[idx+1:]
	tampered, _ := json.Marshal(e)
	if _, err := decryptVault(tampered, "p"); err != ErrWrongPassphrase {
		t.Errorf("tampered ciphertext: err = %v, want ErrWrongPassphrase", err)
	}
}

func TestDecrypt_EmptyPassphraseReturnsRequired(t *testing.T) {
	env, _ := encryptVault(map[string]string{"K": "V"}, "p")
	_, err := decryptVault(env, "")
	if err != ErrPassphraseRequired {
		t.Errorf("err = %v, want ErrPassphraseRequired", err)
	}
}

func TestDecrypt_RejectsLowIterationCount(t *testing.T) {
	// Adversary lowers KDFIter to make brute force cheap. Reject.
	env, _ := encryptVault(map[string]string{"K": "V"}, "p")
	var e encryptedEnvelope
	_ = json.Unmarshal(env, &e)
	e.KDFIter = 100
	tampered, _ := json.Marshal(e)
	_, err := decryptVault(tampered, "p")
	if err == nil || !strings.Contains(err.Error(), "kdf_iter") {
		t.Errorf("err = %v, want kdf_iter-too-low rejection", err)
	}
}

func TestIsEncryptedVault_DistinguishesPlainAndEncrypted(t *testing.T) {
	plain := []byte(`{"OPENAI_API_KEY":"x","ANTHROPIC_API_KEY":"y"}`)
	if isEncryptedVault(plain) {
		t.Error("plaintext vault detected as encrypted")
	}
	enc, _ := encryptVault(map[string]string{"K": "V"}, "p")
	if !isEncryptedVault(enc) {
		t.Error("encrypted vault not detected")
	}
	// Garbage isn't an encrypted vault either.
	if isEncryptedVault([]byte("not json")) {
		t.Error("garbage detected as encrypted")
	}
}

func TestStore_SaveEncryptedWhenPassphraseSet(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.SetPassphraseFn(func() string { return "secret-pass" })
	_ = s.Set("OPENAI_API_KEY", "sk-test")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// File on disk must be the envelope format, NOT plaintext JSON.
	raw, err := readFile(t, s.Path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !isEncryptedVault(raw) {
		t.Errorf("saved file is not encrypted envelope: %s", string(raw))
	}
	if strings.Contains(string(raw), "sk-test") {
		t.Errorf("plaintext credential visible in saved file: %s", string(raw))
	}
}

func TestStore_SavePlaintextWhenNoPassphrase(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.SetPassphraseFn(func() string { return "" })
	_ = s.Set("OPENAI_API_KEY", "sk-test")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, _ := readFile(t, s.Path)
	if isEncryptedVault(raw) {
		t.Errorf("file is encrypted but no passphrase set: %s", string(raw))
	}
	if !strings.Contains(string(raw), "sk-test") {
		t.Errorf("plaintext save missing the value: %s", string(raw))
	}
}

func TestStore_LoadEncryptedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	s1.SetPassphraseFn(func() string { return "p" })
	_ = s1.Set("K", "V")
	if err := s1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2 := NewStore(dir)
	s2.SetPassphraseFn(func() string { return "p" })
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s2.IsEncrypted() {
		t.Error("IsEncrypted = false after loading encrypted vault")
	}
	if s2.Get("K") != "V" {
		t.Errorf("Get(K) = %q, want V", s2.Get("K"))
	}
}

func TestStore_LoadEncryptedRequiresPassphrase(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	s1.SetPassphraseFn(func() string { return "p" })
	_ = s1.Set("K", "V")
	_ = s1.Save()

	s2 := NewStore(dir)
	s2.SetPassphraseFn(func() string { return "" })
	err := s2.Load()
	if err != ErrPassphraseRequired {
		t.Errorf("err = %v, want ErrPassphraseRequired", err)
	}
}

func TestStore_LoadEncryptedWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	s1.SetPassphraseFn(func() string { return "right" })
	_ = s1.Set("K", "V")
	_ = s1.Save()

	s2 := NewStore(dir)
	s2.SetPassphraseFn(func() string { return "wrong" })
	err := s2.Load()
	if err != ErrWrongPassphrase {
		t.Errorf("err = %v, want ErrWrongPassphrase", err)
	}
}

func TestStore_LegacyPlaintextStillLoads(t *testing.T) {
	// An M1.o vault (flat JSON map, no envelope) must continue to
	// load on M1.w. Operators don't need to migrate immediately.
	dir := t.TempDir()
	s := NewStore(dir)
	// Write a legacy plaintext file directly.
	writeFile(t, s.Path, []byte(`{"OPENAI_API_KEY":"sk-legacy"}`))

	if err := s.Load(); err != nil {
		t.Fatalf("Load legacy: %v", err)
	}
	if s.IsEncrypted() {
		t.Error("IsEncrypted = true for plaintext file")
	}
	if got := s.Get("OPENAI_API_KEY"); got != "sk-legacy" {
		t.Errorf("Get = %q, want sk-legacy", got)
	}
}

// ---- helpers ----

func readFile(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(path)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

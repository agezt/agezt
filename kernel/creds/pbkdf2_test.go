// SPDX-License-Identifier: MIT

package creds

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// TestDeriveKeyPBKDF2_KnownAnswers pins deriveKeyPBKDF2 to published
// PBKDF2-HMAC-SHA256 (dkLen=32) test vectors. This is the correctness proof for
// M172: if the XOR-accumulation or the INT32BE(1) block index were wrong, these
// would not match. (The derived key is the full 32-byte output here.)
func TestDeriveKeyPBKDF2_KnownAnswers(t *testing.T) {
	cases := []struct {
		pass, salt string
		iter       int
		wantHex    string
	}{
		{"password", "salt", 1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{"password", "salt", 2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{"passwordPASSWORDpassword", "saltSALTsaltSALTsaltSALTsaltSALTsalt", 4096, "348c89dbcbd32b2f32d814b8116e84cf2b17347ebc1800181c4e2a1fb8dd53e1"},
	}
	for _, c := range cases {
		got := hex.EncodeToString(deriveKeyPBKDF2([]byte(c.pass), []byte(c.salt), c.iter))
		if got != c.wantHex {
			t.Errorf("PBKDF2(%q,%q,%d) = %s, want %s", c.pass, c.salt, c.iter, got, c.wantHex)
		}
	}
}

// TestDecrypt_LegacyKDFStillReadable — a vault written with the pre-M172 legacy
// KDF (hmac-sha256-iter) must still decrypt (backward compatibility), while new
// vaults use PBKDF2.
func TestDecrypt_LegacyKDFStillReadable(t *testing.T) {
	const pass = "operator-passphrase-123"
	plain := map[string]string{"OPENAI_API_KEY": "sk-legacyvaultsecret"}

	legacy := buildLegacyEnvelope(t, plain, pass)

	got, err := decryptVault(legacy, pass)
	if err != nil {
		t.Fatalf("legacy vault must still decrypt: %v", err)
	}
	if got["OPENAI_API_KEY"] != "sk-legacyvaultsecret" {
		t.Errorf("legacy decrypt wrong value: %v", got)
	}
	// Wrong passphrase on a legacy vault still fails cleanly.
	if _, err := decryptVault(legacy, "wrong"); err != ErrWrongPassphrase {
		t.Errorf("legacy wrong-passphrase err = %v, want ErrWrongPassphrase", err)
	}
}

// TestEncrypt_UsesPBKDF2 — new envelopes declare the PBKDF2 KDF id and round-trip.
func TestEncrypt_UsesPBKDF2(t *testing.T) {
	const pass = "operator-passphrase-123"
	raw, err := encryptVault(map[string]string{"K": "v_secret_value_1234"}, pass)
	if err != nil {
		t.Fatal(err)
	}
	var env encryptedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.KDF != KDFPBKDF2 {
		t.Errorf("new vault KDF = %q, want %q", env.KDF, KDFPBKDF2)
	}
	back, err := decryptVault(raw, pass)
	if err != nil || back["K"] != "v_secret_value_1234" {
		t.Errorf("PBKDF2 round-trip failed: %v / %v", err, back)
	}
}

// TestDecrypt_RejectsLowIterFloor — an envelope claiming an implausibly low
// iteration count is refused (M172 raised the floor to KDFIterMinAccepted).
func TestDecrypt_RejectsLowIterFloor(t *testing.T) {
	const pass = "operator-passphrase-123"
	raw, _ := encryptVault(map[string]string{"K": "v_secret_value_1234"}, pass)
	var env encryptedEnvelope
	_ = json.Unmarshal(raw, &env)
	env.KDFIter = 1000 // below the floor; above the old 1000-floor boundary
	tampered, _ := json.Marshal(env)
	if _, err := decryptVault(tampered, pass); err == nil || !strings.Contains(err.Error(), "implausibly low") {
		t.Errorf("low-iter envelope err = %v, want 'implausibly low'", err)
	}
}

// buildLegacyEnvelope re-creates exactly what the pre-M172 encryptVault produced:
// a fresh salt, the legacy HMAC-chain key, AES-256-GCM, KDF id hmac-sha256-iter.
func buildLegacyEnvelope(t *testing.T, plain map[string]string, pass string) []byte {
	t.Helper()
	salt := make([]byte, SaltBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		t.Fatal(err)
	}
	key := deriveKeyLegacyHMAC([]byte(pass), salt, KDFIterations)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, NonceBytes)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	pt, _ := json.Marshal(plain)
	ct := gcm.Seal(nil, nonce, pt, nil)
	env := encryptedEnvelope{
		Schema:     SchemaEncrypted,
		Encryption: AlgorithmAESGCM,
		KDF:        KDFIteratedHMAC,
		KDFIter:    KDFIterations,
		KDFSalt:    base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}
	raw, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

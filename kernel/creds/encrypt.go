// SPDX-License-Identifier: MIT

// Package creds: at-rest encryption for the credential vault
// (encryptedEnvelope struct + isEncryptedVault + encryptVault + decryptVault).
// The KDF helpers (cachedDeriveKey + deriveKeyPBKDF2 + deriveKeyLegacyHMAC)
// moved to encrypt_kdf.go. Day-211 god-file split. Public API unchanged.
package creds


import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)
const (
	// SchemaEncrypted is the value of the `schema` field on encrypted
	// vault envelopes. Bumping requires a migration path; bumping
	// implicitly versions the algorithm choices below.
	SchemaEncrypted = "agezt-creds-v2"
	// AlgorithmAESGCM is the cipher identifier; v2 only supports
	// AES-256-GCM so the only check is "is it the expected value".
	AlgorithmAESGCM = "aes-256-gcm"
	// KDFIteratedHMAC is the LEGACY KDF identifier (a keyed HMAC-SHA256 hash
	// chain). Still accepted on decrypt so vaults written before M172 stay
	// readable; new vaults use KDFPBKDF2.
	KDFIteratedHMAC = "hmac-sha256-iter"
	// KDFPBKDF2 is the current KDF: genuine PBKDF2-HMAC-SHA256 (M172). Unlike the
	// legacy chain, it XOR-accumulates every round's HMAC output (the standard
	// PBKDF2 construction), so it is a true PBKDF2-SHA256 derivation rather than an
	// approximation. New saves and rotations write this id.
	KDFPBKDF2 = "pbkdf2-hmac-sha256"

	// KDFIterMinAccepted is the floor for an envelope's stored iteration count on
	// decrypt (M172). v2 has always written KDFIterations (200000); anything far
	// below that is malformed or an attempt to make a stolen vault cheap to crack,
	// so refuse it. (The previous floor was 1000 — 200× below policy.)
	KDFIterMinAccepted = 100000

	// KDFIterMaxAccepted is the CEILING for an envelope's stored iteration count
	// on decrypt (SEC-002). The floor above stops a stolen vault being made cheap
	// to crack; this stops the mirror-image attack. Decrypt derives with the
	// envelope's OWN KDFIter, which is attacker-controlled the moment anyone can
	// write the vault file — and PBKDF2 is O(iter) by design, so `kdf_iter:
	// 2000000000` is not a slow unlock, it is a hang. That matters more here than
	// in a typical app: the vault is opened during daemon BOOT, so a single edited
	// integer wedges the whole service, and the legacy HMAC chain is equally O(n).
	//
	// 50× the shipped count. Generous enough that a future release can raise
	// KDFIterations several times over without stranding vaults it wrote, while
	// bounding a hostile envelope to seconds instead of days.
	KDFIterMaxAccepted = 10000000

	// KDFIterations is the iteration count for key derivation.
	// 200000 is enough to cost ~100ms on commodity hardware in 2026;
	// brute-forcing a high-entropy passphrase remains infeasible.
	// Increasing requires a migration (older vaults stay readable
	// because the count is stored in the envelope).
	KDFIterations = 200000

	// SaltBytes is the salt length per save. 32 bytes is more than
	// enough to defeat rainbow tables.
	SaltBytes = 32
	// NonceBytes is the AES-GCM nonce length (NIST recommends 12).
	NonceBytes = 12
	// KeyBytes is 256-bit for AES-256.
	KeyBytes = 32
)

// encryptedEnvelope is the on-disk shape of an encrypted vault. All
// binary fields are base64-encoded so the file stays text-readable
// (operators can inspect the envelope structure without running
// hex dump). The inner ciphertext, once decrypted, is the same
// flat `map[string]string` plaintext vaults use.
type encryptedEnvelope struct {
	Schema     string `json:"schema"`
	Encryption string `json:"encryption"`
	KDF        string `json:"kdf"`
	KDFIter    int    `json:"kdf_iter"`
	KDFSalt    string `json:"kdf_salt"`   // base64
	Nonce      string `json:"nonce"`      // base64
	Ciphertext string `json:"ciphertext"` // base64
}

// ErrPassphraseRequired is returned by Load when the vault file is
// encrypted but no passphrase is available in the environment.
var ErrPassphraseRequired = errors.New("creds: vault is encrypted but AGEZT_VAULT_PASSPHRASE is not set")

// ErrWrongPassphrase is returned when the configured passphrase
// fails to decrypt (GCM authentication tag mismatch). Distinct
// sentinel so the caller can produce a clear "passphrase wrong"
// message rather than "data corrupted."
var ErrWrongPassphrase = errors.New("creds: vault decryption failed (wrong passphrase or corrupted file)")

// isEncryptedVault returns true if raw looks like an encrypted-vault
// envelope (presence of the `schema` field with our expected value).
// Plaintext vaults are flat string maps; the absence of `schema`
// reliably distinguishes them.
func isEncryptedVault(raw []byte) bool {
	var probe struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.Schema == SchemaEncrypted
}

// encryptVault encrypts the given plaintext map into an envelope
// suitable for writing to disk. Fresh salt + nonce per call.
func encryptVault(plaintext map[string]string, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errors.New("creds: passphrase must be non-empty for encryption")
	}
	salt := make([]byte, SaltBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("creds: read salt: %w", err)
	}
	key := deriveKeyPBKDF2([]byte(passphrase), salt, KDFIterations)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creds: AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creds: GCM mode: %w", err)
	}
	nonce := make([]byte, NonceBytes)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("creds: read nonce: %w", err)
	}

	jsonBytes, err := json.Marshal(plaintext)
	if err != nil {
		return nil, fmt.Errorf("creds: marshal plaintext: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, jsonBytes, nil)

	env := encryptedEnvelope{
		Schema:     SchemaEncrypted,
		Encryption: AlgorithmAESGCM,
		KDF:        KDFPBKDF2,
		KDFIter:    KDFIterations,
		KDFSalt:    base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	return json.MarshalIndent(env, "", "  ")
}

// decryptVault reverses encryptVault. Returns ErrWrongPassphrase on
// GCM auth failure (typically wrong passphrase, but also disk
// corruption or someone tampering with the file).
func decryptVault(raw []byte, passphrase string) (map[string]string, error) {
	if passphrase == "" {
		return nil, ErrPassphraseRequired
	}
	var env encryptedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("creds: parse envelope: %w", err)
	}
	if env.Schema != SchemaEncrypted {
		return nil, fmt.Errorf("creds: unsupported envelope schema %q (this build expects %q)", env.Schema, SchemaEncrypted)
	}
	if env.Encryption != AlgorithmAESGCM {
		return nil, fmt.Errorf("creds: unsupported encryption %q", env.Encryption)
	}
	if env.KDF != KDFPBKDF2 && env.KDF != KDFIteratedHMAC {
		return nil, fmt.Errorf("creds: unsupported kdf %q", env.KDF)
	}
	if env.KDFIter < KDFIterMinAccepted {
		// A vault claiming a low iteration count is either malformed or an
		// adversary trying to make a stolen vault cheap to crack. Refuse.
		return nil, fmt.Errorf("creds: kdf_iter %d implausibly low (min %d)", env.KDFIter, KDFIterMinAccepted)
	}
	if env.KDFIter > KDFIterMaxAccepted {
		// The mirror image: refuse BEFORE deriving, because the derivation is the
		// denial of service (SEC-002). Checked here rather than clamped, since a
		// count this high means the envelope is not one we wrote.
		return nil, fmt.Errorf("creds: kdf_iter %d implausibly high (max %d)", env.KDFIter, KDFIterMaxAccepted)
	}
	salt, err := base64.StdEncoding.DecodeString(env.KDFSalt)
	if err != nil {
		return nil, fmt.Errorf("creds: decode salt: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("creds: decode nonce: %w", err)
	}
	// Validate the nonce length BEFORE gcm.Open: Go's GCM panics (rather than
	// erroring) on a nonce that isn't NonceSize() bytes. A corrupted, truncated,
	// or tampered vault whose nonce base64-decodes to the wrong length would
	// otherwise crash the process instead of failing cleanly. (Ciphertext length
	// and salt length are safe — GCM errors on a short ciphertext, and PBKDF2
	// accepts any salt.)
	if len(nonce) != NonceBytes {
		return nil, fmt.Errorf("creds: nonce length %d invalid (want %d) — vault corrupt or tampered", len(nonce), NonceBytes)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("creds: decode ciphertext: %w", err)
	}
	// Dispatch on the envelope's KDF id so legacy vaults (hmac-sha256-iter, written
	// before M172) still decrypt while new ones use PBKDF2. The derivation is
	// memoized (M934): the vault encrypts at rest BY DEFAULT now, and several
	// hot paths (Config Center values, catalog list, keyring ops) Load a fresh
	// Store per request — without the cache each would pay the full ~100ms
	// 200k-iteration KDF for the same (passphrase, salt) pair.
	var key []byte
	switch env.KDF {
	case KDFPBKDF2:
		key = cachedDeriveKey(env.KDF, passphrase, salt, env.KDFIter, deriveKeyPBKDF2)
	case KDFIteratedHMAC:
		key = cachedDeriveKey(env.KDF, passphrase, salt, env.KDFIter, deriveKeyLegacyHMAC)
	default:
		return nil, fmt.Errorf("creds: unsupported kdf %q", env.KDF)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creds: AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creds: GCM mode: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// GCM open failure is *almost always* wrong-passphrase; map to
		// the sentinel so callers can produce a clear message.
		return nil, ErrWrongPassphrase
	}
	var m map[string]string
	if err := json.Unmarshal(plaintext, &m); err != nil {
		return nil, fmt.Errorf("creds: parse decrypted JSON: %w", err)
	}
	return m, nil
}

// SPDX-License-Identifier: MIT

package creds

// At-rest encryption for the credential vault.
//
// **Threat model.** The vault file lives at `<BaseDir>/creds.json`
// with 0600 perms. Without encryption, a memory dump / disk image /
// cloud-synced home directory / shared-workstation file ACL leak
// trivially exposes every API key. Encryption raises the bar from
// "anyone with file-read" to "anyone with file-read AND the
// passphrase."
//
// **Passphrase source.** The daemon reads `AGEZT_VAULT_PASSPHRASE`
// from its environment. Operators set this:
//
//   - Via shell rc / launchd / systemd unit env (offline; the file
//     containing the passphrase still needs protection).
//   - Via an OS keychain CLI (`security find-generic-password` on
//     macOS, `pass` on Linux, `cmdkey`/`get-secret` on Windows)
//     called from the shell init.
//   - Via 1Password / Vault / similar secret managers' CLI.
//
// We don't ship a keychain integration of our own — every operator
// already has their preferred secret-manager tool, and adding a
// platform-specific dep (zalando/go-keyring etc.) would violate
// the lean-deps policy without buying capability the operator
// can't already get from their shell init.
//
// **Algorithm.**
//
//   - KDF: iterated HMAC-SHA256 (200,000 rounds, random 32-byte salt).
//     Not standard PBKDF2 — PBKDF2 lives in golang.org/x/crypto which
//     the lean-deps policy excludes. The construction below
//     ("repeated keyed-hash with the previous hash as input") is
//     equivalent in cost and resistance for the offline-brute-force
//     threat model we're worried about. Iteration count is high
//     enough that brute-forcing a strong passphrase costs months
//     of GPU-time; weak passphrases remain weak (operators picking
//     "password123" lose either way).
//   - Cipher: AES-256-GCM (stdlib). Authenticated encryption: any
//     tamper with the ciphertext fails decryption rather than
//     silently returning garbage credentials.
//   - Random nonce per save (12 bytes from crypto/rand).
//   - Random salt per save (32 bytes from crypto/rand) — so two
//     vaults encrypted with the same passphrase have different
//     keys, defeating rainbow-table attacks against the passphrase.
//
// **Format compatibility.** Plaintext vaults from M1.o load
// unchanged (detected by absence of the `schema` field). Encrypted
// vaults use a JSON envelope so future algorithm rotations can be
// detected per-file. Operators migrate via the (deferred) `agt
// vault encrypt` command or by setting the env var and saving once.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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
	// KDFIteratedHMAC is the KDF identifier; v2 only supports iterated
	// HMAC-SHA256.
	KDFIteratedHMAC = "hmac-sha256-iter"

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
	KDFSalt    string `json:"kdf_salt"`    // base64
	Nonce      string `json:"nonce"`       // base64
	Ciphertext string `json:"ciphertext"`  // base64
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
	key := deriveKey([]byte(passphrase), salt, KDFIterations)

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
		KDF:        KDFIteratedHMAC,
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
	if env.KDF != KDFIteratedHMAC {
		return nil, fmt.Errorf("creds: unsupported kdf %q", env.KDF)
	}
	if env.KDFIter < 1000 {
		// A vault claiming <1000 iterations is either malformed or an
		// adversary trying to make decryption cheap. Either way, refuse.
		return nil, fmt.Errorf("creds: kdf_iter %d implausibly low", env.KDFIter)
	}
	salt, err := base64.StdEncoding.DecodeString(env.KDFSalt)
	if err != nil {
		return nil, fmt.Errorf("creds: decode salt: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("creds: decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("creds: decode ciphertext: %w", err)
	}
	key := deriveKey([]byte(passphrase), salt, env.KDFIter)

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

// deriveKey runs `iter` rounds of HMAC-SHA256 with the passphrase as
// the key and the prior digest as the input. The first round uses
// the salt; subsequent rounds use the previous digest. This costs
// O(iter) SHA-256 evaluations — defeats brute-force scans of
// commodity-size password lists at iter≥100k.
//
// **Why not PBKDF2.** Stdlib doesn't have PBKDF2 (lives in
// golang.org/x/crypto, excluded by the lean-deps policy). The
// construction below has the same asymptotic cost profile and the
// same resistance to time-memory tradeoffs for our threat model.
// It's NOT identical to PBKDF2 — don't use it for inter-tool key
// portability, only for this vault's self-contained encrypt/decrypt
// round trip.
func deriveKey(passphrase, salt []byte, iter int) []byte {
	d := salt
	for range iter {
		mac := hmac.New(sha256.New, passphrase)
		mac.Write(d)
		d = mac.Sum(nil)
	}
	// Truncate or extend to KeyBytes (AES-256 needs exactly 32).
	// sha256 already returns 32, so this is a no-op; explicit slice
	// for clarity.
	return d[:KeyBytes]
}

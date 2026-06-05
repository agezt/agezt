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
// detected per-file. Operators encrypt a plaintext vault via `agt
// vault encrypt` (or by setting the env var and saving once), and
// upgrade an older encrypted vault to the current key-derivation
// policy in place via `agt vault migrate`.

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
	// before M172) still decrypt while new ones use PBKDF2.
	var key []byte
	switch env.KDF {
	case KDFPBKDF2:
		key = deriveKeyPBKDF2([]byte(passphrase), salt, env.KDFIter)
	case KDFIteratedHMAC:
		key = deriveKeyLegacyHMAC([]byte(passphrase), salt, env.KDFIter)
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

// deriveKeyPBKDF2 is PBKDF2-HMAC-SHA256 (RFC 8018), implemented with stdlib only
// (x/crypto is excluded by the lean-deps policy). The derived key length (32) ==
// the PRF output length (SHA-256, 32), so there is exactly one PBKDF2 block:
//
//	U_1 = HMAC(P, salt || INT32BE(1));  U_j = HMAC(P, U_{j-1});  DK = U_1 ⊕ … ⊕ U_iter
//
// The XOR accumulation over every round is what makes this a genuine PBKDF2
// derivation (M172) rather than the legacy hash chain, which fed only the final
// round's output forward. Verified against RFC published SHA-256 test vectors.
func deriveKeyPBKDF2(passphrase, salt []byte, iter int) []byte {
	mac := hmac.New(sha256.New, passphrase)
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1}) // INT32BE(1): the single block index
	u := mac.Sum(nil)
	dk := make([]byte, len(u))
	copy(dk, u)
	for i := 1; i < iter; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range dk {
			dk[j] ^= u[j]
		}
	}
	return dk[:KeyBytes]
}

// deriveKeyLegacyHMAC is the pre-M172 KDF: a keyed HMAC-SHA256 hash chain (the
// passphrase keys every round; the prior digest is the input; the salt seeds round
// one). Retained ONLY to decrypt vaults written before M172 — new vaults use
// deriveKeyPBKDF2. It costs O(iter) SHA-256 evaluations like PBKDF2 but, lacking
// XOR accumulation, the final key depends only on the last round's output.
func deriveKeyLegacyHMAC(passphrase, salt []byte, iter int) []byte {
	d := salt
	for range iter {
		mac := hmac.New(sha256.New, passphrase)
		mac.Write(d)
		d = mac.Sum(nil)
	}
	return d[:KeyBytes]
}

// SPDX-License-Identifier: MIT

// Package creds: KDF helpers (cachedDeriveKey + deriveKeyPBKDF2 +
// deriveKeyLegacyHMAC + kdfCache var). Extracted from encrypt.go during the
// Day-211 god-file split. Public API unchanged.
package creds


import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"sync"
)
// kdfCache memoizes derived keys by (kdf id, iterations, salt, passphrase
// digest) so repeated Loads of the SAME envelope (per-request Store instances
// on the Config Center / catalog / keyring paths) pay the 200k-iteration KDF
// once, not per request. Bounded in practice: one entry per distinct envelope
// save, and a save replaces the salt — the map stays tiny for a daemon's
// lifetime. The cache key uses a SHA-256 of the passphrase (never the
// passphrase itself) so the passphrase doesn't sit in a map key string.
var kdfCache sync.Map // string → []byte (the derived key; never mutated)

func cachedDeriveKey(kdf, passphrase string, salt []byte, iter int, derive func(pass, salt []byte, iter int) []byte) []byte {
	pd := sha256.Sum256([]byte(passphrase))
	ck := fmt.Sprintf("%s|%d|%x|%x", kdf, iter, salt, pd[:])
	if v, ok := kdfCache.Load(ck); ok {
		return v.([]byte)
	}
	key := derive([]byte(passphrase), salt, iter)
	kdfCache.Store(ck, key)
	return key
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

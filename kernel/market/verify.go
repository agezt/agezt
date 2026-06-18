// SPDX-License-Identifier: MIT

package market

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// VerifyPack checks a pack's optional Ed25519 signature (UPD-001 primitive).
// It returns (signed, error):
//   - signed=false, err=nil  → the pack is unsigned (allowed, but flagged).
//   - signed=true,  err=nil  → a signature was present and fully verified.
//   - err != nil             → a signature was present but INVALID (caller refuses).
//
// requirePubKeyHex, when non-empty, pins the expected signer: a pack signed by a
// different key is rejected even if its own signature is internally valid. This
// is how a Source binds the packs it serves to a known publisher key.
func VerifyPack(p Pack, requirePubKeyHex string) (bool, error) {
	if p.Signature == nil {
		if requirePubKeyHex != "" {
			return false, fmt.Errorf("pack %q is unsigned but a signer key is required", p.Name)
		}
		return false, nil
	}
	pub, err := hex.DecodeString(p.Signature.PubKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("pack %q: bad public key", p.Name)
	}
	if requirePubKeyHex != "" && !streq(p.Signature.PubKey, requirePubKeyHex) {
		return false, fmt.Errorf("pack %q signed by an unexpected key", p.Name)
	}
	sig, err := hex.DecodeString(p.Signature.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("pack %q: bad signature encoding", p.Name)
	}
	canon, err := p.CanonicalBytes()
	if err != nil {
		return false, err
	}
	// The embedded sha256 must match the canonical payload (binds sig to content).
	sum := sha256.Sum256(canon)
	if !streq(p.Signature.SHA256, hex.EncodeToString(sum[:])) {
		return false, fmt.Errorf("pack %q: signature sha256 does not match content", p.Name)
	}
	if !ed25519.Verify(pub, canon, sig) {
		return false, fmt.Errorf("pack %q: signature verification failed", p.Name)
	}
	return true, nil
}

// SignPack produces a Signature over the pack's canonical bytes with priv. Used
// by `agt market publish` (Phase 3) and tests; kept here beside VerifyPack.
func SignPack(p Pack, priv ed25519.PrivateKey, signedAtMS int64) (*Signature, error) {
	canon, err := p.CanonicalBytes()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(canon)
	sig := ed25519.Sign(priv, canon)
	return &Signature{
		SHA256:   hex.EncodeToString(sum[:]),
		Sig:      hex.EncodeToString(sig),
		PubKey:   hex.EncodeToString(priv.Public().(ed25519.PublicKey)),
		SignedAt: signedAtMS,
	}, nil
}

// streq is a tiny case-sensitive equality helper (hex is lowercased on encode).
func streq(a, b string) bool { return a == b }

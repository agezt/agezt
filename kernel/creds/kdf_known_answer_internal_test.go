// SPDX-License-Identifier: MIT

package creds

import (
	"bytes"
	stdpbkdf2 "crypto/pbkdf2"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// The vault KDFs are hand-rolled (the lean-deps policy predated crypto/pbkdf2
// landing in the stdlib at Go 1.24).
//
// deriveKeyPBKDF2 already has a hard-coded-vector test (TestDeriveKeyPBKDF2_
// KnownAnswers in pbkdf2_test.go); TestDeriveKeyPBKDF2_MatchesStdlib below
// *strengthens* it by cross-checking against the authoritative stdlib
// implementation (so the pinning can never itself be wrong or drift) and adds
// empty-passphrase / empty-salt / unicode cases the existing vectors omit.
//
// deriveKeyLegacyHMAC, by contrast, was NOT pinned: every test that exercised it
// round-trips with the SAME function on both sides, so a regression — e.g.
// removing `mac.Write(d)` from the chain — still round-trips and passes. Mutation
// testing (M494) confirmed that exact mutant survived. TestDeriveKeyLegacyHMAC_
// GoldenAnswer below closes that gap with golden digests from an independent
// reimplementation, which matters because the legacy KDF is frozen: any change to
// its output makes pre-M172 vaults undecryptable.

// deriveKeyPBKDF2 must match the standard-library PBKDF2-HMAC-SHA256 exactly.
// crypto/pbkdf2 (Go 1.24+) is the authoritative implementation and is stdlib, so
// this adds no module dependency. The comparison is live (no hard-coded digests),
// so it can never drift. Any mutation of the PBKDF2 internals diverges from stdlib
// and fails here.
func TestDeriveKeyPBKDF2_MatchesStdlib(t *testing.T) {
	cases := []struct {
		pass, salt string
		iter       int
	}{
		{"password", "salt", 1},
		{"password", "salt", 2},
		{"password", "salt", 4096},
		{"", "saltsaltsaltsalt", 1000},                               // empty passphrase
		{"correct horse battery staple", "", 1},                      // empty salt
		{"p@ss wörd🔑", "NaCl-ish-32-byte-salt-value-here!!", 100000}, // unicode + realistic policy iter
	}
	for _, c := range cases {
		got := deriveKeyPBKDF2([]byte(c.pass), []byte(c.salt), c.iter)
		want, err := stdpbkdf2.Key(sha256.New, c.pass, []byte(c.salt), c.iter, KeyBytes)
		if err != nil {
			t.Fatalf("stdlib pbkdf2 error for (%q,%q,%d): %v", c.pass, c.salt, c.iter, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("deriveKeyPBKDF2(%q,%q,%d) = %x,\n  want (stdlib) %x", c.pass, c.salt, c.iter, got, want)
		}
		if len(got) != KeyBytes {
			t.Errorf("deriveKeyPBKDF2 returned %d bytes, want %d", len(got), KeyBytes)
		}
	}
}

// deriveKeyLegacyHMAC is a frozen algorithm — it exists only to decrypt vaults
// written before M172, so its output must NEVER change or those vaults become
// unreadable. There is no stdlib equivalent (it is a non-standard keyed hash
// chain), so it is pinned against golden digests computed by an independent
// reimplementation of the documented construction
// (d := salt; repeat: d = HMAC-SHA256(passphrase, d); take first 32 bytes).
func TestDeriveKeyLegacyHMAC_GoldenAnswer(t *testing.T) {
	cases := []struct {
		pass, salt, wantHex string
		iter                int
	}{
		{"password", "saltsaltsaltsalt", "e322444aeb12e2b729f82c2d1a871e0002c0929066ab010d0eef47289653af4e", 1000},
		{"hunter2", "deadbeef", "f2f6ddb4a02acf0458f71e43b057251bbf58bef74ec7893a0fd5b4243cbc7621", 100000},
	}
	for _, c := range cases {
		got := hex.EncodeToString(deriveKeyLegacyHMAC([]byte(c.pass), []byte(c.salt), c.iter))
		if got != c.wantHex {
			t.Errorf("deriveKeyLegacyHMAC(%q,%q,%d) = %s,\n  want (frozen golden) %s — a legacy-vault decryption regression", c.pass, c.salt, c.iter, got, c.wantHex)
		}
	}
}

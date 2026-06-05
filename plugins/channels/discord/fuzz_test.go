// SPDX-License-Identifier: MIT

package discord

import (
	"crypto/ed25519"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

// FuzzVerify hardens Discord's inbound Ed25519 signature check (over `ts+body`,
// with a freshness window). A bypass is forged-interaction injection. Invariants:
// verify never panics on any (ts, sigHex, body), a genuine signature at a fresh
// timestamp is accepted, and no other signature is accepted.
func FuzzVerify(f *testing.F) {
	// Deterministic keypair from a fixed seed (reproducible across fuzz runs).
	priv := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	pub := priv.Public().(ed25519.PublicKey)
	pubHex := hex.EncodeToString(pub)

	f.Add("body", "deadbeef", "1700000000")
	f.Add("", "", "")
	f.Add("x", "zz", "not-a-number")

	f.Fuzz(func(t *testing.T, body, fuzzSigHex, ts string) {
		c := New(Config{PublicKey: pubHex})
		fixed := time.Unix(1_700_000_000, 0)
		c.now = func() time.Time { return fixed }
		b := []byte(body)

		_ = c.verify(ts, fuzzSigHex, b) // never panics

		freshTS := strconv.FormatInt(fixed.Unix(), 10)
		msg := append([]byte(freshTS), b...)
		correct := hex.EncodeToString(ed25519.Sign(priv, msg))

		if !c.verify(freshTS, correct, b) {
			t.Fatalf("correct signature rejected for body %q", body)
		}
		if fuzzSigHex != correct && c.verify(freshTS, fuzzSigHex, b) {
			t.Errorf("forged signature accepted: %q (body=%q)", fuzzSigHex, body)
		}
	})
}

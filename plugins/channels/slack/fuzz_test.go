// SPDX-License-Identifier: MIT

package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

// FuzzVerify hardens Slack's inbound signature check (HMAC-SHA256 over the
// `v0:ts:body` scheme, with a freshness window). A bypass is forged-command
// injection. Invariants: verify never panics on any (ts, sig, body), the correct
// signature at a fresh timestamp is accepted, and no other signature is accepted.
func FuzzVerify(f *testing.F) {
	const secret = "slack-signing-secret-value-12345"
	f.Add("body", "v0=deadbeef", "1700000000")
	f.Add("", "", "")
	f.Add("x", "v0=", "not-a-number")

	f.Fuzz(func(t *testing.T, body, fuzzSig, ts string) {
		c := New(Config{SigningSecret: secret})
		fixed := time.Unix(1_700_000_000, 0)
		c.now = func() time.Time { return fixed }
		b := []byte(body)

		_ = c.verify(ts, fuzzSig, b) // never panics

		freshTS := strconv.FormatInt(fixed.Unix(), 10)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte("v0:" + freshTS + ":"))
		mac.Write(b)
		correct := "v0=" + hex.EncodeToString(mac.Sum(nil))

		if !c.verify(freshTS, correct, b) {
			t.Fatalf("correct signature rejected for body %q", body)
		}
		if fuzzSig != correct && c.verify(freshTS, fuzzSig, b) {
			t.Errorf("forged signature accepted: %q (body=%q)", fuzzSig, body)
		}
	})
}

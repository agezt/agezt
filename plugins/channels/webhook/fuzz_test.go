// SPDX-License-Identifier: MIT

package webhook

import "testing"

// FuzzVerify hardens the inbound HMAC signature check — the gate that decides
// whether an untrusted-internet POST is an authentic command. A bypass here is
// forged-command injection. Invariants: verify never panics on any (sig, body),
// the correct signature is accepted, and NO signature other than the correct one
// is ever accepted (no forgery).
func FuzzVerify(f *testing.F) {
	const secret = "webhook-signing-secret-value-1234"
	f.Add("payload body", "sha256=deadbeef")
	f.Add("", "")
	f.Add("x", "sha256=")

	f.Fuzz(func(t *testing.T, body, fuzzSig string) {
		c := New(Config{Secret: secret})
		b := []byte(body)

		_ = c.verify(fuzzSig, b) // never panics

		correct := "sha256=" + sign(secret, b)
		if !c.verify(correct, b) {
			t.Fatalf("correct signature rejected for body %q", body)
		}
		if fuzzSig != correct && c.verify(fuzzSig, b) {
			t.Errorf("forged signature accepted: %q (body=%q)", fuzzSig, body)
		}
	})
}

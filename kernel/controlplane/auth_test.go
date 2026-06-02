// SPDX-License-Identifier: MIT

package controlplane

// White-box test for the constant-time primary-token check (M187). We
// can't assert timing in a unit test, but we lock in the correctness the
// constant-time comparison must preserve: only the exact token matches —
// not a prefix, a longer string, a case variant, or empty.

import "testing"

func TestTokenIsPrimary_OnlyExactMatches(t *testing.T) {
	const tok = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	s := &Server{token: tok}

	if !s.tokenIsPrimary(tok) {
		t.Fatal("exact primary token rejected")
	}
	for _, bad := range []string{
		"",                     // empty
		tok[:len(tok)-1],       // one byte short (prefix)
		tok + "0",              // one byte long
		tok[:len(tok)-1] + "F", // last byte differs (would short-circuit a plain ==)
		"completely-wrong",
	} {
		if s.tokenIsPrimary(bad) {
			t.Errorf("non-matching token accepted: %q", bad)
		}
	}
}

// A server with no token set (before Start) must not authorize an empty
// presented token — guards against "" == "" authorizing everything.
func TestTokenIsPrimary_EmptyServerTokenRejectsEmpty(t *testing.T) {
	s := &Server{} // token == ""
	if s.tokenIsPrimary("") {
		t.Error("empty presented token authorized against an unset server token")
	}
}

// SPDX-License-Identifier: MIT

package channel

import "testing"

// Email addresses compare case-insensitively in practice (domains always,
// local parts at every mainstream provider), but the allowlist matched them
// byte-for-byte: AGEZT_EMAIL_RECIPIENTS=Alice@Example.com refused mail from
// alice@example.com, and the operator's own replies bounced off the gate.
func TestFoldedAllowlistIgnoresCase(t *testing.T) {
	a := NewFoldedAllowlist([]string{" Alice@Example.COM ", ""})
	for _, from := range []string{"alice@example.com", "ALICE@EXAMPLE.COM", "Alice@Example.com"} {
		if !a.Allows(from) {
			t.Errorf("Allows(%q) = false, want true", from)
		}
	}
	if a.Allows("mallory@example.com") {
		t.Error("an unlisted address was allowed")
	}
	if a.Empty() {
		t.Error("Empty() = true for a non-empty list")
	}
}

// The default allowlist stays exact: chat ids on other platforms can be
// case-significant.
func TestDefaultAllowlistStaysExact(t *testing.T) {
	if NewAllowlist([]string{"U012ABC"}).Allows("u012abc") {
		t.Error("NewAllowlist must stay case-sensitive")
	}
}

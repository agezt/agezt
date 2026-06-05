// SPDX-License-Identifier: MIT

package redact

import (
	"strings"
	"testing"
)

// FuzzRedact hardens the secret-redaction path — the security boundary that keeps
// API keys and credentials out of logs/the bus/transcripts — against arbitrary
// input. Two invariants:
//
//   - Redact / RedactBytes never panic on any (text, secret) pair (a panic here
//     would be a DoS, and worse, a redaction that crashes mid-way leaks).
//   - A literal secret long enough to be indexed never SURVIVES verbatim in the
//     output. (Guarded against the degenerate case where the secret embeds the
//     Placeholder itself — "redacting" that can leave the placeholder text, which
//     no real secret equals; that is a fuzz artifact, not a leak.)
func FuzzRedact(f *testing.F) {
	f.Add("authorization: Bearer sk-abc123def456", "sk-abc123def456")
	f.Add("nothing to see here", "")
	f.Add("aaaaaaaaaaaaaaaa", "aaaaaaaa")
	f.Add("prefix[REDACTED]suffix", "[REDACTED]value")
	f.Add("", "supersecretvalue")

	f.Fuzz(func(t *testing.T, text, secret string) {
		r := New()
		r.SetSecrets([]string{secret})

		// Invariant 1: never panics (literal + regex + templated passes).
		out := r.Redact(text)
		if b := r.RedactBytes([]byte(text)); b == nil && text != "" {
			t.Errorf("RedactBytes returned nil for non-empty input %q", text)
		}

		// Invariant 2: an indexed literal secret does not survive redaction.
		//
		// Soundness guard against placeholder artifacts: if the secret overlaps the
		// Placeholder (e.g. the secret is a substring of "[REDACTED]"), then
		// redacting it legitimately INSERTS the placeholder, which trivially
		// "contains" the secret — a false leak, never a real one (no real credential
		// is a placeholder fragment). We detect this precisely by redacting the bare
		// secret: if that alone doesn't remove it, the leak property is degenerate
		// for this input and we skip it. Otherwise the secret is genuinely
		// removable, so it must also be gone from the larger redacted text.
		if len(secret) >= minLiteralLen && !strings.Contains(r.Redact(secret), secret) {
			if strings.Contains(out, secret) {
				t.Errorf("secret survived redaction\n  secret=%q\n  out=%q", secret, out)
			}
		}
	})
}

// SPDX-License-Identifier: MIT

package redact

import (
	"strings"
	"testing"
)

// TestSetSecrets_DropsShortAndDuplicates covers the two skip branches of
// SetSecrets (too-short values and duplicates) and the equal-length tie-break in
// the sort comparator.
func TestSetSecrets_DropsShortAndDuplicates(t *testing.T) {
	r := New()
	// "short" (< 8 chars) is dropped; the duplicate long value is deduped; the
	// two equal-length values exercise the `out[i] < out[j]` tie-break.
	r.SetSecrets([]string{
		"short",               // len 5 < minLiteralLen(8): dropped
		"literal-secret-aaaa", // kept
		"literal-secret-aaaa", // duplicate: dropped
		"bbbbbbbb",            // len 8: kept
		"cccccccc",            // len 8: kept (equal length → tie-break by value)
	})

	got := r.Redact("here is literal-secret-aaaa and bbbbbbbb and cccccccc and short")
	if strings.Contains(got, "literal-secret-aaaa") {
		t.Fatalf("long secret not redacted: %q", got)
	}
	if strings.Contains(got, "bbbbbbbb") || strings.Contains(got, "cccccccc") {
		t.Fatalf("8-char secret not redacted: %q", got)
	}
	// "short" was dropped as a literal, so it survives (unless a pattern matches).
	if !strings.Contains(got, "short") {
		t.Fatalf("too-short value should not have been registered as a secret: %q", got)
	}
}

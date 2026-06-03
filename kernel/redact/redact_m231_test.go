// SPDX-License-Identifier: MIT

package redact_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/redact"
)

// TestPatterns_M231 covers the two OpenAI-compatible vendors M230 made
// first-class (work with just an API key) whose key formats the redactor still
// missed: Perplexity (pplx-…) and Fireworks (fw_…). Configuring one of these
// and having its key land in a log/journal/plugin-stderr would otherwise leak.
func TestPatterns_M231(t *testing.T) {
	r := redact.New()
	cases := []struct{ name, in, secret string }{
		{"perplexity", "PERPLEXITY_API_KEY=pplx-0123456789abcdefghijABCDEFGHIJ0123 ok", "pplx-0123456789abcdefghijABCDEFGHIJ0123"},
		{"fireworks", "fw key fw_0123456789abcdefghijABCDEFGHIJ done", "fw_0123456789abcdefghijABCDEFGHIJ"},
	}
	for _, c := range cases {
		out := r.Redact(c.in)
		if strings.Contains(out, c.secret) {
			t.Errorf("%s: secret survived redaction: %q -> %q", c.name, c.in, out)
		}
		if !strings.Contains(out, redact.Placeholder) {
			t.Errorf("%s: expected placeholder in %q", c.name, out)
		}
	}
}

func TestPatterns_M231_Categories(t *testing.T) {
	cases := map[string]string{
		"pplx-0123456789abcdefghijABCDEFGHIJ0123": "perplexity-key",
		"fw_0123456789abcdefghijABCDEFGHIJ":       "fireworks-key",
	}
	for in, want := range cases {
		found := false
		for _, g := range redact.MatchedCategories(in) {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("MatchedCategories(%q) missing %q (got %v)", in, want, redact.MatchedCategories(in))
		}
	}
}

func TestPatterns_M231_NoFalsePositives(t *testing.T) {
	r := redact.New()
	safe := []string{
		"the fw_handler ran",         // fw_ but a short, non-token word
		"see pplx- docs for setup",   // pplx- with no token body
		"fw_ok and pplx-ok are fine", // both prefixes, sub-floor bodies
	}
	for _, in := range safe {
		if out := r.Redact(in); out != in {
			t.Errorf("ordinary text was redacted: %q -> %q", in, out)
		}
	}
}

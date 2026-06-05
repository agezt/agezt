// SPDX-License-Identifier: MIT

package redact_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/redact"
)

func TestPatterns(t *testing.T) {
	r := redact.New()
	cases := []struct {
		name, in string
		// redacted reports whether the secret token should be gone from output.
		secret string
	}{
		{"openai", "key is sk-abcdefghijklmnopqrstuvwx done", "sk-abcdefghijklmnopqrstuvwx"},
		{"anthropic", "ANTHROPIC=sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAA end", "sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAA"},
		{"aws", "akid AKIAIOSFODNN7EXAMPLE here", "AKIAIOSFODNN7EXAMPLE"},
		{"github", "tok ghp_0123456789012345678901234567890123456 x", "ghp_0123456789012345678901234567890123456"},
		{"slack", "xoxb-1234567890-abcdefghijkl note", "xoxb-1234567890-abcdefghijkl"},
		{"google", "AIzaabcdefghijabcdefghijabcdefghijabcde g", "AIzaabcdefghijabcdefghijabcdefghijabcde"},
		{"bearer", "Authorization: Bearer abcdefghijklmnopqrstuvwxyz12 ok", "abcdefghijklmnopqrstuvwxyz12"},
		// M418: the AWS secret access key, keyed to its assignment label.
		{"aws-secret", "aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY here", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
		{"aws-secret-quoted", `AWS_SECRET_ACCESS_KEY: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"`, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
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

// TestPatterns_M170 covers the patterns hardened/added in M170: base64-bearing
// sk-/bearer tokens (the char class now includes + / =), JWTs, and GitHub
// fine-grained PATs. Before M170 the base64 tokens leaked ENTIRELY (a + or / cut
// the match below the {20,} length floor, so nothing matched).
func TestPatterns_M170(t *testing.T) {
	r := redact.New()
	cases := []struct{ name, in, secret string }{
		{"sk-with-base64", "key sk-AbCd1234/EFgh5678+IJkl90==mnop done", "sk-AbCd1234/EFgh5678+IJkl90==mnop"},
		{"bearer-base64-ya29", "Authorization: Bearer ya29.A0ARrda+B/Cdef==ghijklmnopqrst ok", "ya29.A0ARrda+B/Cdef==ghijklmnopqrst"},
		{"jwt", "tok eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36 end", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36"},
		{"github-fine-grained-pat", "tok github_pat_11ABC23456789012345678_abcdefghijklmnopqrstuvwxyz end", "github_pat_11ABC23456789012345678_abcdefghijklmnopqrstuvwxyz"},
	}
	for _, c := range cases {
		out := r.Redact(c.in)
		if strings.Contains(out, c.secret) {
			t.Errorf("%s: secret survived: %q -> %q", c.name, c.in, out)
		}
		if !strings.Contains(out, redact.Placeholder) {
			t.Errorf("%s: expected placeholder in %q", c.name, out)
		}
	}
}

func TestPEMPrivateKey(t *testing.T) {
	r := redact.New()
	in := "before\n-----BEGIN RSA PRIVATE KEY-----\nMIIEdeadbeef\nlines\n-----END RSA PRIVATE KEY-----\nafter"
	out := r.Redact(in)
	if strings.Contains(out, "MIIEdeadbeef") {
		t.Errorf("private key body survived: %q", out)
	}
	if !strings.HasPrefix(out, "before\n") || !strings.HasSuffix(out, "\nafter") {
		t.Errorf("surrounding text should be preserved: %q", out)
	}
}

func TestLiterals(t *testing.T) {
	r := redact.New()
	r.SetSecrets([]string{"super-secret-value-123", "tok_LONGENOUGH_abcdef"})
	in := "x super-secret-value-123 y tok_LONGENOUGH_abcdef z"
	out := r.Redact(in)
	if strings.Contains(out, "super-secret-value-123") || strings.Contains(out, "tok_LONGENOUGH_abcdef") {
		t.Errorf("literal secret survived: %q", out)
	}
	if out != "x [REDACTED] y [REDACTED] z" {
		t.Errorf("unexpected: %q", out)
	}
}

func TestLiterals_ShortAndEmptyIgnored(t *testing.T) {
	r := redact.New()
	r.SetSecrets([]string{"", "short", "ab"}) // all below minLiteralLen
	in := "the word short appears here"
	if out := r.Redact(in); out != in {
		t.Errorf("short/empty literals must not redact: %q -> %q", in, out)
	}
}

// TestAWSSecret_NotOverRedacted: a bare 40-char base64 string with no
// aws_secret_access_key label (e.g. a hash/id) must be left intact — the AWS
// secret-key rule is deliberately keyed to its assignment label (M418).
func TestAWSSecret_NotOverRedacted(t *testing.T) {
	r := redact.New()
	in := "checksum wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY ok" // 40 base64 chars, no label
	if out := r.Redact(in); out != in {
		t.Errorf("a bare base64 string without the AWS label must not be redacted: %q -> %q", in, out)
	}
}

func TestNoFalsePositiveOnOrdinaryText(t *testing.T) {
	r := redact.New()
	in := "The quick brown fox sk-too-short jumps; bearer of bad news."
	// "sk-too-short" is below the 20-char tail; "bearer of" lacks a token.
	if out := r.Redact(in); out != in {
		t.Errorf("ordinary text should be untouched: %q -> %q", in, out)
	}
}

func TestRedactBytes_UnchangedReturnsSameSlice(t *testing.T) {
	r := redact.New()
	b := []byte("nothing secret here")
	out := r.RedactBytes(b)
	if string(out) != string(b) {
		t.Errorf("content changed unexpectedly: %q", out)
	}
}

func TestRedactBytes_KeepsJSONValid(t *testing.T) {
	r := redact.New()
	payload := map[string]any{"cmd": "export KEY=sk-abcdefghijklmnopqrstuvwx", "ok": true}
	raw, _ := json.Marshal(payload)
	red := r.RedactBytes(raw)
	if strings.Contains(string(red), "sk-abcdefghijklmnopqrstuvwx") {
		t.Fatalf("secret survived in JSON: %s", red)
	}
	var back map[string]any
	if err := json.Unmarshal(red, &back); err != nil {
		t.Fatalf("redacted JSON is invalid: %v (%s)", err, red)
	}
	if back["ok"] != true {
		t.Errorf("non-secret field corrupted: %v", back)
	}
}

func TestMatchedCategories(t *testing.T) {
	cases := []struct {
		in   string
		want string // expected category, "" = none
	}{
		{"sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123", "openai/anthropic-key"},
		{"AKIA0123456789ABCDEF", "aws-access-key-id"},
		{"ghp_0123456789012345678901234567890123456789", "github-token"},
		{"xoxb-0123456789-abcdef", "slack-token"},
		{"AIza" + strings.Repeat("a", 35), "google-api-key"},
		{"Authorization: Bearer abcdefghijklmnopqrstuvwxyz", "bearer-token"},
		{"just some ordinary prose without secrets", ""},
	}
	for _, c := range cases {
		got := redact.MatchedCategories(c.in)
		if c.want == "" {
			if len(got) != 0 {
				t.Errorf("MatchedCategories(%q) = %v, want none", c.in, got)
			}
			continue
		}
		found := false
		for _, g := range got {
			if g == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("MatchedCategories(%q) = %v, want to include %q", c.in, got, c.want)
		}
	}
	// Empty input → nil.
	if got := redact.MatchedCategories(""); got != nil {
		t.Errorf("MatchedCategories(\"\") = %v, want nil", got)
	}
}

// TestPatternsDerivedFromNamed pins that the redaction list and the labelled
// list stay in lockstep — a Redact match implies a MatchedCategories label.
func TestPatternsDerivedFromNamed(t *testing.T) {
	r := redact.New()
	for _, s := range []string{
		"sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123",
		"AKIA0123456789ABCDEF",
	} {
		if r.Redact(s) == s {
			t.Errorf("Redact did not scrub %q", s)
		}
		if len(redact.MatchedCategories(s)) == 0 {
			t.Errorf("MatchedCategories empty for a string Redact scrubs: %q", s)
		}
	}
}

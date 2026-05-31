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

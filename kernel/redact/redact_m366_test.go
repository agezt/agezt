// SPDX-License-Identifier: MIT

package redact

import (
	"strings"
	"testing"
)

// TestRedact_ConnectionStringPassword covers the SPEC-06 §4 "connection-string
// passwords" target: the credential in a scheme://user:password@host URI
// (Postgres/MySQL/MongoDB/Redis/AMQP/…) must be masked before it reaches the
// journal, logs, or a provider — while the scheme/user/host survive so an
// operator can still tell which database leaked the secret (§4: "keep a
// recognizable prefix/suffix").
func TestRedact_ConnectionStringPassword(t *testing.T) {
	r := New()
	cases := []struct {
		in        string
		secret    string // must NOT appear in the output
		keepParts []string
	}{
		{"postgres://app:s3cr3t@db.internal:5432/prod", "s3cr3t", []string{"postgres://app:", "@db.internal:5432/prod"}},
		{"mysql://root:p4ssw0rd@127.0.0.1/app", "p4ssw0rd", []string{"mysql://root:", "@127.0.0.1/app"}},
		{"mongodb://svc:Tok3n@cluster0.mongodb.net/db", "Tok3n", []string{"mongodb://svc:", "@cluster0.mongodb.net/db"}},
		{"amqp://guest:guestpw@rabbit:5672/", "guestpw", []string{"amqp://guest:", "@rabbit:5672/"}},
		// Empty user (Redis-style).
		{"redis://:hunter2@cache:6379/0", "hunter2", []string{"redis://:", "@cache:6379/0"}},
		// Raw '@' inside the password must be fully masked (greedy to last '@').
		{"postgres://app:p@ss@db:5432/x", "p@ss", []string{"postgres://app:", "@db:5432/x"}},
	}
	for _, c := range cases {
		got := r.Redact(c.in)
		if strings.Contains(got, c.secret) {
			t.Errorf("Redact(%q) = %q — still contains secret %q", c.in, got, c.secret)
		}
		if !strings.Contains(got, Placeholder) {
			t.Errorf("Redact(%q) = %q — expected %s", c.in, got, Placeholder)
		}
		for _, keep := range c.keepParts {
			if !strings.Contains(got, keep) {
				t.Errorf("Redact(%q) = %q — expected to preserve %q", c.in, got, keep)
			}
		}
	}
}

// TestRedact_ConnectionStringNoFalsePositives: strings that look URL-ish but
// carry no userinfo password must be left untouched — a host:port, a
// userinfo-without-password, and an '@' in a path/query.
func TestRedact_ConnectionStringNoFalsePositives(t *testing.T) {
	r := New()
	clean := []string{
		"postgres://db.host:5432/app",                 // host:port, no userinfo
		"https://example.com:8080/path?x=1",           // port, no userinfo
		"https://user@host/path",                      // userinfo, no password colon
		"see https://api.example.com/cb?redirect=a@b", // '@' only in the query
		"connect to db.host:5432 as app",              // not a URI at all
	}
	for _, s := range clean {
		if got := r.Redact(s); got != s {
			t.Errorf("Redact(%q) = %q — should be unchanged (no connection-string password)", s, got)
		}
	}
}

// TestRedact_ConnectionStringTwoURIsOnOneLine: two space-separated URIs must be
// masked independently — the greedy password match must not bleed across the
// whitespace into one giant redaction.
func TestRedact_ConnectionStringTwoURIsOnOneLine(t *testing.T) {
	r := New()
	got := r.Redact("a postgres://u1:pw1@h1/db and mysql://u2:pw2@h2/db")
	for _, secret := range []string{"pw1", "pw2"} {
		if strings.Contains(got, secret) {
			t.Errorf("output still contains %q: %s", secret, got)
		}
	}
	for _, keep := range []string{"@h1/db", "@h2/db", "postgres://u1:", "mysql://u2:"} {
		if !strings.Contains(got, keep) {
			t.Errorf("expected to preserve %q in: %s", keep, got)
		}
	}
}

// TestMatchedCategories_ReportsConnectionString: `agt redact test` diagnostics
// must name the connection-string detector when one matches.
func TestMatchedCategories_ReportsConnectionString(t *testing.T) {
	cats := MatchedCategories("postgres://app:s3cr3t@db:5432/x")
	found := false
	for _, c := range cats {
		if c == "connection-string-password" {
			found = true
		}
	}
	if !found {
		t.Errorf("MatchedCategories did not report connection-string-password: %v", cats)
	}
}

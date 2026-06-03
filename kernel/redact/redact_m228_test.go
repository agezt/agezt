// SPDX-License-Identifier: MIT

package redact_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/redact"
)

// TestPatterns_M228 covers secret formats agezt's own integrations handle but
// that the earlier rule set missed: Telegram bot tokens (Telegram channel),
// Slack app-level tokens (Slack channel), and Groq / xAI provider keys (compat
// providers). Without these they would reach a log or the journal in the clear.
func TestPatterns_M228(t *testing.T) {
	r := redact.New()
	// A 35-char Telegram secret suffix (26 letters + 9 digits).
	const tgSuffix = "abcdefghijklmnopqrstuvwxyz012345678"
	cases := []struct{ name, in, secret string }{
		{"telegram", "AGEZT_TELEGRAM_TOKEN=123456789:" + tgSuffix + " set", "123456789:" + tgSuffix},
		{"slack-app", "tok xapp-1-A1B2C3D4E5-9876543210-abcdefghij end", "xapp-1-A1B2C3D4E5-9876543210-abcdefghij"},
		{"groq", "GROQ key gsk_0123456789abcdefghijABCDEFGHIJ ok", "gsk_0123456789abcdefghijABCDEFGHIJ"},
		{"xai", "XAI_API_KEY=xai-0123456789abcdefghijABCDEFGHIJ done", "xai-0123456789abcdefghijABCDEFGHIJ"},
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

func TestPatterns_M228_Categories(t *testing.T) {
	const tgSuffix = "abcdefghijklmnopqrstuvwxyz012345678"
	cases := map[string]string{
		"123456789:" + tgSuffix:               "telegram-bot-token",
		"xapp-1-A1B2C3D4E5-9876543210-abcdef": "slack-app-token",
		"gsk_0123456789abcdefghijABCDEFGHIJ":  "groq-key",
		"xai-0123456789abcdefghijABCDEFGHIJ":  "xai-key",
	}
	for in, want := range cases {
		got := redact.MatchedCategories(in)
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("MatchedCategories(%q) = %v, want to include %q", in, got, want)
		}
	}
}

// TestPatterns_M228_NoFalsePositives guards the new rules against over-redacting
// ordinary text — they must fire only on the real token shapes.
func TestPatterns_M228_NoFalsePositives(t *testing.T) {
	r := redact.New()
	safe := []string{
		"build 123456789:ok",                  // colon but a short, non-token suffix
		"see commit gsk in the notes",         // 'gsk' without the _<token>
		"the xai team shipped grok",           // 'xai' as a bare word, no xai-<token>
		"timeout 12345678:30 retries",         // digits:digits, not a 35-char secret
		"xapp-thing without the version dash", // 'xapp-' but not the xapp-<n>-… shape
	}
	for _, in := range safe {
		if out := r.Redact(in); out != in {
			t.Errorf("ordinary text was redacted: %q -> %q", in, out)
		}
	}
}

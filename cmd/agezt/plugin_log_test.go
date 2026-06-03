// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/redact"
)

func TestPluginLogLine_RedactsSecrets(t *testing.T) {
	r := redact.New()
	cases := []struct{ name, line, secret string }{
		{"openai key", "auth failed with sk-abcdefghijklmnopqrstuvwx", "sk-abcdefghijklmnopqrstuvwx"},
		{"groq key", "GROQ_API_KEY=gsk_0123456789abcdefghijABCDEFGHIJ", "gsk_0123456789abcdefghijABCDEFGHIJ"},
		{"telegram", "bot 123456789:abcdefghijklmnopqrstuvwxyz012345678 ready", "123456789:abcdefghijklmnopqrstuvwxyz012345678"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := pluginLogLine(r, "search", c.line)
			if strings.Contains(out, c.secret) {
				t.Errorf("secret leaked into plugin log line: %q", out)
			}
			if !strings.Contains(out, redact.Placeholder) {
				t.Errorf("expected redaction placeholder in %q", out)
			}
			// The prefix is preserved so the operator still sees which plugin spoke.
			if !strings.HasPrefix(out, "[plugin:search] ") {
				t.Errorf("prefix lost: %q", out)
			}
		})
	}
}

func TestPluginLogLine_OrdinaryLineUnchanged(t *testing.T) {
	r := redact.New()
	out := pluginLogLine(r, "scrape", "fetched 12 pages in 1.3s")
	if out != "[plugin:scrape] fetched 12 pages in 1.3s" {
		t.Errorf("ordinary line altered: %q", out)
	}
}

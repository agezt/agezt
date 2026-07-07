// SPDX-License-Identifier: MIT

package browser

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestBrowserReadDefinitionAndHelpers(t *testing.T) {
	tl := New()
	def := tl.Definition()
	if def.Name != "browser.read" {
		t.Fatalf("Name = %q", def.Name)
	}
	if !strings.Contains(def.Description, "visible text") {
		t.Fatalf("description should mention visible text, got %q", def.Description)
	}
	if def.Effect.Class != agent.EffectReversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectReversible)
	}
	if len(def.InputSchema) == 0 {
		t.Fatal("InputSchema should not be empty")
	}
	if !strings.Contains(string(def.InputSchema), `"url"`) {
		t.Fatalf("schema should require url, got %s", def.InputSchema)
	}

	// hostAllowed paths: empty hostlist, bare host, subdomain wildcard.
	cases := map[string]bool{
		"example.com":  true,
		"sub.x.com":    true,
		"deep.x.com":   true,
		"y.com":        false,
		"evil.com":     false,
		"sub.evil.com": false,
	}
	for host, allowed := range cases {
		if got := hostAllowed(host, []string{"example.com", "*.x.com"}); got != allowed {
			t.Fatalf("hostAllowed(%q) = %v, want %v", host, got, allowed)
		}
	}
}

func TestBrowserActionDefinitionAndNewAction(t *testing.T) {
	// Empty driver path → tool disabled.
	if got := NewAction("node", ""); got != nil {
		t.Fatalf("NewAction with empty driver should return nil, got %+v", got)
	}
	// Real driver path → tool with default node + default MaxTextChars.
	tl := NewAction("", "/usr/local/bin/agezt-action")
	def := tl.Definition()
	if def.Name != "browser.action" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectIrreversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectIrreversible)
	}
	if !strings.Contains(def.Description, "browser") {
		t.Fatalf("description should mention browser, got %q", def.Description)
	}
	if !strings.Contains(string(def.InputSchema), "actions") {
		t.Fatalf("schema should include actions array, got %s", def.InputSchema)
	}
	if !strings.Contains(string(def.InputSchema), "tab_id") {
		t.Fatalf("schema should include tab_id, got %s", def.InputSchema)
	}

	if got := tl.MaxTextChars; got == 0 {
		t.Fatalf("default MaxTextChars should be non-zero, got %d", got)
	}
}

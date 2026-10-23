// SPDX-License-Identifier: MIT

package pluginapi_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pluginapi"
)

// TestParseToolAllowlistSpec_Basic exercises the env-var parser.
func TestParseToolAllowlistSpec_Basic(t *testing.T) {
	spec := "search=q+lookup,scrape=read+headers+post"
	parsed, err := pluginapi.ParseToolAllowlistSpec(spec)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := parsed["search"]; len(got) != 2 || got[0] != "q" || got[1] != "lookup" {
		t.Errorf("search = %v, want [q lookup]", got)
	}
	if got := parsed["scrape"]; len(got) != 3 {
		t.Errorf("scrape = %v, want 3 entries", got)
	}
}

// TestParseToolAllowlistSpec_RejectsBadFormat — bad entries error
// at parse time so operators get fast startup feedback.
func TestParseToolAllowlistSpec_RejectsBadFormat(t *testing.T) {
	for _, c := range []string{
		"search",       // no '='
		"=foo+bar",     // empty prefix
		"search=",      // empty tool list (use "unset" instead)
		"search=  +  ", // whitespace-only tools
	} {
		_, err := pluginapi.ParseToolAllowlistSpec(c)
		if err == nil {
			t.Errorf("ParseToolAllowlistSpec(%q): expected error", c)
		}
	}
}

// TestParseToolAllowlistSpec_RejectsDuplicatePrefix ensures a duplicate prefix is a
// hard error (it would otherwise silently keep the last list, unexpectedly changing
// what the plugin may advertise) — M216.
func TestParseToolAllowlistSpec_RejectsDuplicatePrefix(t *testing.T) {
	_, err := pluginapi.ParseToolAllowlistSpec("search=a+b,scrape=c,search=d")
	if err == nil {
		t.Fatal("a duplicate allowlist prefix should be rejected")
	}
	if !strings.Contains(err.Error(), "search") || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("error should name the duplicate prefix: %v", err)
	}
}

// TestParseToolAllowlistSpec_Empty: whitespace-only → empty map,
// no error (matches PinSpec behaviour).
func TestParseToolAllowlistSpec_Empty(t *testing.T) {
	for _, c := range []string{"", "  ", ","} {
		got, err := pluginapi.ParseToolAllowlistSpec(c)
		if err != nil {
			t.Errorf("ParseToolAllowlistSpec(%q): %v", c, err)
		}
		if len(got) != 0 {
			t.Errorf("ParseToolAllowlistSpec(%q) = %v, want empty", c, got)
		}
	}
}

// TestToolAllowlistSpec_Unused — diff helper reports stale entries.
func TestToolAllowlistSpec_Unused(t *testing.T) {
	spec := pluginapi.ToolAllowlistSpec{
		"search": []string{"q"},
		"ghost":  []string{"phantom"},
	}
	stale := spec.Unused([]string{"search"})
	if len(stale) != 1 || stale[0] != "ghost" {
		t.Errorf("Unused = %v, want [ghost]", stale)
	}
}

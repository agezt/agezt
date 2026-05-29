// SPDX-License-Identifier: MIT

package plugin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/plugin"
)

// TestSpawn_AllowlistAcceptsListedTools: when the allowlist
// covers every advertised tool, Spawn succeeds normally.
func TestSpawn_AllowlistAcceptsListedTools(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:         bin,
		AllowedTools: []string{"echo", "fail", "slowwork", "callhost"}, // all four advertised tools
	})
	if err != nil {
		t.Fatalf("Spawn with covering allowlist: %v", err)
	}
	defer p.Close()
	tools := p.Tools("")
	if len(tools) != 4 {
		t.Errorf("expected 4 tools registered, got %d", len(tools))
	}
}

// TestSpawn_AllowlistRejectsExtraTool: when the allowlist omits
// one of the advertised tools, Spawn fails BEFORE registering
// any tools — operators want a hard fail on capability drift,
// not a partial allow that silently registers the listed tools.
func TestSpawn_AllowlistRejectsExtraTool(t *testing.T) {
	bin := buildEchoPlugin(t)
	_, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:         bin,
		AllowedTools: []string{"echo"}, // omits "fail"
	})
	if err == nil {
		t.Fatal("Spawn with under-covering allowlist: expected error")
	}
	if !errors.Is(err, plugin.ErrToolAllowlistMismatch) {
		t.Errorf("err is not ErrToolAllowlistMismatch: %v", err)
	}
	if !strings.Contains(err.Error(), "fail") {
		t.Errorf("error doesn't mention the offending tool name: %v", err)
	}
}

// TestSpawn_EmptyAllowlistMeansNoCheck: an empty AllowedTools
// disables verification, matching the opt-in security pattern
// PinnedHash uses.
func TestSpawn_EmptyAllowlistMeansNoCheck(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:         bin,
		AllowedTools: nil, // explicitly empty
	})
	if err != nil {
		t.Fatalf("Spawn with empty allowlist: %v", err)
	}
	defer p.Close()
	if len(p.Tools("")) != 4 {
		t.Error("expected 4 tools without an allowlist")
	}
}

// TestParseToolAllowlistSpec_Basic exercises the env-var parser.
func TestParseToolAllowlistSpec_Basic(t *testing.T) {
	spec := "search=q+lookup,scrape=read+headers+post"
	parsed, err := plugin.ParseToolAllowlistSpec(spec)
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
		"search",         // no '='
		"=foo+bar",       // empty prefix
		"search=",        // empty tool list (use "unset" instead)
		"search=  +  ",   // whitespace-only tools
	} {
		_, err := plugin.ParseToolAllowlistSpec(c)
		if err == nil {
			t.Errorf("ParseToolAllowlistSpec(%q): expected error", c)
		}
	}
}

// TestParseToolAllowlistSpec_Empty: whitespace-only → empty map,
// no error (matches PinSpec behaviour).
func TestParseToolAllowlistSpec_Empty(t *testing.T) {
	for _, c := range []string{"", "  ", ","} {
		got, err := plugin.ParseToolAllowlistSpec(c)
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
	spec := plugin.ToolAllowlistSpec{
		"search": []string{"q"},
		"ghost":  []string{"phantom"},
	}
	stale := spec.Unused([]string{"search"})
	if len(stale) != 1 || stale[0] != "ghost" {
		t.Errorf("Unused = %v, want [ghost]", stale)
	}
}

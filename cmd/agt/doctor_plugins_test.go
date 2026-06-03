// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// validPin is a syntactically valid BLAKE3-256 pin (64 lowercase hex).
const validPin = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// setPluginEnv sets all three plugin env-specs explicitly so a test is
// never influenced by the ambient environment.
func setPluginEnv(t *testing.T, plugins, pins, tools string) {
	t.Helper()
	t.Setenv("AGEZT_PLUGINS", plugins)
	t.Setenv("AGEZT_PLUGIN_PINS", pins)
	t.Setenv("AGEZT_PLUGIN_TOOLS", tools)
}

func TestCheckPlugins_NotConfigured(t *testing.T) {
	setPluginEnv(t, "", "", "")
	c := checkPlugins()
	if c.Status != statusOK {
		t.Fatalf("no plugins should be OK, got %s: %s", c.Status.label(), c.Detail)
	}
}

func TestCheckPlugins_Valid(t *testing.T) {
	setPluginEnv(t, "search=/bin/x,scrape=/bin/y -v", "", "")
	c := checkPlugins()
	if c.Status != statusOK {
		t.Fatalf("valid spec should be OK, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "2 plugin(s)") {
		t.Errorf("detail = %q, want a 2-plugin count", c.Detail)
	}
}

func TestCheckPlugins_QuotedSpacedPath(t *testing.T) {
	// Ties M224 quoting into the pre-flight: a quoted Windows path is valid.
	setPluginEnv(t, `win="C:/Program Files/agezt-tool.exe" --verbose`, "", "")
	c := checkPlugins()
	if c.Status != statusOK {
		t.Fatalf("quoted spaced path should be OK, got %s: %s", c.Status.label(), c.Detail)
	}
}

func TestCheckPlugins_MalformedSpecFails(t *testing.T) {
	cases := map[string]struct{ plugins, pins, tools string }{
		"missing equals":   {"search/bin/x", "", ""},
		"duplicate prefix": {"a=/x,a=/y", "", ""},
		"bad pin":          {"search=/bin/x", "search=nothex", ""},
		"empty tool list":  {"search=/bin/x", "", "search="},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			setPluginEnv(t, tc.plugins, tc.pins, tc.tools)
			c := checkPlugins()
			if c.Status != statusFail {
				t.Fatalf("expected FAIL, got %s: %s", c.Status.label(), c.Detail)
			}
			if c.Hint == "" {
				t.Error("a FAIL should carry a fix hint")
			}
		})
	}
}

func TestCheckPlugins_StalePinWarns(t *testing.T) {
	// A pin for a prefix that no configured plugin uses.
	setPluginEnv(t, "search=/bin/x", "ghost="+validPin, "")
	c := checkPlugins()
	if c.Status != statusWarn {
		t.Fatalf("stale pin should WARN, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "pin:ghost") {
		t.Errorf("detail = %q, want it to name pin:ghost", c.Detail)
	}
}

func TestCheckPlugins_StaleToolWarns(t *testing.T) {
	setPluginEnv(t, "search=/bin/x", "", "ghost=foo+bar")
	c := checkPlugins()
	if c.Status != statusWarn {
		t.Fatalf("stale tool allowlist should WARN, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "tools:ghost") {
		t.Errorf("detail = %q, want it to name tools:ghost", c.Detail)
	}
}

func TestCheckPlugins_ValidWithPinsAndTools(t *testing.T) {
	setPluginEnv(t, "search=/bin/x", "search="+validPin, "search=query+index")
	c := checkPlugins()
	if c.Status != statusOK {
		t.Fatalf("valid pinned+allowlisted plugin should be OK, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "1 pinned") || !strings.Contains(c.Detail, "1 allow-listed") {
		t.Errorf("detail = %q, want pinned + allow-listed annotations", c.Detail)
	}
}

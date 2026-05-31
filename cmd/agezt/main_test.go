// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
)

func TestRunVersion(t *testing.T) {
	for _, flag := range []string{"-v", "--version", "version"} {
		var out, errOut bytes.Buffer
		code := run([]string{flag}, &out, &errOut)
		if code != 0 {
			t.Errorf("%s: exit=%d want 0; stderr=%q", flag, code, errOut.String())
		}
		if !strings.Contains(out.String(), brand.Version) {
			t.Errorf("%s: stdout missing version %q; got %q", flag, brand.Version, out.String())
		}
		if !strings.Contains(out.String(), brand.Binary) {
			t.Errorf("%s: stdout missing binary name %q; got %q", flag, brand.Binary, out.String())
		}
	}
}

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("help missing 'usage:'; got %q", out.String())
	}
	if !strings.Contains(out.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("help missing ANTHROPIC_API_KEY note; got %q", out.String())
	}
}

func TestRunUnknown(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"bogus"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Errorf("stderr missing error; got %q", errOut.String())
	}
}

// Note: runDaemon needs a real ANTHROPIC_API_KEY to start, so we don't
// exercise it here. The end-to-end test under kernel/controlplane covers
// the same wire format with a mock provider.

func TestModelAdvisory(t *testing.T) {
	cat := catalog.NewEmpty()
	cat.Providers["acme"] = &catalog.Provider{
		ID: "acme", NPM: "@ai-sdk/openai-compatible",
		Models: map[string]*catalog.Model{
			"mini":  {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 32768}},
			"large": {ID: "large", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
		},
	}
	// Tool-less model → advisory mentions tool-use.
	if adv := modelAdvisory(cat, "mini"); !strings.Contains(adv, "tool-use") {
		t.Errorf("mini advisory should mention tool-use; got %q", adv)
	}
	// Tool-capable model → no advisory.
	if adv := modelAdvisory(cat, "large"); adv != "" {
		t.Errorf("large advisory should be empty; got %q", adv)
	}
	// Unknown model / mock / empty → no false alarm.
	for _, m := range []string{"", "mock", "not-in-catalog"} {
		if adv := modelAdvisory(cat, m); adv != "" {
			t.Errorf("modelAdvisory(%q) should be empty; got %q", m, adv)
		}
	}
	if adv := modelAdvisory(nil, "mini"); adv != "" {
		t.Errorf("nil catalog should yield no advisory; got %q", adv)
	}
}

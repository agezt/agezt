// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func TestProviderFallbackCheck_NoneIsQuiet(t *testing.T) {
	for name, byKind := range map[string]map[string]any{
		"nil map":       nil,
		"kind absent":   {"run.started": float64(4)},
		"explicit zero": {string(event.KindProviderFallback): float64(0)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, show := providerFallbackCheck(byKind); show {
				t.Errorf("expected no check line for %q", name)
			}
		})
	}
}

func TestProviderFallbackCheck_Warns(t *testing.T) {
	byKind := map[string]any{string(event.KindProviderFallback): float64(2)}
	c, show := providerFallbackCheck(byKind)
	if !show {
		t.Fatal("a non-zero fallback count should surface a check")
	}
	if c.Status != statusWarn {
		t.Fatalf("fallbacks should WARN, got %s", c.Status.label())
	}
	if !strings.Contains(c.Detail, "2 provider fallback(s)") {
		t.Errorf("detail = %q, want the count", c.Detail)
	}
	if c.Hint == "" {
		t.Error("a WARN should carry an investigation hint")
	}
}

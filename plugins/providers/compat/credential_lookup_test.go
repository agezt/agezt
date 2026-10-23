// SPDX-License-Identifier: MIT

package compat

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/catalogapi"
)

// TestTheScopedCredentialIsFoundFirst: a provider's credential may live in
// the provider-scoped vault key ("provider:<id>:<ENV>") or in the bare env
// var. The lookup must try the SCOPED key first and fall back to the bare
// name — a provider that only found the bare name would read another
// provider's credential, and one that only found the scoped key would miss a
// legacy entry.
func TestTheScopedCredentialIsFoundFirst(t *testing.T) {
	p := &catalogapi.Provider{ID: "anthropic", Env: []string{"ANTHROPIC_API_KEY"}}

	scopedOnly := func(name string) string {
		if name == "provider:anthropic:ANTHROPIC_API_KEY" {
			return "scoped-secret"
		}
		return ""
	}
	if got := providerEnvLookup(p, scopedOnly, "ANTHROPIC_API_KEY"); got != "scoped-secret" {
		t.Fatalf("the scoped credential was not found: %q", got)
	}

	bareOnly := func(name string) string {
		if name == "ANTHROPIC_API_KEY" {
			return "bare-secret"
		}
		return ""
	}
	if got := providerEnvLookup(p, bareOnly, "ANTHROPIC_API_KEY"); got != "bare-secret" {
		t.Fatalf("the bare credential was not found: %q", got)
	}

	both := func(name string) string {
		switch name {
		case "provider:anthropic:ANTHROPIC_API_KEY":
			return "scoped-secret"
		case "ANTHROPIC_API_KEY":
			return "bare-secret"
		}
		return ""
	}
	if got := providerEnvLookup(p, both, "ANTHROPIC_API_KEY"); got != "scoped-secret" {
		t.Fatalf("the scoped credential must win: %q", got)
	}

	// No lookup at all means no credential — and no panic.
	if got := providerEnvLookup(p, nil, "ANTHROPIC_API_KEY"); got != "" {
		t.Fatalf("a nil lookup must yield no credential, got %q", got)
	}
}

// SPDX-License-Identifier: MIT

package runtime

import (
	"strings"
	"testing"
)

func TestInjectUserProfile(t *testing.T) {
	profile := "- communication style: terse\n- expertise: Go and React"
	got := injectUserProfile("You are a helpful agent.", profile)

	// The profile block leads; the persona is preserved beneath it.
	if !strings.Contains(got, "operator you work for") {
		t.Errorf("missing the profile lead-in: %q", got)
	}
	if !strings.Contains(got, "communication style: terse") {
		t.Errorf("missing facet content: %q", got)
	}
	if !strings.HasSuffix(got, "You are a helpful agent.") {
		t.Errorf("persona should sit beneath the profile block: %q", got)
	}
	// The profile block must appear ABOVE the persona (prepended).
	if strings.Index(got, "expertise: Go and React") > strings.Index(got, "You are a helpful agent.") {
		t.Errorf("profile should be prepended above the persona: %q", got)
	}
}

func TestInjectUserProfile_EmptyPersona(t *testing.T) {
	got := injectUserProfile("", "- current focus: shipping the voice feature")
	if !strings.Contains(got, "current focus: shipping the voice feature") {
		t.Errorf("missing facet: %q", got)
	}
	// No dangling persona separator when there's no persona.
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("trailing blank persona separator: %q", got)
	}
}

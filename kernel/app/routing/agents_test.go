// SPDX-License-Identifier: MIT

package routing

import (
	"testing"

	"github.com/agezt/agezt/kernel/roster"
)

func TestAgentsMapsTheRosterProfiles(t *testing.T) {
	got := Agents([]roster.Profile{
		{Slug: "researcher", Model: "model-a", Fallbacks: []string{"model-b", "model-c"}},
		{Slug: "writer", Model: "model-d"},
	})
	if len(got) != 2 {
		t.Fatalf("agents = %v, want two", got)
	}
	if got[0].Slug != "researcher" || got[0].Model != "model-a" {
		t.Errorf("agent = %+v", got[0])
	}
	if len(got[0].Fallbacks) != 2 || got[0].Fallbacks[0] != "model-b" || got[0].Fallbacks[1] != "model-c" {
		t.Errorf("fallbacks = %v, want the ordered chain", got[0].Fallbacks)
	}
	if got[1].Slug != "writer" || got[1].Model != "model-d" || got[1].Fallbacks != nil {
		t.Errorf("agent = %+v", got[1])
	}
}

// An empty roster maps to an empty agent list — not a nil one, so the
// routing view never has to special-case it.
func TestAgentsMapsAnEmptyRoster(t *testing.T) {
	got := Agents(nil)
	if got == nil {
		t.Fatalf("agents = nil, want an empty list")
	}
	if len(got) != 0 {
		t.Errorf("agents = %v, want none", got)
	}
}

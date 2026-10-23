// SPDX-License-Identifier: MIT

package agentapi

import "testing"

// TestManagedBy: the manager is the parent agent, or the owner when there is
// no parent, and nothing when neither is set. The rule belongs to the row —
// the schedule tool and the overseer's repair surface both answer "who
// manages this agent?" and must not answer differently.
func TestManagedBy(t *testing.T) {
	cases := []struct {
		name string
		p    Profile
		want string
	}{
		{"the parent manages", Profile{Slug: "sub", ParentAgent: "boss"}, "boss"},
		{"the owner manages when there is no parent", Profile{Slug: "sub", OwnerAgent: "owner"}, "owner"},
		{"the parent wins over the owner", Profile{Slug: "sub", ParentAgent: "boss", OwnerAgent: "owner"}, "boss"},
		{"nothing manages it", Profile{Slug: "solo"}, ""},
		{"blank is not a manager", Profile{Slug: "sub", ParentAgent: "   ", OwnerAgent: "  "}, ""},
		{"the manager is trimmed", Profile{Slug: "sub", ParentAgent: "  boss  "}, "boss"},
	}
	for _, tc := range cases {
		if got := tc.p.ManagedBy(); got != tc.want {
			t.Errorf("%s: ManagedBy() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

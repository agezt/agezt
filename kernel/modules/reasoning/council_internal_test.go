// SPDX-License-Identifier: MIT

package reasoning

import (
	"testing"

	"github.com/agezt/agezt/kernel/modules/reasoning/api"
)

// TestSplitConsensusDissent covers the chair's CONSENSUS / DISSENT split, the
// parser that decides what the council actually reports.
func TestSplitConsensusDissent(t *testing.T) {
	cases := []struct{ in, wantC, wantD string }{
		{"CONSENSUS: do X.\nDISSENT: Beta prefers Y.", "do X.", "Beta prefers Y."},
		{"CONSENSUS: do X.\nDISSENT: none", "do X.", ""},
		{"just a plain answer", "just a plain answer", ""},
		// Markdown-header style (what real chair models actually emit).
		{"## CONSENSUS\n\nUse tabs.\n\n## DISSENT\n\nGamma prefers spaces.", "Use tabs.", "Gamma prefers spaces."},
		{"## Consensus\nShip it.\n## Dissent\nnone", "Ship it.", ""},
		{"**CONSENSUS:** do the thing", "do the thing", ""},
	}
	for _, c := range cases {
		gotC, gotD := splitConsensusDissent(c.in)
		if gotC != c.wantC || gotD != c.wantD {
			t.Errorf("split(%q) = (%q,%q), want (%q,%q)", c.in, gotC, gotD, c.wantC, c.wantD)
		}
	}
}

// TestNormalizeMembers covers the panel rule the council and the conductor
// share: trim, drop the modelless seats, name the blank ones and dedupe.
func TestNormalizeMembers(t *testing.T) {
	got := api.NormalizeMembers([]CouncilMember{
		{Seat: "  Alpha ", Model: " model-a "},
		{Seat: "", Model: "model-b"},
		{Seat: "NoModel", Model: "  "},
		{Seat: "Alpha", Model: "model-a"},
	})
	if len(got) != 2 {
		t.Fatalf("members = %+v, want 2 rows", got)
	}
	if got[0].Seat != "Alpha" || got[0].Model != "model-a" {
		t.Errorf("first row = %+v", got[0])
	}
	if got[1].Seat != "Elder 2" || got[1].Model != "model-b" {
		t.Errorf("second row = %+v", got[1])
	}
}

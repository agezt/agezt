// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/roster"
)

// A future cutoff makes every existing agent "old enough to judge", so the scan
// flags exactly the enabled, non-retired one with no activity — and a past cutoff
// (everything within the grace window) flags nothing.
func TestReaperScan_FlagsIdleFiltersRetiredPausedAndNew(t *testing.T) {
	k := openCausesKernel(t)
	for _, slug := range []string{"live", "retired", "paused"} {
		if _, err := k.Roster().Add(roster.Profile{Slug: slug}); err != nil {
			t.Fatalf("add %s: %v", slug, err)
		}
	}
	if _, err := k.Roster().SetRetired("retired", true); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, err := k.Roster().SetEnabled("paused", false); err != nil {
		t.Fatalf("pause: %v", err)
	}

	future := time.Now().Add(time.Hour).UnixMilli()
	rep := k.ReaperScan(future, future)
	if len(rep.DeadAgents) != 1 || rep.DeadAgents[0].Slug != "live" {
		t.Fatalf("dead agents = %+v, want exactly [live]", rep.DeadAgents)
	}
	if rep.Empty() {
		t.Errorf("report with a dead agent should not be Empty()")
	}

	// Past cutoff: every agent is within the grace window → nothing flagged.
	past := time.Now().Add(-time.Hour).UnixMilli()
	if rep2 := k.ReaperScan(past, past); len(rep2.DeadAgents) != 0 || !rep2.Empty() {
		t.Errorf("past cutoff should flag nothing, got %+v", rep2.DeadAgents)
	}
}

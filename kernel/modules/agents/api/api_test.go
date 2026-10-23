// SPDX-License-Identifier: MIT

package api

// The module's own vocabulary: the rows it publishes and the shape they keep.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestFindingRowsCarryEveryField: the reaper's findings are published as they
// are — every field is present, the empty optional fields included. The
// operator's scan publishes these rows, so a row must never lose a key.
func TestFindingRowsCarryEveryField(t *testing.T) {
	for _, row := range []struct {
		name string
		raw  any
		want []string
	}{
		{"dead", ReaperAgent{Slug: "d"}, []string{`"slug":"d"`, `"name":""`, `"last_active_ms":0`}},
		{"degraded", DegradedAgent{Slug: "g"}, []string{`"name":""`, `"failures":0`, `"window":0`, `"threshold":0`, `"doctor_agent":""`, `"self_repair_enabled":false`, `"escalate_to":""`, `"last_failure_ms":0`, `"last_reason":""`}},
		{"misconfigured", MisconfiguredAgent{Slug: "m"}, []string{`"name":""`, `"issues":null`, `"doctor_agent":""`, `"self_repair_enabled":false`, `"escalate_to":""`}},
		{"retry pressure", RetryPressureAgent{Slug: "r"}, []string{`"count":0`, `"threshold":0`, `"window_sec":0`, `"last_retry_ms":0`, `"next_attempt":0`, `"max_attempts":0`}},
		{"routing pressure", RoutingPressureAgent{Slug: "p"}, []string{`"count":0`, `"last_fallback_ms":0`, `"last_failed_model":""`, `"last_next_model":""`, `"task_type":""`}},
		{"forced", RoutingForcedAgent{Slug: "f"}, []string{`"count":0`, `"last_forced_ms":0`, `"forced_chain":null`, `"routing_force_generation":0`, `"task_type":""`}},
		{"unstable", RoutingUnstableAgent{Slug: "u"}, []string{`"count":0`, `"last_rollback_ms":0`, `"current_chain":null`, `"previous_chain":null`, `"last_reason":""`}},
	} {
		raw, err := json.Marshal(row.raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range row.want {
			if !strings.Contains(string(raw), want) {
				t.Errorf("the %s row lost a field: %s not in %s", row.name, want, string(raw))
			}
		}
	}
}

// TestTheThreeForcedPhasesAreOneRow: the forced findings — in probation,
// failed after it and exhausted — are one row; the three names stay.
func TestTheThreeForcedPhasesAreOneRow(t *testing.T) {
	if reflect.TypeOf(RoutingForcedProbationAgent{}) != reflect.TypeOf(RoutingForcedAgent{}) {
		t.Error("the probation row drifted from the forced row")
	}
	if reflect.TypeOf(RoutingForcedFailedAgent{}) != reflect.TypeOf(RoutingForcedAgent{}) {
		t.Error("the failed row drifted from the forced row")
	}
	if reflect.TypeOf(RoutingForcedExhaustedAgent{}) != reflect.TypeOf(RoutingForcedAgent{}) {
		t.Error("the exhausted row drifted from the forced row")
	}
}

// TestEmptyReportsNothingToReap: a report with no finding and no stale
// artifact is empty; every kind of finding makes it non-empty.
func TestEmptyReportsNothingToReap(t *testing.T) {
	if !(ReaperReport{}).Empty() {
		t.Error("a fresh report must be empty")
	}
	notEmpty := []ReaperReport{
		{DeadAgents: []ReaperAgent{{Slug: "d"}}},
		{DegradedAgents: []DegradedAgent{{Slug: "g"}}},
		{MisconfiguredAgents: []MisconfiguredAgent{{Slug: "m"}}},
		{RetryPressure: []RetryPressureAgent{{Slug: "r"}}},
		{RoutingPressure: []RoutingPressureAgent{{Slug: "p"}}},
		{RoutingForced: []RoutingForcedAgent{{Slug: "f"}}},
		{RoutingForcedFailed: []RoutingForcedFailedAgent{{Slug: "ff"}}},
		{RoutingForcedExhausted: []RoutingForcedExhaustedAgent{{Slug: "fe"}}},
		{RoutingUnstable: []RoutingUnstableAgent{{Slug: "u"}}},
		{StaleArtifacts: 1},
	}
	for i, rep := range notEmpty {
		if rep.Empty() {
			t.Errorf("report %d carries a finding and must not be empty", i)
		}
	}
}

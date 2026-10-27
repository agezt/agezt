// SPDX-License-Identifier: MIT

package api

// The module's own vocabulary: the rows it publishes and the shape they keep.

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// TestTheReaperRowsAreTheContractRows: the rows live in the CONTRACT layer
// and this package re-exports them as aliases — one row, one spelling.
//
// The identity matters: a row the ADAPTER can only name by naming this
// module is a row the adapter is not supposed to know, and the reaper's
// findings are read by the operator's scan in the application layer and
// handed on by the adapter. If this package declared its own copy, the two
// would drift the moment one of them changed.
func TestTheReaperRowsAreTheContractRows(t *testing.T) {
	pairs := []struct {
		name     string
		module   any
		contract any
	}{
		{"reaper agent", ReaperAgent{}, agentapi.ReaperAgent{}},
		{"degraded agent", DegradedAgent{}, agentapi.DegradedAgent{}},
		{"misconfigured agent", MisconfiguredAgent{}, agentapi.MisconfiguredAgent{}},
		{"routing pressure agent", RoutingPressureAgent{}, agentapi.RoutingPressureAgent{}},
		{"retry pressure agent", RetryPressureAgent{}, agentapi.RetryPressureAgent{}},
		{"forced agent", RoutingForcedAgent{}, agentapi.RoutingForcedAgent{}},
		{"unstable agent", RoutingUnstableAgent{}, agentapi.RoutingUnstableAgent{}},
		{"reaper report", ReaperReport{}, agentapi.ReaperReport{}},
	}
	for _, p := range pairs {
		if reflect.TypeOf(p.module) != reflect.TypeOf(p.contract) {
			t.Errorf("the module's %s is not the contract's %s", p.name, p.name)
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

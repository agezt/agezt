// SPDX-License-Identifier: MIT

package reaper

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/modules/agents/api"
)

// TestRowsAreTheModuleFindings: the scan's rows ARE the agents module's
// finding rows, not a copy of them. The application does not rebuild a finding
// field by field any more, so the two can never drift apart.
func TestRowsAreTheModuleFindings(t *testing.T) {
	for _, pair := range []struct {
		name string
		row  any
		find any
	}{
		{"dead", DeadRow{}, api.ReaperAgent{}},
		{"degraded", DegradedRow{}, api.DegradedAgent{}},
		{"misconfigured", MisconfiguredRow{}, api.MisconfiguredAgent{}},
		{"retry pressure", RetryRow{}, api.RetryPressureAgent{}},
		{"routing pressure", RoutingRow{}, api.RoutingPressureAgent{}},
		{"forced", ForcedRow{}, api.RoutingForcedAgent{}},
		{"unstable", UnstableRow{}, api.RoutingUnstableAgent{}},
	} {
		if reflect.TypeOf(pair.row) != reflect.TypeOf(pair.find) {
			t.Errorf("the %s row drifted from the module's finding row: %v vs %v", pair.name, reflect.TypeOf(pair.row), reflect.TypeOf(pair.find))
		}
	}
}

// TestTheThreeForcedPhasesAreOneRow: the forced findings — in probation,
// failed after it and exhausted — were three identical declarations of the
// same row. They are one row now; the three names stay.
func TestTheThreeForcedPhasesAreOneRow(t *testing.T) {
	if reflect.TypeOf(api.RoutingForcedProbationAgent{}) != reflect.TypeOf(api.RoutingForcedAgent{}) {
		t.Error("the probation row drifted from the forced row")
	}
	if reflect.TypeOf(api.RoutingForcedFailedAgent{}) != reflect.TypeOf(api.RoutingForcedAgent{}) {
		t.Error("the failed row drifted from the forced row")
	}
	if reflect.TypeOf(api.RoutingForcedExhaustedAgent{}) != reflect.TypeOf(api.RoutingForcedAgent{}) {
		t.Error("the exhausted row drifted from the forced row")
	}
}

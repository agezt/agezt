// SPDX-License-Identifier: MIT

package reaper

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
)

// TestRowsAreTheRuntimeFindings: the scan's rows ARE the runtime's finding
// rows, not a copy of them. The application does not rebuild a finding field
// by field any more, so the two can never drift apart.
func TestRowsAreTheRuntimeFindings(t *testing.T) {
	for _, pair := range []struct {
		name string
		row  any
		find any
	}{
		{"dead", DeadRow{}, runtime.ReaperAgent{}},
		{"degraded", DegradedRow{}, runtime.DegradedAgent{}},
		{"misconfigured", MisconfiguredRow{}, runtime.MisconfiguredAgent{}},
		{"retry pressure", RetryRow{}, runtime.RetryPressureAgent{}},
		{"routing pressure", RoutingRow{}, runtime.RoutingPressureAgent{}},
		{"forced", ForcedRow{}, runtime.RoutingForcedAgent{}},
		{"unstable", UnstableRow{}, runtime.RoutingUnstableAgent{}},
	} {
		if reflect.TypeOf(pair.row) != reflect.TypeOf(pair.find) {
			t.Errorf("the %s row drifted from the runtime's finding row: %v vs %v", pair.name, reflect.TypeOf(pair.row), reflect.TypeOf(pair.find))
		}
	}
}

// TestTheThreeForcedPhasesAreOneRow: the forced findings — in probation,
// failed after it and exhausted — were three identical declarations of the
// same row. They are one row now; the three names stay.
func TestTheThreeForcedPhasesAreOneRow(t *testing.T) {
	if reflect.TypeOf(runtime.RoutingForcedProbationAgent{}) != reflect.TypeOf(runtime.RoutingForcedAgent{}) {
		t.Error("the probation row drifted from the forced row")
	}
	if reflect.TypeOf(runtime.RoutingForcedFailedAgent{}) != reflect.TypeOf(runtime.RoutingForcedAgent{}) {
		t.Error("the failed row drifted from the forced row")
	}
	if reflect.TypeOf(runtime.RoutingForcedExhaustedAgent{}) != reflect.TypeOf(runtime.RoutingForcedAgent{}) {
		t.Error("the exhausted row drifted from the forced row")
	}
}

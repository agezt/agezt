// SPDX-License-Identifier: MIT

package execprofile

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/executionprofile"
)

// TestRowsAreTheModuleRows: the operation's rows ARE the module's rows, not a
// copy of them. The application does not rebuild a profile, a check or a
// secret policy field by field any more, so the two cannot drift apart.
func TestRowsAreTheModuleRows(t *testing.T) {
	for _, pair := range []struct {
		name string
		row  any
		find any
	}{
		{"secret policy", SecretPolicy{}, executionprofile.SecretPolicy{}},
		{"profile", Profile{}, executionprofile.Profile{}},
		{"check", Check{}, executionprofile.HealthCheck{}},
		{"inventory", ListOutput{}, executionprofile.Inventory{}},
		{"health report", CheckOutput{}, executionprofile.HealthReport{}},
	} {
		if reflect.TypeOf(pair.row) != reflect.TypeOf(pair.find) {
			t.Errorf("the %s row drifted from the module's row: %v vs %v", pair.name, reflect.TypeOf(pair.row), reflect.TypeOf(pair.find))
		}
	}
}

// TestTheProfileRowIsTheModuleRow: the module's profile row is the row the
// operation publishes, so the profile's own fields — the tools, the backends,
// the limits, the notes and the secret policy — travel as they are.
func TestTheProfileRowIsTheModuleRow(t *testing.T) {
	p := executionprofile.Profile{ID: "remote", Name: "Remote", Status: executionprofile.Status("supported"), Tools: []string{"shell"}}
	row := profile(p)
	if row.ID != "remote" || row.Status != executionprofile.Status("supported") {
		t.Fatalf("the row lost the profile's identity: %+v", row)
	}
	if len(row.Tools) != 1 || row.Tools[0] != "shell" {
		t.Fatalf("the row lost the profile's tools: %+v", row.Tools)
	}
	// The list rule is the application's own: an empty list is null.
	empty := profile(executionprofile.Profile{ID: "bare"})
	if empty.Tools != nil || empty.Notes != nil {
		t.Fatalf("an empty list is null on the wire: %+v", empty)
	}
}

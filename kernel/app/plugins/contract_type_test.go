// SPDX-License-Identifier: MIT

package plugins

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/types"
)

// TestRegistrationIsTheContractType: the application's plugin registration IS
// the contract type, not a copy of it. The host therefore hands its manifest
// over without a field-by-field projection, and the two sides cannot drift.
func TestRegistrationIsTheContractType(t *testing.T) {
	if reflect.TypeOf(Registration{}) != reflect.TypeOf(types.PluginInfo{}) {
		t.Fatalf("the plugin registration drifted from the contract type: %v vs %v", reflect.TypeOf(Registration{}), reflect.TypeOf(types.PluginInfo{}))
	}
	// A registration is usable as the contract type without a conversion...
	var registration types.PluginInfo = Registration{Prefix: "px", Path: "/bin/px", ToolCount: 2}
	if registration.Prefix != "px" || registration.Path != "/bin/px" || registration.ToolCount != 2 {
		t.Fatal(registration)
	}
	// ...and so is a whole slice, which is what the host hands over.
	var registrations []Registration = []types.PluginInfo{{Prefix: "px"}}
	if len(registrations) != 1 || registrations[0].Prefix != "px" {
		t.Fatal(registrations)
	}
}

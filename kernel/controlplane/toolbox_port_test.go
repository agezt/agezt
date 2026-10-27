// SPDX-License-Identifier: MIT

package controlplane_test

// Provenance: the control plane's own test for the toolbox's PORT (W5.4b).
//
//	The control plane holds the CONTRACT's port — not the toolbox module — so
//	it may not name the module, and the daemon (the composition root) hands
//	over the port.
//
//	The face is asserted STATICALLY (the declared parameter type is compared),
//	because an anonymous interface with the same verbs would still compile and
//	would be a second shape of the same port, drifting from the contract on its
//	own.

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolboxapi"
	"github.com/agezt/agezt/kernel/controlplane"
)

// TestTheControlPlaneHoldsTheToolboxContractPort: the toolbox reaches the
// control plane through the contract's port.
func TestTheControlPlaneHoldsTheToolboxContractPort(t *testing.T) {
	hostPort := reflect.TypeOf((*toolboxapi.Host)(nil)).Elem()

	set, ok := reflect.TypeOf((*controlplane.Server)(nil)).MethodByName("SetToolbox")
	if !ok {
		t.Fatal("the control plane has no SetToolbox")
	}
	if got := set.Type.In(1); got != hostPort {
		t.Fatalf("SetToolbox takes %v, want the contract's toolbox port %v", got, hostPort)
	}

	field, ok := reflect.TypeOf(controlplane.Deps{}).FieldByName("Toolbox")
	if !ok {
		t.Fatal("the control plane's deps have no Toolbox")
	}
	if field.Type != hostPort {
		t.Fatalf("Deps.Toolbox is %v, want the contract's toolbox port %v", field.Type, hostPort)
	}
}

// SPDX-License-Identifier: MIT

package controlplane_test

// Provenance: the control plane's own test for the update PORT (W5.4c).
//
//	The control plane holds the CONTRACT's port — not the update module — so
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

	"github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/controlplane"
)

// TestTheControlPlaneHoldsTheUpdateContractPort: the update engine reaches
// the control plane through the contract's port.
func TestTheControlPlaneHoldsTheUpdateContractPort(t *testing.T) {
	backendPort := reflect.TypeOf((*updateapi.Backend)(nil)).Elem()

	set, ok := reflect.TypeOf((*controlplane.Server)(nil)).MethodByName("SetUpdateService")
	if !ok {
		t.Fatal("the control plane has no SetUpdateService")
	}
	if got := set.Type.In(1); got != backendPort {
		t.Fatalf("SetUpdateService takes %v, want the contract's update port %v", got, backendPort)
	}

	field, ok := reflect.TypeOf(controlplane.Deps{}).FieldByName("UpdateSvc")
	if !ok {
		t.Fatal("the control plane's deps have no UpdateSvc")
	}
	if field.Type != backendPort {
		t.Fatalf("Deps.UpdateSvc is %v, want the contract's update port %v", field.Type, backendPort)
	}
}

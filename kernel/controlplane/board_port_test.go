// SPDX-License-Identifier: MIT

package controlplane_test

// Provenance: the control plane's own test for the board's PORT (W3.59).
//
//	The control plane holds the CONTRACT's port — not the module's concrete
//	store — so it may not name the module, and it never opens the board's
//	file behind the daemon's back.
//
//	The face is asserted STATICALLY (the declared parameter type is compared),
//	because an anonymous interface with the same verbs would still compile and
//	would be a second shape of the same port, drifting from the contract on its
//	own.

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/controlplane"
)

// TestTheControlPlaneHoldsTheBoardsContractPort: the board reaches the
// control plane through the contract's port, and the notifier speaks the
// contract's row.
func TestTheControlPlaneHoldsTheBoardsContractPort(t *testing.T) {
	storePort := reflect.TypeOf((*boardapi.Store)(nil)).Elem()
	row := reflect.TypeOf((*boardapi.Message)(nil)).Elem()

	set, ok := reflect.TypeOf((*controlplane.Server)(nil)).MethodByName("SetBoard")
	if !ok {
		t.Fatal("the control plane has no SetBoard")
	}
	if got := set.Type.In(1); got != storePort {
		t.Fatalf("SetBoard takes %v, want the contract's store port %v", got, storePort)
	}
	if got := set.Type.In(2).In(0); got != row {
		t.Fatalf("the notifier speaks %v, want the contract's row %v", got, row)
	}

	field, ok := reflect.TypeOf(controlplane.Deps{}).FieldByName("Board")
	if !ok {
		t.Fatal("the control plane's deps have no Board")
	}
	if field.Type != storePort {
		t.Fatalf("Deps.Board is %v, want the contract's store port %v", field.Type, storePort)
	}
}

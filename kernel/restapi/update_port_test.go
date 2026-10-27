// SPDX-License-Identifier: MIT

package restapi_test

// Provenance: the REST surface's own test for the update PORT (W5.4c).
//
//	The REST surface is an ADAPTER, and an adapter reaches the app layer's
//	operations, never a module — so it holds the CONTRACT's port. The package's
//	own comment used to claim it was "decoupled from kernel/update" while
//	importing it; the claim is now true of the code.
//
//	The face is asserted STATICALLY, because an anonymous interface with the
//	same verbs would compile and would be a second shape of the same port.

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/restapi"
)

// TestTheRestAPIHoldsTheUpdateContractPort: the update engine reaches the
// REST surface through the contract's port.
func TestTheRestAPIHoldsTheUpdateContractPort(t *testing.T) {
	backendPort := reflect.TypeOf((*updateapi.Backend)(nil)).Elem()

	set, ok := reflect.TypeOf((*restapi.Server)(nil)).MethodByName("SetUpdateService")
	if !ok {
		t.Fatal("the REST surface has no SetUpdateService")
	}
	if got := set.Type.In(1); got != backendPort {
		t.Fatalf("SetUpdateService takes %v, want the contract's update port %v", got, backendPort)
	}
}

// SPDX-License-Identifier: MIT
package update

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/updateapi"
)

// TestTheAppBackendIsTheContractPort: the app's port IS the contract's port.
//
// It is an ALIAS, not a second interface with the same verbs: two shapes of
// the same port would drift apart on their own, and the adapters hold the
// contract's.
func TestTheAppBackendIsTheContractPort(t *testing.T) {
	appPort := reflect.TypeOf((*Backend)(nil)).Elem()
	contractPort := reflect.TypeOf((*updateapi.Backend)(nil)).Elem()
	if appPort != contractPort {
		t.Fatalf("the app's backend port is %v, want the contract's port %v", appPort, contractPort)
	}
}
